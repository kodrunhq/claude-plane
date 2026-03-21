package provision

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"text/template"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

var installScriptTmpl = template.Must(template.New("install").Parse(`#!/usr/bin/env bash
set -euo pipefail

# claude-plane agent install script
# Machine: {{.MachineID}}
# Server: {{.ServerAddress}}
# OS: {{.TargetOS}}

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/claude-plane"

echo "==> Installing claude-plane-agent for {{.MachineID}}..."

# Download agent binary
echo "==> Downloading agent binary..."
curl -sfL "{{.DownloadURL}}" -o "${INSTALL_DIR}/claude-plane-agent"
chmod +x "${INSTALL_DIR}/claude-plane-agent"

# Portable base64 decode (GNU uses -d, BSD/macOS uses -D)
b64decode() {
    if command -v base64 >/dev/null 2>&1; then
        if base64 --help 2>&1 | grep -q '\-d'; then
            base64 -d
        else
            base64 -D
        fi
    else
        openssl base64 -d
    fi
}

# Write certificates
echo "==> Writing certificates..."
mkdir -p "${CONFIG_DIR}/certs"
echo "{{.CACertB64}}" | b64decode > "${CONFIG_DIR}/certs/ca.pem"
echo "{{.AgentCertB64}}" | b64decode > "${CONFIG_DIR}/certs/agent.pem"
echo "{{.AgentKeyB64}}" | b64decode > "${CONFIG_DIR}/certs/agent-key.pem"
chmod 600 "${CONFIG_DIR}/certs/agent-key.pem"

# Generate agent config
echo "==> Generating agent configuration..."
cat > "${CONFIG_DIR}/agent.toml" << 'TOML'
[agent]
machine_id = "{{.MachineID}}"
data_dir = "/var/lib/claude-plane"

[server]
address = "{{.GRPCAddress}}"

[tls]
ca_cert = "/etc/claude-plane/certs/ca.pem"
agent_cert = "/etc/claude-plane/certs/agent.pem"
agent_key = "/etc/claude-plane/certs/agent-key.pem"
TOML

# Create data directory
mkdir -p /var/lib/claude-plane
{{if eq .TargetOS "darwin"}}
# Create launchd service (macOS)
echo "==> Creating launchd service..."
cat > /Library/LaunchDaemons/com.claude-plane.agent.plist << 'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.claude-plane.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/claude-plane-agent</string>
        <string>run</string>
        <string>--config</string>
        <string>/etc/claude-plane/agent.toml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/claude-plane-agent.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/claude-plane-agent.log</string>
</dict>
</plist>
PLIST

launchctl load /Library/LaunchDaemons/com.claude-plane.agent.plist

echo "==> claude-plane-agent installed and started for {{.MachineID}}"
echo "==> Check status: launchctl list | grep claude-plane"
{{else}}
# Create systemd service (Linux)
echo "==> Creating systemd service..."
cat > /etc/systemd/system/claude-plane-agent.service << 'UNIT'
[Unit]
Description=claude-plane agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/claude-plane-agent run --config /etc/claude-plane/agent.toml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT

# Enable and start
systemctl daemon-reload
systemctl enable claude-plane-agent
systemctl start claude-plane-agent

echo "==> claude-plane-agent installed and started for {{.MachineID}}"
echo "==> Check status: systemctl status claude-plane-agent"
{{end}}`))

type scriptData struct {
	MachineID    string
	TargetOS     string
	ServerAddress string
	GRPCAddress  string
	DownloadURL  string
	CACertB64    string
	AgentCertB64 string
	AgentKeyB64  string
}

// RenderInstallScript generates the install script for a provisioning token.
func RenderInstallScript(token *store.ProvisioningToken) (string, error) {
	downloadURL := token.ServerAddress + "/dl/agent/" + token.TargetOS + "-" + token.TargetArch

	data := scriptData{
		MachineID:    token.MachineID,
		TargetOS:     token.TargetOS,
		ServerAddress: token.ServerAddress,
		GRPCAddress:  token.GRPCAddress,
		DownloadURL:  downloadURL,
		CACertB64:    base64.StdEncoding.EncodeToString([]byte(token.CACertPEM)),
		AgentCertB64: base64.StdEncoding.EncodeToString([]byte(token.AgentCertPEM)),
		AgentKeyB64:  base64.StdEncoding.EncodeToString([]byte(token.AgentKeyPEM)),
	}

	var buf bytes.Buffer
	if err := installScriptTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render install script: %w", err)
	}
	return buf.String(), nil
}
