package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/kodrunhq/claude-plane/internal/agent"
	"github.com/kodrunhq/claude-plane/internal/agent/config"
)

var version = "0.1.0-dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "claude-plane-agent",
		Short:   "Agent for Claude CLI session management on worker machines",
		Version: version,
	}

	rootCmd.AddCommand(
		newRunCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Connect to the server and start managing sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")

			cfg, err := config.LoadAgentConfig(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			shutdownTimeout, err := cfg.Shutdown.ParseTimeout()
			if err != nil {
				return fmt.Errorf("parse shutdown timeout: %w", err)
			}

			// Root context cancelled on SIGINT or SIGTERM.
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			// Ensure data directory exists.
			if err := os.MkdirAll(cfg.Agent.DataDir, 0o750); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}

			// Session manager (handles PTY sessions).
			sessionMgr := agent.NewSessionManager(cfg.Agent.ClaudeCLIPath, cfg.Agent.DataDir, slog.Default())

			// gRPC client with reconnection.
			client, err := agent.NewAgentClient(cfg, sessionMgr, slog.Default())
			if err != nil {
				return fmt.Errorf("create agent client: %w", err)
			}

			slog.Info("Agent starting",
				"machine_id", cfg.Agent.MachineID,
				"server_address", cfg.Server.Address,
				"max_sessions", cfg.Agent.MaxSessions,
				"shutdown_timeout", shutdownTimeout,
			)

			// Run the gRPC client (blocks until ctx is cancelled, auto-reconnects).
			go func() {
				if err := client.Run(ctx); err != nil && ctx.Err() == nil {
					slog.Error("agent client error", "error", err)
				}
			}()

			// Block until shutdown signal.
			<-ctx.Done()
			slog.Info("Shutdown signal received, starting graceful shutdown",
				"timeout", shutdownTimeout,
			)

			slog.Info("Agent shutdown complete")
			return nil
		},
	}
	cmd.Flags().String("config", "agent.toml", "Path to agent TOML config file")
	return cmd
}
