# claude-plane

[![CI](https://github.com/kodrunhq/claude-plane/actions/workflows/ci.yml/badge.svg)](https://github.com/kodrunhq/claude-plane/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=white)](https://react.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A self-hosted control plane for managing interactive [Claude CLI](https://docs.anthropic.com/en/docs/claude-code) sessions across distributed machines. Run Claude on any number of worker machines and manage them all from a single web interface.

## Features

- **Remote session management** — Create, monitor, and interact with Claude CLI sessions on any connected machine from your browser
- **Persistent sessions** — CLI sessions survive browser disconnects; reconnect and pick up where you left off with full scrollback replay
- **Job system** — Define multi-step jobs as DAGs, trigger runs, and let Claude work across machines with dependency-aware orchestration
- **Real-time terminal** — Full terminal emulation in the browser via xterm.js with live WebSocket streaming
- **Zero-config networking** — Agents dial in to the server, so workers can be behind NATs and firewalls
- **Single binary per role** — No runtime dependencies. `scp` the binary, add a config file, and run
- **mTLS security** — Agent-to-server communication secured with mutual TLS and a built-in CA

## Architecture

<p align="center">
  <img src="docs/assets/architecture.svg" alt="claude-plane architecture" width="800">
</p>

## Quickstart

Get claude-plane running on a single machine:

```bash
# Build
go build -o claude-plane-server ./cmd/server
go build -o claude-plane-agent ./cmd/agent

# Run (generates certs, configs, admin account, starts everything)
./quickstart.sh
```

The script prints your admin credentials and opens the dashboard at `http://127.0.0.1:8080`. Ctrl+C stops everything.

See the [Quickstart Guide](docs/quickstart.md) for manual setup and configuration options.

## Documentation

| Guide | Description |
|-------|-------------|
| [Quickstart](docs/quickstart.md) | Single-machine setup for evaluation |
| [Server Installation](docs/install-server.md) | Production server deployment |
| [Agent Installation](docs/install-agent.md) | Worker machine agent setup |
| [Configuration Reference](docs/configuration.md) | All config file options |
| [Architecture](docs/architecture.md) | System design and data flows |

## Build from Source

**Prerequisites:** Go 1.25+, Node.js 22+

```bash
# Backend binaries
go build -o claude-plane-server ./cmd/server
go build -o claude-plane-agent ./cmd/agent

# Frontend (output goes to internal/server/frontend/dist/ for embedding)
cd web
npm install
npm run build
cd ..

# Run tests
go test -race ./...
cd web && npm run test -- --run
```

## Project Structure

```
cmd/server/              Server entrypoint (serve, CA tools, seed-admin)
cmd/agent/               Agent entrypoint (run)
internal/server/         Server business logic
  api/                   REST handlers, middleware, router
  auth/                  JWT authentication, token blocklist
  config/                Server TOML config loading
  connmgr/               Agent connection manager
  grpc/                  gRPC server for agent connections
  handler/               Job and run REST handlers
  httputil/              Shared HTTP response helpers
  orchestrator/          Job DAG execution engine
  session/               Session management, WebSocket handlers
  store/                 SQLite data access layer
  frontend/              Embedded frontend assets
internal/agent/          Agent business logic
  config/                Agent TOML config loading
internal/shared/         Shared code (proto, TLS utilities)
proto/                   Protobuf definitions
web/                     React frontend (Vite + TypeScript)
docs/                    Project documentation
```

## Contributing

1. Fork the repository
2. Create a feature branch from `main`
3. Run `go test -race ./...` and `cd web && npm run test -- --run` before submitting
4. Open a pull request — CI will run automatically

## License

MIT
