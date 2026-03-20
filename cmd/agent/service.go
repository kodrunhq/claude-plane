package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kodrunhq/claude-plane/internal/agent/lifecycle"
)

func installService(binPath, configPath, runAsUser string) error {
	switch runtime.GOOS {
	case "linux":
		return installSystemd(binPath, configPath, runAsUser)
	case "darwin":
		return installLaunchd(binPath, configPath)
	default:
		return fmt.Errorf("unsupported platform: %s (supported: linux, darwin)", runtime.GOOS)
	}
}

func installSystemd(binPath, configPath, runAsUser string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("install-service requires root. Run with:\n  sudo %s install-service --config %s", binPath, configPath)
	}

	// Stop existing service if running (makes install-service idempotent).
	stopCmd := exec.Command("systemctl", "is-active", "--quiet", "claude-plane-agent")
	if stopCmd.Run() == nil {
		fmt.Printf("Stopping existing claude-plane-agent service...\n")
		_ = exec.Command("systemctl", "stop", "claude-plane-agent").Run()
		_ = exec.Command("systemctl", "disable", "claude-plane-agent").Run()
	}

	// Determine user/group for the service.
	if runAsUser == "" {
		// Default to the user who ran sudo.
		runAsUser = os.Getenv("SUDO_USER")
		if runAsUser == "" {
			runAsUser = "root"
		}
	}

	// Look up the user's home directory for the working directory.
	u, err := user.Lookup(runAsUser)
	if err != nil {
		return fmt.Errorf("lookup user %q: %w", runAsUser, err)
	}

	// Ensure data directory exists.
	dataDir := filepath.Join(u.HomeDir, ".claude-plane", "data")
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	// Chown to the service user.
	if uid, gid, ok := lookupIDs(u); ok {
		os.Chown(dataDir, uid, gid)
	}

	unit := fmt.Sprintf(`[Unit]
Description=claude-plane agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=%s
ExecStart=%s run --config %s
Restart=always
RestartSec=5
LimitNOFILE=65536
WorkingDirectory=%s

[Install]
WantedBy=multi-user.target
`, runAsUser, binPath, configPath, u.HomeDir)

	servicePath := "/etc/systemd/system/claude-plane-agent.service"
	if err := os.WriteFile(servicePath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	// Reload, enable, start.
	commands := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "claude-plane-agent"},
		{"systemctl", "start", "claude-plane-agent"},
	}
	for _, c := range commands {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(c, " "), err)
		}
	}

	fmt.Printf("\n==> claude-plane-agent installed as systemd service\n")
	fmt.Printf("    Service file: %s\n", servicePath)
	fmt.Printf("    Running as:   %s\n", runAsUser)
	fmt.Printf("    Config:       %s\n\n", configPath)
	fmt.Printf("Useful commands:\n")
	fmt.Printf("  sudo systemctl status claude-plane-agent   # check status\n")
	fmt.Printf("  sudo journalctl -u claude-plane-agent -f   # view logs\n")
	fmt.Printf("  sudo systemctl restart claude-plane-agent  # restart\n")
	fmt.Printf("  sudo systemctl stop claude-plane-agent     # stop\n\n")
	return nil
}

func installLaunchd(binPath, configPath string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("install-service requires root. Run with:\n  sudo %s install-service --config %s", binPath, configPath)
	}

	// Stop existing service if running.
	listCmd := exec.Command("launchctl", "list")
	if out, err := listCmd.Output(); err == nil && strings.Contains(string(out), "com.claude-plane.agent") {
		fmt.Printf("Stopping existing claude-plane-agent service...\n")
		_ = exec.Command("launchctl", "bootout", "system/com.claude-plane.agent").Run()
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.claude-plane.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>run</string>
        <string>--config</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/claude-plane-agent.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/claude-plane-agent.log</string>
</dict>
</plist>`, binPath, configPath)

	plistPath := "/Library/LaunchDaemons/com.claude-plane.agent.plist"
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	cmd := exec.Command("launchctl", "bootstrap", "system", plistPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("launchctl load: %w", err)
	}

	fmt.Printf("\n==> claude-plane-agent installed as launchd service\n")
	fmt.Printf("    Plist: %s\n", plistPath)
	fmt.Printf("    Config: %s\n\n", configPath)
	fmt.Printf("Useful commands:\n")
	fmt.Printf("  launchctl list | grep claude-plane   # check status\n")
	fmt.Printf("  tail -f /var/log/claude-plane-agent.log   # view logs\n\n")
	return nil
}

// lookupIDs extracts numeric UID/GID from a user.User. Returns false if parsing fails.
func lookupIDs(u *user.User) (uid, gid int, ok bool) {
	var uidN, gidN int
	if _, err := fmt.Sscanf(u.Uid, "%d", &uidN); err != nil {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(u.Gid, "%d", &gidN); err != nil {
		return 0, 0, false
	}
	return uidN, gidN, true
}

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
	} else {
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
	if configDir == "" {
		return fmt.Errorf("cannot determine config directory for purge. Use --config-dir to specify")
	}

	// Safety: don't delete root or home directory.
	if configDir == "/" || configDir == os.Getenv("HOME") {
		return fmt.Errorf("refusing to purge %q — does not look like a claude-plane config directory", configDir)
	}

	// Kill remaining agent processes before purging.
	dataDir := filepath.Join(configDir, "data")
	lifecycle.StopExisting(dataDir, slog.Default())

	if err := os.RemoveAll(configDir); err != nil {
		return fmt.Errorf("remove config directory: %w", err)
	}

	fmt.Printf("    Config:  %s (purged)\n\n", configDir)
	return nil
}
