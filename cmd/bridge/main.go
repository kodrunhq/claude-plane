// Package main is the entry point for the claude-plane-bridge binary.
// The bridge connects external services (Telegram, Slack, etc.) to a
// claude-plane server via its REST API.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/kodrunhq/claude-plane/internal/bridge"
	"github.com/kodrunhq/claude-plane/internal/bridge/client"
	"github.com/kodrunhq/claude-plane/internal/bridge/config"
	"github.com/kodrunhq/claude-plane/internal/bridge/connector/github"
	"github.com/kodrunhq/claude-plane/internal/bridge/connector/telegram"
	"github.com/kodrunhq/claude-plane/internal/bridge/state"
	"github.com/kodrunhq/claude-plane/internal/shared/buildinfo"
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

			apiClient := client.New(cfg.ClaudePlane.APIURL, cfg.ClaudePlane.APIKey)

			// Set up log forwarding to the server.
			logFwd := bridge.NewLogForwarder(apiClient, "bridge", bridge.WithMaxBatch(50), bridge.WithFlushInterval(2*time.Second))
			slog.SetDefault(slog.New(logFwd))
			defer logFwd.Close()

			// Set up lifecycle event emitter.
			emitter := bridge.NewEventEmitter(apiClient, slog.Default())
			emitter.Emit("bridge.started", map[string]any{"version": buildinfo.Version})
			defer emitter.Emit("bridge.stopped", map[string]any{"reason": "shutdown"})

			stateStore := state.New(cfg.State.Path)
			if err := stateStore.Load(); err != nil {
				slog.Warn("Could not load bridge state", "path", cfg.State.Path, "error", err)
			}

			b := bridge.New(apiClient, stateStore, cfg.Health.Address, slog.Default())

			connConfigs, err := apiClient.GetConnectorConfigs(cmd.Context())
			if err != nil {
				return fmt.Errorf("fetch connector configs: %w", err)
			}

			for _, cc := range connConfigs {
				if !cc.Enabled {
					continue
				}
				switch cc.ConnectorType {
				case "telegram":
					var tCfg telegram.Config
					if err := json.Unmarshal([]byte(cc.Config), &tCfg); err != nil {
						slog.Error("parse telegram config", "connector_id", cc.ConnectorID, "error", err)
						continue
					}
					if cc.ConfigSecret != "" {
						var secretCfg struct {
							BotToken string `json:"bot_token"`
						}
						if err := json.Unmarshal([]byte(cc.ConfigSecret), &secretCfg); err == nil {
							tCfg.BotToken = secretCfg.BotToken
						}
					}
					conn := telegram.New(cc.ConnectorID, tCfg, apiClient, stateStore, slog.Default())
					b.AddConnector(conn)
					slog.Info("Registered connector", "type", "telegram", "name", cc.Name, "id", cc.ConnectorID)
					emitter.Emit("bridge.connector.started", map[string]any{
						"connector_id":   cc.ConnectorID,
						"connector_type": cc.ConnectorType,
						"name":           cc.Name,
					})
				case "github":
					var gCfg github.Config
					if err := json.Unmarshal([]byte(cc.Config), &gCfg); err != nil {
						slog.Error("parse github config", "connector_id", cc.ConnectorID, "error", err)
						continue
					}
					if cc.ConfigSecret != "" {
						var secretCfg struct {
							Token string `json:"token"`
						}
						if err := json.Unmarshal([]byte(cc.ConfigSecret), &secretCfg); err == nil {
							gCfg.Token = secretCfg.Token
						}
					}
					conn := github.New(cc.ConnectorID, gCfg, apiClient, stateStore, slog.Default())
					b.AddConnector(conn)
					slog.Info("Registered connector", "type", "github", "name", cc.Name, "id", cc.ConnectorID)
					emitter.Emit("bridge.connector.started", map[string]any{
						"connector_id":   cc.ConnectorID,
						"connector_type": cc.ConnectorType,
						"name":           cc.Name,
					})
				default:
					slog.Warn("Unknown connector type", "type", cc.ConnectorType, "id", cc.ConnectorID)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			return b.Run(ctx)
		},
	}

	cmd.Flags().String("config", "bridge.toml", "Path to bridge TOML config file")
	return cmd
}
