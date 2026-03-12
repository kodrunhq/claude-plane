package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validAgentTOML = `
[server]
address = "control.example.com:9090"

[tls]
ca_cert = "/etc/cp/ca.pem"
agent_cert = "/etc/cp/agent.pem"
agent_key = "/etc/cp/agent-key.pem"

[agent]
machine_id = "worker-42"
data_dir = "/var/lib/claude-agent"
max_sessions = 10
claude_cli_path = "/usr/local/bin/claude"
`

func writeTOML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAgentConfig_Valid(t *testing.T) {
	path := writeTOML(t, validAgentTOML)
	cfg, err := LoadAgentConfig(path)
	if err != nil {
		t.Fatalf("LoadAgentConfig failed: %v", err)
	}

	if cfg.Server.Address != "control.example.com:9090" {
		t.Errorf("Server.Address = %q, want %q", cfg.Server.Address, "control.example.com:9090")
	}
	if cfg.TLS.CACert != "/etc/cp/ca.pem" {
		t.Errorf("TLS.CACert = %q, want %q", cfg.TLS.CACert, "/etc/cp/ca.pem")
	}
	if cfg.TLS.AgentCert != "/etc/cp/agent.pem" {
		t.Errorf("TLS.AgentCert = %q, want %q", cfg.TLS.AgentCert, "/etc/cp/agent.pem")
	}
	if cfg.TLS.AgentKey != "/etc/cp/agent-key.pem" {
		t.Errorf("TLS.AgentKey = %q, want %q", cfg.TLS.AgentKey, "/etc/cp/agent-key.pem")
	}
	if cfg.Agent.MachineID != "worker-42" {
		t.Errorf("Agent.MachineID = %q, want %q", cfg.Agent.MachineID, "worker-42")
	}
	if cfg.Agent.DataDir != "/var/lib/claude-agent" {
		t.Errorf("Agent.DataDir = %q, want %q", cfg.Agent.DataDir, "/var/lib/claude-agent")
	}
	if cfg.Agent.MaxSessions != 10 {
		t.Errorf("Agent.MaxSessions = %d, want 10", cfg.Agent.MaxSessions)
	}
	if cfg.Agent.ClaudeCLIPath != "/usr/local/bin/claude" {
		t.Errorf("Agent.ClaudeCLIPath = %q, want %q", cfg.Agent.ClaudeCLIPath, "/usr/local/bin/claude")
	}
}

func TestLoadAgentConfig_Defaults(t *testing.T) {
	toml := `
[server]
address = "server:9090"

[tls]
ca_cert = "/ca.pem"

[agent]
machine_id = "worker-1"
`
	path := writeTOML(t, toml)
	cfg, err := LoadAgentConfig(path)
	if err != nil {
		t.Fatalf("LoadAgentConfig failed: %v", err)
	}

	if cfg.Agent.MaxSessions != 5 {
		t.Errorf("Agent.MaxSessions default = %d, want 5", cfg.Agent.MaxSessions)
	}
	if cfg.Agent.ClaudeCLIPath != "claude" {
		t.Errorf("Agent.ClaudeCLIPath default = %q, want %q", cfg.Agent.ClaudeCLIPath, "claude")
	}
}

func TestLoadAgentConfig_MissingServerAddress(t *testing.T) {
	toml := `
[tls]
ca_cert = "/ca.pem"
[agent]
machine_id = "worker-1"
`
	path := writeTOML(t, toml)
	_, err := LoadAgentConfig(path)
	if err == nil {
		t.Fatal("expected error for missing server.address")
	}
	if !strings.Contains(err.Error(), "server.address") {
		t.Errorf("error should mention server.address, got: %v", err)
	}
}

func TestLoadAgentConfig_MissingMachineID(t *testing.T) {
	toml := `
[server]
address = "server:9090"
[tls]
ca_cert = "/ca.pem"
[agent]
data_dir = "/tmp"
`
	path := writeTOML(t, toml)
	_, err := LoadAgentConfig(path)
	if err == nil {
		t.Fatal("expected error for missing agent.machine_id")
	}
	if !strings.Contains(err.Error(), "agent.machine_id") {
		t.Errorf("error should mention agent.machine_id, got: %v", err)
	}
}

func TestLoadAgentConfig_MissingTLSCACert(t *testing.T) {
	toml := `
[server]
address = "server:9090"
[agent]
machine_id = "worker-1"
`
	path := writeTOML(t, toml)
	_, err := LoadAgentConfig(path)
	if err == nil {
		t.Fatal("expected error for missing tls.ca_cert")
	}
	if !strings.Contains(err.Error(), "tls.ca_cert") {
		t.Errorf("error should mention tls.ca_cert, got: %v", err)
	}
}

func TestLoadAgentConfig_InvalidTOML(t *testing.T) {
	path := writeTOML(t, "not valid {{{{ toml at all")
	_, err := LoadAgentConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestLoadAgentConfig_FileNotFound(t *testing.T) {
	_, err := LoadAgentConfig("/nonexistent/agent.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
