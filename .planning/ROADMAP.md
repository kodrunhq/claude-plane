# Roadmap: claude-plane

## Overview

claude-plane delivers a self-hosted control plane for managing interactive Claude CLI sessions across distributed machines. The build progresses from shared infrastructure (protobuf contracts, mTLS, SQLite) through agent and server binaries, to the architectural crux of end-to-end terminal streaming, then wraps it in a full React SPA and adds the job notebook system. Each phase delivers a verifiable capability that unblocks the next.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Foundation** - Compilable binaries, shared contracts, CA tooling, SQLite persistence, config parsing
- [ ] **Phase 2: Agent Core** - Agent binary connects to server via mTLS, spawns PTYs, maintains scrollback
- [ ] **Phase 3: Server Core** - Server accepts agents, exposes REST API, handles user authentication
- [ ] **Phase 4: Terminal Streaming** - End-to-end terminal I/O from browser through server to agent PTY
- [ ] **Phase 5: Frontend** - Full React SPA with session dashboard, machine status, and polished UX
- [ ] **Phase 6: Job System** - Multi-step interactive notebook execution on top of session infrastructure

## Phase Details

### Phase 1: Foundation
**Goal**: Both binaries compile and share a protobuf contract; CA tooling can generate certs for mTLS; SQLite schema is initialized with correct concurrency settings; both binaries parse TOML config files
**Depends on**: Nothing (first phase)
**Requirements**: INFR-01, INFR-02, INFR-03, INFR-04, AGNT-01, AUTH-04
**Success Criteria** (what must be TRUE):
  1. Running `go build` produces a server binary and an agent binary with no external runtime dependencies
  2. A CLI command generates a root CA, server certificate, and agent certificate that can be used for mTLS
  3. Server starts, initializes a SQLite database with WAL mode, and creates all schema tables
  4. Both binaries read configuration from a TOML file and fail with clear errors on missing required fields
  5. An admin account can be seeded via server CLI command on first run
**Plans**: 3 plans

Plans:
- [ ] 01-01-PLAN.md — Go module scaffold, protobuf contract, binary entrypoints
- [ ] 01-02-PLAN.md — CA tooling (mTLS certs) + TOML config parsing
- [ ] 01-03-PLAN.md — SQLite store (WAL mode, full schema) + admin seeding

### Phase 2: Agent Core
**Goal**: Agent binary authenticates to server using its mTLS certificate and maintains a persistent bidirectional gRPC stream with reconnection on failure
**Depends on**: Phase 1
**Requirements**: AGNT-02, AGNT-03
**Success Criteria** (what must be TRUE):
  1. Agent presents its mTLS certificate to the server and is accepted or rejected based on CA trust chain
  2. Agent establishes a persistent bidirectional gRPC stream to the server that stays alive across idle periods
  3. Agent automatically reconnects with exponential backoff when the server connection drops
  4. Agent can spawn a Claude CLI process in a PTY and relay its output to the gRPC stream
**Plans**: 2 plans

Plans:
- [ ] 02-01-PLAN.md — Server gRPC listener (mTLS) + Agent client (dial, register, stream, reconnect)
- [ ] 02-02-PLAN.md — PTY session lifecycle + session manager with command dispatch

### Phase 3: Server Core
**Goal**: Server accepts agent connections over mTLS, tracks connected machines, and exposes authenticated REST API endpoints for user and session management
**Depends on**: Phase 1
**Requirements**: AUTH-01, AUTH-02, AUTH-03, AGNT-04
**Success Criteria** (what must be TRUE):
  1. User can create an account with email and password, log in, and receive a JWT token
  2. User can log out and their JWT is invalidated
  3. Server accepts incoming mTLS agent connections and tracks them as online
  4. Server displays a list of connected agents with online/offline status via REST API
  5. Unauthenticated API requests are rejected with 401
**Plans**: 3 plans

