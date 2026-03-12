package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/kodrunhq/claude-plane/internal/agent/config"

	// Prove generated proto package compiles.
	_ "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
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

			slog.Info("Agent starting",
				"machine_id", cfg.Agent.MachineID,
				"server_address", cfg.Server.Address,
				"max_sessions", cfg.Agent.MaxSessions,
				"shutdown_timeout", shutdownTimeout,
			)

			// TODO: Establish gRPC connection to server here (pass ctx).
			// TODO: Start session manager / PTY supervisor here.

			// Block until shutdown signal.
			<-ctx.Done()
			slog.Info("Shutdown signal received, starting graceful shutdown",
				"timeout", shutdownTimeout,
			)

			// Create a timeout context for the shutdown sequence.
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer shutdownCancel()
			_ = shutdownCtx // used by shutdown calls below

			// TODO: Close all active PTY sessions gracefully (use shutdownCtx for deadline).
			// TODO: Disconnect gRPC stream so the server knows we're leaving.

			slog.Info("Agent shutdown complete")
			return nil
		},
	}
	cmd.Flags().String("config", "agent.toml", "Path to agent TOML config file")
	return cmd
}
