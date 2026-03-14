package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kodrunhq/claude-plane/internal/bridge/config"
)

func writeTOML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "bridge-*.toml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	return f.Name()
}

func TestLoad_ValidConfig(t *testing.T) {
	path := writeTOML(t, `
[claude_plane]
api_url = "http://localhost:8080"
api_key = "cpk_abc12345_randombase64urlkey"

[state]
path = "./my-state.json"

[health]
address = ":9090"
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ClaudePlane.APIURL != "http://localhost:8080" {
		t.Errorf("api_url = %q, want %q", cfg.ClaudePlane.APIURL, "http://localhost:8080")
	}
	if cfg.ClaudePlane.APIKey != "cpk_abc12345_randombase64urlkey" {
		t.Errorf("api_key = %q, want %q", cfg.ClaudePlane.APIKey, "cpk_abc12345_randombase64urlkey")
	}
	if cfg.State.Path != "./my-state.json" {
		t.Errorf("state.path = %q, want %q", cfg.State.Path, "./my-state.json")
	}
	if cfg.Health.Address != ":9090" {
		t.Errorf("health.address = %q, want %q", cfg.Health.Address, ":9090")
	}
}

func TestLoad_Defaults(t *testing.T) {
	path := writeTOML(t, `
[claude_plane]
api_url = "http://localhost:8080"
api_key = "cpk_abc12345_randombase64urlkey"
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.State.Path != "./bridge-state.json" {
		t.Errorf("default state.path = %q, want %q", cfg.State.Path, "./bridge-state.json")
	}
	if cfg.Health.Address != ":8081" {
		t.Errorf("default health.address = %q, want %q", cfg.Health.Address, ":8081")
	}
}

func TestLoad_MissingAPIURL(t *testing.T) {
	path := writeTOML(t, `
[claude_plane]
api_key = "cpk_abc12345_randombase64urlkey"
`)

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing api_url, got nil")
	}
}

func TestLoad_MissingAPIKey(t *testing.T) {
	path := writeTOML(t, `
[claude_plane]
api_url = "http://localhost:8080"
`)

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing api_key, got nil")
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	path := writeTOML(t, `not valid toml ===`)

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestValidate_BothFieldsMissing(t *testing.T) {
	cfg := &config.Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty config, got nil")
	}
}
