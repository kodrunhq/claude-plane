# Quickstart

Get claude-plane running on a single machine for evaluation. This guide sets up the server and one agent on the same host.

> **Note:** The full HTTP serve loop is under active development. The server binary currently supports config loading, database initialization, CA tooling, and admin seeding. The serve command will be fully wired in an upcoming release.

## Prerequisites

- **Go 1.25+** — [install instructions](https://go.dev/doc/install)
- **Node.js 22+** — [install instructions](https://nodejs.org/)
- **Claude CLI** — installed and authenticated on the machine where the agent runs

## 1. Build the Binaries

```bash
git clone https://github.com/kodrunhq/claude-plane.git
cd claude-plane

go build -o claude-plane-server ./cmd/server
go build -o claude-plane-agent ./cmd/agent
```

## 2. Build the Frontend

```bash
cd web
npm install
npm run build
cd ..
```

The build output lands in `internal/server/frontend/dist/` and is embedded into the server binary at compile time. If you want the server to serve the frontend, rebuild the server binary after building the frontend:

```bash
go build -o claude-plane-server ./cmd/server
```

## 3. Set Up TLS Certificates

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

## 4. Create Server Config

Create `server.toml`:

```toml
[http]
listen = "0.0.0.0:8443"
tls_cert = "server-cert/server.pem"
tls_key = "server-cert/server-key.pem"

[grpc]
listen = "0.0.0.0:9443"

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

## 5. Create Agent Config

Create `agent.toml`:

```toml
[server]
address = "localhost:9443"

[tls]
ca_cert = "ca/ca.pem"
agent_cert = "agent-cert/agent.pem"
agent_key = "agent-cert/agent-key.pem"

[agent]
machine_id = "worker-1"
max_sessions = 5
```

## 6. Seed the Admin Account

```bash
./claude-plane-server seed-admin --email admin@example.com
# Enter password when prompted (min 8 characters)
```

Or non-interactively:

```bash
echo "your-password-here" | ./claude-plane-server seed-admin --email admin@example.com
```

## 7. Start the Server

```bash
./claude-plane-server serve --config server.toml
```

## 8. Start the Agent

In a separate terminal:

```bash
./claude-plane-agent run --config agent.toml
```

## 9. Open the Dashboard

Navigate to `https://localhost:8443` in your browser. Log in with the admin credentials you created in step 6.

You should see the agent machine listed in the Machines page. From here you can create Claude CLI sessions and interact with them through the terminal view.

## Next Steps

- [Server Installation](install-server.md) — Production deployment with systemd
- [Agent Installation](install-agent.md) — Deploy agents on remote worker machines
- [Configuration Reference](configuration.md) — All available config options
