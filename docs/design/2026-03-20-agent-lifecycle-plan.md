# Agent Lifecycle Management — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add full lifecycle management to the agent binary — PID lock, orphan reaping, auto-stop on re-join, `--service` flag, `uninstall-service` command, and updated documentation.

**Architecture:** New `internal/agent/lifecycle/` package provides shared utilities (PID file, process detection, stop logic) used by `cmd/agent/` commands. The `join` command gains `--service` flag and auto-stop. The `run` command gains PID lock and orphan reaping. A new `uninstall-service` command handles teardown.

**Tech Stack:** Go 1.25, `os/exec`, `syscall`, `/proc` filesystem (Linux), `pgrep`/`ps` (macOS), cobra CLI framework.

**Spec:** `docs/design/2026-03-20-agent-lifecycle-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/agent/lifecycle/pidfile.go` | PID file write/check/remove |
| `internal/agent/lifecycle/process.go` | Process scanning, signal sending, orphan detection |
| `internal/agent/lifecycle/service.go` | systemd/launchd detection and stop |
| `internal/agent/lifecycle/stop.go` | `StopExisting` orchestrator combining all three |
| `internal/agent/lifecycle/pidfile_test.go` | PID file unit tests |
| `internal/agent/lifecycle/process_test.go` | Process scanning tests |
| `internal/agent/lifecycle/stop_test.go` | StopExisting integration tests |
| `cmd/agent/main.go` | Modified: PID lock in `run`, `--service` in `join`, `uninstall-service` command |
| `cmd/agent/service.go` | Modified: idempotent guards, `uninstallSystemd`/`uninstallLaunchd`, macOS `bootstrap`/`bootout` |
| `internal/agent/join.go` | Modified: call `lifecycle.StopExisting` before join |
| `docs/install-agent.md` | Rewritten: quick join primary, uninstall section |
| `docs/quickstart.md` | Modified: simplified agent setup |
| `docs/configuration.md` | Modified: new CLI commands, PID file docs |
| `CLAUDE.md` | Modified: agent architecture table, CLI reference |

---

## Task 1: PID File Utilities

**Files:**
- Create: `internal/agent/lifecycle/pidfile.go`
- Create: `internal/agent/lifecycle/pidfile_test.go`

- [ ] **Step 1: Write failing tests for CheckPIDFile**

```go
// internal/agent/lifecycle/pidfile_test.go
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
	if pid != 0 || alive {
		t.Fatalf("expected (0, false), got (%d, %t)", pid, alive)
	}
}

func TestCheckPIDFile_StalePID(t *testing.T) {
	dir := t.TempDir()
	// Write a PID that definitely doesn't exist (max int).
	if err := os.WriteFile(filepath.Join(dir, "agent.pid"), []byte("999999999"), 0o644); err != nil {
		t.Fatal(err)
	}
	pid, alive, err := CheckPIDFile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != 999999999 || alive {
		t.Fatalf("expected (999999999, false), got (%d, %t)", pid, alive)
	}
}

func TestCheckPIDFile_AlivePID(t *testing.T) {
	dir := t.TempDir()
	// Current process is definitely alive.
	myPID := os.Getpid()
	if err := os.WriteFile(filepath.Join(dir, "agent.pid"), []byte(strconv.Itoa(myPID)), 0o644); err != nil {
		t.Fatal(err)
	}
	pid, alive, err := CheckPIDFile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != myPID || !alive {
		t.Fatalf("expected (%d, true), got (%d, %t)", myPID, pid, alive)
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go test ./internal/agent/lifecycle/ -v -run TestCheckPIDFile`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement CheckPIDFile**

```go
// internal/agent/lifecycle/pidfile.go
package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const pidFileName = "agent.pid"

// CheckPIDFile reads the PID file from dataDir and checks if the process is alive.
// Returns (0, false, nil) if no PID file exists.
// Returns (pid, false, nil) if PID file exists but process is dead.
// Returns (pid, true, nil) if process is alive.
func CheckPIDFile(dataDir string) (int, bool, error) {
	pidPath := filepath.Join(dataDir, pidFileName)
	data, err := os.ReadFile(pidPath)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read pid file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false, fmt.Errorf("parse pid file: %w", err)
	}

	// Signal 0 checks if process exists without sending a signal.
	err = syscall.Kill(pid, 0)
	if err == nil {
		return pid, true, nil
	}
	return pid, false, nil
}
```

