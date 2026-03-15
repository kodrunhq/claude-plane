package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJoin_Success(t *testing.T) {
	// Mock server returning valid join response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/provision/join" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var req struct{ Code string `json:"code"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Code != "A3X9K2" {
			t.Errorf("code = %q, want A3X9K2", req.Code)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"machine_id":     "worker-01",
			"grpc_address":   "10.0.1.50:9090",
			"ca_cert_pem":    "-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----",
			"agent_cert_pem": "-----BEGIN CERTIFICATE-----\nAGENT\n-----END CERTIFICATE-----",
			"agent_key_pem":  "-----BEGIN RSA PRIVATE KEY-----\nKEY\n-----END RSA PRIVATE KEY-----",
		})
	}))
	defer server.Close()

	configDir := t.TempDir()

	err := ExecuteJoin(server.URL, "A3X9K2", configDir)
	if err != nil {
		t.Fatalf("ExecuteJoin: %v", err)
	}

	// Verify files written
	assertFileContains(t, filepath.Join(configDir, "certs", "ca.pem"), "-----BEGIN CERTIFICATE-----\nCA")
	assertFileContains(t, filepath.Join(configDir, "certs", "agent.pem"), "-----BEGIN CERTIFICATE-----\nAGENT")
	assertFileContains(t, filepath.Join(configDir, "certs", "agent-key.pem"), "-----BEGIN RSA PRIVATE KEY-----\nKEY")
	assertFileContains(t, filepath.Join(configDir, "agent.toml"), "machine_id")
	assertFileContains(t, filepath.Join(configDir, "agent.toml"), "worker-01")
}

func TestJoin_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired code"})
	}))
	defer server.Close()

	err := ExecuteJoin(server.URL, "ZZZZZZ", t.TempDir())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveServerURL(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		envVar  string
		wantURL string
		wantErr bool
	}{
		{"flag provided", "https://example.com", "", "https://example.com", false},
		{"env provided", "", "https://env.example.com", "https://env.example.com", false},
		{"flag takes precedence", "https://flag.com", "https://env.com", "https://flag.com", false},
		{"neither provided", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVar != "" {
				t.Setenv("CLAUDE_PLANE_SERVER", tt.envVar)
			} else {
				os.Unsetenv("CLAUDE_PLANE_SERVER")
			}

			got, err := ResolveServerURL(tt.flag)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.wantURL {
				t.Errorf("url = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func assertFileContains(t *testing.T, path, substr string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), substr) {
		t.Errorf("%s does not contain %q; contents:\n%s", path, substr, string(data))
	}
}
