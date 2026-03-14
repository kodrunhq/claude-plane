# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

claude-plane is a self-hosted control plane for managing interactive Claude CLI sessions across distributed machines. Two Go binaries + a React frontend:

- **`claude-plane-server`** — Control plane. Serves the frontend, manages sessions, orchestrates jobs, accepts inbound gRPC connections from agents. SQLite storage.
- **`claude-plane-agent`** — Runs on worker machines. Manages Claude CLI processes in PTYs, buffers terminal output, maintains persistent gRPC connection to the server.
- **Frontend** — React 19 + TypeScript SPA embedded via `go:embed` into the server binary. Two modes: Command Center (dashboard) and Workbench (IDE-like terminal view).

## Architecture Principles

1. **Agents dial in, server never dials out.** Workers can be behind NATs/firewalls.
2. **Sessions survive disconnection.** CLI sessions keep running; reconnection replays missed output.
3. **Jobs are interactive notebooks, not CI pipelines.** Built through the frontend, not YAML.
4. **Single binary per role.** No runtime dependencies. `scp` the binary and run.

## Build & Run

```bash
# Build binaries
go build -o claude-plane-server ./cmd/server
go build -o claude-plane-agent ./cmd/agent

# Quickstart (generates certs, configs, admin user, starts server+agent)
./quickstart.sh admin@localhost mypassword

# Server subcommands
./claude-plane-server serve --config server.toml
./claude-plane-server ca init --out-dir ./ca
./claude-plane-server ca issue-server --ca-dir ./ca --out-dir ./server-cert
./claude-plane-server ca issue-agent --ca-dir ./ca --machine-id "worker-1"
./claude-plane-server seed-admin --email admin@example.com --name Admin
```

## Testing

```bash
# Go — all tests
go test -race ./...

# Go — single test
go test -race ./internal/server/handler -run TestJobHandler_CreateJob

# Frontend — all tests
cd web && npx vitest run

# Frontend — single test
cd web && npx vitest src/components/jobs/JobCard.test.tsx

# Frontend — lint
cd web && npm run lint
```

## Frontend Development

```bash
cd web && npm install && npm run dev    # Vite dev server on port 3000
```

Vite proxies `/api` and `/ws` to `https://localhost:8443` (the server). Production builds output to `internal/server/frontend/dist/` for `go:embed`.

## Proto Generation

```bash
buf generate    # or protoc with go-grpc plugin
```

Single proto file: `proto/claudeplane/v1/agent.proto`. Defines `Register()` and `CommandStream()` (bidirectional streaming). Generated stubs land in `internal/shared/proto/`.

## Key Communication Patterns

- **Terminal data flow:** Browser ↔ WebSocket ↔ Server ↔ gRPC stream ↔ Agent ↔ PTY ↔ Claude CLI
- **Agent connection:** Agents register via `Register()` RPC, then maintain a `CommandStream()` bidirectional stream for receiving commands and sending events
- **Frontend real-time:** Single multiplexed WebSocket per browser tab, messages tagged with `session_id` for routing

## Backend Architecture (`internal/server/`)

| Package | Purpose |
|---------|---------|
| `api/` | Chi router setup, middleware chain (JWT, rate limiting, security headers, max bytes) |
| `auth/` | JWT (HS256), Argon2id password hashing, token blocklist/revocation |
| `store/` | SQLite via `database/sql` with parametrized queries. Separate writer (1 conn) and reader (4 conn) pools for WAL mode. Migrations in `store/migrations.go` |
| `handler/` | REST handlers by domain: jobs, runs, sessions, events, webhooks, triggers, schedules, users, credentials, provisioning, ingest |
| `session/` | Session registry, WebSocket multiplexing for terminal I/O and events |
| `grpc/` | mTLS gRPC server for agent connections |
| `connmgr/` | Connection manager tracking active agents with health monitoring |
| `orchestrator/` | DAG runner for job execution — validates topology, snapshots steps into runs |
| `executor/` | Bridges job orchestration to agent session management |
| `scheduler/` | Cron-based scheduler using `robfig/cron/v3`, publishes schedule trigger events |
| `event/` | In-process pub/sub bus with glob-style pattern matching, WebSocket fanout, webhook delivery with retry |
| `provision/` | Agent provisioning token generation and install script building |
| `agentdl/` | Multi-platform agent binary download endpoints (embedded via `go:embed`) |
| `config/` | TOML config parsing |

