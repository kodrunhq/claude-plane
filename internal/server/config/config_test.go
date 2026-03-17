package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validServerTOML = `
[http]
listen = ":8080"
tls_cert = "/etc/cp/server.pem"
tls_key = "/etc/cp/server-key.pem"

[grpc]
listen = ":9090"

[tls]
ca_cert = "/etc/cp/ca.pem"
server_cert = "/etc/cp/server.pem"
server_key = "/etc/cp/server-key.pem"

[database]
path = "/var/lib/claude-plane/data.db"

[auth]
jwt_secret = "test-secret-key-32-bytes-long!!!!"
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

func TestLoadServerConfig_Valid(t *testing.T) {
	path := writeTOML(t, validServerTOML)
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}

	if cfg.HTTP.Listen != ":8080" {
		t.Errorf("HTTP.Listen = %q, want %q", cfg.HTTP.Listen, ":8080")
	}
	if cfg.HTTP.TLSCert != "/etc/cp/server.pem" {
		t.Errorf("HTTP.TLSCert = %q, want %q", cfg.HTTP.TLSCert, "/etc/cp/server.pem")
	}
	if cfg.HTTP.TLSKey != "/etc/cp/server-key.pem" {
		t.Errorf("HTTP.TLSKey = %q, want %q", cfg.HTTP.TLSKey, "/etc/cp/server-key.pem")
	}
	if cfg.GRPC.Listen != ":9090" {
		t.Errorf("GRPC.Listen = %q, want %q", cfg.GRPC.Listen, ":9090")
	}
	if cfg.TLS.CACert != "/etc/cp/ca.pem" {
		t.Errorf("TLS.CACert = %q, want %q", cfg.TLS.CACert, "/etc/cp/ca.pem")
	}
	if cfg.Database.Path != "/var/lib/claude-plane/data.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "/var/lib/claude-plane/data.db")
	}
	if cfg.Auth.JWTSecret != "test-secret-key-32-bytes-long!!!!" {
		t.Errorf("Auth.JWTSecret = %q, want %q", cfg.Auth.JWTSecret, "test-secret-key-32-bytes-long!!!!")
	}
}

func TestLoadServerConfig_MissingHTTPListen(t *testing.T) {
	toml := `
[grpc]
listen = ":9090"
[tls]
ca_cert = "/ca.pem"
server_cert = "/server.pem"
server_key = "/server-key.pem"
[database]
path = "/data.db"
[auth]
jwt_secret = "secret"
`
	path := writeTOML(t, toml)
	_, err := LoadServerConfig(path)
	if err == nil {
		t.Fatal("expected error for missing http.listen")
	}
	if !strings.Contains(err.Error(), "http.listen") {
		t.Errorf("error should mention http.listen, got: %v", err)
	}
}

func TestLoadServerConfig_MissingGRPCListen(t *testing.T) {
	toml := `
[http]
listen = ":8080"
[tls]
ca_cert = "/ca.pem"
server_cert = "/server.pem"
server_key = "/server-key.pem"
[database]
path = "/data.db"
[auth]
jwt_secret = "secret"
`
	path := writeTOML(t, toml)
	_, err := LoadServerConfig(path)
	if err == nil {
		t.Fatal("expected error for missing grpc.listen")
	}
	if !strings.Contains(err.Error(), "grpc.listen") {
		t.Errorf("error should mention grpc.listen, got: %v", err)
	}
}

func TestLoadServerConfig_MissingDatabasePath(t *testing.T) {
	toml := `
[http]
listen = ":8080"
[grpc]
listen = ":9090"
[tls]
ca_cert = "/ca.pem"
server_cert = "/server.pem"
server_key = "/server-key.pem"
[auth]
jwt_secret = "secret"
`
	path := writeTOML(t, toml)
	_, err := LoadServerConfig(path)
	if err == nil {
		t.Fatal("expected error for missing database.path")
	}
	if !strings.Contains(err.Error(), "database.path") {
		t.Errorf("error should mention database.path, got: %v", err)
	}
}

func TestLoadServerConfig_MissingTLSCACert(t *testing.T) {
	toml := `
