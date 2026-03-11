# Project Research Summary

**Project:** claude-plane
**Domain:** Remote CLI session management control plane (AI agent orchestration)
**Researched:** 2026-03-11
**Confidence:** HIGH

## Executive Summary

claude-plane is a remote terminal control plane — two Go binaries (server + agent) that let teams manage persistent Claude CLI sessions across multiple machines via a React web UI. The architecture is a well-understood pattern: agent-initiated persistent gRPC bidirectional streams (NAT/firewall-friendly, agents dial in), WebSocket bridging to xterm.js on the frontend, and SQLite as the embedded datastore. The project differentiates itself from generic web terminals (ttyd, GoTTY, Guacamole) through two unique features: session persistence across disconnection (no competitor does this) and an interactive job notebook system (no competitor in this class offers structured multi-step task execution). Both the stack and architecture have HIGH confidence — they are grounded in prior internal design documents and validated against mature open-source reference implementations (Teleport, Coder).

The recommended approach is Go 1.23+ for both binaries (pure-Go SQLite driver for single-binary cross-compilation, grpc-go for agent comms, coder/websocket for browser-facing WebSocket), React 19 + Vite 7 + xterm.js 6 for the frontend. The architecture has six natural build phases derived from component dependency chains: foundation (shared contracts and persistence), agent core (PTY + scrollback), server core (gRPC + session management), terminal streaming (the WebSocket-gRPC bridge), frontend UI, and job system. The job system is deliberately last — it layers cleanly on top of working session infrastructure and can be deferred without blocking the core value proposition.

The highest-risk technical challenges are: correct PTY lifecycle management (fd leaks and zombie processes), gRPC bidirectional stream goroutine management on reconnect, end-to-end terminal I/O backpressure (xterm.js can drown without explicit flow control), and the scrollback replay handover race condition. All four must be addressed as correctness requirements during their respective foundational phases — not deferred as "optimizations." SQLite write contention and mTLS certificate expiry are additional operational pitfalls with clear prevention strategies. Every one of these risks has verified root causes with documented fixes from the grpc-go and xterm.js issue trackers.

## Key Findings

### Recommended Stack

The backend stack is anchored by Go 1.23+ for both binaries. `modernc.org/sqlite` (pure Go, no CGO) is the correct SQLite driver because it enables `CGO_ENABLED=0` cross-compilation — a hard project constraint. The agent-server communication uses `google.golang.org/grpc` v1.79+ for its native bidirectional streaming over mTLS; ConnectRPC was considered and rejected because its browser-compatibility advantage is irrelevant for agent-to-server comms. `coder/websocket` handles browser-facing WebSocket (context-aware, binary messages, integrates with `net/http`/chi). `creack/pty` is the unambiguous standard for PTY allocation. The frontend is React 19 + Vite 7 + TypeScript strict, with xterm.js v6 (30% smaller bundle than v5, scoped `@xterm/*` packages only), Zustand 5 for UI state, and TanStack Query v5 for REST server state.

**Core technologies:**
- Go 1.23+ with `go-chi/chi` v5: HTTP/WS server — lightweight, idiomatic, `net/http`-native
- `google.golang.org/grpc` v1.79: agent-server protocol — native bidi streaming + mTLS
- `coder/websocket` v2: browser terminal transport — context-aware, binary-safe, chi-compatible
- `creack/pty` v1.1: PTY allocation — the Go standard, used by every terminal project
- `modernc.org/sqlite`: embedded persistence — pure Go, CGO-free, WAL mode required
- `golang-jwt/jwt` v5 + `golang.org/x/crypto/argon2`: authentication — JWT sessions, Argon2id password hashing
- `buf` CLI + `protoc-gen-go-grpc`: protobuf tooling — faster than raw protoc, managed mode
- React 19 + Vite 7 + TypeScript 5.7: frontend foundation — current stable versions, greenfield advantage
- `@xterm/xterm` v6 + addons: terminal emulation — the only production-grade option; use scoped packages
- Tailwind CSS v4: styling — CSS-native config, zero runtime, 5x faster builds than v3
- TanStack Query v5 + Zustand 5: data fetching + UI state — clear separation, minimal boilerplate

### Expected Features

The research confirms a sharp divide between table stakes (what users assume exists) and the two genuine differentiators that justify building this over reaching for ttyd or Teleport.

**Must have (table stakes):**
- Browser terminal via xterm.js + WebSocket — this is the baseline; missing it means no product
- Session persistence across disconnection — the primary pain point being solved; highest technical complexity
- Session lifecycle management (create/attach/detach/terminate) — expected by every web terminal user
- mTLS agent authentication — no negotiation; agents on untrusted networks without cert auth is a non-starter
- Agent auto-reconnection — network blips are inevitable; sessions must not die on transient drops
- Machine/agent registry — users must see which machines are available and their status
- Per-user authentication — even small teams need identity for audit and access
- Terminal resize handling — broken resize rendering is UX-disqualifying
- Real-time I/O streaming — sub-100ms keystroke latency or users will SSH instead
- Session dashboard — cross-fleet session visibility is table stakes once you have >1 machine

