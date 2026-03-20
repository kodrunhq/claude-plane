# Configuration Reference

Both the server and agent use TOML configuration files.

## Server Configuration (`server.toml`)

### `[http]` — HTTP/WebSocket Listener

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `listen` | string | Yes | Bind address for the HTTP server (e.g., `"0.0.0.0:4200"`) |
| `tls_cert` | string | No | Path to TLS certificate for HTTPS |
| `tls_key` | string | No | Path to TLS private key for HTTPS |

### `[grpc]` — gRPC Listener

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `listen` | string | Yes | Bind address for the gRPC server (e.g., `"0.0.0.0:4201"`) |

### `[tls]` — mTLS Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ca_cert` | string | Yes | Path to the CA certificate for verifying agent certificates |
| `server_cert` | string | Yes | Path to the server's TLS certificate |
| `server_key` | string | Yes | Path to the server's TLS private key |

### `[database]` — SQLite Database

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Path to the SQLite database file (e.g., `"/var/lib/claude-plane/claude-plane.db"`) |

The database file is created automatically if it doesn't exist. Migrations run on startup.

### `[auth]` — Authentication

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `jwt_secret` | string | Yes | Secret key for signing JWT tokens. Must be at least 32 characters. |
| `token_ttl` | string | No | JWT token lifetime as a Go duration string. Default: `"60m"` |

### `[log]` — Logging

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `level` | string | No | `"info"` | Log level: `debug`, `info`, `warn`, `error` |
| `format` | string | No | `"text"` | Log format: `text`, `json` |
| `retention_days` | int | No | `7` | Auto-purge logs older than this many days |
| `buffer_size` | int | No | `1000` | Async write buffer size |

Logs are stored in a separate SQLite database (`logs.db`) alongside the main database. Both server and agent logs are captured — agents forward their logs to the server automatically via gRPC. Logs can be viewed in the web UI under **Logs** in the Monitoring section.

### Full Example

```toml
[http]
listen = "0.0.0.0:4200"
tls_cert = "/etc/claude-plane/server-cert/server.pem"
tls_key = "/etc/claude-plane/server-cert/server-key.pem"

[grpc]
listen = "0.0.0.0:4201"

[tls]
ca_cert = "/etc/claude-plane/ca/ca.pem"
server_cert = "/etc/claude-plane/server-cert/server.pem"
server_key = "/etc/claude-plane/server-cert/server-key.pem"

[database]
path = "/var/lib/claude-plane/claude-plane.db"

[auth]
jwt_secret = "generate-with-openssl-rand-base64-48-minimum-32-chars"
token_ttl = "60m"

[log]
level = "info"
format = "text"
retention_days = 7
buffer_size = 1000
```

---

## Agent Configuration (`agent.toml`)

### `[server]` — Server Connection

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `address` | string | Yes | Server gRPC address (e.g., `"server.example.com:4201"`) |

### `[tls]` — mTLS Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ca_cert` | string | Yes | Path to the CA certificate for verifying the server |
| `agent_cert` | string | Yes | Path to this agent's mTLS certificate |
| `agent_key` | string | Yes | Path to this agent's mTLS private key |

### `[agent]` — Agent Settings

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `machine_id` | string | Yes | — | Unique identifier for this machine. Must match the certificate CN. |
| `max_sessions` | int | No | `5` | Maximum number of concurrent Claude CLI sessions |
| `claude_cli_path` | string | No | `"claude"` | Path to the Claude CLI binary |
| `data_dir` | string | No | `""` | Local data directory for agent state |

### Full Example

```toml
[server]
address = "server.example.com:4201"

[tls]
ca_cert = "/etc/claude-plane/ca.pem"
agent_cert = "/etc/claude-plane/agent.pem"
agent_key = "/etc/claude-plane/agent-key.pem"

[agent]
machine_id = "worker-1"
max_sessions = 10
claude_cli_path = "/usr/local/bin/claude"
data_dir = "/var/lib/claude-plane-agent"
```

---

## CLI Flags

### Server (`claude-plane-server`)

```
claude-plane-server serve --config server.toml
claude-plane-server ca init --out-dir ./ca
claude-plane-server ca issue-server --ca-dir ./ca --out-dir ./server-cert --hostnames host1,host2
claude-plane-server ca issue-agent --ca-dir ./ca --out-dir ./agent-cert --machine-id worker-1
claude-plane-server seed-admin --db claude-plane.db --email admin@example.com [--password-file pw.txt] [--name Admin]
```

### Agent (`claude-plane-agent`)

```
claude-plane-agent run --config agent.toml
claude-plane-agent join CODE --server https://server:4200 [--insecure] [--service] [--config-dir path]
claude-plane-agent install-service --config agent.toml [--user username]
claude-plane-agent uninstall-service [--purge] [--config-dir path]
```

#### Runtime Files

| File | Location | Description |
|------|----------|-------------|
| `agent.pid` | `{data_dir}/agent.pid` | PID lock file written by `run`, prevents duplicate agents |
