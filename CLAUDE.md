# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

claude-plane is a self-hosted control plane for managing interactive Claude CLI sessions across distributed machines. Three Go binaries + a React frontend:

- **`claude-plane-server`** — Control plane. Serves the frontend, manages sessions, orchestrates jobs, accepts inbound gRPC connections from agents. SQLite storage.
- **`claude-plane-agent`** — Runs on worker machines. Manages Claude CLI processes in PTYs, buffers terminal output, maintains persistent gRPC connection to the server.
- **`claude-plane-bridge`** — Connects external services (GitHub, Telegram, Slack) to the server via its REST API. Polls for events, triggers jobs, and relays notifications. Auto-configured when running via Docker.
- **Frontend** — React 19 + TypeScript SPA embedded via `go:embed` into the server binary. Modes: Command Center (dashboard), single-session terminal view, and Multi-View (2-6 sessions in resizable split panes).

## Product-First Development (CRITICAL)

**Every code change must be validated against the full user workflow it affects.** We have repeatedly introduced bugs by fixing one piece of code in isolation without tracing how it impacts the rest of the system. Before implementing any feature or fix:

1. **Trace the full user story.** Walk through the complete user journey that touches the code you're changing — from the UI action, through the API, to the backend, to the agent, and back. Map every component in the chain.
2. **Check all consumers.** A session status change affects: the session terminal header, the SessionsPage list, the CommandCenter dashboard cards, the MultiView picker, the event stream, and the reaper. If you change how status works, verify ALL of them.
3. **Think about state transitions.** What happens when a session goes from `created` → `running` → `waiting_for_input` → `running` → `completed`? What about `created` → `running` → `errored`? What about a machine disconnecting mid-session? Trace every transition.
4. **Test with real Claude CLI.** The CLI has behaviors that are invisible in unit tests: Ink status bar redraws, periodic tips/updates below the prompt, cursor repositioning escape sequences. Assumptions about terminal output patterns have caused multiple critical bugs.
5. **Frontend views share data but display independently.** When you invalidate a query, use `refetchType: 'all'` to ensure unmounted components (other pages the user navigates to) also get fresh data. Stale caches cause status mismatches between views.

**The cost of tracing a full workflow is 10 minutes. The cost of a bug that reaches production is hours of debugging.**

## Architecture Principles

1. **Agents dial in, server never dials out.** Workers can be behind NATs/firewalls.
2. **Sessions survive disconnection.** CLI sessions keep running; reconnection replays missed output.
3. **Jobs are interactive notebooks, not CI pipelines.** Built through the frontend, not YAML.
4. **Single binary per role.** No runtime dependencies. `scp` the binary and run.

## Build & Run

### Docker (recommended — production & development)

```bash
# Start server + bridge (bridge auto-configures on first run)
docker compose up -d

# The server image includes the bridge binary. On first start,
# docker-entrypoint.sh auto-generates an API key and bridge.toml.
# No manual bridge configuration needed.
```

### Local Development

**Prerequisites:** Go 1.25+, Node.js 22+

```bash
# Build binaries
go build -o claude-plane-server ./cmd/server
go build -o claude-plane-agent ./cmd/agent
go build -o claude-plane-bridge ./cmd/bridge

# Build frontend (required before Go build — server embeds dist/ via go:embed)
cd web && npm install && npm run build && cd ..

# IMPORTANT: If you skip the frontend build, Go build still works if
# internal/server/frontend/dist/.gitkeep exists (CI stubs this).

# Server subcommands
./claude-plane-server serve --config server.toml
./claude-plane-server ca init --out-dir ./ca
./claude-plane-server ca issue-server --ca-dir ./ca --out-dir ./server-cert
./claude-plane-server ca issue-agent --ca-dir ./ca --machine-id "worker-1"
./claude-plane-server seed-admin --email admin@example.com --name Admin
./claude-plane-server create-api-key --name bridge --admin  # for manual bridge setup

# Agent subcommands
./claude-plane-agent run --config agent.toml
./claude-plane-agent join CODE --server https://server:4200 [--insecure] [--service] [--config-dir path]
./claude-plane-agent install-service --config agent.toml [--user username]
./claude-plane-agent uninstall-service [--purge]
```

## Important: Frontend Embedding

The server binary embeds the frontend via `go:embed` from `internal/server/frontend/dist/`. **Source changes in `web/src/` are NOT live until you rebuild:**

