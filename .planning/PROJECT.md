# claude-plane

## What This Is

A self-hosted control plane for managing interactive Claude CLI sessions across distributed machines. A small team opens a browser, sees their fleet of worker machines, creates and attaches to Claude CLI sessions, and runs multi-step jobs as interactive notebooks — all without SSH-ing into anything. Go backend (server + agent binaries), React frontend.

## Core Value

A developer can open the browser, connect to a Claude CLI session running on any remote machine, and interact with it as if they were sitting at that terminal — with sessions that survive disconnection.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Remote terminal access to Claude CLI sessions through the browser
- [ ] Session lifecycle: create, attach, detach, terminate sessions on any connected machine
- [ ] Session persistence: sessions survive browser disconnection, reconnection replays missed output
- [ ] Per-user accounts with authentication (each team member has their own login)
- [ ] Job system: multi-step interactive notebooks with step creation, execution, rerun, and output review
- [ ] Agent binary that runs on worker machines, manages Claude CLI processes in PTYs, connects back to server
- [ ] Server binary that serves frontend, manages sessions/jobs, accepts agent connections
- [ ] mTLS-based agent authentication with built-in CA tooling
- [ ] Real-time terminal I/O streaming (browser ↔ WebSocket ↔ server ↔ gRPC ↔ agent ↔ PTY)

### Out of Scope

- Machine health dashboard (Command Center) — defer to v2, v1 focuses on sessions and jobs
- Workspace isolation (per-session git worktrees) — defer to v2
- Arena mode (parallel Claude sessions competing on same task) — defer to v3+
- Cost tracking / token analytics — defer to v2
- OIDC/OAuth2 — defer to v2, v1 uses simpler per-user auth
- Mobile-optimized UI — defer, desktop-first

## Context

- Team is currently SSH-ing into machines to use Claude CLI — painful to manage, no session persistence, no visibility into what's running where
- Detailed architecture docs exist in `docs/internal/product/` covering backend, frontend, and supplementary systems — these are the full vision, v1 is a focused subset
- Backend architecture: two Go binaries (server + agent), gRPC for agent↔server communication, REST + WebSocket for frontend↔server, SQLite for storage
- Frontend architecture: React 18 + TypeScript, Vite, xterm.js for terminal emulation, Zustand for state, TanStack Query for data fetching
- Agent dials in to server (never the reverse) — workers can be behind NATs/firewalls
- Single binary per role, no runtime dependencies

## Constraints

- **Tech stack**: Go backend, React frontend — as specified in architecture docs
- **Deployment**: Self-hosted, single binary per role, no external dependencies beyond SQLite
- **Security**: mTLS for agent↔server communication is non-negotiable
- **Architecture**: Agents dial in, server never dials out (NAT/firewall friendly)
- **Protocol**: gRPC for agent comms, WebSocket for real-time terminal streaming to browser

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go for backend | Single binary deployment, strong concurrency, gRPC ecosystem | — Pending |
| React + xterm.js for frontend | Standard ecosystem, xterm.js is the terminal emulator | — Pending |
| SQLite over Postgres | Zero-dependency, sufficient for control plane workload, embedded | — Pending |
| mTLS over bearer tokens | Mutual auth, no secrets to rotate, battle-tested | — Pending |
| Agents dial in (not server dial out) | NAT/firewall friendly, server is only reachable address | — Pending |
| Per-user auth for v1 (not Basic Auth) | Small team needs to see who's doing what | — Pending |
| Full job system in v1 | Core workflow: team runs multi-step tasks, not just single sessions | — Pending |

---
*Last updated: 2026-03-11 after initialization*
