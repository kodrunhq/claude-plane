# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

claude-plane is a self-hosted control plane for managing interactive Claude CLI sessions across distributed machines. It consists of two Go binaries and a React frontend:

- **`claude-plane-server`** — Control plane. Serves the frontend, manages sessions, orchestrates jobs, accepts inbound gRPC connections from agents. Uses SQLite for storage.
- **`claude-plane-agent`** — Runs on worker machines. Manages Claude CLI processes in PTYs, buffers terminal output, maintains persistent gRPC connection to the server.
- **Frontend** — React 19 + TypeScript SPA served by the server binary (embedded via `go:embed`). Two modes: Command Center (dashboard) and Workbench (IDE-like terminal view).

## Architecture Principles

1. **Agents dial in, server never dials out.** Workers can be behind NATs/firewalls.
2. **Sessions survive disconnection.** CLI sessions keep running; reconnection replays missed output.
3. **Jobs are interactive notebooks, not CI pipelines.** Built through the frontend, not YAML.
4. **Single binary per role.** No runtime dependencies. `scp` the binary and run.

## Tech Stack

- **Backend:** Go, gRPC (agent↔server), REST+WebSocket (frontend↔server), SQLite (via `modernc.org/sqlite`), mTLS for agent auth
- **Frontend:** React 19, TypeScript, Vite, xterm.js (terminal), Zustand (state), TanStack Query (data fetching), Tailwind CSS
- **Protocol:** Agent↔Server uses gRPC with bidirectional streaming. Frontend↔Server uses REST for CRUD + WebSocket for real-time terminal I/O and events.

## Key Communication Patterns

- **Terminal data flow:** Browser ↔ WebSocket ↔ Server ↔ gRPC stream ↔ Agent ↔ PTY ↔ Claude CLI
- **Agent connection:** Agents register via `Register()` RPC, then maintain a `CommandStream()` bidirectional stream for receiving commands and sending events
- **Frontend real-time:** Single multiplexed WebSocket per browser tab, messages tagged with `session_id` for routing

## Security Model

- Agent↔Server: mTLS with built-in CA tooling (`claude-plane-server ca init/issue-server/issue-agent`)
- Frontend↔Server: JWT via httpOnly cookies (login page gating all routes), OIDC/OAuth2 planned for V2
- Agent certificate CN must be `agent-{machine-id}` (gRPC interceptor strips the `agent-` prefix to derive the logical machine ID)

## Data Model (SQLite)

Core tables: `machines`, `sessions`, `jobs`, `job_steps`, `job_runs`, `job_step_results`, `token_usage`, `model_pricing`

## Design Documents

Detailed architecture specs live in `docs/internal/product/`:
- `backend_v1.md` — Server/agent architecture, gRPC protocol, data model, security
- `frontend_v1.md` — UI architecture, component hierarchy, WebSocket protocol, state management
- `suplementary_v1.md` — Workspace isolation, arena mode, observability, cost tracking, phased roadmap

**Always consult these docs before making architectural decisions.**

## Build & Run

```bash
# Build
go build -o claude-plane-server ./cmd/server
go build -o claude-plane-agent ./cmd/agent

# Quickstart (generates certs, configs, admin, starts everything)
./quickstart.sh

# Tests
go test -race ./...
cd web && npx vitest run

# Frontend dev (inside web/ directory)
npm install
npm run dev          # Vite dev server
npm run build        # Production build (output embedded into server binary)
npm run lint         # ESLint

# Proto generation
buf generate         # or protoc with go-grpc plugin
```

## Project Structure

```
cmd/server/          # Server entrypoint (serve, CA tools, seed-admin)
cmd/agent/           # Agent entrypoint (run)
internal/server/     # Server business logic
internal/agent/      # Agent business logic
internal/shared/     # Shared code (proto, TLS utilities)
proto/               # .proto definitions
web/                 # React frontend (Vite project)
quickstart.sh        # One-command local setup
```