Plans:
- [ ] 03-01-PLAN.md — JWT auth service, token blocklist, revoked token store
- [ ] 03-02-PLAN.md — Machine store CRUD, connection manager with DB-backed status
- [ ] 03-03-PLAN.md — Chi REST API: router, JWT middleware, auth + machine handlers

### Phase 4: Terminal Streaming
**Goal**: A user in the browser can create a session on a remote machine, see real-time terminal output, type commands, and have the session persist when the browser disconnects
**Depends on**: Phase 2, Phase 3
**Requirements**: SESS-01, SESS-02, SESS-03, SESS-05, SESS-06, TERM-01, TERM-02, TERM-03, TERM-04
**Success Criteria** (what must be TRUE):
  1. User can create a new Claude CLI session on any connected machine and see terminal output in real time via xterm.js
  2. User can type into the browser terminal and keystrokes reach the remote CLI process
  3. User can detach from a session and reattach later, with the session still running on the agent
  4. User can terminate a session and the underlying PTY process is cleaned up
  5. Resizing the browser terminal window updates the remote PTY dimensions so output reflows correctly
**Plans**: 3 plans

Plans:
- [ ] 04-01-PLAN.md — Agent scrollback persistence + session attach/detach lifecycle
- [ ] 04-02-PLAN.md — Server session registry, REST handlers, WebSocket terminal bridge
- [ ] 04-03-PLAN.md — Frontend xterm.js terminal component + WebSocket hook

### Phase 5: Frontend
**Goal**: Users interact with claude-plane through a polished React SPA that provides cross-fleet session visibility, machine status, and streamlined session lifecycle controls
**Depends on**: Phase 4
**Requirements**: SESS-04
**Success Criteria** (what must be TRUE):
  1. User can view a dashboard listing all active sessions across all connected machines with their status
  2. User can see which machines are online/offline and navigate to create or attach sessions from the machine list
  3. Session lifecycle actions (create, attach, detach, terminate) are accessible through the UI without using the API directly
  4. The frontend is embedded in the server binary and served as a single-page application
**Plans**: 4 plans

Plans:
- [ ] 05-00-PLAN.md — Test infrastructure (Vitest config, stub test files)
- [ ] 05-01-PLAN.md — Project config, Vite + Tailwind v4 theming, routing scaffold
- [ ] 05-02-PLAN.md — API clients, shared types, Zustand store, app shell layout, shared components
- [ ] 05-03-PLAN.md — Command Center dashboard, Sessions list, Machines view, event stream, lifecycle actions
- [ ] 05-04-PLAN.md — Go embed SPA handler for serving frontend from server binary

### Phase 6: Job System
**Goal**: Users can create and execute multi-step jobs as interactive notebooks, with ordered steps that run on sessions and support rerun
**Depends on**: Phase 4
**Requirements**: JOBS-01, JOBS-02, JOBS-03, JOBS-04
**Success Criteria** (what must be TRUE):
  1. User can create a job with multiple ordered steps through the UI
  2. User can execute individual steps and see their output in real time
  3. User can rerun a previously completed step and see updated output
  4. Steps with dependencies wait for prerequisite steps to complete before executing
**Plans**: 3 plans

Plans:
- [ ] 06-01-PLAN.md — Job store (DB CRUD) + DAGRunner (execution engine) + Orchestrator (run lifecycle)
- [ ] 06-02-PLAN.md — REST handlers for jobs, steps, runs (full API surface)
- [ ] 06-03-PLAN.md — Frontend: API client, DAG canvas (ReactFlow), job editor, run detail with live status

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5 -> 6
Note: Phases 2 and 3 can execute in parallel (both depend only on Phase 1).

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation | 0/3 | Planning complete | - |
| 2. Agent Core | 0/2 | Planning complete | - |
| 3. Server Core | 0/3 | Planning complete | - |
| 4. Terminal Streaming | 0/3 | Planning complete | - |
| 5. Frontend | 0/3 | Planning complete | - |
| 6. Job System | 0/3 | Planning complete | - |