**Should have (differentiators):**
- Session output buffering with replay — missed output on reconnect is the killer feature gap vs. all competitors
- Job system with multi-step interactive notebooks — unique; no competitor in this class has this
- NAT/firewall-friendly architecture — agents dial in, zero inbound port requirements; significant ops advantage
- Single binary deployment — `scp + chmod + run`; beats every alternative (Teleport, Guacamole, JumpServer)
- Multi-machine session visibility — cross-fleet view in one dashboard; Teleport has node lists but not managed-session view

**Defer (v2+):**
- OIDC/OAuth2 — SSO is enterprise scope; per-user auth is sufficient for a small team
- RBAC / fine-grained permissions — premature; hard to model correctly, expensive to change
- Full session recording/audit replay — storage-heavy, compliance-scope; log to files on agent for v1
- Workspace isolation (git worktrees) — already designed, defer to v2
- Arena mode, cost tracking, webhook integrations — v3+ territory

### Architecture Approach

The system has a clear three-tier topology: browser (React + xterm.js) communicates with the server via REST over HTTPS for control operations and WebSocket over HTTPS for terminal I/O; the server bridges those WebSocket connections to per-agent gRPC bidirectional streams over mTLS on a separate port; each agent runs Claude CLI processes in PTYs, writes output always to scrollback files regardless of viewer state, and streams live output only when a session is attached. Four architectural patterns are load-bearing: (1) one persistent bidi gRPC stream per agent multiplexed by session ID, (2) the WebSocket-to-gRPC bridge (Session Router) as the server's central routing concern, (3) always-on scrollback with attach/detach fan-out at the PTY read loop, and (4) three-layer flow control across the entire streaming path.

**Major components:**
1. HTTP/WS Server (`internal/server/http`) — REST API for CRUD, WebSocket upgrade for terminal I/O, serves embedded React SPA
2. gRPC Server (`internal/server/grpc`) — accepts agent connections over mTLS, manages bidirectional streams
3. Session Router (`internal/server/session`) — the critical bridge: maps session IDs to WebSocket connections and agent gRPC streams
4. Agent Connection Manager (`internal/server/agent`) — tracks `machineID → gRPC stream`, detects disconnections
5. Agent Session Manager (`internal/agent/session`) — spawns Claude CLI in PTYs, manages scrollback writer and stream relay
6. Job Orchestrator (`internal/server/job`) — DAG runner for multi-step notebook execution
7. SQLite Store (`internal/server/store`) — persists sessions, jobs, machines, users; WAL mode, single writer connection

### Critical Pitfalls

1. **PTY file descriptor and zombie process leaks** — Every `pty.Start()` must be paired with a supervisor goroutine that owns the full lifecycle (close master fd + `cmd.Wait()` on every exit path, including error paths). Design session ownership semantics before writing any PTY code. Monitor `/proc/self/fd` in agent health reports.

2. **gRPC bidi stream goroutine leaks on disconnect** — The send/recv goroutine pair on the agent must be coordinated via a shared context. Configure gRPC keepalive on both sides (`Time: 10s, Timeout: 5s`). Always call `CloseSend()`. Validate with `goleak` integration tests that simulate network drops.

3. **Terminal I/O backpressure failure** — xterm.js has a 50MB input buffer; burst output (e.g., Claude CLI verbose mode) will fill it and crash the tab. Implement three-layer flow control: xterm.js write callbacks with high/low watermarks (500KB/100KB), server-side per-session pending byte tracking, and kernel PTY flow control propagated via gRPC HTTP/2 flow control. This is a correctness requirement, not an optimization.

4. **Scrollback replay race condition** — Use offset-based handover: when the agent receives `RequestScrollbackCmd`, it records the current scrollback file byte offset, sends data up to that offset, then begins live streaming from that offset. The scrollback writer and live streamer share a mutex-protected offset counter. Build this protocol into the protobuf definitions from the start.

5. **SQLite "database is locked" under concurrent load** — Use a single writer connection pattern (`MaxOpenConns=1` for the write pool, separate read pool). Set `busy_timeout=5000` via the DSN string so every connection in the pool gets it. Use `BEGIN IMMEDIATE` for write transactions. This must be correct from day one; retrofitting is painful.

## Implications for Roadmap

Based on the dependency chains in ARCHITECTURE.md and the phase warnings in PITFALLS.md, a 6-phase structure emerges naturally.

