// Package main is the entry point for the claude-plane-bridge binary.
// The bridge connects external services (Telegram, Slack, etc.) to a
// claude-plane server via its REST API.
package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
	"github.com/kodrunhq/claude-plane/internal/bridge/config"
	"github.com/kodrunhq/claude-plane/internal/bridge/state"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "claude-plane-bridge",
		Short: "Bridge for connecting external services to claude-plane",
	}

	rootCmd.AddCommand(newServeCmd())

	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the bridge",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")

			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			slog.Info("Bridge config loaded",
				"api_url", cfg.ClaudePlane.APIURL,
				"state_path", cfg.State.Path,
				"health_address", cfg.Health.Address,
			)

			// Initialise REST client.
			_ = client.New(cfg.ClaudePlane.APIURL, cfg.ClaudePlane.APIKey)

			// Initialise state store and load persisted state from disk.
			stateStore := state.New(cfg.State.Path)
			if err := stateStore.Load(); err != nil {
				slog.Warn("Could not load bridge state", "path", cfg.State.Path, "error", err)
			}

			// Bridge lifecycle (Task 9) and connector instantiation will be wired here.
			slog.Info("Bridge initialised — lifecycle not yet implemented (Task 9)")

			return nil
		},
	}

	cmd.Flags().String("config", "bridge.toml", "Path to bridge TOML config file")
	return cmd
}
