// Package config provides TOML configuration loading for the claude-plane agent.
package config

import (
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

// AgentConfig is the top-level configuration for the claude-plane agent.
type AgentConfig struct {
	Server   ServerConnConfig `toml:"server"`
	TLS      TLSConfig        `toml:"tls"`
	Agent    AgentSettings    `toml:"agent"`
	Shutdown ShutdownConfig   `toml:"shutdown"`
}

// ShutdownConfig controls graceful shutdown behavior.
type ShutdownConfig struct {
	Timeout string `toml:"timeout"`
}

// ParseTimeout parses the Timeout string as a time.Duration.
// Returns 15 seconds as the default if Timeout is empty.
func (s *ShutdownConfig) ParseTimeout() (time.Duration, error) {
	if s.Timeout == "" {
		return 15 * time.Second, nil
	}
	d, err := time.ParseDuration(s.Timeout)
	if err != nil {
		return 0, fmt.Errorf("parse shutdown.timeout %q: %w", s.Timeout, err)
	}
	return d, nil
}

// ServerConnConfig holds the server connection address.
type ServerConnConfig struct {
	Address string `toml:"address"`
}

// TLSConfig holds paths to TLS certificates for mTLS.
type TLSConfig struct {
	CACert   string `toml:"ca_cert"`
	AgentCert string `toml:"agent_cert"`
	AgentKey  string `toml:"agent_key"`
}

// AgentSettings holds agent-specific configuration.
type AgentSettings struct {
	MachineID          string `toml:"machine_id"`
	DataDir            string `toml:"data_dir"`
	MaxSessions        int    `toml:"max_sessions"`
	ClaudeCLIPath      string `toml:"claude_cli_path"`
	IdlePromptMarker   string `toml:"idle_prompt_marker"`
	IdleStartupTimeout string `toml:"idle_startup_timeout"`
}

// LoadAgentConfig reads a TOML config file, parses it into an AgentConfig,
// applies defaults, and validates that all required fields are present.
func LoadAgentConfig(path string) (*AgentConfig, error) {
	var cfg AgentConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// applyDefaults sets default values for optional fields that were not specified.
func (c *AgentConfig) applyDefaults() {
	if c.Agent.MaxSessions == 0 {
		c.Agent.MaxSessions = 5
	}
	if c.Agent.ClaudeCLIPath == "" {
		c.Agent.ClaudeCLIPath = "claude"
	}
}

// Validate checks that all required configuration fields are set.
// Returns a descriptive error identifying the missing field.
func (c *AgentConfig) Validate() error {
	if c.Server.Address == "" {
		return fmt.Errorf("server.address is required")
	}
	if c.Agent.MachineID == "" {
		return fmt.Errorf("agent.machine_id is required")
	}
	if c.TLS.CACert == "" {
		return fmt.Errorf("tls.ca_cert is required")
	}
	if c.TLS.AgentCert == "" {
		return fmt.Errorf("tls.agent_cert is required")
	}
	if c.TLS.AgentKey == "" {
		return fmt.Errorf("tls.agent_key is required")
	}
	return nil
}
