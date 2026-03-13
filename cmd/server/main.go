package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kodrunhq/claude-plane/internal/server/api"
	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/config"
	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/frontend"
	grpcserver "github.com/kodrunhq/claude-plane/internal/server/grpc"
	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/orchestrator"
	"github.com/kodrunhq/claude-plane/internal/server/scheduler"
	"github.com/kodrunhq/claude-plane/internal/server/session"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	"github.com/kodrunhq/claude-plane/internal/shared/tlsutil"

	// Prove generated proto package compiles.
	_ "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

var version = "0.1.0-dev"

// noopExecutor is a stub StepExecutor that logs instead of executing.
// Used until the real session-backed executor is wired up.
type noopExecutor struct{}

func (noopExecutor) ExecuteStep(_ context.Context, step store.RunStep, onComplete func(string, int)) {
	slog.Warn("step execution not implemented yet", "step_id", step.StepID)
	onComplete(step.StepID, 1)
}

func main() {
	rootCmd := &cobra.Command{
		Use:     "claude-plane-server",
		Short:   "Control plane for Claude CLI sessions",
		Version: version,
	}

	rootCmd.AddCommand(
		newServeCmd(),
		newCACmd(),
		newSeedAdminCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the control plane server",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")

			cfg, err := config.LoadServerConfig(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			shutdownTimeout, err := cfg.Shutdown.ParseTimeout()
			if err != nil {
				return fmt.Errorf("parse shutdown timeout: %w", err)
			}

			tokenTTL, err := cfg.Auth.ParseTokenTTL()
			if err != nil {
				return fmt.Errorf("parse token TTL: %w", err)
			}

			// Root context cancelled on SIGINT or SIGTERM.
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			s, err := store.NewStore(cfg.Database.Path)
			if err != nil {
				return fmt.Errorf("initialize database: %w", err)
			}

			// Auth service
			blocklist, err := auth.NewBlocklist(s)
			if err != nil {
				return fmt.Errorf("initialize blocklist: %w", err)
			}
			authSvc := auth.NewService([]byte(cfg.Auth.JWTSecret), tokenTTL, blocklist)

			// mTLS config for gRPC
			tlsCfg, err := tlsutil.ServerTLSConfig(cfg.TLS.CACert, cfg.TLS.ServerCert, cfg.TLS.ServerKey)
			if err != nil {
				return fmt.Errorf("load TLS config: %w", err)
			}

			// Connection manager and session registry
			connMgr := connmgr.NewConnectionManager(s, slog.Default())
			registry := session.NewRegistry(slog.Default())

			// gRPC server
			grpcSrv := grpcserver.NewGRPCServer(tlsCfg, connMgr, slog.Default())
			grpcSrv.SetRegistry(registry)

			grpcLis, err := net.Listen("tcp", cfg.GRPC.Listen)
			if err != nil {
				return fmt.Errorf("listen gRPC on %s: %w", cfg.GRPC.Listen, err)
			}

			go func() {
				if err := grpcSrv.Serve(grpcLis); err != nil {
					slog.Error("gRPC server error", "error", err)
				}
			}()

			// Claims getter adapters
			sessionClaimsGetter := func(r *http.Request) *session.UserClaims {
				c := api.GetClaims(r)
				if c == nil {
					return nil
				}
				return &session.UserClaims{UserID: c.UserID, Role: c.Role}
			}
			handlerClaimsGetter := func(r *http.Request) *handler.UserClaims {
				c := api.GetClaims(r)
				if c == nil {
					return nil
				}
				return &handler.UserClaims{UserID: c.UserID, Role: c.Role}
			}

			// Orchestrator (stub executor until session-backed execution is wired)
			orch := orchestrator.NewOrchestrator(ctx, s, noopExecutor{})

			// ---- Event bus and subscribers ----
			eventBus := event.NewBus(slog.Default())
			defer eventBus.Close()

			// Persist every event to SQLite.
			persistSub := event.NewPersistSubscriber(s, slog.Default())
			persistSub.Subscribe(eventBus)

			// Periodic event retention cleanup.
			retentionCleaner := event.NewRetentionCleaner(s, slog.Default())
			retentionCleaner.Start(ctx)

			// WebSocket fan-out for the /ws/events endpoint.
			wsFanout := event.NewWSFanout(eventBus, slog.Default())
			wsFanout.Start()
			defer wsFanout.Close()

			// Outbound webhook deliverer — adapter translates store types to event types.
			webhookStore := &event.WebhookStoreFuncs{
				ListWebhooksFn: func(c context.Context) ([]event.Webhook, error) {
					storeWebhooks, err := s.ListWebhooks(c)
					if err != nil {
						return nil, err
					}
					result := make([]event.Webhook, len(storeWebhooks))
					for i, sw := range storeWebhooks {
						result[i] = event.Webhook{
							WebhookID: sw.WebhookID,
							URL:       sw.URL,
							Secret:    sw.Secret,
							Events:    sw.Events,
							Enabled:   sw.Enabled,
						}
					}
					return result, nil
				},
				CreateDeliveryFn: func(c context.Context, d event.WebhookDelivery) error {
					return s.CreateDelivery(c, store.WebhookDelivery{
						DeliveryID:   d.DeliveryID,
						WebhookID:    d.WebhookID,
						EventID:      d.EventID,
						Status:       d.Status,
						Attempts:     d.Attempts,
						ResponseCode: d.ResponseCode,
						LastError:    d.LastError,
						NextRetryAt:  d.NextRetryAt,
					})
				},
				UpdateDeliveryFn: func(c context.Context, d event.WebhookDelivery) error {
					return s.UpdateDelivery(c, store.WebhookDelivery{
						DeliveryID:   d.DeliveryID,
						WebhookID:    d.WebhookID,
						EventID:      d.EventID,
						Status:       d.Status,
						Attempts:     d.Attempts,
						ResponseCode: d.ResponseCode,
						LastError:    d.LastError,
						NextRetryAt:  d.NextRetryAt,
					})
				},
				PendingDeliveriesFn: func(c context.Context) ([]event.WebhookDelivery, error) {
					storeDeliveries, err := s.PendingDeliveries(c)
					if err != nil {
						return nil, err
					}
					result := make([]event.WebhookDelivery, len(storeDeliveries))
					for i, sd := range storeDeliveries {
						result[i] = event.WebhookDelivery{
							DeliveryID:   sd.DeliveryID,
							WebhookID:    sd.WebhookID,
							EventID:      sd.EventID,
							Status:       sd.Status,
							Attempts:     sd.Attempts,
							ResponseCode: sd.ResponseCode,
							LastError:    sd.LastError,
							NextRetryAt:  sd.NextRetryAt,
						}
					}
					return result, nil
				},
				GetEventByIDFn: func(c context.Context, eventID string) (*event.Event, error) {
					return s.GetEventByID(c, eventID)
				},
			}
			webhookDeliverer := event.NewWebhookDeliverer(webhookStore, &http.Client{Timeout: 10 * time.Second}, slog.Default())
			eventBus.Subscribe("*", webhookDeliverer.Handler(), event.SubscriberOptions{Concurrency: 4, BufferSize: 256})
			webhookDeliverer.StartRetryLoop(ctx)

			// Trigger subscriber — fires job runs when matching events arrive.
			triggerStore := &event.TriggerStoreFuncs{
				ListEnabledTriggersFn: func(c context.Context) ([]event.JobTrigger, error) {
					storeTriggers, err := s.ListEnabledTriggers(c)
					if err != nil {
						return nil, err
					}
					result := make([]event.JobTrigger, len(storeTriggers))
					for i, st := range storeTriggers {
						result[i] = event.JobTrigger{
							TriggerID: st.TriggerID,
							JobID:     st.JobID,
							EventType: st.EventType,
							Filter:    st.Filter,
							Enabled:   st.Enabled,
						}
					}
					return result, nil
				},
			}
			orchAdapter := &event.OrchestratorFuncs{
				CreateRunFn: func(c context.Context, jobID, triggerType string) error {
					return orch.CreateRunErr(c, jobID, triggerType)
				},
			}
			triggerSub := event.NewTriggerSubscriber(triggerStore, orchAdapter, slog.Default())
			eventBus.Subscribe("*", triggerSub.Handler(), event.SubscriberOptions{Concurrency: 2, BufferSize: 256})

			// Wire event publisher into components.
			connMgr.SetPublisher(eventBus)
			orch.SetPublisher(eventBus)

			// ---- Cron scheduler ----
			schedStore := &scheduler.ScheduleStoreFuncs{
				ListEnabledSchedulesFn: func(c context.Context) ([]scheduler.CronSchedule, error) {
					storeSchedules, err := s.ListEnabledSchedules(c)
					if err != nil {
						return nil, err
					}
					result := make([]scheduler.CronSchedule, len(storeSchedules))
					for i, ss := range storeSchedules {
						result[i] = scheduler.CronSchedule{
							ScheduleID:      ss.ScheduleID,
							JobID:           ss.JobID,
							CronExpr:        ss.CronExpr,
							Timezone:        ss.Timezone,
							Enabled:         ss.Enabled,
							NextRunAt:       ss.NextRunAt,
							LastTriggeredAt: ss.LastTriggeredAt,
						}
					}
					return result, nil
				},
				GetScheduleFn: func(c context.Context, scheduleID string) (*scheduler.CronSchedule, error) {
					ss, err := s.GetSchedule(c, scheduleID)
					if err != nil {
						return nil, err
					}
					return &scheduler.CronSchedule{
						ScheduleID:      ss.ScheduleID,
						JobID:           ss.JobID,
						CronExpr:        ss.CronExpr,
						Timezone:        ss.Timezone,
						Enabled:         ss.Enabled,
						NextRunAt:       ss.NextRunAt,
						LastTriggeredAt: ss.LastTriggeredAt,
					}, nil
				},
				UpdateScheduleTimestampsFn: func(c context.Context, scheduleID string, lastTriggered, nextRun time.Time) error {
					return s.UpdateScheduleTimestamps(c, scheduleID, lastTriggered, nextRun)
				},
			}

			// Event publisher adapter — bridges scheduler.Event to event.Event for the bus.
			schedEventBus := &scheduler.EventPublisherFuncs{
				PublishFn: func(c context.Context, ev scheduler.Event) error {
					return eventBus.Publish(c, event.Event{
						EventID:   ev.EventID,
						Type:      ev.Type,
						Timestamp: ev.Timestamp,
						Source:    ev.Source,
						Payload:   ev.Payload,
					})
				},
			}

			sched := scheduler.NewScheduler(schedStore, schedEventBus, slog.Default())
			if err := sched.Start(ctx); err != nil {
				return fmt.Errorf("start scheduler: %w", err)
			}

			// Handlers
			sessionHandler := session.NewSessionHandler(s, connMgr, registry, sessionClaimsGetter, slog.Default())
			sessionHandler.SetPublisher(eventBus)

			wsHandler := session.HandleTerminalWS(s, connMgr, registry, authSvc, slog.Default())
			eventsWSHandler := session.HandleEventsWS(authSvc, wsFanout, slog.Default())

			jobHandler := handler.NewJobHandler(s, handlerClaimsGetter)
			runHandler := handler.NewRunHandler(s, orch, handlerClaimsGetter)

			// New event/webhook/trigger/ingest handlers.
			eventHandler := handler.NewEventHandler(s)
			webhookHandler := handler.NewWebhookHandler(s)
			triggerHandler := handler.NewTriggerHandler(s)
			ingestSecrets := cfg.Webhooks.InboundSecrets()
			ingestHandler := handler.NewIngestHandler(eventBus, ingestSecrets, slog.Default())

			scheduleHandler := handler.NewScheduleHandler(s, s, sched, handlerClaimsGetter)

			// HTTP router
			handlers := api.NewHandlers(s, authSvc, connMgr, cfg.Auth.GetRegistrationMode(), cfg.Auth.InviteCode)
			router := api.NewRouter(handlers, sessionHandler, wsHandler, eventsWSHandler, jobHandler, runHandler, eventHandler, webhookHandler, triggerHandler, ingestHandler, scheduleHandler)

			// Mount SPA frontend as catch-all
			router.Handle("/*", frontend.NewSPAHandler())

			httpServer := &http.Server{
				Addr:    cfg.HTTP.Listen,
				Handler: router,
			}

			go func() {
				slog.Info("HTTP server starting", "addr", cfg.HTTP.Listen)
				if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					slog.Error("HTTP server error", "error", err)
				}
			}()

			slog.Info("Server initialized",
				"http", cfg.HTTP.Listen,
				"grpc", cfg.GRPC.Listen,
				"database", cfg.Database.Path,
				"shutdown_timeout", shutdownTimeout,
			)

			// Block until shutdown signal.
			<-ctx.Done()
			slog.Info("Shutdown signal received, starting graceful shutdown",
				"timeout", shutdownTimeout,
			)

			// Create a timeout context for the shutdown sequence.
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer shutdownCancel()

			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				slog.Error("HTTP shutdown error", "error", err)
			}

			sched.Stop()

			// GracefulStop can block indefinitely waiting for in-flight streams.
			// Run it in a goroutine and fall back to Stop() if the timeout expires.
			grpcStopped := make(chan struct{})
			go func() {
				grpcSrv.GracefulStop()
				close(grpcStopped)
			}()
			select {
			case <-grpcStopped:
			case <-shutdownCtx.Done():
				slog.Warn("gRPC graceful stop timed out, forcing stop")
				grpcSrv.Stop()
			}

			// Close the database as the final cleanup step.
			if err := s.Close(); err != nil {
				slog.Error("Error closing database", "error", err)
			}

			slog.Info("Shutdown complete")
			return nil
		},
	}
	cmd.Flags().String("config", "server.toml", "Path to server TOML config file")
	return cmd
}