- [ ] **Step 4: Run tests — verify they pass**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go test ./internal/agent/lifecycle/ -v -run TestCheckPIDFile`
Expected: PASS (3/3).

- [ ] **Step 5: Write failing tests for WritePIDFile**

Add to `pidfile_test.go`:

```go
func TestWritePIDFile(t *testing.T) {
	dir := t.TempDir()
	cleanup, err := WritePIDFile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PID file should exist with our PID.
	pid, alive, err := CheckPIDFile(dir)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if pid != os.Getpid() || !alive {
		t.Fatalf("expected current PID alive, got (%d, %t)", pid, alive)
	}

	// Cleanup should remove the file.
	cleanup()
	pidPath := filepath.Join(dir, "agent.pid")
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatal("PID file should be removed after cleanup")
	}

	// Double cleanup should not panic.
	cleanup()
}

func TestWritePIDFile_RemovesStale(t *testing.T) {
	dir := t.TempDir()
	// Write a stale PID file.
	if err := os.WriteFile(filepath.Join(dir, "agent.pid"), []byte("999999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	// WritePIDFile should succeed (stale PID is dead).
	cleanup, err := WritePIDFile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	pid, alive, err := CheckPIDFile(dir)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if pid != os.Getpid() || !alive {
		t.Fatalf("expected current PID, got (%d, %t)", pid, alive)
	}
}
```

- [ ] **Step 6: Run test — verify it fails**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go test ./internal/agent/lifecycle/ -v -run TestWritePIDFile`
Expected: FAIL — `WritePIDFile` not defined.

- [ ] **Step 7: Implement WritePIDFile and RemovePIDFile**

Add to `pidfile.go`:

```go
// WritePIDFile writes the current process PID to dataDir/agent.pid.
// Returns a cleanup function that removes the file. Safe to call multiple times.
func WritePIDFile(dataDir string) (func(), error) {
	pidPath := filepath.Join(dataDir, pidFileName)
	content := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(pidPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write pid file: %w", err)
	}

	var removed bool
	cleanup := func() {
		if removed {
			return
		}
		removed = true
		os.Remove(pidPath)
	}
	return cleanup, nil
}

// RemovePIDFile removes the PID file from dataDir. No error if it doesn't exist.
func RemovePIDFile(dataDir string) {
	os.Remove(filepath.Join(dataDir, pidFileName))
}
```

- [ ] **Step 8: Run all PID file tests**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go test ./internal/agent/lifecycle/ -v -run TestCheckPIDFile\|TestWritePIDFile`
Expected: PASS (5/5).

- [ ] **Step 9: Commit**

```bash
git add internal/agent/lifecycle/pidfile.go internal/agent/lifecycle/pidfile_test.go
git commit -m "feat: add PID file utilities for agent lifecycle management"
```

---

## Task 2: Process Scanning and Signal Utilities

**Files:**
- Create: `internal/agent/lifecycle/process.go`
- Create: `internal/agent/lifecycle/process_test.go`

- [ ] **Step 1: Write failing test for FindAgentProcesses**

```go
// internal/agent/lifecycle/process_test.go
package lifecycle

import (
	"os"
	"testing"
)

func TestFindAgentProcesses_ExcludesSelf(t *testing.T) {
	// FindAgentProcesses should never return the current process.
	pids, err := FindAgentProcesses()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	myPID := os.Getpid()
	for _, p := range pids {
		if p == myPID {
			t.Fatal("FindAgentProcesses returned current process PID")
		}
	}
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go test ./internal/agent/lifecycle/ -v -run TestFindAgentProcesses`
Expected: FAIL — `FindAgentProcesses` not defined.

- [ ] **Step 3: Implement process scanning (Linux /proc + macOS pgrep)**

```go
// internal/agent/lifecycle/process.go
package lifecycle

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// FindAgentProcesses returns PIDs of running "claude-plane-agent run" processes
// owned by the current user, excluding the current process.
func FindAgentProcesses() ([]int, error) {
	switch runtime.GOOS {
	case "linux":
		return findAgentProcessesLinux()
	case "darwin":
		return findAgentProcessesDarwin()
	default:
		return nil, nil
	}
}

func findAgentProcessesLinux() ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}

	myPID := os.Getpid()
	myUID := os.Getuid()
	var pids []int

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // not a PID directory
		}
		if pid == myPID {
			continue
		}

		// Check ownership.
		info, err := os.Stat(filepath.Join("/proc", entry.Name()))
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != myUID {
			continue
		}

		// Check command line.
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		cmdStr := string(cmdline)
		if strings.Contains(cmdStr, "claude-plane-agent") && strings.Contains(cmdStr, "run") {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

func findAgentProcessesDarwin() ([]int, error) {
	cmd := exec.Command("pgrep", "-f", "claude-plane-agent run")
	out, err := cmd.Output()
	if err != nil {
		// pgrep exits 1 when no processes match — that's fine.
		return nil, nil
	}

	myPID := os.Getpid()
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil || pid == myPID {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// FindOrphanedClaudeProcesses returns PIDs of `claude` processes with PPID 1
// (reparented orphans) owned by the current user.
func FindOrphanedClaudeProcesses() ([]int, error) {
	switch runtime.GOOS {
	case "linux":
		return findOrphanedClaudeLinux()
	case "darwin":
		return findOrphanedClaudeDarwin()
	default:
		return nil, nil
	}
}

func findOrphanedClaudeLinux() ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}

	myUID := os.Getuid()
	var pids []int

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// Check ownership.
		info, err := os.Stat(filepath.Join("/proc", entry.Name()))
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != myUID {
			continue
		}

		// Check PPID == 1 (orphan).
		statusData, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "status"))
		if err != nil {
			continue
		}
		if !hasPPID1(string(statusData)) {
			continue
		}

		// Check command line contains "claude".
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if strings.Contains(string(cmdline), "claude") {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

func hasPPID1(statusContent string) bool {
	for _, line := range strings.Split(statusContent, "\n") {
		if strings.HasPrefix(line, "PPid:") {
			ppid := strings.TrimSpace(strings.TrimPrefix(line, "PPid:"))
			return ppid == "1"
		}
	}
	return false
}

func findOrphanedClaudeDarwin() ([]int, error) {
	// ps -eo pid,ppid,comm — filter for ppid=1 and "claude" in command.
	cmd := exec.Command("ps", "-eo", "pid,ppid,comm")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	myPID := os.Getpid()
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid == myPID {
			continue
		}
		ppid := fields[1]
		comm := strings.Join(fields[2:], " ")
		if ppid == "1" && strings.Contains(comm, "claude") {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// SignalAndWait sends SIGTERM to a process, waits up to timeout for it to exit,
// then sends SIGKILL if still alive.
func SignalAndWait(pid int, timeout time.Duration, logger *slog.Logger) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		logger.Debug("SIGTERM failed", "pid", pid, "error", err)
		return
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			logger.Warn("process did not exit after SIGTERM, sending SIGKILL", "pid", pid)
			_ = proc.Signal(syscall.SIGKILL)
			return
		case <-ticker.C:
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				// Process exited.
				return
			}
		}
	}
}

