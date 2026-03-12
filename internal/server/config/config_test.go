package config

import (
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
jwt_secret = "supersecret"
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
	if cfg.Auth.JWTSecret != "supersecret" {
		t.Errorf("Auth.JWTSecret = %q, want %q", cfg.Auth.JWTSecret, "supersecret")
	}
}

func TestLoadServerConfig_MissingHTTPListen(t *testing.T) {
	toml := `
[grpc]
listen = ":9090"
[tls]
ca_cert = "/ca.pem"
[database]
path = "/data.db"
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
[database]
path = "/data.db"
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
