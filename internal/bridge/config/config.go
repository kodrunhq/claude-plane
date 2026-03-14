// Package config provides TOML configuration loading for the claude-plane bridge.
package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

const (
	defaultStatePath     = "./bridge-state.json"
	defaultHealthAddress = ":8081"
)

// Config is the top-level configuration for the claude-plane bridge binary.
type Config struct {
	ClaudePlane ClaudePlaneConfig `toml:"claude_plane"`
	State       StateConfig       `toml:"state"`
	Health      HealthConfig      `toml:"health"`
}

// ClaudePlaneConfig holds connection settings for the claude-plane server.
type ClaudePlaneConfig struct {
	APIURL string `toml:"api_url"`
	APIKey string `toml:"api_key"`
}

// StateConfig configures state persistence for the bridge.
type StateConfig struct {
	Path string `toml:"path"`
}

// HealthConfig configures the bridge health/readiness endpoint.
type HealthConfig struct {
	Address string `toml:"address"`
}

// Load reads a TOML config file from path, applies defaults, and validates the result.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// applyDefaults fills in zero-value fields with their defaults.
func (c *Config) applyDefaults() {
	if c.State.Path == "" {
		c.State.Path = defaultStatePath
	}
	if c.Health.Address == "" {
		c.Health.Address = defaultHealthAddress
	}
}

// Validate checks that all required configuration fields are set.
func (c *Config) Validate() error {
	if c.ClaudePlane.APIURL == "" {
		return fmt.Errorf("claude_plane.api_url is required")
	}
	if c.ClaudePlane.APIKey == "" {
		return fmt.Errorf("claude_plane.api_key is required")
	}
	return nil
}