```bash
cd web && npm run build && cd ..    # outputs to internal/server/frontend/dist/
go build -o claude-plane-server ./cmd/server  # embeds the new dist/
```

When debugging frontend issues in production mode, verify the bundle hash in the browser Network tab matches the file in `dist/assets/`. For development, use `cd web && npm run dev` (Vite dev server on port 3000 with proxy).

## Gotchas

**Terminal resize:** xterm.js `fitAddon.fit()` fires on `requestAnimationFrame` (before the WebSocket opens). The `term.onResize` callback checks `ws.readyState === WebSocket.OPEN` and silently drops the resize. Always send an **explicit** resize message on `ws.onopen` — never rely on `fit()` triggering `onResize` during connection setup.

**Shell task Command field:** The backend (`ValidateJobSteps` in `orchestrator/dag_runner.go`) requires shell tasks to have a non-empty `Command` field. The `session_executor.go` also aborts if `CommandSnapshot` is empty. The frontend TaskEditor must preserve and submit the `command` field for shell tasks — don't clear it.

**WebSocket attach failure:** When `runSession` in `session/ws.go` fails to attach to the agent, it must publish end markers (`scrollback_end` + `session_ended`) AND close the WebSocket. Do not fall through to the relay loops — the reader loop would repeatedly call `sendToAgent()` against a missing agent.

**Idle detection and CLI noise:** The idle detector (`idle_detector.go`) uses silence-based timing, NOT prompt marker matching. Claude CLI renders its UI with Ink (React for terminals), which sends cursor repositioning escape sequences (`\x1b7`, `\x1b8`, `\x1b[<n>;<m>H`, `\x1b[<n>A/B/C/D`) for status bar redraws even when idle. The `isRepositioningNoise()` classifier filters these out — only sequential text with SGR color codes (`\x1b[...m`) counts as real output. Do NOT attempt to detect idle state by matching prompt characters (❯) — they are persistent TUI elements, not line-delimited prompts.

**Machine connection debounce:** When an agent registers, there's a brief gap between `Register()` and `CommandStream()` that causes a legitimate disconnect/reconnect cycle. The connection manager uses a 5-second grace period (`disconnectGrace`) before publishing `machine.disconnected`. Never publish disconnect events immediately.

**Session status across views:** Session status is displayed in: terminal header, SessionsPage list, CommandCenter cards, MultiView picker, and event stream. All these views use independent TanStack Query caches. When invalidating session queries, always use `refetchType: 'all'` to ensure unmounted views also refresh — otherwise users see stale status when navigating between pages.

**Bridge auto-config in Docker:** The server Docker image includes the bridge binary. `docker-entrypoint.sh` auto-generates an API key via `create-api-key` CLI command and writes `bridge.toml` on first start. Do not require manual bridge configuration for Docker deployments.

## Testing

```bash
# Go — all tests
go test -race ./...

# Go — single package
go test -race ./internal/server/handler -run TestJobHandler_CreateJob

# Go — vet (CI runs this)
go vet ./...

# Frontend — all tests (single pass, no watch)
cd web && npx vitest run

# Frontend — watch mode
cd web && npx vitest

# Frontend — single test
cd web && npx vitest src/components/jobs/JobCard.test.tsx

# Frontend — typecheck
cd web && npx tsc --noEmit

# Frontend — lint
cd web && npm run lint
```

## CI Checks (must pass before merge)

Backend: `go vet ./...` → `go test -race ./...` → build both binaries → validate goreleaser config.
Frontend: `tsc --noEmit` → `eslint` → `vitest --run` → `npm run build` (`tsc -b && vite build`).

**Important:** The CI build step runs `tsc -b` (project references mode), which type-checks **all three** tsconfig projects: `tsconfig.app.json` (source), `tsconfig.node.json` (Vite config), and `tsconfig.test.json` (tests). This is stricter than `tsc --noEmit` alone. Always run `cd web && npm run build` locally before pushing to catch build-only type errors in test files.

**Event types sync check:** CI runs `go generate ./internal/server/event/...` and checks that `event_types.json` is up to date. If you add/modify event types, regenerate before committing.

## Frontend Development

```bash
cd web && npm install && npm run dev    # Vite dev server on port 3000
```