[http]
listen = ":8080"
[grpc]
listen = ":9090"
[database]
path = "/data.db"
[auth]
jwt_secret = "secret"
`
	path := writeTOML(t, toml)
	_, err := LoadServerConfig(path)
	if err == nil {
		t.Fatal("expected error for missing tls.ca_cert")
	}
	if !strings.Contains(err.Error(), "tls.ca_cert") {
		t.Errorf("error should mention tls.ca_cert, got: %v", err)
	}
}

func TestValidate_JWTSecretTooShort(t *testing.T) {
	toml := `
[http]
listen = ":8080"
[grpc]
listen = ":9090"
[tls]
ca_cert = "/ca.pem"
server_cert = "/server.pem"
server_key = "/server-key.pem"
[database]
path = "/data.db"
[auth]
jwt_secret = "short"
`
	path := writeTOML(t, toml)
	_, err := LoadServerConfig(path)
	if err == nil {
		t.Fatal("expected error for short JWT secret")
	}
	if !strings.Contains(err.Error(), "at least 32") {
		t.Errorf("error should mention 'at least 32', got: %v", err)
	}
}

func TestLoadServerConfig_InvalidTOML(t *testing.T) {
	path := writeTOML(t, "this is not valid {{{{ toml")
	_, err := LoadServerConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestLoadServerConfig_FileNotFound(t *testing.T) {
	_, err := LoadServerConfig("/nonexistent/config.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// baseConfigTOML returns a valid config TOML without the [auth] section.
func baseConfigTOML() string {
	return `
[http]
listen = ":8080"
tls_cert = "/etc/cp/server.pem"
tls_key = "/etc/cp/server-key.pem"

[grpc]
listen = ":9090"

[tls]
ca_cert = "/etc/cp/ca.pem"
server_cert = "/etc/cp/server.pem"
server_key = "/etc/cp/server-key.pem"

[database]
path = "/var/lib/claude-plane/data.db"
`
}

func writeSecretFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "jwt_secret")
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadServerConfig_JWTSecretFile(t *testing.T) {
	secretFile := writeSecretFile(t, "this-is-a-secret-from-file-32-chars!!")
	tomlContent := baseConfigTOML() + fmt.Sprintf(`
[auth]
jwt_secret_file = %q
`, secretFile)
	path := writeTOML(t, tomlContent)
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}
	if cfg.Auth.JWTSecret != "this-is-a-secret-from-file-32-chars!!" {
		t.Errorf("Auth.JWTSecret = %q, want %q", cfg.Auth.JWTSecret, "this-is-a-secret-from-file-32-chars!!")
	}
}

func TestLoadServerConfig_JWTSecretFileTrimsWhitespace(t *testing.T) {
	secretFile := writeSecretFile(t, "  this-is-a-secret-from-file-32-chars!!\n\n")
	tomlContent := baseConfigTOML() + fmt.Sprintf(`
[auth]
jwt_secret_file = %q
`, secretFile)
	path := writeTOML(t, tomlContent)
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}
	if cfg.Auth.JWTSecret != "this-is-a-secret-from-file-32-chars!!" {
		t.Errorf("Auth.JWTSecret = %q, want %q", cfg.Auth.JWTSecret, "this-is-a-secret-from-file-32-chars!!")
	}
}

func TestLoadServerConfig_JWTSecretAndFileMutuallyExclusive(t *testing.T) {
	secretFile := writeSecretFile(t, "this-is-a-secret-from-file-32-chars!!")
	tomlContent := baseConfigTOML() + fmt.Sprintf(`
