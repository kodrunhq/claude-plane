package lifecycle

import (
	"bufio"
	"bytes"
	"errors"
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
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// FindOrphanedClaudeProcesses returns PIDs of "claude" processes with PPID 1
// (reparented orphans) owned by the current user.
func FindOrphanedClaudeProcesses() ([]int, error) {
	switch runtime.GOOS {
	case "linux":
		return findOrphanedClaudeProcessesLinux()
	case "darwin":
		return findOrphanedClaudeProcessesDarwin()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// SignalAndWait sends SIGTERM to the given PID, polls every 200ms, and
// escalates to SIGKILL after timeout. It is a no-op if the process is
// already gone.
func SignalAndWait(pid int, timeout time.Duration, logger *slog.Logger) {
	// Check if process exists.
	if err := syscall.Kill(pid, 0); err != nil {
		if !errors.Is(err, syscall.EPERM) {
			return // ESRCH — process doesn't exist
		}
		// EPERM = process exists, different user
	}

	logger.Info("sending SIGTERM to process", "pid", pid)
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		logger.Warn("failed to send SIGTERM", "pid", pid, "error", err)
		return
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		if time.Now().After(deadline) {
			logger.Warn("process did not exit after SIGTERM, sending SIGKILL", "pid", pid)
			_ = syscall.Kill(pid, syscall.SIGKILL)
			return
		}
		if err := syscall.Kill(pid, 0); err != nil {
			if !errors.Is(err, syscall.EPERM) {
				return // process exited
			}
			// EPERM = still alive, different user
		}
	}
}

// ReapOrphanedProcesses finds orphaned claude processes and terminates them.
// This is best-effort: errors are logged as warnings.
func ReapOrphanedProcesses(logger *slog.Logger) {
	pids, err := FindOrphanedClaudeProcesses()
	if err != nil {
		logger.Warn("failed to find orphaned claude processes", "error", err)
		return
	}

	for _, pid := range pids {
		logger.Info("reaping orphaned claude process", "pid", pid)
		SignalAndWait(pid, 3*time.Second, logger)
	}
}

// hasPPID1 parses /proc/PID/status content and returns true if PPid is 1.
func hasPPID1(statusContent string) bool {
	scanner := bufio.NewScanner(strings.NewReader(statusContent))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PPid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] == "1" {
				return true
			}
			return false
		}
	}
	return false
}

// findAgentProcessesLinux scans /proc for claude-plane-agent run processes.
func findAgentProcessesLinux() ([]int, error) {
	currentPID := os.Getpid()
	currentUID := uint32(os.Getuid())

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("reading /proc: %w", err)
	}

	var pids []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		if pid == currentPID {
			continue
		}

		// Check ownership.
		procPath := filepath.Join("/proc", entry.Name())
		info, err := os.Stat(procPath)
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != currentUID {
			continue
		}

		// Check cmdline for "claude-plane-agent" and "run".
		cmdline, err := os.ReadFile(filepath.Join(procPath, "cmdline"))
		if err != nil {
			continue
		}
		args := parseProcCmdline(cmdline)
		if matchesAgentRunCommand(args) {
			pids = append(pids, pid)
		}
	}

	return pids, nil
}

// findAgentProcessesDarwin uses pgrep to find claude-plane-agent run processes.
func findAgentProcessesDarwin() ([]int, error) {
	currentPID := os.Getpid()

	out, err := exec.Command("pgrep", "-u", strconv.Itoa(os.Getuid()), "-f", "claude-plane-agent run").Output()
	if err != nil {
		// pgrep exits 1 when no match — that's fine.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("pgrep: %w", err)
	}

	return parsePIDList(out, currentPID)
}

// findOrphanedClaudeProcessesLinux scans /proc for claude processes with PPID 1.
func findOrphanedClaudeProcessesLinux() ([]int, error) {
	currentPID := os.Getpid()
	currentUID := uint32(os.Getuid())

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("reading /proc: %w", err)
	}

	var pids []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		if pid == currentPID {
			continue
		}

		procPath := filepath.Join("/proc", entry.Name())

		// Check ownership.
		info, err := os.Stat(procPath)
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != currentUID {
			continue
		}

		// Check if PPID is 1.
		statusData, err := os.ReadFile(filepath.Join(procPath, "status"))
		if err != nil {
			continue
		}
		if !hasPPID1(string(statusData)) {
			continue
		}

		// Check cmdline for "claude".
		cmdline, err := os.ReadFile(filepath.Join(procPath, "cmdline"))
		if err != nil {
			continue
		}
		args := parseProcCmdline(cmdline)
		if matchesClaudeCommand(args) {
			pids = append(pids, pid)
		}
	}

	return pids, nil
}

// findOrphanedClaudeProcessesDarwin uses ps to find orphaned claude processes.
func findOrphanedClaudeProcessesDarwin() ([]int, error) {
	currentPID := os.Getpid()
	currentUID := os.Getuid()

	out, err := exec.Command("ps", "-eo", "pid,ppid,uid,args").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}

	var pids []int
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}

		if pid == currentPID || ppid != 1 || uid != currentUID {
			continue
		}

		// args may contain spaces, so join all remaining fields.
		fullCmd := strings.Join(fields[3:], " ")
		if strings.Contains(fullCmd, "claude") {
			pids = append(pids, pid)
		}
	}

	return pids, nil
}

// parseProcCmdline splits a /proc/PID/cmdline (null-byte separated) into args.
func parseProcCmdline(data []byte) []string {
	// Trim trailing null bytes.
	data = bytes.TrimRight(data, "\x00")
	if len(data) == 0 {
		return nil
	}
	parts := bytes.Split(data, []byte{0})
	args := make([]string, len(parts))
	for i, p := range parts {
		args[i] = string(p)
	}
	return args
}

// matchesAgentRunCommand checks if cmdline args represent a claude-plane-agent run command.
func matchesAgentRunCommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	base := filepath.Base(args[0])
	if !strings.Contains(base, "claude-plane-agent") {
		return false
	}
	for _, arg := range args[1:] {
		if arg == "run" {
			return true
		}
	}
	return false
}

// matchesClaudeCommand checks if the command name contains "claude".
func matchesClaudeCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return strings.Contains(filepath.Base(args[0]), "claude")
}

// parsePIDList parses newline-separated PID output, excluding the given PID.
func parsePIDList(data []byte, excludePID int) ([]int, error) {
	var pids []int
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		if pid == excludePID {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}
