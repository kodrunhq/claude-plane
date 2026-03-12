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

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kodrunhq/claude-plane/internal/server/api"
	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/config"
	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/frontend"
	grpcserver "github.com/kodrunhq/claude-plane/internal/server/grpc"
	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/orchestrator"
	"github.com/kodrunhq/claude-plane/internal/server/session"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	"github.com/kodrunhq/claude-plane/internal/shared/tlsutil"

	// Prove generated proto package compiles.
	_ "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

var version = "0.1.0-dev"

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

			// Handlers
			sessionHandler := session.NewSessionHandler(s, connMgr, registry, sessionClaimsGetter, slog.Default())
			wsHandler := session.HandleTerminalWS(s, connMgr, registry, authSvc, slog.Default())
			eventsWSHandler := session.HandleEventsWS(authSvc, slog.Default())

			// Orchestrator (no-op executor for now)
			orch := orchestrator.NewOrchestrator(ctx, s, nil)
			jobHandler := handler.NewJobHandler(s, handlerClaimsGetter)
			runHandler := handler.NewRunHandler(s, orch, handlerClaimsGetter)

			// HTTP router
			handlers := api.NewHandlers(s, authSvc, connMgr, cfg.Auth.GetRegistrationMode(), cfg.Auth.InviteCode)
			router := api.NewRouter(handlers, sessionHandler, wsHandler, eventsWSHandler, jobHandler, runHandler)

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
			grpcSrv.GracefulStop()

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
