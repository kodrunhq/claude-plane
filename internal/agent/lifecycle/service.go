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

// StopServiceIfActive attempts to stop the agent system service (systemd on
// Linux, launchd on macOS). Returns true if a running service was found and a
// stop command was issued, false otherwise. Errors are logged as warnings
// rather than returned — callers always proceed to the next stop layer.
func StopServiceIfActive(logger *slog.Logger) bool {
	switch runtime.GOOS {
	case "linux":
		return stopSystemdService(logger)
	case "darwin":
		return stopLaunchdService(logger)
	default:
		return false
	}
}

// IsServiceInstalled reports whether the agent service unit file (systemd) or
// plist (launchd) exists on disk.
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

// stopSystemdService checks if the systemd service is active and stops it.
func stopSystemdService(logger *slog.Logger) bool {
	// Check if systemctl is available.
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}

	// "systemctl is-active --quiet" exits 0 when active.
	if err := exec.Command("systemctl", "is-active", "--quiet", systemdServiceName).Run(); err != nil {
		return false
	}

	logger.Info("stopping systemd service", "service", systemdServiceName)
	if err := exec.Command("systemctl", "stop", systemdServiceName).Run(); err != nil {
		logger.Warn("failed to stop systemd service", "service", systemdServiceName, "error", err)
		return false
	}

	logger.Info("systemd service stopped", "service", systemdServiceName)
	return true
}

// stopLaunchdService checks if the launchd service is loaded and boots it out.
func stopLaunchdService(logger *slog.Logger) bool {
	// Check if launchctl is available.
	if _, err := exec.LookPath("launchctl"); err != nil {
		return false
	}

	// Check if the service is loaded by listing and grepping.
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return false
	}

	if !strings.Contains(string(out), launchdLabel) {
		return false
	}

	logger.Info("stopping launchd service", "label", launchdLabel)
	target := "system/" + launchdLabel
	if err := exec.Command("launchctl", "bootout", target).Run(); err != nil {
		logger.Warn("failed to bootout launchd service", "label", launchdLabel, "error", err)
		return false
	}

	logger.Info("launchd service stopped", "label", launchdLabel)
	return true
}
