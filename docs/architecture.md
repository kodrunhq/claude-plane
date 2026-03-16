# Architecture

## System Overview

claude-plane consists of four components:

<p align="center">
  <img src="assets/architecture.svg" alt="claude-plane architecture" width="900">
</p>

**Key principle: agents dial in, server never dials out.** Workers can be behind NATs, firewalls, or in private networks.

## Communication Protocols

### Browser to Server

- **REST API** — CRUD operations for sessions, machines, jobs, runs. JWT Bearer token auth.
- **WebSocket (terminal)** — Per-session bidirectional stream for terminal I/O. Binary frames carry PTY data, text frames carry control messages (resize, auth).
- **WebSocket (events)** — Multiplexed event stream for real-time UI updates (machine status, session changes).

### Server to Agent (gRPC)

Defined in `proto/claudeplane/v1/agent.proto`:

- **`Register` RPC** — Agent calls once on startup to identify itself (machine ID, capabilities, existing sessions).
- **`CommandStream` RPC** — Persistent bidirectional stream. The server sends commands, the agent sends events.

**Server-to-agent commands:**
| Command | Purpose |
|---------|---------|
| `CreateSession` | Start a new Claude CLI process in a PTY |
| `AttachSession` | Subscribe to live output from an existing session |
| `DetachSession` | Stop receiving output (session keeps running) |
| `KillSession` | Terminate a session |
| `ResizeTerminal` | Update PTY dimensions |
| `InputData` | Send keystrokes to the PTY |
| `RequestScrollback` | Request buffered output replay |

**Agent-to-server events:**
| Event | Purpose |
|-------|---------|
| `SessionOutput` | Terminal output data from PTY |
| `SessionStatus` | Session state changes (running, stopped) |
| `SessionExit` | Session terminated with exit code |
| `Health` | Periodic resource usage report |
| `ScrollbackChunk` | Buffered output replay data |

## Terminal Data Flow

The full path for a keystroke from browser to Claude CLI and back:

```
Browser (keypress)
  → WebSocket (binary frame)
    → Server (session router)
      → gRPC CommandStream (InputDataCmd)
        → Agent (session manager)
          → PTY (write)
            → Claude CLI (processes input)
            → PTY (output)
          → Agent (scrollback buffer + event)
        → gRPC CommandStream (SessionOutputEvent)
      → Server (session registry)
    → WebSocket (binary frame)
  → Browser (xterm.js renders)
```

## Job Execution Flow

Jobs are DAGs of steps. Each step runs a Claude CLI session on a specified machine.

```
User creates job (REST API)
  → Defines steps with dependencies
  → Triggers a run

Run starts:
  → DAG engine resolves ready steps (in-degree = 0)
  → For each ready step:
    → Create session on target machine
    → Wait for completion
    → On success: mark complete, check dependents
    → On failure: apply failure policy (fail_run, skip_dependents, continue)
  → Repeat until all steps complete or run fails/cancels
```

## Security Model

### Agent Authentication (mTLS)

- Server runs a built-in CA (`claude-plane-server ca init`)
- Each agent gets a unique certificate with its machine ID as the CN
- The gRPC listener requires client certificates signed by the CA
- Agent identity is extracted from the certificate — no passwords or tokens

### Browser Authentication (JWT)

- Users authenticate via email/password to get a JWT
- JWT includes user ID and role (admin or regular user)
- All REST API calls require a Bearer token
- WebSocket connections authenticate via first-message auth:
  1. Browser connects without credentials
  2. Sends `{"type":"auth","token":"<jwt>"}` as first message
  3. Server validates within 5-second timeout
  4. Query parameter auth (`?token=<jwt>`) supported for backwards compatibility

### Authorization

- Admin users can access all resources
- Regular users can only access their own sessions, jobs, and runs
- Session access is verified against the session's owner before allowing terminal connections

## Data Model

SQLite database with these core tables:

| Table | Purpose |
|-------|---------|
| `users` | User accounts (email, password hash, role) |
| `machines` | Registered agent machines (ID, status, resources) |
| `sessions` | CLI sessions (owner, machine, status, timestamps) |
| `jobs` | Job definitions (name, description, owner) |
| `job_steps` | Steps within a job (command, machine, dependencies) |
| `step_dependencies` | DAG edges between steps |
| `job_runs` | Run instances of a job (status, trigger type) |
| `run_steps` | Per-step results within a run (status, output, timing) |
| `token_blocklist` | Revoked JWT tokens (for logout) |

## Frontend Architecture

React 19 SPA built with Vite and TypeScript:

- **State management:** Zustand for client-side state (auth, UI, multiview workspaces), TanStack Query for server data
- **Terminal:** xterm.js with WebGL renderer (canvas fallback for 5+ panes) for full terminal emulation
- **Multi-View:** 2-6 terminal sessions in resizable split-pane layouts via `react-resizable-panels`, with saved workspace persistence in localStorage
- **Routing:** React Router with these views:
  - Command Center — Dashboard overview
  - Sessions — List and manage CLI sessions, multi-select for Multi-View
  - Multi-View — Simultaneous terminal sessions with configurable grid layouts
  - Machines — View connected agents
  - Jobs — Create and edit job DAGs
  - Run Detail — Monitor job execution with live DAG visualization
- **Real-time updates:** WebSocket event stream for live UI refresh
- **DAG visualization:** React Flow for interactive job step graph editing
