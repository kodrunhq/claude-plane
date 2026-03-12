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
	return &cobra.Command{
		Use:   "run",
		Short: "Connect to the server and start managing sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
}
