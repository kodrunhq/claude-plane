package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/claudeplane/claude-plane/internal/server/config"
	"github.com/claudeplane/claude-plane/internal/server/store"
	"github.com/claudeplane/claude-plane/internal/shared/tlsutil"

	// Prove generated proto package compiles.
	_ "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
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

			s, err := store.NewStore(cfg.Database.Path)
			if err != nil {
				return fmt.Errorf("initialize database: %w", err)
			}
			defer s.Close()

			slog.Info("Server initialized with database", "path", cfg.Database.Path)
			slog.Info("Server ready (full serve loop not yet implemented — Phase 2+)")
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
			} else {
				// Read from stdin
				fmt.Fprint(os.Stderr, "Enter admin password: ")
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					password = scanner.Text()
				}
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("reading password from stdin: %w", err)
				}
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
