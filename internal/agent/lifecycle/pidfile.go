package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const pidFileName = "agent.pid"

// CheckPIDFile reads the PID file from dataDir and checks whether the
// recorded process is still alive. Returns (0, false, nil) when no PID
// file exists, (pid, false, nil) when the file exists but the process is
// stale, and (pid, true, nil) when the process is alive.
func CheckPIDFile(dataDir string) (int, bool, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, pidFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("reading pid file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false, fmt.Errorf("parsing pid file: %w", err)
	}

	// Signal 0 checks existence without actually sending a signal.
	if err := syscall.Kill(pid, 0); err != nil {
		return pid, false, nil
	}

	return pid, true, nil
}

// WritePIDFile writes the current process PID to {dataDir}/agent.pid.
// It returns a cleanup function that removes the file. The cleanup
// function is idempotent and safe to call multiple times.
func WritePIDFile(dataDir string) (func(), error) {
	pidPath := filepath.Join(dataDir, pidFileName)
	content := []byte(strconv.Itoa(os.Getpid()))

	if err := os.WriteFile(pidPath, content, 0o644); err != nil {
		return nil, fmt.Errorf("writing pid file: %w", err)
	}

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_ = os.Remove(pidPath)
		})
	}

	return cleanup, nil
}

// RemovePIDFile removes the PID file from dataDir. It is a no-op if the
// file does not exist.
func RemovePIDFile(dataDir string) {
	_ = os.Remove(filepath.Join(dataDir, pidFileName))
}
