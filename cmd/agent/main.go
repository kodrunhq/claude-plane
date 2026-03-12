package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/claudeplane/claude-plane/internal/agent/config"

	// Prove generated proto package compiles.
	_ "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
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

			slog.Info("Agent config loaded",
				"machine_id", cfg.Agent.MachineID,
				"server_address", cfg.Server.Address,
				"max_sessions", cfg.Agent.MaxSessions,
			)
			slog.Info("Agent ready (gRPC connection not yet implemented — Phase 2)")
			return nil
		},
	}
	cmd.Flags().String("config", "agent.toml", "Path to agent TOML config file")
	return cmd
}
