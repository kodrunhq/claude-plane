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
	assertFileContains(t, filepath.Join(configDir, "agent.toml"), "10.0.1.50:9090")

	// Verify file permissions (all should be 0600).
	permChecks := []struct {
		path string
		name string
	}{
		{filepath.Join(configDir, "certs", "ca.pem"), "ca.pem"},
		{filepath.Join(configDir, "certs", "agent.pem"), "agent.pem"},
		{filepath.Join(configDir, "certs", "agent-key.pem"), "agent-key.pem"},
		{filepath.Join(configDir, "agent.toml"), "agent.toml"},
	}
	for _, pc := range permChecks {
		info, err := os.Stat(pc.path)
		if err != nil {
			t.Fatalf("stat %s: %v", pc.name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o, want 600", pc.name, info.Mode().Perm())
		}
	}
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

func TestValidateServerURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		insecure  bool
		wantErr   bool
		errSubstr string
	}{
		{"https not insecure", "https://example.com", false, false, ""},
		{"http not insecure", "http://example.com", false, true, "HTTPS"},
		{"http insecure", "http://example.com", true, false, ""},
		{"uppercase HTTP not insecure", "HTTP://example.com", false, true, "HTTPS"},
		{"invalid url", "://bad", false, true, "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServerURL(tt.url, tt.insecure)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestJoin_NetworkFailure(t *testing.T) {
	err := ExecuteJoin("http://localhost:1", "A3X9K2", t.TempDir())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connect to server") {
		t.Errorf("error %q does not contain %q", err.Error(), "connect to server")
	}
}

func TestJoin_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	err := ExecuteJoin(server.URL, "A3X9K2", t.TempDir())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse response") {
		t.Errorf("error %q does not contain %q", err.Error(), "parse response")
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
