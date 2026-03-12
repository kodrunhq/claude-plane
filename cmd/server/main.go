package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

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
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the control plane server",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
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
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the certificate authority",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
}

func newCAIssueServerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "issue-server",
		Short: "Issue a server TLS certificate",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
}

func newCAIssueAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "issue-agent",
		Short: "Issue an agent mTLS certificate",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
}

func newSeedAdminCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed-admin",
		Short: "Create the initial admin account",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
}