Vite proxies `/api` and `/ws` to `https://localhost:4200` (self-signed cert, `secure: false`). The `/ws` proxy enables WebSocket passthrough. Production builds output to `internal/server/frontend/dist/` for `go:embed`.

## Proto Generation

```bash
buf generate    # uses buf.gen.yaml with remote plugins (not local protoc)
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
| `httputil/` | Shared HTTP response helpers, API key auth context utilities |
| `session/` | Session registry, WebSocket multiplexing for terminal I/O and events |
| `grpc/` | mTLS gRPC server for agent connections |
| `connmgr/` | Connection manager tracking active agents with health monitoring |
| `orchestrator/` | DAG runner for job execution — validates topology, snapshots steps into runs |
| `executor/` | Bridges job orchestration to agent session management |
| `scheduler/` | Cron-based scheduler using `robfig/cron/v3`, publishes schedule trigger events |
| `event/` | In-process pub/sub bus with glob-style pattern matching, WebSocket fanout, webhook delivery with retry |
| `provision/` | Agent provisioning token generation and install script building |
| `agentdl/` | Multi-platform agent binary download endpoints (embedded via `go:embed`) |
| `broker/` | Message broker for inter-component communication |
| `ingest/` | Data ingestion pipeline for agent metrics and token usage |
| `logging/` | Structured logging infrastructure — slog TeeHandler writing to stderr + SQLite (async batch) + WebSocket broadcast |
| `notify/` | Notification delivery (email, Slack, etc.) |
| `reaper/` | Background cleanup — terminates idle sessions, sweeps stale `created` sessions stuck > 5 min |
| `retention/` | Data retention policies and cleanup for old sessions/runs/events |
| `config/` | TOML config parsing |

**Key patterns:**
- Store encapsulates all data access (repository pattern). Each entity has Create/Read/List/Update/Delete methods.
- Handlers receive store, auth service, connection manager, claims getter via constructor injection.
- Narrow interfaces for behavior: `Publisher`, `StepExecutor`, `TokenRevoker`.
- Logging: `log/slog` (structured).

## Bridge Architecture (`internal/bridge/`)

| Package | Purpose |
|---------|---------|
| `bridge.go` | Lifecycle orchestration, HTTP health endpoint, restart signal polling |
| `client/` | REST API client for communicating with the server |
| `config/` | TOML config loading |
| `connector/` | Connector interface + implementations (GitHub, Telegram) |
| `state/` | State management for connector sync |

Connectors implement a common interface. Each polls an external service, maps events to job triggers, and relays via the server's REST API. In Docker deployments, the bridge binary is embedded in the server image and auto-configured by `docker-entrypoint.sh` — no manual setup required.

## Agent Architecture (`internal/agent/`)

- `session.go` / `session_manager.go` — PTY-backed process management using `creack/pty`, scrollback file storage, output buffering
- `client.go` — gRPC client with automatic reconnection and backoff
- `backoff.go` — Exponential backoff logic for reconnection
- `health.go` — Health check reporting
- `idle_detector.go` — Silence-based idle detection with noise classifier. Uses `isRepositioningNoise()` to filter Ink cursor-positioning escape sequences from real Claude output. Configurable silence timeout, minimum activity bytes threshold.
- `scrollback.go` — Scrollback buffer persistence
- `directory.go` — Working directory resolution and validation
- `join.go` — Agent provisioning join flow (exchanges join code for mTLS certs)
- `log_sink.go` — Structured log forwarding to server via gRPC
- `config/` — Agent TOML config loading
- `lifecycle/` — Agent lifecycle utilities: PID file, process scanning, orphan reaping, service detection

## Frontend TypeScript Configuration

The frontend uses **project references** (`web/tsconfig.json` → `tsconfig.app.json`, `tsconfig.node.json`, `tsconfig.test.json`):
- `tsconfig.app.json` — Source code (`src/`), excludes `src/__tests__` and `src/test`. Uses `types: ["vite/client"]`.
- `tsconfig.test.json` — Test files (`src/__tests__`, `src/test`). Uses `types: ["vitest/globals", "vite/client", "node"]`.
- `tsconfig.node.json` — Vite config files.

When using `vi.fn()` in tests, use the Vitest 3.x single-type-parameter form: `vi.fn<(arg: Type) => ReturnType>()` — not the legacy `vi.fn<[Args], Return>()`.

## Frontend Architecture (`web/src/`)

| Directory | Purpose |
|-----------|---------|
| `api/` | HTTP client (`/api/v1` base), per-domain API functions |
| `stores/` | Zustand stores: auth, jobs, runs, logs, UI state, multiview (workspace persistence via localStorage) |
| `hooks/` | TanStack Query hooks for data fetching (~29 hooks); `useTerminalSession()` for xterm.js + WebSocket (supports optional WebGL toggle); `useEventStream()` for multiplexed event WS with exponential backoff and `refetchType: 'all'` |
| `types/` | TypeScript interfaces for all domain entities |
| `components/` | Feature-organized: layout, jobs, runs, terminal, sessions, multiview, webhooks, triggers, events, admin, credentials, dag, shared, apikeys, connectors, docs, logs, machines, provisioning, templates, settings, dashboard |
| `views/` | Page-level route components |
| `lib/` | Utility functions |

**Notable frontend libraries:** `@xyflow/react` + `@dagrejs/dagre` for DAG visualization, `cron-parser` + `cronstrue` for cron display, `xterm.js` with WebGL addon for terminal rendering, `react-resizable-panels` for multi-view split panes.

**Routing (React Router 7):** Protected redirect to LoginPage. `/users` requires admin role (`AdminRoute` guard). Routes: `/` (CommandCenter), `/sessions`, `/sessions/:sessionId`, `/multiview`, `/multiview/:workspaceId`, `/machines`, `/jobs`, `/jobs/new`, `/jobs/:id`, `/templates`, `/templates/new`, `/templates/:id/edit`, `/runs`, `/runs/:id`, `/webhooks`, `/webhooks/:id/deliveries`, `/triggers`, `/schedules`, `/events`, `/logs`, `/users` (admin), `/provisioning`, `/credentials`, `/api-keys`, `/connectors`, `/connectors/:connectorId`, `/search`, `/settings`, `/docs`, `/docs/:guideId`.

**WebSocket patterns:**
- Terminal WS (`/ws/terminal/{sessionID}`): binary data, real-time terminal I/O, scrollback replay on connect
- Events WS (`/ws/events`): JSON with `type`/`payload`, triggers React Query cache invalidation
- Both use cookie-based auth (httpOnly `session_token`)
- Multi-View opens one terminal WS per pane (up to 6 concurrent), with WebGL fallback to canvas at 5+ panes

## Security Model

- Agent↔Server: mTLS with built-in CA tooling. Agent certificate CN must be `agent-{machine-id}` (gRPC interceptor strips prefix to derive machine ID).
- Frontend↔Server: JWT via httpOnly cookies. Login page gates all routes.
- Credentials stored with optional encryption vault (requires encryption key in server config).
- Webhooks: HMAC signature verification.

## Data Model (SQLite)

Core tables: `machines`, `sessions`, `jobs`, `steps`, `step_dependencies`, `runs`, `run_steps`, `run_step_values`, `users`, `api_keys`, `credentials`, `webhooks`, `webhook_deliveries`, `events`, `cron_schedules`, `job_triggers`, `session_templates`, `injections`, `bridge_connectors`, `bridge_control`, `provisioning_tokens`, `revoked_tokens`, `audit_log`, `user_preferences`, `notification_channels`, `notification_subscriptions`, `server_settings`

## Release Process

goreleaser (`.goreleaser.yml`) handles releases on `v*` tags. Pre-hooks build the frontend and cross-compile agent binaries (`scripts/build-agent-binaries.sh` → `internal/server/agentdl/binaries/`). Builds inject version info via ldflags into `internal/shared/buildinfo/`.

## Documentation

- `docs/architecture.md` — System overview, communication protocols, component architecture
- `docs/configuration.md` — Server, agent, and bridge configuration reference
- `docs/quickstart.md` — Quick start guide
- `docs/install-server.md` / `docs/install-agent.md` — Installation guides

## Tech Stack Quick Reference

- **Go module:** `github.com/kodrunhq/claude-plane` (Go 1.25)
- **Backend:** Go, Chi router, gRPC, SQLite (`modernc.org/sqlite`), mTLS, `log/slog`
- **Frontend:** React 19, TypeScript (strict), Vite, xterm.js, Zustand, TanStack Query, Tailwind CSS, lucide-react
- **Release:** goreleaser (builds agent binaries for linux/darwin amd64/arm64)
