# Server Installation (Advanced)

> **For most users:** Use Docker instead — see the [Quickstart](quickstart.md). This guide is for bare-metal deployments without Docker.

Production deployment guide for `claude-plane-server`.

## Prerequisites

- Linux server (Ubuntu 22.04+, Debian 12+, or similar)
- Go 1.25+ (for building from source) or a pre-built binary
- Node.js 22+ (for building the frontend)

## 1. Build the Binary

```bash
# Build frontend first
cd web && npm install && npm run build && cd ..

# Build server with embedded frontend
go build -o claude-plane-server ./cmd/server
```

Copy the binary to your server:

```bash
scp claude-plane-server user@server:/usr/local/bin/
```

## 2. Set Up Directories

Create the required directories before running the CA tool:

```bash
sudo mkdir -p /etc/claude-plane/ca
sudo mkdir -p /etc/claude-plane/server-cert
sudo mkdir -p /var/lib/claude-plane
```

## 3. Set Up TLS

### Initialize the Certificate Authority

```bash
claude-plane-server ca init --out-dir /etc/claude-plane/ca
```

This creates:
- `/etc/claude-plane/ca/ca.pem` — CA certificate (distribute to agents)
- `/etc/claude-plane/ca/ca-key.pem` — CA private key (keep secure)

### Issue the Server Certificate

```bash
claude-plane-server ca issue-server \
  --ca-dir /etc/claude-plane/ca \
  --out-dir /etc/claude-plane/server-cert \
  --hostnames your-server-hostname.example.com
```

This creates `server.pem` and `server-key.pem` in the output directory.

The `--hostnames` flag adds Subject Alternative Names. `localhost` and `127.0.0.1` are always included automatically.

### Issue Agent Certificates

For each worker machine:

```bash
mkdir -p /tmp/agent-certs/worker-1
claude-plane-server ca issue-agent \
  --ca-dir /etc/claude-plane/ca \
  --out-dir /tmp/agent-certs/worker-1 \
  --machine-id worker-1
```

This creates `agent.pem` and `agent-key.pem`. Securely transfer the agent certificate, key, and CA certificate to each worker machine.

## 4. Create Configuration

Create `/etc/claude-plane/server.toml`:

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
# Generate: openssl rand -base64 48
jwt_secret = "YOUR-SECRET-HERE"
# Optional: token_ttl = "60m" (default: 60 minutes)
```

See [Configuration Reference](configuration.md) for all options.

## 5. Create System User and Set Permissions

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin claude-plane
sudo chown -R claude-plane:claude-plane /var/lib/claude-plane
sudo chmod 700 /var/lib/claude-plane
sudo chmod 600 /etc/claude-plane/server.toml
sudo chown claude-plane:claude-plane /etc/claude-plane/server.toml
```

## 6. Seed the Admin Account

```bash
claude-plane-server seed-admin \
  --db /var/lib/claude-plane/claude-plane.db \
  --email admin@example.com \
  --password-file /path/to/admin-password.txt
```

Or interactively (from a TTY):

```bash
claude-plane-server seed-admin \
  --db /var/lib/claude-plane/claude-plane.db \
  --email admin@example.com
# Enter password at prompt
```

## 7. Create systemd Service

Create `/etc/systemd/system/claude-plane-server.service`:

```ini
[Unit]
Description=claude-plane control plane server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=claude-plane
Group=claude-plane
ExecStart=/usr/local/bin/claude-plane-server serve --config /etc/claude-plane/server.toml
Restart=on-failure
RestartSec=5s

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/claude-plane
ReadOnlyPaths=/etc/claude-plane

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable claude-plane-server
sudo systemctl start claude-plane-server
sudo systemctl status claude-plane-server
```

## 8. Firewall Rules

The server needs two ports open:

| Port | Protocol | Purpose |
|------|----------|---------|
| 4200 | TCP | HTTPS — browser access (REST API + WebSocket) |
| 4201 | TCP | gRPC — agent connections (mTLS) |

If using `ufw`:

```bash
sudo ufw allow 4200/tcp
sudo ufw allow 4201/tcp
```

## 9. Reverse Proxy (Optional)

If you want to put the server behind a reverse proxy for standard HTTPS on port 443:

**Nginx example:**

```nginx
server {
    listen 443 ssl;
    server_name claude-plane.example.com;

    ssl_certificate /path/to/public-cert.pem;
    ssl_certificate_key /path/to/private-key.pem;

    location / {
        proxy_pass https://127.0.0.1:4200;
        proxy_ssl_verify off;  # Internal connection
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

Note: The gRPC port (4201) should be exposed directly — agents connect via gRPC, not HTTP.

## Verifying the Installation

Once the serve loop is fully wired, verify the server is running:

```bash
curl -k https://localhost:4200/api/v1/auth/login -X POST \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"your-password"}'
```

A successful response returns a JWT token.

## Next Steps

- [Agent Installation](install-agent.md) — Deploy agents on worker machines
- [Configuration Reference](configuration.md) — All config options
