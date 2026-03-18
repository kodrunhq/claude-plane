# Agent Setup Guide

This guide covers how to connect agent machines to your claude-plane server.

## Method 1: Quick Join (Recommended)

The fastest way to add a new agent:

### 1. Generate a Provisioning Code

In the claude-plane dashboard, go to **Provisioning** (Admin section) and click **Generate Token**. Note the 6-character short code.

### 2. Download and Join

On the target machine:

```bash
# Download the agent binary
curl -o claude-plane-agent http://your-server:4200/dl/agent/linux-amd64
chmod +x claude-plane-agent

# Join using the short code
./claude-plane-agent join CODE --server http://your-server:4200
```

For HTTPS servers (recommended for production):
```bash
./claude-plane-agent join CODE --server https://your-server:4200
```

For plain HTTP (development only):
```bash
./claude-plane-agent join CODE --server http://your-server:4200 --insecure
```

### 3. Install as Background Service

Install the agent as a system service so it runs in the background, survives SSH disconnects, and starts automatically on boot:

```bash
sudo ./claude-plane-agent install-service --config ~/.claude-plane/agent.toml
```

On **Linux**, this creates a systemd service. On **macOS**, it creates a launchd daemon.

### 4. Verify

```bash
# Check the service is running
sudo systemctl status claude-plane-agent

# View live logs
sudo journalctl -u claude-plane-agent -f
```

The machine should appear as "connected" on the **Machines** page in the dashboard.

## Method 2: Provisioning Script

For automated deployments, use the one-line install script:

1. Go to **Provisioning** and click **Generate Token**.
2. Copy the install command — it includes the token and server address.
3. Run with `sudo` on the target machine:

```bash
curl -sfL 'https://your-server/api/v1/provision/TOKEN/install' | sudo bash
```

This downloads the binary, writes certificates, creates the config, installs a systemd/launchd service, and starts the agent — all in one step.

## Managing the Agent Service

### Linux (systemd)

```bash
sudo systemctl status claude-plane-agent    # check status
sudo systemctl restart claude-plane-agent   # restart
sudo systemctl stop claude-plane-agent      # stop
sudo journalctl -u claude-plane-agent -f    # view logs
sudo journalctl -u claude-plane-agent --since "1 hour ago"  # recent logs
```

### macOS (launchd)

```bash
launchctl list | grep claude-plane          # check status
sudo launchctl unload /Library/LaunchDaemons/com.claude-plane.agent.plist  # stop
sudo launchctl load /Library/LaunchDaemons/com.claude-plane.agent.plist    # start
tail -f /var/log/claude-plane-agent.log     # view logs
```

## Configuration

The agent config is stored at `~/.claude-plane/agent.toml` (user install) or `/etc/claude-plane/agent.toml` (system install):

```toml
[server]
address = "your-server:4201"

[tls]
ca_cert = "/path/to/ca.pem"
agent_cert = "/path/to/agent.pem"
agent_key = "/path/to/agent-key.pem"

[agent]
machine_id = "worker-1"
max_sessions = 5
# claude_cli_path = "claude"   # Default: looks up "claude" in PATH
```

## Troubleshooting

**Agent can't connect:**
- Verify outbound access to the server's gRPC port: `nc -zv server-host 4201`
- Check that the CA certificate matches the server's CA
- View agent logs: `sudo journalctl -u claude-plane-agent -f`

**"failed to dispatch session to agent":**
- Check the **Logs** page in the dashboard for detailed error messages
- The agent may have reconnected but the old connection wasn't cleaned up — this is auto-resolved within 30 seconds

**Claude CLI not found:**
- Set `claude_cli_path` in `agent.toml` to the full path
- Ensure the service user has `claude` in its PATH

**Machine shows "disconnected" but agent is running:**
- Check agent logs for TLS errors
- Verify the machine ID in the config matches the certificate
- The health sweep runs every 30 seconds — wait briefly and refresh
