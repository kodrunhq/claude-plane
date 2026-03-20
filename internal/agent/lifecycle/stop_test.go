package lifecycle

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestStopExisting_RemovesStalePIDFile(t *testing.T) {
	dir := t.TempDir()

	// Write a PID file with a PID that almost certainly does not exist.
	pidPath := filepath.Join(dir, pidFileName)
	if err := os.WriteFile(pidPath, []byte("999999999"), 0o644); err != nil {
		t.Fatalf("writing stale pid file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	if err := StopExisting(dir, logger); err != nil {
		t.Fatalf("StopExisting returned error: %v", err)
	}

	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("expected stale pid file to be removed, but it still exists")
	}
}

func TestStopExisting_NoDataDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Use a directory that does not exist.
	err := StopExisting("/tmp/claude-plane-test-nonexistent-dir-"+t.Name(), logger)
	if err != nil {
		t.Fatalf("StopExisting with non-existent dir returned error: %v", err)
	}
}
