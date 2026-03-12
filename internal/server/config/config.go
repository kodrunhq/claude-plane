// Package config provides TOML configuration loading for the claude-plane server.
package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// ServerConfig is the top-level configuration for the claude-plane server.
type ServerConfig struct {
	HTTP     HTTPConfig     `toml:"http"`
	GRPC     GRPCConfig     `toml:"grpc"`
	TLS      TLSConfig      `toml:"tls"`
	Database DatabaseConfig `toml:"database"`
	Auth     AuthConfig     `toml:"auth"`
}

// HTTPConfig configures the HTTP/WebSocket listener.
type HTTPConfig struct {
	Listen  string `toml:"listen"`
	TLSCert string `toml:"tls_cert"`
	TLSKey  string `toml:"tls_key"`
}

// GRPCConfig configures the gRPC listener for agent connections.
type GRPCConfig struct {
	Listen string `toml:"listen"`
}

// TLSConfig holds paths to TLS certificates for mTLS.
type TLSConfig struct {
	CACert    string `toml:"ca_cert"`
	ServerCert string `toml:"server_cert"`
	ServerKey  string `toml:"server_key"`
}

// DatabaseConfig configures the SQLite database.
type DatabaseConfig struct {
	Path string `toml:"path"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	JWTSecret string `toml:"jwt_secret"`
}

// LoadServerConfig reads a TOML config file, parses it into a ServerConfig,
// and validates that all required fields are present.
func LoadServerConfig(path string) (*ServerConfig, error) {
	var cfg ServerConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate checks that all required configuration fields are set.
// Returns a descriptive error identifying the missing field.
func (c *ServerConfig) Validate() error {
	if c.HTTP.Listen == "" {
		return fmt.Errorf("http.listen is required")
	}
	if c.GRPC.Listen == "" {
		return fmt.Errorf("grpc.listen is required")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}
	if c.TLS.CACert == "" {
		return fmt.Errorf("tls.ca_cert is required")
	}
	if c.TLS.ServerCert == "" {
		return fmt.Errorf("tls.server_cert is required")
	}
	if c.TLS.ServerKey == "" {
		return fmt.Errorf("tls.server_key is required")
	}
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("auth.jwt_secret is required")
	}
	return nil
}
