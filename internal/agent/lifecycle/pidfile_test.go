package lifecycle

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestCheckPIDFile_NoPIDFile(t *testing.T) {
	dir := t.TempDir()

	pid, alive, err := CheckPIDFile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != 0 {
		t.Errorf("expected pid 0, got %d", pid)
	}
	if alive {
		t.Error("expected alive=false, got true")
	}
}

func TestCheckPIDFile_StalePID(t *testing.T) {
	dir := t.TempDir()

	// Write a PID that almost certainly doesn't exist
	err := os.WriteFile(filepath.Join(dir, pidFileName), []byte("999999999"), 0o644)
	if err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}

	pid, alive, err := CheckPIDFile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != 999999999 {
		t.Errorf("expected pid 999999999, got %d", pid)
	}
	if alive {
		t.Error("expected alive=false for stale PID")
	}
}

func TestCheckPIDFile_AlivePID(t *testing.T) {
	dir := t.TempDir()

	currentPID := os.Getpid()
	err := os.WriteFile(filepath.Join(dir, pidFileName), []byte(strconv.Itoa(currentPID)), 0o644)
	if err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}

	pid, alive, err := CheckPIDFile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != currentPID {
		t.Errorf("expected pid %d, got %d", currentPID, pid)
	}
	if !alive {
		t.Error("expected alive=true for current process PID")
	}
}

func TestWritePIDFile(t *testing.T) {
	dir := t.TempDir()

	cleanup, err := WritePIDFile(dir)
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	// Verify the PID file is readable and reports our PID as alive
	pid, alive, err := CheckPIDFile(dir)
	if err != nil {
		t.Fatalf("CheckPIDFile failed: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("expected pid %d, got %d", os.Getpid(), pid)
	}
	if !alive {
		t.Error("expected alive=true")
	}

	// Cleanup removes the file
	cleanup()

	_, err = os.Stat(filepath.Join(dir, pidFileName))
	if !os.IsNotExist(err) {
		t.Error("expected pid file to be removed after cleanup")
	}

	// Double cleanup should not panic
	cleanup()
}

func TestWritePIDFile_RemovesStale(t *testing.T) {
	dir := t.TempDir()

	// Write a stale PID first
	err := os.WriteFile(filepath.Join(dir, pidFileName), []byte("999999999"), 0o644)
	if err != nil {
		t.Fatalf("failed to write stale pid file: %v", err)
	}

	cleanup, err := WritePIDFile(dir)
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}
	defer cleanup()

	// Should now contain current PID
	pid, alive, err := CheckPIDFile(dir)
	if err != nil {
		t.Fatalf("CheckPIDFile failed: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("expected pid %d, got %d", os.Getpid(), pid)
	}
	if !alive {
		t.Error("expected alive=true after overwriting stale PID")
	}
}
