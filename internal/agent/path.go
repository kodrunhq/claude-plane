package agent

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// EnrichPATH resolves the user's full PATH by sourcing their login shell
// profile. This ensures commands like "claude" (installed via npm in ~/.local/bin
// or ~/.nvm/.../bin) are discoverable even when the agent starts from a
// non-login context (systemd service, bare SSH, etc.).
func EnrichPATH(logger *slog.Logger) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
		if _, err := os.Stat(shell); err != nil {
			shell = "/bin/sh"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, shell, "-lc", "echo $PATH").Output()
	if err != nil {
		logger.Warn("could not resolve login shell PATH", "shell", shell, "error", err)
		return
	}

	loginPath := strings.TrimSpace(string(out))
	if loginPath == "" {
		return
	}

	currentPath := os.Getenv("PATH")
	merged := mergePaths(loginPath, currentPath)

	if merged != currentPath {
		os.Setenv("PATH", merged)
		logger.Info("enriched PATH from login shell",
			"shell", shell,
			"added_dirs", countNewDirs(currentPath, merged))
	}
}

// mergePaths combines loginPath and currentPath, deduplicating entries.
// Login shell paths come first (higher priority).
func mergePaths(loginPath, currentPath string) string {
	seen := make(map[string]bool)
	var parts []string

	for _, p := range strings.Split(loginPath, ":") {
		if p != "" && !seen[p] {
			seen[p] = true
			parts = append(parts, p)
		}
	}
	for _, p := range strings.Split(currentPath, ":") {
		if p != "" && !seen[p] {
			seen[p] = true
			parts = append(parts, p)
		}
	}

	return strings.Join(parts, ":")
}

func countNewDirs(before, after string) int {
	beforeSet := make(map[string]bool)
	for _, p := range strings.Split(before, ":") {
		beforeSet[p] = true
	}
	count := 0
	for _, p := range strings.Split(after, ":") {
		if !beforeSet[p] {
			count++
		}
	}
	return count
}