func newCACmd() *cobra.Command {
	caCmd := &cobra.Command{
		Use:   "ca",
		Short: "Certificate authority operations",
	}
	caCmd.AddCommand(
		newCAInitCmd(),
		newCAIssueServerCmd(),
		newCAIssueAgentCmd(),
	)
	return caCmd
}

func newCAInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the certificate authority",
		RunE: func(cmd *cobra.Command, args []string) error {
			outDir, _ := cmd.Flags().GetString("out-dir")

			if err := tlsutil.GenerateCA(outDir); err != nil {
				return fmt.Errorf("generate CA: %w", err)
			}

			slog.Info("CA initialized", "dir", outDir)
			return nil
		},
	}
	cmd.Flags().String("out-dir", "./ca", "Output directory for CA certificate and key")
	return cmd
}

func newCAIssueServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue-server",
		Short: "Issue a server TLS certificate",
		RunE: func(cmd *cobra.Command, args []string) error {
			caDir, _ := cmd.Flags().GetString("ca-dir")
			outDir, _ := cmd.Flags().GetString("out-dir")
			hostnames, _ := cmd.Flags().GetStringSlice("hostnames")

			if err := tlsutil.IssueServerCert(caDir, outDir, hostnames); err != nil {
				return fmt.Errorf("issue server cert: %w", err)
			}

			slog.Info("Server certificate issued", "dir", outDir)
			return nil
		},
	}
	cmd.Flags().String("ca-dir", "./ca", "Directory containing CA certificate and key")
	cmd.Flags().String("out-dir", "./server-cert", "Output directory for server certificate and key")
	cmd.Flags().StringSlice("hostnames", []string{}, "Additional hostnames/IPs for the server certificate (localhost and 127.0.0.1 are always included)")
	return cmd
}

func newCAIssueAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue-agent",
		Short: "Issue an agent mTLS certificate",
		RunE: func(cmd *cobra.Command, args []string) error {
			caDir, _ := cmd.Flags().GetString("ca-dir")
			outDir, _ := cmd.Flags().GetString("out-dir")
			machineID, _ := cmd.Flags().GetString("machine-id")

			if machineID == "" {
				return fmt.Errorf("--machine-id is required")
			}

			if err := tlsutil.IssueAgentCert(caDir, outDir, machineID); err != nil {
				return fmt.Errorf("issue agent cert: %w", err)
			}

			slog.Info("Agent certificate issued", "dir", outDir, "machine_id", machineID)
			return nil
		},
	}
	cmd.Flags().String("ca-dir", "./ca", "Directory containing CA certificate and key")
	cmd.Flags().String("out-dir", "./agent-cert", "Output directory for agent certificate and key")
	cmd.Flags().String("machine-id", "", "Machine identifier for the agent certificate CN")
	return cmd
}

func newSeedAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seed-admin",
		Short: "Create the initial admin account",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			email, _ := cmd.Flags().GetString("email")
			passwordFile, _ := cmd.Flags().GetString("password-file")
			name, _ := cmd.Flags().GetString("name")

			if email == "" {
				return fmt.Errorf("--email is required")
			}

			var password string
			if passwordFile != "" {
				data, err := os.ReadFile(passwordFile)
				if err != nil {
					return fmt.Errorf("reading password file: %w", err)
				}
				password = strings.TrimSpace(string(data))
			} else if term.IsTerminal(int(os.Stdin.Fd())) {
				// Interactive TTY: read without echo
				fmt.Fprint(os.Stderr, "Enter admin password: ")
				pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Fprintln(os.Stderr) // newline after hidden input
				if err != nil {
					return fmt.Errorf("reading password from stdin: %w", err)
				}
				password = strings.TrimSpace(string(pwBytes))
			} else {
				// Non-TTY (piped/redirected): read from stdin
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("reading password from stdin: %w", err)
				}
				password = strings.TrimSpace(string(data))
			}

			if len(password) < 8 {
				return fmt.Errorf("password must be at least 8 characters")
			}

			s, err := store.NewStore(dbPath)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer s.Close()

			if err := s.SeedAdmin(email, password, name); err != nil {
				return fmt.Errorf("seed admin: %w", err)
			}

			slog.Info("Admin account created", "email", email)
			return nil
		},
	}
	cmd.Flags().String("db", "claude-plane.db", "Path to SQLite database file")
	cmd.Flags().String("email", "", "Admin email address")
	cmd.Flags().String("password-file", "", "Path to file containing admin password")
	cmd.Flags().String("name", "Admin", "Admin display name")
	return cmd
}
