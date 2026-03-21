# Quickstart

Get claude-plane running in under a minute.

## Prerequisites

- **Docker** — [install instructions](https://docs.docker.com/get-docker/)
- **Claude CLI** — installed and authenticated on the machine(s) where agents will run

## 1. Start the Server

```bash
docker run -d --name claude-plane \
  -p 4200:4200 -p 4201:4201 \
  -v claude-plane-data:/data \
  -e ADMIN_EMAIL=admin@localhost \
  -e ADMIN_PASSWORD=changeme123 \
  -e SERVER_URL=http://YOUR_IP:4200 \
  jurel89/claude-plane:latest
```

Replace `YOUR_IP` with your machine's IP address (agents need this to connect). Check the logs for confirmation:

```bash
docker logs claude-plane
```

The server, bridge, and web UI are now running at **http://YOUR_IP:4200**.

## 2. Add an Agent

On any machine where you want to run Claude sessions:

```bash
# Download the agent binary from your server
curl -o claude-plane-agent http://YOUR_IP:4200/dl/agent/linux-amd64
chmod +x claude-plane-agent

# Generate a provisioning code from the dashboard (Provisioning page)
# Then join:
./claude-plane-agent join CODE --server http://YOUR_IP:4200 --insecure

# Install as a background service (recommended)
sudo ./claude-plane-agent install-service --config ~/.claude-plane/agent.toml
```

Use `linux-arm64`, `darwin-amd64`, or `darwin-arm64` for other platforms. The `--insecure` flag is needed for HTTP (non-TLS) servers.

## 3. Start Using It

1. Open **http://YOUR_IP:4200** and log in
2. Your agent machine appears on the **Machines** page
3. Create a session from the **Command Center** or **Sessions** page
4. For GitHub/Telegram integrations, go to **Connectors**

## Using Docker Compose

For a more permanent setup, create a `docker-compose.yml`:

```yaml
services:
  server:
    image: jurel89/claude-plane:latest
    ports:
      - "4200:4200"
      - "4201:4201"
    volumes:
      - data:/data
    environment:
      - ADMIN_EMAIL=admin@localhost
      - ADMIN_PASSWORD=changeme123
      - SERVER_URL=http://YOUR_IP:4200
    restart: unless-stopped

volumes:
  data:
```

```bash
docker compose up -d
```

## Next Steps

- [Agent Installation](install-agent.md) — Detailed agent setup for remote workers
- [Configuration Reference](configuration.md) — All config options
- [Architecture](architecture.md) — System design and data flows

## Building from Source

For contributors and advanced users who want to run without Docker:

```bash
git clone https://github.com/kodrunhq/claude-plane.git
cd claude-plane

# Build frontend
cd web && npm install && npm run build && cd ..

# Build binaries
go build -o claude-plane-server ./cmd/server
go build -o claude-plane-agent ./cmd/agent
go build -o claude-plane-bridge ./cmd/bridge
```

See the [Configuration Reference](configuration.md) for server.toml, agent.toml, and bridge.toml options.