### Phase 1: Foundation
**Rationale:** Protobuf definitions are the server-agent contract and block everything. CA tooling and mTLS setup must exist before any agent can connect. SQLite with the correct connection configuration must be in place before any server logic is built. Config parsing is needed by both binaries.
**Delivers:** Two compilable binaries, shared protobuf contract, CA tooling for cert generation, SQLite schema with migrations, config file parsing, no user-visible features yet.
**Addresses:** Machine/agent registry (schema), per-user auth (schema + hashing)
**Avoids:** SQLite concurrency pitfall (single writer + WAL from day one), mTLS cert expiry pitfall (long validity defaults + expiry tracking in DB from day one)

### Phase 2: Agent Core
**Rationale:** The agent is simpler and more self-contained than the server. Building PTY management and scrollback writing before the server-side streaming components means these can be tested in isolation. The scrollback format (asciicast v2 with offset tracking) must be established before the reconnect protocol is designed.
**Delivers:** Agent binary that spawns Claude CLI in PTYs, maintains scrollback files, connects to server via gRPC with reconnection backoff.
**Uses:** `creack/pty`, scrollback writer goroutine, gRPC client with keepalive
**Avoids:** PTY fd/zombie leak pitfall (supervisor goroutine pattern established here), gRPC goroutine leak pitfall (keepalive + context coordination from the start)

### Phase 3: Server Core
**Rationale:** Can be built in parallel with Phase 2. The server needs to accept agent connections, track their state, and expose a REST API for session and machine management before the terminal streaming layer can be built on top.
**Delivers:** Server binary that accepts mTLS agent connections, tracks connected agents, exposes REST CRUD endpoints for sessions/machines, returns data from SQLite.
**Implements:** gRPC server, Agent Connection Manager, Session Registry, REST API handlers
**Avoids:** WebSocket auth pitfall (REST auth middleware established here, will extend to WS)

### Phase 4: Terminal Streaming (the architectural crux)
**Rationale:** This phase integrates all prior work. It is the highest-risk phase because it spans all components and implements the most complex patterns (WS-gRPC bridge, attach/detach/reconnect, scrollback replay). Getting this working end-to-end — even with a minimal UI — is the project's central milestone.
**Delivers:** Working browser terminal: connect to server, attach to an agent session, type commands, see output, close browser, reopen, see missed output.
**Implements:** WebSocket terminal handler, Session Router (WS-gRPC bridge), attach/detach/reconnect flow, scrollback replay, terminal resize propagation
**Avoids:** Backpressure pitfall (three-layer flow control required before "done"), scrollback replay race condition (offset-based handover), WebSocket lifecycle pitfall (ping/pong + write deadline), resize propagation pitfall (debounce + size-on-reconnect)

### Phase 5: Frontend UI
**Rationale:** Once the backend terminal streaming is functional, the frontend can be built against a real API. Building UI against a working backend avoids building mock infrastructure.
**Delivers:** Full React SPA: session dashboard, machine status view, xterm.js terminal component with connection state indicator, session lifecycle controls (create/attach/detach/kill).
**Uses:** React 19, Vite 7, xterm.js v6 with WebGL addon, TanStack Query v5, Zustand 5, Tailwind CSS v4, react-router v7
**Implements:** xterm.js wrapper component with flow control callbacks, WebSocket manager with reconnect and input queuing, REST client via TanStack Query

### Phase 6: Job System
**Rationale:** The job system is the product's primary differentiator but requires working session infrastructure to function. Sessions are the execution primitive for job steps. Building this last ensures the foundation is solid.
**Delivers:** Interactive job notebook UI, job/step CRUD API, DAG runner, job step execution via sessions, step rerun capability.
**Implements:** Job Orchestrator with DAG resolution, Job API handlers, notebook-style frontend with step execution and output display

### Phase Ordering Rationale

- Protobuf + CA tooling first because they are shared contracts — changing them later forces regeneration across both binaries.
- Agent before server streaming because PTY lifecycle correctness (fd leaks, zombie processes) is harder to retrofit than server-side logic.
- Server REST API before WebSocket terminal because authentication middleware, session model, and routing must exist before the WS handler can use them.
- Terminal streaming before frontend because building UI against a working backend avoids building mock layers.
- Job system last because it is the highest-complexity component and requires all prior infrastructure; deferring it does not block the core value proposition (persistent sessions).

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 4 (Terminal Streaming):** The scrollback replay offset protocol and three-layer flow control are novel enough that implementation details may surface edge cases not covered in research. Plan for a spike or dedicated research task on the reconnect state machine before committing to an implementation.
- **Phase 6 (Job System):** DAG execution semantics, step dependency resolution, and the notebook UX pattern have multiple valid approaches. Research ReactFlow for the DAG visualization component before committing to a UI implementation approach.

