# Agent Installation

Deploy `claude-plane-agent` on worker machines where Claude CLI sessions will run.

## Prerequisites

- Linux machine (Ubuntu 22.04+, Debian 12+, or similar)
- Go 1.25+ (for building from source) or a pre-built binary
- **Claude CLI** — installed and authenticated. The agent spawns Claude CLI processes in PTYs.
- Agent certificate, key, and CA certificate from the server (see [Server Installation](install-server.md#3-set-up-tls))
- Outbound TCP access to the server's gRPC port (default: 4201)

## 1. Build the Binary

On your build machine:

```bash
go build -o claude-plane-agent ./cmd/agent
```

Copy to the worker:

```bash
scp claude-plane-agent user@worker:/usr/local/bin/
```

## 2. Transfer Certificates

The server admin generates agent certificates using the server's CA tool. You need three files on the worker:

```
/etc/claude-plane/ca.pem             — CA certificate (same for all agents)
/etc/claude-plane/agent.pem          — This agent's certificate
/etc/claude-plane/agent-key.pem      — This agent's private key
```

The machine ID embedded in the certificate CN must match the `agent.machine_id` in the config file.

## 3. Create Configuration

Create `/etc/claude-plane/agent.toml`:

```toml
[server]
address = "your-server-hostname.example.com:4201"

[tls]
ca_cert = "/etc/claude-plane/ca.pem"
agent_cert = "/etc/claude-plane/agent.pem"
agent_key = "/etc/claude-plane/agent-key.pem"

[agent]
machine_id = "worker-1"
max_sessions = 5
# claude_cli_path = "claude"   # Default: looks up "claude" in PATH
# data_dir = ""                 # Optional: local data directory
```

See [Configuration Reference](configuration.md) for all options.

## 4. Create System User and Directories

```bash
sudo useradd --system --create-home --shell /bin/bash claude-plane
sudo mkdir -p /etc/claude-plane
sudo chmod 600 /etc/claude-plane/agent.toml
sudo chown claude-plane:claude-plane /etc/claude-plane/agent.toml
```

Note: The agent user needs a real home directory and shell because it spawns Claude CLI processes. The Claude CLI may need access to its own config in the user's home directory.

## 5. Ensure Claude CLI Access

The Claude CLI must be accessible to the agent's system user:

```bash
# Verify Claude CLI is available
sudo -u claude-plane claude --version

# If Claude CLI is installed for a different user, either:
# 1. Install it globally
# 2. Set claude_cli_path in agent.toml to the full path
```

## Alternative: Quick Join

If the server admin has generated a provisioning short code, you can skip the manual certificate setup:

```bash
# Download from the server
curl -o claude-plane-agent http://server:4200/dl/agent/linux-amd64
chmod +x claude-plane-agent

# Join with the short code
./claude-plane-agent join CODE --server http://server:4200 --insecure

# Install as a service
sudo ./claude-plane-agent install-service --config ~/.claude-plane/agent.toml
```

## 6. Install as System Service

The recommended way to run the agent is as a system service:

```bash
sudo claude-plane-agent install-service --config /etc/claude-plane/agent.toml
```

This automatically creates and enables:
- **Linux**: A systemd service (`/etc/systemd/system/claude-plane-agent.service`)
- **macOS**: A launchd daemon (`/Library/LaunchDaemons/com.claude-plane.agent.plist`)

The service starts on boot, restarts on crash (5s delay), and runs as the specified user.

### Manual systemd setup (alternative)

If you prefer to create the service manually, create `/etc/systemd/system/claude-plane-agent.service`:

```ini
[Unit]
Description=claude-plane agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=claude-plane
Group=claude-plane
ExecStart=/usr/local/bin/claude-plane-agent run --config /etc/claude-plane/agent.toml
Restart=on-failure
RestartSec=5s

# Agent needs to spawn processes and manage PTYs
NoNewPrivileges=false
ProtectSystem=strict
ReadOnlyPaths=/etc/claude-plane

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable claude-plane-agent
sudo systemctl start claude-plane-agent
sudo systemctl status claude-plane-agent
```

## Verifying Connectivity

Check the agent logs:

```bash
journalctl -u claude-plane-agent -f
```

You should see the agent register with the server. In the web dashboard, the machine should appear on the Machines page with an "online" status.

## Multi-Agent Setup

To run multiple agents:

1. Generate a unique certificate for each worker:
   ```bash
   # On the server
   claude-plane-server ca issue-agent --machine-id worker-2
   claude-plane-server ca issue-agent --machine-id worker-3
   ```

2. Each agent gets its own config with a unique `machine_id`
3. All agents use the same CA certificate and connect to the same server address

## Troubleshooting

**Agent can't connect:**
- Verify outbound access to the server's gRPC port: `nc -zv server-host 4201`
- Check that the CA certificate matches the one used to issue the server certificate
- Verify the machine ID in the config matches the certificate CN

**Claude CLI not found:**
- Set `claude_cli_path` in `agent.toml` to the full path of the Claude CLI binary
- Ensure the agent's system user has the necessary environment (PATH, home directory)

**Permission denied on PTY:**
- The agent user needs permission to allocate PTYs
- Ensure `NoNewPrivileges=false` in the systemd unit (agent spawns child processes)

## Next Steps

- [Configuration Reference](configuration.md) — All config options
- [Architecture](architecture.md) — How agents communicate with the server
