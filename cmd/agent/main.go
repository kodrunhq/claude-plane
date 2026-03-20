package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/kodrunhq/claude-plane/internal/agent"
	"github.com/kodrunhq/claude-plane/internal/agent/config"
	"github.com/kodrunhq/claude-plane/internal/agent/lifecycle"
	"github.com/kodrunhq/claude-plane/internal/shared/buildinfo"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "claude-plane-agent",
		Short:   "Agent for Claude CLI session management on worker machines",
		Version: buildinfo.String(),
	}

	rootCmd.AddCommand(
		newRunCmd(),
		newJoinCmd(),
		newInstallServiceCmd(),
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

			// --- PID lock ---
			pid, alive, err := lifecycle.CheckPIDFile(cfg.Agent.DataDir)
			if err != nil {
				slog.Warn("failed to check PID file", "error", err)
			} else if alive {
				return fmt.Errorf("agent already running (PID %d). Stop it first or use 'join' to re-register", pid)
			} else if pid != 0 {
				slog.Warn("removed stale PID file", "pid", pid)
				lifecycle.RemovePIDFile(cfg.Agent.DataDir)
			}

			pidCleanup, err := lifecycle.WritePIDFile(cfg.Agent.DataDir)
			if err != nil {
				return fmt.Errorf("write PID file: %w", err)
			}
			defer pidCleanup()

			// Reap orphaned claude processes from a previous crash.
			lifecycle.ReapOrphanedProcesses(slog.Default())

			// Build idle detector options from config.
			var idleOpts []agent.IdleDetectorOption
			if cfg.Agent.IdlePromptMarker != "" {
				idleOpts = append(idleOpts, agent.WithPromptMarker([]byte(cfg.Agent.IdlePromptMarker)))
			}
			if cfg.Agent.IdleStartupTimeout != "" {
				if d, err := time.ParseDuration(cfg.Agent.IdleStartupTimeout); err == nil {
					idleOpts = append(idleOpts, agent.WithStartupTimeout(d))
				} else {
					slog.Warn("invalid idle_startup_timeout, using default", "value", cfg.Agent.IdleStartupTimeout, "error", err)
				}
			}

			// Session manager (handles PTY sessions).
			sessionMgr := agent.NewSessionManager(cfg.Agent.ClaudeCLIPath, cfg.Agent.DataDir, slog.Default(), idleOpts...)

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

			// Run the gRPC client — blocks until ctx is cancelled, auto-reconnects.
			// Running in the main goroutine ensures the process waits for the client
			// to unwind and close the gRPC connection cleanly before exiting.
			if err := client.Run(ctx); err != nil && ctx.Err() == nil {
				return fmt.Errorf("agent client: %w", err)
			}

			slog.Info("Agent shutdown complete")
			return nil
		},
	}
	cmd.Flags().String("config", "agent.toml", "Path to agent TOML config file")
	return cmd
}

func newJoinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "join CODE",
		Short: "Join a server using a 6-character provisioning code",
		Long: `Redeems a short provisioning code to configure this agent with TLS certificates
and server connection details. Any existing agent (service or process) is automatically
stopped before reconfiguring. Use --service to install as a system service in one step.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			code := args[0]
			serverFlag, _ := cmd.Flags().GetString("server")
			configDir, _ := cmd.Flags().GetString("config-dir")
			insecure, _ := cmd.Flags().GetBool("insecure")
			installService, _ := cmd.Flags().GetBool("service")

			if os.Getuid() == 0 && installService {
				return fmt.Errorf("do not run 'join' as root. Run as your normal user — only the service installation needs sudo")
			}

			serverURL, err := agent.ResolveServerURL(serverFlag)
			if err != nil {
				return err
			}

			if err := agent.ValidateServerURL(serverURL, insecure); err != nil {
				return err
			}

			// Stop any existing agent before re-configuring.
			dataDir := filepath.Join(configDir, "data")
			os.MkdirAll(dataDir, 0o750)
			lifecycle.StopExisting(dataDir, slog.Default())

			if err := agent.ExecuteJoin(serverURL, code, configDir); err != nil {
				return err
			}

			configPath := filepath.Join(configDir, "agent.toml")
			fmt.Printf("\nAgent configured for machine joining\n")
			fmt.Printf("Certificates written to %s/certs/\n", configDir)
			fmt.Printf("Config written to %s\n\n", configPath)

			if installService {
				binPath, err := os.Executable()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not determine binary path: %v\n", err)
					fmt.Printf("Install the service manually:\n")
					fmt.Printf("  sudo claude-plane-agent install-service --config %s\n\n", configPath)
					return nil
				}
				binPath, _ = filepath.EvalSymlinks(binPath)
				absConfig, _ := filepath.Abs(configPath)

				fmt.Printf("Installing systemd service (requires sudo)...\n\n")
				sudoCmd := exec.Command("sudo", binPath, "install-service", "--config", absConfig)
				sudoCmd.Stdin = os.Stdin
				sudoCmd.Stdout = os.Stdout
				sudoCmd.Stderr = os.Stderr
				if err := sudoCmd.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "\nWarning: service installation failed: %v\n", err)
					fmt.Printf("You can install the service manually:\n")
					fmt.Printf("  sudo %s install-service --config %s\n\n", binPath, absConfig)
				}
			} else {
				fmt.Printf("Start the agent:\n")
				fmt.Printf("  claude-plane-agent run --config %s\n\n", configPath)
				fmt.Printf("Install as a background service (recommended):\n")
				fmt.Printf("  sudo claude-plane-agent install-service --config %s\n\n", configPath)
			}
			return nil
		},
	}

	// Determine default config dir based on user
	defaultConfigDir := os.Getenv("HOME") + "/.claude-plane"
	if os.Getuid() == 0 {
		defaultConfigDir = "/etc/claude-plane"
	}

	cmd.Flags().String("server", "", "Server HTTP URL (falls back to CLAUDE_PLANE_SERVER env var)")
	cmd.Flags().String("config-dir", defaultConfigDir, "Directory for config and certificates")
	cmd.Flags().Bool("insecure", false, "Allow plain HTTP server URL (prints warning)")
	cmd.Flags().Bool("service", false, "Install and start the agent as a system service after joining (requires sudo)")
	return cmd
}

func newInstallServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install-service",
		Short: "Install agent as a system service (systemd on Linux, launchd on macOS)",
		Long: `Installs the claude-plane-agent as a background system service that starts
on boot and restarts automatically if it crashes. Safe to run multiple times —
any existing service is stopped and replaced.

Requires root/sudo on Linux (systemd) or macOS (launchd).
The agent binary must already be in a system-accessible location.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			user, _ := cmd.Flags().GetString("user")

			// Resolve absolute paths for the service file.
			absConfig, err := filepath.Abs(configPath)
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}

			// Find the current binary path.
			binPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("find executable path: %w", err)
			}
			binPath, err = filepath.EvalSymlinks(binPath)
			if err != nil {
				return fmt.Errorf("resolve executable path: %w", err)
			}

			// Verify the config file exists.
			if _, err := os.Stat(absConfig); os.IsNotExist(err) {
				return fmt.Errorf("config file not found: %s\nRun 'claude-plane-agent join' first to configure the agent", absConfig)
			}

			return installService(binPath, absConfig, user)
		},
	}
	cmd.Flags().String("config", os.Getenv("HOME")+"/.claude-plane/agent.toml", "Path to agent TOML config file")
	cmd.Flags().String("user", "", "User to run the service as (default: current user)")
	return cmd
}