**Key patterns:**
- Store encapsulates all data access (repository pattern). Each entity has Create/Read/List/Update/Delete methods.
- Handlers receive store, auth service, connection manager, claims getter via constructor injection.
- Narrow interfaces for behavior: `Publisher`, `StepExecutor`, `TokenRevoker`.
- Logging: `log/slog` (structured).

## Agent Architecture (`internal/agent/`)

- `session.go` / `session_manager.go` — PTY-backed process management using `creack/pty`, scrollback file storage, output buffering
- `client.go` — gRPC client with automatic reconnection and backoff
- `idle_detector.go` — Session idle tracking

## Frontend Architecture (`web/src/`)

| Directory | Purpose |
|-----------|---------|
| `api/` | HTTP client (`/api/v1` base), per-domain API functions |
| `stores/` | Zustand stores: auth, jobs, runs, UI state |
| `hooks/` | TanStack Query hooks for data fetching; `useTerminalSession()` for xterm.js + WebSocket; `useEventStream()` for multiplexed event WS with exponential backoff |
| `types/` | TypeScript interfaces for all domain entities |
| `components/` | Feature-organized: layout, jobs, runs, terminal, sessions, webhooks, triggers, events, admin, credentials, dag, shared |
| `views/` | Page-level route components |
| `lib/` | Utility functions |

**Routing (React Router 7):** Protected redirect to LoginPage. Routes: `/` (CommandCenter), `/sessions`, `/machines`, `/jobs`, `/runs`, `/webhooks`, `/events`, `/users`, `/provisioning`, `/credentials`.

**WebSocket patterns:**
- Terminal WS (`/ws/terminal/{sessionID}`): binary data, real-time terminal I/O, scrollback replay on connect
- Events WS (`/ws/events`): JSON with `type`/`payload`, triggers React Query cache invalidation
- Both use cookie-based auth (httpOnly `session_token`)

## Security Model

- Agent↔Server: mTLS with built-in CA tooling. Agent certificate CN must be `agent-{machine-id}` (gRPC interceptor strips prefix to derive machine ID).
- Frontend↔Server: JWT via httpOnly cookies. Login page gates all routes.
- Credentials stored with optional encryption vault (requires encryption key in server config).
- Webhooks: HMAC signature verification.

## Data Model (SQLite)

Core tables: `machines`, `sessions`, `jobs`, `job_steps`, `job_runs`, `job_step_results`, `token_usage`, `model_pricing`

## Design Documents

Detailed architecture specs live in `docs/internal/product/`:
- `backend_v1.md` — Server/agent architecture, gRPC protocol, data model, security
- `frontend_v1.md` — UI architecture, component hierarchy, WebSocket protocol, state management
- `suplementary_v1.md` — Workspace isolation, arena mode, observability, cost tracking, phased roadmap

**Always consult these docs before making architectural decisions.**

## Tech Stack Quick Reference

- **Go module:** `github.com/kodrunhq/claude-plane` (Go 1.25)
- **Backend:** Go, Chi router, gRPC, SQLite (`modernc.org/sqlite`), mTLS, `log/slog`
- **Frontend:** React 19, TypeScript (strict), Vite, xterm.js, Zustand, TanStack Query, Tailwind CSS, lucide-react
- **Release:** goreleaser (builds agent binaries for linux/darwin amd64/arm64, cross-compile scripts in `scripts/`)