// ReapOrphanedProcesses finds and kills orphaned claude processes.
func ReapOrphanedProcesses(logger *slog.Logger) error {
	pids, err := FindOrphanedClaudeProcesses()
	if err != nil {
		logger.Warn("failed to scan for orphaned processes", "error", err)
		return nil // best-effort
	}

	for _, pid := range pids {
		logger.Info("Reaped orphaned session process", "pid", pid)
		SignalAndWait(pid, 3*time.Second, logger)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go test ./internal/agent/lifecycle/ -v -run TestFindAgentProcesses`
Expected: PASS.

- [ ] **Step 5: Run go vet on the new package**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go vet ./internal/agent/lifecycle/`
Expected: Clean.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/lifecycle/process.go internal/agent/lifecycle/process_test.go
git commit -m "feat: add process scanning and signal utilities for lifecycle management"
```

---

## Task 3: Service Detection and StopExisting Orchestrator

**Files:**
- Create: `internal/agent/lifecycle/service.go`
- Create: `internal/agent/lifecycle/stop.go`
- Create: `internal/agent/lifecycle/stop_test.go`

- [ ] **Step 1: Implement service detection**

```go
// internal/agent/lifecycle/service.go
package lifecycle

import (
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	systemdServiceName = "claude-plane-agent"
	systemdServicePath = "/etc/systemd/system/claude-plane-agent.service"
	launchdLabel       = "com.claude-plane.agent"
	launchdPlistPath   = "/Library/LaunchDaemons/com.claude-plane.agent.plist"
)

// StopServiceIfActive checks if the agent system service is running and stops it.
// Returns true if a service was found and stopped.
func StopServiceIfActive(logger *slog.Logger) bool {
	switch runtime.GOOS {
	case "linux":
		return stopSystemdIfActive(logger)
	case "darwin":
		return stopLaunchdIfActive(logger)
	default:
		return false
	}
}

func stopSystemdIfActive(logger *slog.Logger) bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", systemdServiceName)
	if err := cmd.Run(); err != nil {
		return false // not active or systemctl not available
	}

	logger.Info("Stopped existing agent (systemd service)")
	stopCmd := exec.Command("systemctl", "stop", systemdServiceName)
	stopCmd.Stdout = os.Stdout
	stopCmd.Stderr = os.Stderr
	_ = stopCmd.Run()
	return true
}

func stopLaunchdIfActive(logger *slog.Logger) bool {
	cmd := exec.Command("launchctl", "list")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	if !strings.Contains(string(out), launchdLabel) {
		return false
	}

	logger.Info("Stopped existing agent (launchd service)")
	bootout := exec.Command("launchctl", "bootout", "system/"+launchdLabel)
	bootout.Stdout = os.Stdout
	bootout.Stderr = os.Stderr
	_ = bootout.Run()
	return true
}

// IsServiceInstalled checks if the service file/plist exists on disk.
func IsServiceInstalled() bool {
	switch runtime.GOOS {
	case "linux":
		_, err := os.Stat(systemdServicePath)
		return err == nil
	case "darwin":
		_, err := os.Stat(launchdPlistPath)
		return err == nil
	default:
		return false
	}
}
```

- [ ] **Step 2: Implement StopExisting orchestrator**

```go
// internal/agent/lifecycle/stop.go
package lifecycle

import (
	"log/slog"
	"time"
)

// StopExisting stops any running agent using a three-layer detection approach:
// 1. systemd/launchd service
// 2. PID file
// 3. Process scan fallback
// Each step is attempted regardless of whether the previous step found anything.
func StopExisting(dataDir string, logger *slog.Logger) error {
	// 1. Stop system service if active.
	StopServiceIfActive(logger)

	// 2. Check PID file.
	pid, alive, err := CheckPIDFile(dataDir)
	if err != nil {
		logger.Warn("failed to check PID file", "error", err)
	} else if alive {
		logger.Info("Stopped existing agent (PID file)", "pid", pid)
		SignalAndWait(pid, 5*time.Second, logger)
		RemovePIDFile(dataDir)
	} else if pid != 0 {
		// Stale PID file — just remove it.
		logger.Info("Removed stale PID file", "pid", pid)
		RemovePIDFile(dataDir)
	}

	// 3. Process scan fallback.
	agentPIDs, err := FindAgentProcesses()
	if err != nil {
		logger.Warn("failed to scan for agent processes", "error", err)
	}
	for _, p := range agentPIDs {
		logger.Info("Stopped orphaned agent process", "pid", p)
		SignalAndWait(p, 3*time.Second, logger)
	}

	return nil
}
```

- [ ] **Step 3: Write test for StopExisting**

```go
// internal/agent/lifecycle/stop_test.go
package lifecycle

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestStopExisting_RemovesStalePIDFile(t *testing.T) {
	dir := t.TempDir()
	// Write a stale PID.
	if err := os.WriteFile(filepath.Join(dir, "agent.pid"), []byte("999999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.Default()
	if err := StopExisting(dir, logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PID file should be removed.
	if _, err := os.Stat(filepath.Join(dir, "agent.pid")); !os.IsNotExist(err) {
		t.Fatal("stale PID file should have been removed")
	}
}

func TestStopExisting_NoDataDir(t *testing.T) {
	// StopExisting should not error on a non-existent directory.
	logger := slog.Default()
	err := StopExisting("/tmp/nonexistent-lifecycle-test-dir", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 4: Run all lifecycle tests**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go test ./internal/agent/lifecycle/ -v`
Expected: All PASS.

- [ ] **Step 5: Run go vet**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go vet ./internal/agent/lifecycle/`
Expected: Clean.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/lifecycle/service.go internal/agent/lifecycle/stop.go internal/agent/lifecycle/stop_test.go
git commit -m "feat: add StopExisting orchestrator and service detection for lifecycle"
```

---

## Task 4: PID Lock in `run` Command

**Files:**
- Modify: `cmd/agent/main.go:39-107`

- [ ] **Step 1: Add lifecycle import and PID lock to newRunCmd**

In `cmd/agent/main.go`, add import `"github.com/kodrunhq/claude-plane/internal/agent/lifecycle"` and modify the `RunE` function in `newRunCmd`. Insert the following **after** the `os.MkdirAll` call for data dir (line 63) and **before** the idle opts block (line 65):

```go
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
```

- [ ] **Step 2: Build and verify**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go build ./cmd/agent/`
Expected: Clean build.

- [ ] **Step 3: Run go vet**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go vet ./cmd/agent/`
Expected: Clean.

- [ ] **Step 4: Commit**

```bash
git add cmd/agent/main.go
git commit -m "feat: add PID lock and orphan reaping to agent run command"
```

---

## Task 5: Enhanced `join` with `--service` and Auto-Stop

**Files:**
- Modify: `cmd/agent/main.go:109-156`
- Modify: `internal/agent/join.go`

- [ ] **Step 1: Add lifecycle.StopExisting call to ExecuteJoin flow**

In `cmd/agent/main.go`, modify the `newJoinCmd` `RunE` function. **Before** the `agent.ExecuteJoin` call, add:

```go
			// Stop any existing agent before re-configuring.
			dataDir := filepath.Join(configDir, "data")
			os.MkdirAll(dataDir, 0o750) // ensure it exists for PID file check
			lifecycle.StopExisting(dataDir, slog.Default())
```

- [ ] **Step 2: Add root guard**

At the beginning of the `RunE` function (after extracting flags), add:

```go
			installService, _ := cmd.Flags().GetBool("service")

			// Prevent running join as root — config files must be owned by the target user.
			if os.Getuid() == 0 && installService {
				return fmt.Errorf("do not run 'join' as root. Run as your normal user — only the service installation needs sudo")
			}
```

- [ ] **Step 3: Add --service flag and sudo re-exec**

Replace the post-join output block (lines 134-141) with:

```go
			configPath := filepath.Join(configDir, "agent.toml")
			fmt.Printf("\nAgent configured for machine joining\n")
			fmt.Printf("Certificates written to %s/certs/\n", configDir)
			fmt.Printf("Config written to %s\n\n", configPath)

			if installService {
				// Re-exec with sudo for just the service installation.
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
```

- [ ] **Step 4: Register the --service flag**

Add after the existing flag definitions (before `return cmd`):

```go
	cmd.Flags().Bool("service", false, "Install and start the agent as a system service after joining (requires sudo)")
```

- [ ] **Step 5: Update join Long description**

Change the `Long` field of the join command to:

```go
		Long: `Redeems a short provisioning code to configure this agent with TLS certificates
and server connection details. Any existing agent (service or process) is automatically
stopped before reconfiguring. Use --service to install as a system service in one step.`,
```

- [ ] **Step 6: Build and verify**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go build ./cmd/agent/`
Expected: Clean build. **Note:** You must ensure `"os/exec"` and `"os/user"` are in the imports block of `main.go` — add them alongside the code that uses them in Steps 2-4, not as a separate step.

- [ ] **Step 7: Run go vet**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go vet ./cmd/agent/`
Expected: Clean.

- [ ] **Step 8: Commit**

```bash
git add cmd/agent/main.go
git commit -m "feat: add --service flag and auto-stop to join command"
```

---

## Task 6: Idempotent `install-service` Guards

**Files:**
- Modify: `cmd/agent/service.go:24-102`

- [ ] **Step 1: Add pre-install guard to installSystemd**

In `installSystemd`, **after** the root check (line 27) and **before** the user resolution (line 30), insert:

```go
	// Stop existing service if running (makes install-service idempotent).
	stopCmd := exec.Command("systemctl", "is-active", "--quiet", "claude-plane-agent")
	if stopCmd.Run() == nil {
		fmt.Printf("Stopping existing claude-plane-agent service...\n")
		_ = exec.Command("systemctl", "stop", "claude-plane-agent").Run()
		_ = exec.Command("systemctl", "disable", "claude-plane-agent").Run()
	}
```

- [ ] **Step 2: Update installLaunchd to use bootstrap instead of load**

Replace line 138 (`exec.Command("launchctl", "load", plistPath)`) with:

```go
	// Use modern launchctl API (bootstrap replaces deprecated load).
	cmd := exec.Command("launchctl", "bootstrap", "system", plistPath)
```

Also add pre-install guard at the top of `installLaunchd` (after root check):

```go
	// Stop existing service if running.
	listCmd := exec.Command("launchctl", "list")
	if out, err := listCmd.Output(); err == nil && strings.Contains(string(out), "com.claude-plane.agent") {
		fmt.Printf("Stopping existing claude-plane-agent service...\n")
		_ = exec.Command("launchctl", "bootout", "system/com.claude-plane.agent").Run()
	}
```

- [ ] **Step 3: Update install-service Long description**

Change the `Long` field:

```go
		Long: `Installs the claude-plane-agent as a background system service that starts
on boot and restarts automatically if it crashes. Safe to run multiple times —
any existing service is stopped and replaced.

Requires root/sudo on Linux (systemd) or macOS (launchd).
The agent binary must already be in a system-accessible location.`,
```

- [ ] **Step 4: Build and verify**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go build ./cmd/agent/`
Expected: Clean build.

- [ ] **Step 5: Commit**

```bash
git add cmd/agent/service.go
git commit -m "feat: make install-service idempotent with pre-install guards"
```

---

## Task 7: `uninstall-service` Command

**Files:**
- Modify: `cmd/agent/main.go` (add `newUninstallServiceCmd`)
- Modify: `cmd/agent/service.go` (add `uninstallSystemd`, `uninstallLaunchd`)

- [ ] **Step 1: Add uninstall functions to service.go**

Add imports to `service.go`: `"log/slog"`, `"path/filepath"`, and `"github.com/kodrunhq/claude-plane/internal/agent/lifecycle"`.

Add to the bottom of `service.go`:

```go
func uninstallService(purge bool, configDir string) error {
	switch runtime.GOOS {
	case "linux":
		return uninstallSystemd(purge, configDir)
	case "darwin":
		return uninstallLaunchd(purge, configDir)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func uninstallSystemd(purge bool, configDir string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("uninstall-service requires root. Run with:\n  sudo claude-plane-agent uninstall-service")
	}

	servicePath := "/etc/systemd/system/claude-plane-agent.service"
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		fmt.Println("No claude-plane-agent service found.")
		if !purge {
			return nil
		}
		// Continue to purge even if no service.
	} else {
		// Stop, disable, remove.
		_ = exec.Command("systemctl", "stop", "claude-plane-agent").Run()
		_ = exec.Command("systemctl", "disable", "claude-plane-agent").Run()
		if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove service file: %w", err)
		}
		_ = exec.Command("systemctl", "daemon-reload").Run()
		fmt.Printf("\n==> Service stopped and removed\n")
		fmt.Printf("    Removed: %s\n", servicePath)
	}

	if purge {
		return purgeConfigDir(configDir)
	}

	fmt.Println()
	return nil
}

func uninstallLaunchd(purge bool, configDir string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("uninstall-service requires root. Run with:\n  sudo claude-plane-agent uninstall-service")
	}

	plistPath := "/Library/LaunchDaemons/com.claude-plane.agent.plist"
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("No claude-plane-agent service found.")
		if !purge {
			return nil
		}
	} else {
		_ = exec.Command("launchctl", "bootout", "system/com.claude-plane.agent").Run()
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove plist: %w", err)
		}
		fmt.Printf("\n==> Service stopped and removed\n")
		fmt.Printf("    Removed: %s\n", plistPath)
	}

	if purge {
		return purgeConfigDir(configDir)
	}

	fmt.Println()
	return nil
}

func purgeConfigDir(configDir string) error {
	// Kill any remaining agent processes before purging.
	dataDir := filepath.Join(configDir, "data")
	lifecycle.StopExisting(dataDir, slog.Default())
	if configDir == "" {
		return fmt.Errorf("cannot determine config directory for purge. Use --config-dir to specify")
	}

	// Safety check: don't delete root or home directory.
	if configDir == "/" || configDir == os.Getenv("HOME") {
		return fmt.Errorf("refusing to purge %q — does not look like a claude-plane config directory", configDir)
	}

	if err := os.RemoveAll(configDir); err != nil {
		return fmt.Errorf("remove config directory: %w", err)
	}

	fmt.Printf("    Config:  %s (purged)\n\n", configDir)
	return nil
}
```

- [ ] **Step 2: Add newUninstallServiceCmd to main.go**

Add the function and register it in `main()`. In the `rootCmd.AddCommand` call, add `newUninstallServiceCmd()`:

```go
	rootCmd.AddCommand(
		newRunCmd(),
		newJoinCmd(),
		newInstallServiceCmd(),
		newUninstallServiceCmd(),
	)
```

Add the command constructor:

```go
func newUninstallServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall-service",
		Short: "Remove the agent system service",
		Long: `Stops and removes the claude-plane-agent system service.
With --purge, also removes all configuration, certificates, and data.

Requires root/sudo.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			purge, _ := cmd.Flags().GetBool("purge")
			configDir, _ := cmd.Flags().GetString("config-dir")

			// Resolve config directory for purge.
			if purge && configDir == "" {
				sudoUser := os.Getenv("SUDO_USER")
				if sudoUser != "" {
					u, err := user.Lookup(sudoUser)
					if err == nil {
						configDir = filepath.Join(u.HomeDir, ".claude-plane")
					}
				}
				if configDir == "" {
					configDir = "/etc/claude-plane"
				}
			}

			return uninstallService(purge, configDir)
		},
	}
	cmd.Flags().Bool("purge", false, "Also remove all config, certificates, and data")
	cmd.Flags().String("config-dir", "", "Config directory to purge (auto-detected from SUDO_USER)")
	return cmd
}
```

- [ ] **Step 3: Build and verify**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go build ./cmd/agent/`
Expected: Clean build. **Note:** You must ensure `"os/user"` is in the imports block of `main.go` — add it when writing the code in Step 2 that uses `user.Lookup`, not as a separate step.

- [ ] **Step 4: Run go vet**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go vet ./cmd/agent/`
Expected: Clean.

- [ ] **Step 5: Commit**

```bash
git add cmd/agent/main.go cmd/agent/service.go
git commit -m "feat: add uninstall-service command with --purge option"
```

---

## Task 8: Run Full Test Suite

**Files:** None modified — validation only.

- [ ] **Step 1: Run all lifecycle package tests**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go test -race ./internal/agent/lifecycle/ -v`
Expected: All PASS.

- [ ] **Step 2: Run all agent package tests**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go test -race ./internal/agent/... -v`
Expected: All PASS.

- [ ] **Step 3: Run go vet on entire project**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go vet ./...`
Expected: Clean.

- [ ] **Step 4: Build both binaries**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go build ./cmd/server/ && go build ./cmd/agent/`
Expected: Clean build.

---

## Task 9: Update Documentation — `docs/install-agent.md`

**Files:**
- Modify: `docs/install-agent.md`

- [ ] **Step 1: Rewrite install-agent.md**

Replace the entire file contents. Key changes:
- Make "Quick Join" the primary recommended flow (section 1, not "Alternative")
- Document `--service` flag on `join`
- Add "Re-registering an Agent" section
- Add "Uninstalling" section with `uninstall-service` and `--purge`
- Update troubleshooting with: duplicate agent, orphaned processes
- Keep manual cert setup as "Advanced" section at the bottom

The new structure should be:

```
# Agent Installation
## Prerequisites
## 1. Quick Start: Join with Provisioning Code  ← PRIMARY
  - Download binary
  - join CODE --server URL --insecure --service   ← one command
## 2. Re-registering an Agent  ← NEW
  - join auto-stops existing agent
## 3. Uninstalling  ← NEW
  - uninstall-service
  - uninstall-service --purge
## 4. Verifying Connectivity
## 5. Advanced: Manual Certificate Setup  ← old sections 1-5, demoted
## Troubleshooting  ← updated
## Next Steps
```

- [ ] **Step 2: Verify no broken links**

Run: `cd /home/joseibanez/develop/projects/claude-plane && grep -r 'install-agent.md' docs/ | grep -v '.md:' || true`
Expected: Any references to install-agent.md still work (section anchors may have changed).

- [ ] **Step 3: Commit**

```bash
git add docs/install-agent.md
git commit -m "docs: rewrite install-agent.md with quick join as primary flow"
```

---

## Task 10: Update Documentation — `docs/quickstart.md`

**Files:**
- Modify: `docs/quickstart.md`

- [ ] **Step 1: Update Option B step 7**

Replace step "7. Start the Agent" (lines 125-137) with:

```markdown
### 7. Start the Agent

In a separate terminal:

```bash
./claude-plane-agent run --config agent.toml
```

Or install as a system service (recommended for production):

```bash
sudo ./claude-plane-agent install-service --config agent.toml
```

> **Tip:** When using provisioning codes on remote machines, you can do everything in one command:
> ```bash
> ./claude-plane-agent join CODE --server https://server:4200 --service
> ```
```

- [ ] **Step 2: Commit**

```bash
git add docs/quickstart.md
git commit -m "docs: add join --service tip to quickstart guide"
```

---

## Task 11: Update Documentation — `docs/configuration.md` and `CLAUDE.md`

**Files:**
- Modify: `docs/configuration.md:144-150`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update CLI Flags section in configuration.md**

Replace the Agent CLI section (lines 144-150) with:

```markdown
### Agent (`claude-plane-agent`)

```
claude-plane-agent run --config agent.toml
claude-plane-agent join CODE --server https://server:4200 [--insecure] [--service]
claude-plane-agent install-service --config agent.toml [--user username]
claude-plane-agent uninstall-service [--purge] [--config-dir path]
```

#### Runtime Files

| File | Location | Description |
|------|----------|-------------|
| `agent.pid` | `{data_dir}/agent.pid` | PID lock file written by `run`, prevents duplicate agents |
```

- [ ] **Step 2: Update CLAUDE.md agent architecture table**

Add to the Agent Architecture table (after `config/` row):

```markdown
- `lifecycle/` — Agent lifecycle utilities: PID file, process scanning, orphan reaping, service detection
```

- [ ] **Step 3: Update CLAUDE.md agent CLI reference**

Find the existing agent binary command reference and add `uninstall-service`:

```markdown
./claude-plane-agent uninstall-service [--purge]
```

- [ ] **Step 4: Commit**

```bash
git add docs/configuration.md CLAUDE.md
git commit -m "docs: update configuration reference and CLAUDE.md with lifecycle commands"
```

---

## Task 12: Final Validation

**Files:** None modified — validation only.

- [ ] **Step 1: Run full test suite**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go test -race ./...`
Expected: All PASS.

- [ ] **Step 2: Run go vet**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go vet ./...`
Expected: Clean.

- [ ] **Step 3: Build all binaries**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go build ./cmd/server/ && go build ./cmd/agent/ && go build ./cmd/bridge/`
Expected: All build cleanly.

- [ ] **Step 4: Verify agent --help shows new commands**

Run: `cd /home/joseibanez/develop/projects/claude-plane && go run ./cmd/agent/ --help`
Expected: Output lists `run`, `join`, `install-service`, `uninstall-service`.

Run: `cd /home/joseibanez/develop/projects/claude-plane && go run ./cmd/agent/ join --help`
Expected: Shows `--service` flag.

Run: `cd /home/joseibanez/develop/projects/claude-plane && go run ./cmd/agent/ uninstall-service --help`
Expected: Shows `--purge` and `--config-dir` flags.

- [ ] **Step 5: Review git log**

Run: `cd /home/joseibanez/develop/projects/claude-plane && git log --oneline -12`
Expected: 10 clean commits matching each task.
