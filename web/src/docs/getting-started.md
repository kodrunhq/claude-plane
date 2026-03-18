# Getting Started with claude-plane

claude-plane is a self-hosted control plane for managing interactive Claude CLI sessions across distributed machines. It consists of three components:

- **Server** — The central control plane that serves the web UI, manages sessions, orchestrates jobs, and accepts connections from agents.
- **Agent** — Runs on worker machines. Manages Claude CLI processes, buffers terminal output, and maintains a persistent connection to the server.
- **Bridge** — Connects external services (GitHub, Telegram, Slack) to the server, enabling automated workflows.

## Architecture Overview

Agents dial in to the server — the server never dials out. This means agents can run behind NATs and firewalls without any port forwarding. Sessions survive disconnection: if an agent temporarily loses connectivity, the CLI session keeps running and output is replayed on reconnection.

## Quick Start

The fastest way to get running is with the install script:

```bash
./install.sh quickstart
```

This builds all binaries, generates TLS certificates, creates configuration files, seeds an admin user, and starts the server and a local agent.

## Provisioning Agent Machines

To add a new worker machine:

1. Navigate to **Provisioning** in the sidebar (Admin section).
2. Click **Generate Token** to create a one-time install token.
3. Copy the generated install command — it includes the token and server address.
4. Run the install command on the target machine. The agent binary is downloaded, certificates are issued, and the agent connects automatically.

Each agent machine needs a unique `machine_id` that matches its mTLS certificate. The provisioning flow handles this automatically.

## Quick Join (Short Code)

For a faster setup, you can join a server using a short provisioning code:

1. Navigate to **Provisioning** in the sidebar and click **Generate Token**.
2. Note the 6-character short code displayed.
3. On the target machine, download the agent and join:

```bash
curl -o claude-plane-agent http://your-server:4200/dl/agent/linux-amd64
chmod +x claude-plane-agent
./claude-plane-agent join CODE --server http://your-server:4200
```

4. Install as a background service (recommended):

```bash
sudo ./claude-plane-agent install-service --config ~/.claude-plane/agent.toml
```

The agent will now run as a systemd service, surviving SSH disconnects and machine reboots. Check status with `sudo systemctl status claude-plane-agent`.

## Creating Sessions

Sessions are interactive Claude CLI instances running on agent machines. To start one:

1. Go to **Sessions** in the sidebar.
2. Click **New Session** and select a target machine.
3. The terminal opens with a live connection to the Claude CLI on that machine.

You can also use **Multi-View** to monitor up to 6 sessions simultaneously in resizable split panes.

## Jobs and Automation

Jobs are multi-step workflows that run across your agent fleet. Each job is a DAG (directed acyclic graph) of steps:

1. Navigate to **Jobs** and click **New Job**.
2. Add steps using the visual DAG editor. Each step defines a prompt, target machine, and dependencies.
3. Run the job — the orchestrator executes steps in dependency order.

## Webhooks

Webhooks send real-time HTTP notifications when events occur in claude-plane. Configure them under **Webhooks** in the sidebar. Each webhook can subscribe to specific event types and includes HMAC signature verification for security.

## System Logs

claude-plane provides built-in structured logging accessible from the web UI:

1. Navigate to **Logs** in the Monitoring section of the sidebar.
2. Filter logs by level (DEBUG/INFO/WARN/ERROR), source (server/agent), component, machine, or free text search.
3. Toggle **Live** mode to stream new log entries in real-time via WebSocket.
4. Use time range presets (1h, 6h, 24h, 7d) to narrow results.

Both server and agent logs are collected — agent logs are forwarded to the server automatically via gRPC. Logs are stored in a separate database with configurable retention (default: 7 days).

The **Command Center** dashboard also shows health cards:
- **Connection Health** — per-agent connectivity status
- **Recent Errors** — last 10 error-level log entries
- **Session Success Rate** — success/failure ratio over the last 24 hours
- **Agent Resources** — CPU and memory usage per agent

Real-time toast notifications appear when agents disconnect, connections go stale, or session dispatch fails.

## External Integrations

The bridge component connects external services to claude-plane:

- **Telegram** — Receive event notifications in a Telegram group and trigger jobs via bot commands. See the [Telegram Setup Guide](/docs/telegram-setup).
- **GitHub** — Poll repositories for events (PRs, issues, reviews) and automatically create sessions. See the [GitHub Setup Guide](/docs/github-setup).
- **SMTP** — Send email notifications for subscribed events. See the [SMTP Setup Guide](/docs/smtp-setup).

## Configuration Files

Both the server and agent use TOML configuration files:

- **Server** (`server.toml`) — HTTP/gRPC listen addresses, TLS paths, database path, JWT secret, log level/format/retention.
- **Agent** (`agent.toml`) — Server address, TLS paths, machine ID, max sessions, Claude CLI path.
- **Bridge** (`bridge.toml`) — Server API URL, API key, connector configurations.

## Security Model

- **Agent-to-Server**: Mutual TLS (mTLS) with a built-in CA. Agent certificates use the CN format `agent-{machine-id}`.
- **Frontend-to-Server**: JWT authentication via httpOnly cookies.
- **Webhooks**: HMAC signature verification on all deliveries.
- **Credentials**: Optional encryption vault for stored secrets.

## Next Steps

- [Set up Telegram notifications](/docs/telegram-setup)
- [Connect GitHub repositories](/docs/github-setup)
- [Configure email notifications](/docs/smtp-setup)
