// Package config provides TOML configuration loading for the claude-plane server.
package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ServerConfig is the top-level configuration for the claude-plane server.
type ServerConfig struct {
	HTTP      HTTPConfig      `toml:"http"`
	GRPC      GRPCConfig      `toml:"grpc"`
	TLS       TLSConfig       `toml:"tls"`
	Database  DatabaseConfig  `toml:"database"`
	Auth      AuthConfig      `toml:"auth"`
	Shutdown  ShutdownConfig  `toml:"shutdown"`
	Webhooks  WebhooksConfig  `toml:"webhooks"`
	Provision ProvisionConfig `toml:"provision"`
	CA        CAConfig        `toml:"ca"`
	Secrets   SecretsConfig   `toml:"secrets"`
}

// WebhooksConfig groups all webhook-related configuration.
type WebhooksConfig struct {
	Inbound WebhookInboundConfig `toml:"inbound"`
}

// WebhookInboundConfig configures inbound webhook event ingestion.
type WebhookInboundConfig struct {
	Sources map[string]WebhookSourceConfig `toml:"sources"`
}

// WebhookSourceConfig holds the per-source configuration for inbound webhooks.
type WebhookSourceConfig struct {
	Secret string `toml:"secret"`
}

// InboundSecrets returns a flat map of source name → HMAC secret suitable
// for passing directly to handler.NewIngestHandler.
// Sources with no configured secret map to an empty string (no auth required).
func (c *WebhooksConfig) InboundSecrets() map[string]string {
	secrets := make(map[string]string, len(c.Inbound.Sources))
	for name, src := range c.Inbound.Sources {
		secrets[name] = src.Secret
	}
	return secrets
}

// ProvisionConfig configures external addresses for provisioning and agent registration.
// These fields are optional and used only when provisioning features are enabled.
type ProvisionConfig struct {
	ExternalHTTPAddress string `toml:"external_http_address"` // e.g. "https://plane.example.com"
	ExternalGRPCAddress string `toml:"external_grpc_address"` // e.g. "plane.example.com:9090"
}

// CAConfig configures the certificate authority directory.
type CAConfig struct {
	Dir string `toml:"dir"` // path to CA directory (default: "./ca")
}

// GetCADir returns the CA directory, defaulting to "./ca".
func (c *CAConfig) GetCADir() string {
	if c.Dir == "" {
		return "./ca"
	}
	return c.Dir
}

// ShutdownConfig controls graceful shutdown behavior.
type ShutdownConfig struct {
	Timeout string `toml:"timeout"`
}

// ParseTimeout parses the Timeout string as a time.Duration.
// Returns 30 seconds as the default if Timeout is empty.
func (s *ShutdownConfig) ParseTimeout() (time.Duration, error) {
	if s.Timeout == "" {
		return 30 * time.Second, nil
	}
	d, err := time.ParseDuration(s.Timeout)
	if err != nil {
		return 0, fmt.Errorf("parse shutdown.timeout %q: %w", s.Timeout, err)
	}
	return d, nil
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
	JWTSecret        string `toml:"jwt_secret"`
	JWTSecretFile    string `toml:"jwt_secret_file"`
	TokenTTL         string `toml:"token_ttl"`
	RegistrationMode string `toml:"registration_mode"`
	InviteCode       string `toml:"invite_code"`
}

// SecretsConfig holds encryption settings for the credentials vault.
type SecretsConfig struct {
	// EncryptionKey is a 32-byte hex-encoded key used for AES-256-GCM encryption.
	// If empty, the CLAUDE_PLANE_ENCRYPTION_KEY environment variable is checked.
	// For production, always set this to a stable 64-hex-character (32-byte) value.
	EncryptionKey     string `toml:"encryption_key"`
	EncryptionKeyFile string `toml:"encryption_key_file"`
}

// ParseEncryptionKey resolves and decodes the 32-byte AES-256 encryption key.
// Resolution order: EncryptionKeyFile > EncryptionKey > CLAUDE_PLANE_ENCRYPTION_KEY env var.
// Returns an error if the resolved key is not a 64-character hex string (32 bytes).
func (s *SecretsConfig) ParseEncryptionKey() ([]byte, error) {
	raw := s.EncryptionKey

	if s.EncryptionKeyFile != "" {
		data, err := os.ReadFile(s.EncryptionKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read secrets.encryption_key_file %q: %w", s.EncryptionKeyFile, err)
		}
		raw = strings.TrimSpace(string(data))
	}

	if raw == "" {
		raw = os.Getenv("CLAUDE_PLANE_ENCRYPTION_KEY")
	}

	if raw == "" {
		return nil, fmt.Errorf("secrets.encryption_key is required (set in config or CLAUDE_PLANE_ENCRYPTION_KEY env var)")
	}

	key, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("secrets.encryption_key must be hex-encoded: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secrets.encryption_key must decode to exactly 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	return key, nil
}

// GetRegistrationMode returns the configured registration mode, defaulting to "closed".
// Valid values are "open", "invite", and "closed".
func (a *AuthConfig) GetRegistrationMode() string {
	if a.RegistrationMode == "" {
		return "closed"
	}
	return a.RegistrationMode
}

// validateRegistrationMode checks that RegistrationMode is a valid value
// and that InviteCode is set when mode is "invite".
func (a *AuthConfig) validateRegistrationMode() error {
	mode := a.GetRegistrationMode()
	switch mode {
	case "open", "invite", "closed":
		// valid
	default:
		return fmt.Errorf("auth.registration_mode must be one of: open, invite, closed; got %q", mode)
	}
	if mode == "invite" && a.InviteCode == "" {
		return fmt.Errorf("auth.invite_code is required when registration_mode is \"invite\"")
	}
	return nil
}

// ParseTokenTTL parses the TokenTTL string as a time.Duration.
// Returns 60 minutes as the default if TokenTTL is empty.
func (a *AuthConfig) ParseTokenTTL() (time.Duration, error) {
	if a.TokenTTL == "" {
		return 60 * time.Minute, nil
	}
	d, err := time.ParseDuration(a.TokenTTL)
	if err != nil {
		return 0, fmt.Errorf("parse token_ttl %q: %w", a.TokenTTL, err)
	}
	return d, nil
}

// resolveJWTSecret populates JWTSecret from JWTSecretFile when appropriate.
// It returns an error if both JWTSecret and JWTSecretFile are set.
func (a *AuthConfig) resolveJWTSecret() error {
	if a.JWTSecret != "" && a.JWTSecretFile != "" {
		return fmt.Errorf("auth.jwt_secret and auth.jwt_secret_file are mutually exclusive")
	}
	if a.JWTSecretFile != "" {
		data, err := os.ReadFile(a.JWTSecretFile)
		if err != nil {
			return fmt.Errorf("read auth.jwt_secret_file %q: %w", a.JWTSecretFile, err)
		}
		a.JWTSecret = strings.TrimSpace(string(data))
	}
	return nil
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
	if err := c.Auth.resolveJWTSecret(); err != nil {
		return err
	}
	if len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("auth.jwt_secret must be at least 32 characters for HS256 security")
	}
	if err := c.Auth.validateRegistrationMode(); err != nil {
		return err
	}
	return nil
}
