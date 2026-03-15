package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type joinResponse struct {
	MachineID    string `json:"machine_id"`
	GRPCAddress  string `json:"grpc_address"`
	CACertPEM    string `json:"ca_cert_pem"`
	AgentCertPEM string `json:"agent_cert_pem"`
	AgentKeyPEM  string `json:"agent_key_pem"`
}

type joinErrorResponse struct {
	Error string `json:"error"`
}

// ResolveServerURL determines the server URL from the flag or environment.
func ResolveServerURL(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if env := os.Getenv("CLAUDE_PLANE_SERVER"); env != "" {
		return env, nil
	}
	return "", fmt.Errorf("server URL required. Use --server or set CLAUDE_PLANE_SERVER")
}

// ExecuteJoin calls the server's join endpoint with the short code,
// writes certificates and config to configDir.
func ExecuteJoin(serverURL, code, configDir string) error {
	body, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := http.Post(serverURL+"/api/v1/provision/join", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp joinErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("server error: %s", errResp.Error)
		}
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var joinResp joinResponse
	if err := json.Unmarshal(respBody, &joinResp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	// Create directories
	certsDir := filepath.Join(configDir, "certs")
	if err := os.MkdirAll(certsDir, 0o750); err != nil {
		return fmt.Errorf("create certs dir: %w", err)
	}

	// Write certificates
	files := map[string]string{
		filepath.Join(certsDir, "ca.pem"):        joinResp.CACertPEM,
		filepath.Join(certsDir, "agent.pem"):      joinResp.AgentCertPEM,
		filepath.Join(certsDir, "agent-key.pem"):   joinResp.AgentKeyPEM,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", filepath.Base(path), err)
		}
	}

	// Write agent.toml
	configContent := fmt.Sprintf(`[server]
address = %q

[tls]
ca_cert   = %q
agent_cert = %q
agent_key  = %q

[agent]
machine_id = %q
data_dir   = %q
`,
		joinResp.GRPCAddress,
		filepath.Join(certsDir, "ca.pem"),
		filepath.Join(certsDir, "agent.pem"),
		filepath.Join(certsDir, "agent-key.pem"),
		joinResp.MachineID,
		filepath.Join(configDir, "data"),
	)

	configPath := filepath.Join(configDir, "agent.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o640); err != nil {
		return fmt.Errorf("write agent.toml: %w", err)
	}

	return nil
}
