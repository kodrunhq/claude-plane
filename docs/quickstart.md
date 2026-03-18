# Quickstart

Get claude-plane running on a single machine for evaluation. This sets up the server and one agent on the same host — no separate machines needed.

## Option A: One-Command Setup

The fastest way to get started. Builds from source, generates certs, seeds an admin account, and starts both server and agent:

```bash
git clone https://github.com/kodrunhq/claude-plane.git
cd claude-plane
./install.sh quickstart
```

This prints the dashboard URL and admin credentials on success. Skip to [Open the Dashboard](#9-open-the-dashboard).

## Option B: Step-by-Step

If you prefer to understand each step, or need to customize the setup:

### Prerequisites

- **Go 1.25+** — [install instructions](https://go.dev/doc/install)
- **Node.js 22+** — [install instructions](https://nodejs.org/)
- **Claude CLI** — installed and authenticated on the machine where the agent runs

### 1. Build the Binaries

```bash
git clone https://github.com/kodrunhq/claude-plane.git
cd claude-plane

# Build frontend first (embedded into server binary via go:embed)
cd web && npm install && npm run build && cd ..

# Build Go binaries
go build -o claude-plane-server ./cmd/server
go build -o claude-plane-agent ./cmd/agent
```

### 2. Set Up TLS Certificates

claude-plane uses mTLS for agent-to-server communication. The server includes a built-in CA tool:

```bash
# Initialize the CA (creates ca/ directory with ca.pem and ca-key.pem)
./claude-plane-server ca init

# Issue a server certificate (creates server-cert/ with server.pem and server-key.pem)
./claude-plane-server ca issue-server --hostnames localhost

# Issue an agent certificate (creates agent-cert/ with agent.pem and agent-key.pem)
./claude-plane-server ca issue-agent --machine-id worker-1
```

This creates three directories:
- `ca/` — `ca.pem` (certificate) and `ca-key.pem` (private key)
- `server-cert/` — `server.pem` and `server-key.pem`
- `agent-cert/` — `agent.pem` and `agent-key.pem`

### 3. Create Server Config

Create `server.toml`:

```toml
[http]
listen = "0.0.0.0:4200"
tls_cert = "server-cert/server.pem"
tls_key = "server-cert/server-key.pem"

[grpc]
listen = "0.0.0.0:4201"

[tls]
ca_cert = "ca/ca.pem"
server_cert = "server-cert/server.pem"
server_key = "server-cert/server-key.pem"

[database]
path = "claude-plane.db"

[auth]
# Must be at least 32 characters. Generate one:
# openssl rand -base64 48
jwt_secret = "CHANGE-ME-generate-a-real-secret-at-least-32-chars"
```

### 4. Create Agent Config

Create `agent.toml`:

```toml
[server]
address = "localhost:4201"

[tls]
ca_cert = "ca/ca.pem"
agent_cert = "agent-cert/agent.pem"
agent_key = "agent-cert/agent-key.pem"

[agent]
machine_id = "worker-1"
max_sessions = 5
```

### 5. Seed the Admin Account

```bash
./claude-plane-server seed-admin --email admin@example.com
# Enter password when prompted (min 8 characters)
```

Or non-interactively:

```bash
echo "your-password-here" | ./claude-plane-server seed-admin --email admin@example.com
```

### 6. Start the Server

```bash
./claude-plane-server serve --config server.toml
```

### 7. Start the Agent

In a separate terminal:

```bash
./claude-plane-agent run --config agent.toml
```

For production use, install as a system service instead:

```bash
sudo ./claude-plane-agent install-service --config agent.toml
```

## Open the Dashboard

Navigate to `http://localhost:4200` in your browser. Log in with the admin credentials from the setup.

You should see the agent machine listed on the **Machines** page. From here you can:

- **Sessions** — Create Claude CLI or shell sessions and interact via the terminal view
- **Multi-View** — Open 2-6 sessions simultaneously in a configurable split-pane layout (select sessions from the Sessions page and click "Open in Multi-View")
- **Jobs** — Build multi-step task DAGs that execute across sessions
- **Events** — Monitor real-time system events

## Next Steps

- [Server Installation](install-server.md) — Production deployment with systemd
- [Agent Installation](install-agent.md) — Deploy agents on remote worker machines
- [Configuration Reference](configuration.md) — All available config options