Phases with standard patterns (skip research-phase):
- **Phase 1 (Foundation):** Go module setup, protobuf + buf tooling, SQLite WAL configuration, and mTLS CA generation are all thoroughly documented with official guides.
- **Phase 2 (Agent Core):** `creack/pty` patterns are well-documented; gRPC client reconnection with exponential backoff is a solved problem with grpc-go examples.
- **Phase 3 (Server Core):** chi REST API patterns and gRPC server setup are standard.
- **Phase 5 (Frontend UI):** React 19 + Vite 7 + xterm.js v6 integration has documentation and working examples (ttyd, Teleport Connect are reference implementations).

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All technology choices verified against official release pages and changelogs as of March 2026. Version numbers are current stable releases. The modernc.org/sqlite vs mattn/go-sqlite3 tradeoff is well-documented with benchmarks. |
| Features | HIGH | Competitor analysis grounded in official documentation for Teleport, Guacamole, ttyd, Cockpit, and JumpServer. Feature prioritization aligned with existing internal design documents. |
| Architecture | HIGH | Core patterns (agent-initiated gRPC bidi stream, WS-gRPC bridge, always-on scrollback) are validated against Teleport's open-source implementation and GoTTY's webtty package. Build order derived from concrete dependency analysis. |
| Pitfalls | HIGH | Every critical pitfall has a verified root cause (grpc-go issue tracker, xterm.js official flow control guide, SQLite concurrent write analysis). Recovery strategies are concrete, not speculative. |

**Overall confidence:** HIGH

### Gaps to Address

- **Scrollback format finalization:** The research recommends asciicast v2 format with a byte-offset index, but the exact encoding of the offset counter in the protobuf protocol needs to be specified during Phase 4 planning.
- **Agent state reconciliation on reconnect:** PITFALLS.md notes that the agent should report existing sessions in `RegisterRequest.existing_sessions` on reconnect. The exact schema for this handshake is not yet defined in the protobuf contract — needs to be addressed in Phase 1 protobuf definitions.
- **Job DAG visualization:** FEATURES.md mentions ReactFlow as the preferred library for the job notebook DAG view. This should be validated (bundle size, API fit) before Phase 6 planning begins.
- **Credential encryption key management:** ARCHITECTURE.md notes API keys are encrypted at rest with AES-256-GCM. The key derivation strategy (from server master key, from per-user key, from config) is not specified and needs a decision before Phase 3.

## Sources

### Primary (HIGH confidence)
- [grpc-go v1.79 releases](https://github.com/grpc/grpc-go/releases) — gRPC framework, bidi streaming patterns
- [grpc-go issue #6457](https://github.com/grpc/grpc-go/issues/6457) — goroutine leak root cause (CloseSend)
- [xterm.js Flow Control guide (official)](https://xtermjs.org/docs/guides/flowcontrol/) — write callback API, watermark strategy
- [xterm.js v6 release](https://github.com/xtermjs/xterm.js/releases) — 30% bundle reduction, scoped packages
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — pure Go driver, CGO-free
- [coder/websocket](https://github.com/coder/websocket) — nhooyr/websocket successor, Coder-maintained
- [Tailwind CSS v4](https://tailwindcss.com/blog/tailwindcss-v4) — CSS-native config rewrite
- [React 19.2](https://react.dev/blog/2025/10/01/react-19-2) — stable since Dec 2024
- [Vite 7](https://vite.dev/blog/announcing-vite7) — current stable build tool
- [React Router v7.13](https://reactrouter.com/changelog) — March 2026 stable
- [Argon2 OWASP recommendation](https://guptadeepak.com/the-complete-guide-to-password-hashing-argon2-vs-bcrypt-vs-scrypt-vs-pbkdf2-2026/) — Argon2id as 2025+ standard
- [SQLite concurrent writes analysis](https://tenthousandmeters.com/blog/sqlite-concurrent-writes-and-database-is-locked-errors/) — WAL + single writer pattern
- [Teleport terminal.go](https://github.com/gravitational/teleport/blob/master/lib/web/terminal.go) — WS terminal handler reference
- [GoTTY webtty package](https://pkg.go.dev/github.com/yudai/gotty/webtty) — PTY-to-WS bridge pattern
- Existing project design documents: `docs/internal/product/backend_v1.md`, `frontend_v1.md`, `suplementary_v1.md`

### Secondary (MEDIUM confidence)
- [reconnecting-websocket v4.4.0](https://github.com/pladaria/reconnecting-websocket) — stable but last published 2019; used for non-terminal WS connections
- [Teleport Features](https://goteleport.com/features/) — competitor feature analysis
- [Apache Guacamole admin docs](https://guacamole.apache.org/doc/gug/administration.html) — session management comparison
- [JumpServer session tracking (DeepWiki)](https://deepwiki.com/jumpserver/jumpserver/10.2-session-tracking-and-replay) — audit replay architecture reference

---
*Research completed: 2026-03-11*
*Ready for roadmap: yes*