[auth]
jwt_secret = "test-secret-key-32-bytes-long!!!!"
jwt_secret_file = %q
`, secretFile)
	path := writeTOML(t, tomlContent)
	_, err := LoadServerConfig(path)
	if err == nil {
		t.Fatal("expected error when both jwt_secret and jwt_secret_file are set")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention 'mutually exclusive', got: %v", err)
	}
}

func TestLoadServerConfig_JWTSecretFileNotFound(t *testing.T) {
	tomlContent := baseConfigTOML() + `
[auth]
jwt_secret_file = "/nonexistent/jwt_secret"
`
	path := writeTOML(t, tomlContent)
	_, err := LoadServerConfig(path)
	if err == nil {
		t.Fatal("expected error for missing jwt_secret_file")
	}
	if !strings.Contains(err.Error(), "jwt_secret_file") {
		t.Errorf("error should mention jwt_secret_file, got: %v", err)
	}
}

func TestLoadServerConfig_JWTSecretFileTooShort(t *testing.T) {
	secretFile := writeSecretFile(t, "short")
	tomlContent := baseConfigTOML() + fmt.Sprintf(`
[auth]
jwt_secret_file = %q
`, secretFile)
	path := writeTOML(t, tomlContent)
	_, err := LoadServerConfig(path)
	if err == nil {
		t.Fatal("expected error for short secret from file")
	}
	if !strings.Contains(err.Error(), "at least 32") {
		t.Errorf("error should mention 'at least 32', got: %v", err)
	}
}

func TestRegistrationMode_DefaultsClosed(t *testing.T) {
	cfg := &AuthConfig{}
	if mode := cfg.GetRegistrationMode(); mode != "closed" {
		t.Errorf("default registration mode = %q, want %q", mode, "closed")
	}
}

func TestRegistrationMode_ValidValues(t *testing.T) {
	for _, mode := range []string{"open", "invite", "closed"} {
		cfg := &AuthConfig{RegistrationMode: mode}
		if mode == "invite" {
			cfg.InviteCode = "test-code"
		}
		if err := cfg.validateRegistrationMode(); err != nil {
			t.Errorf("mode %q should be valid, got error: %v", mode, err)
		}
	}
}

func TestRegistrationMode_InvalidValue(t *testing.T) {
	cfg := &AuthConfig{RegistrationMode: "unknown"}
	if err := cfg.validateRegistrationMode(); err == nil {
		t.Error("expected error for invalid registration mode")
	}
}

func TestRegistrationMode_InviteRequiresCode(t *testing.T) {
	cfg := &AuthConfig{RegistrationMode: "invite"}
	err := cfg.validateRegistrationMode()
	if err == nil {
		t.Error("expected error when invite mode has no invite_code")
	}
	if !strings.Contains(err.Error(), "invite_code") {
		t.Errorf("error should mention invite_code, got: %v", err)
	}
}

func TestLoadServerConfig_RegistrationModeInvite(t *testing.T) {
	tomlContent := baseConfigTOML() + `
[auth]
jwt_secret = "test-secret-key-32-bytes-long!!!!"
registration_mode = "invite"
invite_code = "my-secret-invite"
`
	path := writeTOML(t, tomlContent)
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}
	if cfg.Auth.GetRegistrationMode() != "invite" {
		t.Errorf("registration_mode = %q, want %q", cfg.Auth.GetRegistrationMode(), "invite")
	}
	if cfg.Auth.InviteCode != "my-secret-invite" {
		t.Errorf("invite_code = %q, want %q", cfg.Auth.InviteCode, "my-secret-invite")
	}
}

func TestLoadServerConfig_InvalidRegistrationMode(t *testing.T) {
	tomlContent := baseConfigTOML() + `
