package config

import (
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
