# Agent Installation

Deploy `claude-plane-agent` on worker machines where Claude CLI sessions will run.

## Prerequisites

- Linux machine (Ubuntu 22.04+, Debian 12+, or similar)
- **Claude CLI** — installed and authenticated. The agent spawns Claude CLI processes in PTYs.
- Outbound TCP access to the server's gRPC port (default: 4201)

## 1. Quick Start: Join with Provisioning Code

The fastest way to set up an agent. The server admin generates a short provisioning code on the Provisioning page (or via the API), then shares it with you.

### Download the binary

```bash
curl -o claude-plane-agent https://your-server:4200/dl/agent/linux-amd64
chmod +x claude-plane-agent
```

### One-command setup (recommended)

The `--service` flag joins the server **and** installs a system service in one step:

```bash
./claude-plane-agent join CODE --server https://your-server:4200 --service
```

This will:
1. Redeem the provisioning code and download TLS certificates
2. Write config to `~/.claude-plane/agent.toml`
3. Install and start a systemd service (prompts for sudo)

For servers running plain HTTP (development only), add `--insecure`:

```bash
./claude-plane-agent join CODE --server http://your-server:4200 --insecure --service
```

### Multi-step variant

If you prefer to join and install the service separately:

```bash
# Step 1: Join — downloads certs and writes config
./claude-plane-agent join CODE --server https://your-server:4200

# Step 2: Install as a service (requires sudo)
sudo ./claude-plane-agent install-service --config ~/.claude-plane/agent.toml
```

## 2. Re-registering an Agent

Running `join` again on a machine that already has an agent is safe and requires no manual cleanup. The command automatically:

- **Stops any existing agent** — whether running as a systemd service or a standalone process
- **Overwrites certificates and config** with the new provisioning response
- **Restarts the service** if `--service` is used

```bash
# Re-register with a new provisioning code
./claude-plane-agent join NEW_CODE --server https://your-server:4200 --service
```

No need to manually stop the service or remove old files first.

## 3. Uninstalling

### Remove the service only

Stops the systemd service and removes the unit file, but leaves config, certificates, and data in place:

```bash
sudo claude-plane-agent uninstall-service
```

### Full removal

Stops the service **and** removes all configuration, certificates, and data:

```bash
sudo claude-plane-agent uninstall-service --purge
```

After `--purge`, the machine can be re-provisioned from scratch with a new `join` command.

## 4. Verifying Connectivity

Check the agent logs:

```bash
journalctl -u claude-plane-agent -f
```

You should see the agent register with the server. In the web dashboard, the machine should appear on the Machines page with an "online" status.

## 5. Advanced: Manual Certificate Setup

If you cannot use provisioning codes (e.g., air-gapped networks), you can configure the agent manually.

### Build the binary

```bash
go build -o claude-plane-agent ./cmd/agent
```

Copy to the worker:

```bash
scp claude-plane-agent user@worker:/usr/local/bin/
```

### Transfer certificates

The server admin generates agent certificates using the server's CA tool. You need three files on the worker:

```
/etc/claude-plane/ca.pem             — CA certificate (same for all agents)
/etc/claude-plane/agent.pem          — This agent's certificate
/etc/claude-plane/agent-key.pem      — This agent's private key
```

The machine ID embedded in the certificate CN must match the `agent.machine_id` in the config file.

### Create configuration

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

### Create system user and directories

```bash
sudo useradd --system --create-home --shell /bin/bash claude-plane
sudo mkdir -p /etc/claude-plane
sudo chmod 600 /etc/claude-plane/agent.toml
sudo chown claude-plane:claude-plane /etc/claude-plane/agent.toml
```

Note: The agent user needs a real home directory and shell because it spawns Claude CLI processes. The Claude CLI may need access to its own config in the user's home directory.

### Ensure Claude CLI access

The Claude CLI must be accessible to the agent's system user:

```bash
# Verify Claude CLI is available
sudo -u claude-plane claude --version

# If Claude CLI is installed for a different user, either:
# 1. Install it globally
# 2. Set claude_cli_path in agent.toml to the full path
```

### Install as system service

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

### Multi-agent setup

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

**Agent already running:**
- The agent uses a PID lock file (`data/agent.pid`) to prevent duplicate instances. If a previous run exited uncleanly and left a stale PID file, the new process detects this automatically and removes it. If the PID file references a process that is still alive, the new agent will refuse to start. To resolve: stop the existing agent (`sudo systemctl stop claude-plane-agent` or kill the process), then start again.

**Orphaned processes after crash:**
- On startup, the agent automatically reaps orphaned child processes (e.g., leftover Claude CLI sessions) from a previous crash. No manual cleanup is needed. Check the agent logs for `reaping orphaned process` messages if you want to confirm this happened.

## Next Steps

- [Configuration Reference](configuration.md) — All config options
- [Architecture](architecture.md) — How agents communicate with the server