[auth]
jwt_secret = "test-secret-key-32-bytes-long!!!!"
registration_mode = "bogus"
`
	path := writeTOML(t, tomlContent)
	_, err := LoadServerConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid registration_mode")
	}
	if !strings.Contains(err.Error(), "registration_mode") {
		t.Errorf("error should mention registration_mode, got: %v", err)
	}
}

func TestEventsConfig_GetRetentionDays_Default(t *testing.T) {
	cfg := &EventsConfig{}
	if days := cfg.GetRetentionDays(); days != 7 {
		t.Errorf("GetRetentionDays() = %d, want 7 for zero value", days)
	}
}

func TestEventsConfig_GetRetentionDays_Negative(t *testing.T) {
	cfg := &EventsConfig{RetentionDays: -5}
	if days := cfg.GetRetentionDays(); days != 7 {
		t.Errorf("GetRetentionDays() = %d, want 7 for negative value", days)
	}
}

func TestEventsConfig_GetRetentionDays_Custom(t *testing.T) {
	cfg := &EventsConfig{RetentionDays: 30}
	if days := cfg.GetRetentionDays(); days != 30 {
		t.Errorf("GetRetentionDays() = %d, want 30", days)
	}
}

func TestLoadServerConfig_EventsRetentionDays(t *testing.T) {
	tomlContent := baseConfigTOML() + `
[auth]
jwt_secret = "test-secret-key-32-bytes-long!!!!"

[events]
retention_days = 14
`
	path := writeTOML(t, tomlContent)
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}
	if days := cfg.Events.GetRetentionDays(); days != 14 {
		t.Errorf("Events.GetRetentionDays() = %d, want 14", days)
	}
}

func TestParseEncryptionKey_AutoGenerate(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &SecretsConfig{}

	key, err := cfg.ParseEncryptionKey(tmpDir)
	if err != nil {
		t.Fatalf("ParseEncryptionKey failed: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d bytes", len(key))
	}

	// Verify key file was written.
	keyPath := filepath.Join(tmpDir, "encryption.key")
	data, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("key file not created: %v", err)
	}
	raw := strings.TrimSpace(string(data))
	if len(raw) != 64 {
		t.Errorf("key file should contain 64 hex chars, got %d", len(raw))
	}

	// Verify file permissions.
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("key file permissions = %o, want 0600", perm)
	}
}

func TestParseEncryptionKey_AutoGenerate_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &SecretsConfig{}

	key1, err := cfg.ParseEncryptionKey(tmpDir)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	key2, err := cfg.ParseEncryptionKey(tmpDir)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if hex.EncodeToString(key1) != hex.EncodeToString(key2) {
		t.Error("second call returned different key; expected idempotent behavior")
	}
}

func TestParseEncryptionKey_ExplicitKey(t *testing.T) {
	tmpDir := t.TempDir()
	// Valid 32-byte hex key (64 hex chars).
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cfg := &SecretsConfig{EncryptionKey: hexKey}

	key, err := cfg.ParseEncryptionKey(tmpDir)
	if err != nil {
		t.Fatalf("ParseEncryptionKey failed: %v", err)
	}
	if hex.EncodeToString(key) != hexKey {
		t.Errorf("key = %x, want %s", key, hexKey)
	}

	// Verify no auto-generated file was created.
	keyPath := filepath.Join(tmpDir, "encryption.key")
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Error("auto-generated key file should not exist when explicit key is configured")
	}
}

func TestParseEncryptionKey_CorruptedKeyFile(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "encryption.key")

	// Write invalid content (not 64 hex chars).
	if err := os.WriteFile(keyPath, []byte("not-a-valid-key\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &SecretsConfig{}
	_, err := cfg.ParseEncryptionKey(tmpDir)
	if err == nil {
		t.Fatal("expected error for corrupted key file")
	}
	if !strings.Contains(err.Error(), "invalid content") {
		t.Errorf("error should mention 'invalid content', got: %v", err)
	}
}

func TestParseEncryptionKey_ExplicitKeyFileError(t *testing.T) {
	cfg := &SecretsConfig{EncryptionKeyFile: "/nonexistent/encryption.key"}

	_, err := cfg.ParseEncryptionKey(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing explicit key file")
	}
	if !strings.Contains(err.Error(), "encryption_key_file") {
		t.Errorf("error should mention encryption_key_file, got: %v", err)
	}
}
