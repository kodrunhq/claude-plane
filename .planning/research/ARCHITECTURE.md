# Architecture Research

**Domain:** Remote CLI session management control plane
**Researched:** 2026-03-11
**Confidence:** HIGH

## Standard Architecture

### System Overview

```
                         Browser (React + xterm.js)
                                │
                    ┌───────────┴───────────┐
                    │  WebSocket (terminal)  │
                    │  REST/HTTP (control)   │
                    └───────────┬───────────┘
                                │
               ┌────────────────┴────────────────┐
               │       claude-plane-server       │
               │                                  │
               │  ┌────────────┐ ┌─────────────┐  │
               │  │ HTTP/WS    │ │ gRPC Server  │  │
               │  │ Server     │ │ (mTLS:9443)  │  │
               │  │ (:8443)    │ │              │  │
               │  └─────┬──────┘ └──────┬───────┘  │
               │        │               │          │
               │  ┌─────┴───────────────┴───────┐  │
               │  │     Session Router          │  │
               │  │  (WebSocket ↔ gRPC bridge)  │  │
               │  └─────────────┬───────────────┘  │
               │                │                  │
               │  ┌─────────────┴───────────────┐  │
               │  │  Agent Connection Manager   │  │
               │  │  (machine_id → gRPC stream) │  │
               │  └─────────────┬───────────────┘  │
               │                │                  │
               │  ┌─────────┐ ┌┴──────────────┐   │
               │  │ SQLite  │ │Job Orchestrator│   │
               │  │ (WAL)   │ │ (DAG runner)   │   │
               │  └─────────┘ └───────────────┘   │
               └────────────────┬────────────────┘
                                │
              ┌─────────────────┼─────────────────┐
              │                 │                  │
     ┌────────┴────────┐  ┌────┴────────┐  ┌─────┴───────┐
     │ claude-plane-   │  │ claude-plane-│  │ claude-plane-│
     │ agent (nuc-01)  │  │ agent (nuc-02│  │ agent (zima) │
     │                 │  │              │  │              │
     │ ┌─────────────┐ │  │  ...         │  │  ...         │
     │ │Session Mgr  │ │  └──────────────┘  └──────────────┘
     │ │ ┌─────────┐ │ │
     │ │ │PTY + CLI│ │ │
     │ │ │(claude) │ │ │
     │ │ └─────────┘ │ │
     │ │ ┌─────────┐ │ │
     │ │ │Scrollback│ │ │
     │ │ │Writer   │ │ │
     │ │ └─────────┘ │ │
     │ └─────────────┘ │
     └─────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| **HTTP/WS Server** | Serves frontend static assets, REST API for CRUD operations, upgrades WebSocket connections for terminal I/O | Go `net/http` with `chi` router, `gorilla/websocket` for WS upgrade |
| **gRPC Server** | Accepts inbound agent connections over mTLS, manages bidirectional streams for command/event exchange | `google.golang.org/grpc` with TLS credentials from `crypto/tls` |
| **Session Router** | Bridges a browser WebSocket to the correct agent gRPC stream for a given session, handles attach/detach/reconnect | In-memory map: `sessionID -> {agentStream, wsConn}`, goroutine per active relay |
| **Agent Connection Manager** | Tracks which agents are connected, routes commands to specific agents, detects disconnections | In-memory map: `machineID -> gRPC stream`, heartbeat monitoring |
| **Job Orchestrator** | DAG-based execution of multi-step jobs, cron scheduling, cross-job triggers | In-process goroutines, `robfig/cron/v3` for cron parsing |
| **SQLite Store** | Persists sessions, jobs, runs, machines, credentials, audit log | `mattn/go-sqlite3` (CGo) or `modernc.org/sqlite` (pure Go), WAL mode |
| **Agent Session Manager** | Spawns Claude CLI in PTYs, manages session lifecycle, buffers output | `creack/pty` for PTY allocation, goroutine per session |
| **Scrollback Writer** | Continuously writes PTY output to disk in asciicast v2 format regardless of attach state | Append-only file writer, rotation at 50MB |
| **Stream Relay** | When a session is attached, copies PTY output to gRPC stream and gRPC input to PTY stdin | `io.Copy`-style goroutine pair, activated on attach, paused on detach |

## Recommended Project Structure

```
claude-plane/
├── cmd/
│   ├── server/              # Server binary entrypoint
│   │   └── main.go
│   └── agent/               # Agent binary entrypoint
│       └── main.go
├── proto/
│   └── claudeplane/         # Protobuf definitions
│       └── agent.proto
├── internal/
│   ├── server/              # Server-only code
│   │   ├── http/            # REST API handlers, WebSocket upgrade
│   │   │   ├── router.go
│   │   │   ├── session_handler.go
│   │   │   ├── job_handler.go
│   │   │   ├── machine_handler.go
│   │   │   └── ws_terminal.go
│   │   ├── grpc/            # gRPC server, agent stream handler
│   │   │   ├── server.go
│   │   │   └── agent_stream.go
│   │   ├── session/         # Session registry, router (WS <-> gRPC bridge)
│   │   │   ├── registry.go
│   │   │   └── router.go
│   │   ├── agent/           # Agent connection manager
│   │   │   └── manager.go
│   │   ├── job/             # Job orchestrator, DAG runner, cron
│   │   │   ├── orchestrator.go
│   │   │   ├── dag.go
│   │   │   └── scheduler.go
│   │   ├── auth/            # Authentication middleware
│   │   │   └── auth.go
│   │   └── store/           # Database layer
│   │       ├── db.go
│   │       ├── migrations/
│   │       ├── sessions.go
│   │       ├── jobs.go
│   │       └── machines.go
│   ├── agent/               # Agent-only code
│   │   ├── client/          # gRPC client, reconnection logic
│   │   │   └── client.go
│   │   ├── session/         # PTY management, scrollback writing
│   │   │   ├── manager.go
│   │   │   ├── pty.go
│   │   │   └── scrollback.go
│   │   └── health/          # System stats collection
│   │       └── reporter.go
│   └── shared/              # Code shared between server and agent
│       ├── config/          # Config file parsing (TOML)
│       ├── tls/             # mTLS setup, CA tooling
│       └── proto/           # Generated protobuf Go code
├── web/                     # React frontend (Vite project)
│   ├── src/
│   │   ├── components/
│   │   │   └── terminal/    # xterm.js wrapper
│   │   ├── hooks/
│   │   ├── stores/          # Zustand stores
│   │   ├── api/             # REST client, WebSocket manager
│   │   └── pages/
│   └── package.json
└── Makefile
```

### Structure Rationale

- **`cmd/`:** Two separate `main.go` files produce two distinct binaries. Single binary per role is a hard constraint.
- **`internal/`:** All non-entrypoint Go code lives here, enforced by Go's `internal` package visibility rules. Server and agent code are strictly separated -- they share only protobuf types and TLS utilities.
- **`internal/server/` vs `internal/agent/`:** Clean boundary. The server never imports agent code and vice versa. This prevents accidental coupling and keeps each binary lean.
- **`proto/`:** Protobuf definitions are the contract between server and agent. Generated code goes into `internal/shared/proto/`.
- **`web/`:** Frontend is a standalone Vite project. The server embeds its build output via `go:embed` for single-binary deployment.

## Architectural Patterns

### Pattern 1: Agent-Initiated Persistent Bidirectional Stream

**What:** The agent dials the server (never the reverse) and opens a single long-lived gRPC bidirectional stream. All commands (server-to-agent) and events (agent-to-server) flow over this one stream. The stream persists for the lifetime of the agent's connection.

**When to use:** Any agent/worker architecture where workers may be behind NATs or firewalls and cannot accept inbound connections.

**Trade-offs:**
- Pro: NAT/firewall friendly -- only the server needs a public address
- Pro: Single multiplexed connection reduces overhead vs. one stream per session
- Pro: gRPC handles HTTP/2 framing, flow control, and keepalive natively
- Con: Single stream is a single point of failure -- if it dies, all sessions lose their relay (but sessions themselves keep running locally)
- Con: Head-of-line blocking is possible if one session produces massive output (mitigated by chunking)

**Example:**
```protobuf
service AgentService {
  rpc AgentStream(stream AgentEvent) returns (stream ServerCommand);
}
```

```go
// Agent side: persistent connection with reconnection
func (a *Agent) maintainConnection(ctx context.Context) {
    for {
        stream, err := a.client.AgentStream(ctx)
        if err != nil {
            a.backoff.Wait()
            continue
        }
        a.backoff.Reset()
        a.runEventLoop(ctx, stream) // blocks until stream breaks
    }
}
```

### Pattern 2: WebSocket-to-gRPC Bridge (Session Router)

**What:** The server sits between the browser and the agent, bridging two different transport protocols. A browser WebSocket carries raw terminal bytes. The server wraps/unwraps these into gRPC `InputDataCmd` / `SessionOutputEvent` messages and routes them to the correct agent stream.

**When to use:** Any multi-hop terminal streaming path where the browser-facing protocol (WebSocket) differs from the backend protocol (gRPC).

**Trade-offs:**
- Pro: Clean protocol separation -- browser talks WebSocket, agent talks gRPC, neither knows about the other
- Pro: Server can add logic at the bridge point (audit logging, access control, rate limiting)
- Con: Adds latency (extra serialization hop). For terminal I/O this is negligible (~1ms)
- Con: Server becomes a throughput bottleneck if many sessions stream simultaneously

**Example:**
```go
// Server-side: bridge a WebSocket connection to a gRPC agent stream
func (r *SessionRouter) Attach(sessionID string, ws *websocket.Conn) {
    agent := r.agentMgr.GetAgentForSession(sessionID)

    // WS -> gRPC (user input)
    go func() {
        for {
            _, data, err := ws.ReadMessage()
            if err != nil { break }
            agent.Send(&ServerCommand{
                Command: &InputDataCmd{SessionId: sessionID, Data: data},
            })
        }
    }()

    // gRPC -> WS (terminal output)
    outputCh := r.registry.Subscribe(sessionID)
    go func() {
        for chunk := range outputCh {
            ws.WriteMessage(websocket.BinaryMessage, chunk)
        }
    }()
}
```

### Pattern 3: Always-On Scrollback with Attach/Detach

**What:** The PTY output is always being written to a scrollback file on the agent, regardless of whether anyone is watching. When a user attaches, the agent first replays the scrollback (or a tail of it), then switches to live streaming. When the user detaches, live streaming stops but the scrollback writer keeps going.

**When to use:** Any system where sessions must survive client disconnection and support reconnection with output replay.

**Trade-offs:**
- Pro: Sessions are fully independent of viewer state -- close laptop, come back tomorrow, nothing lost
- Pro: Scrollback files serve as a permanent audit trail / recording
- Con: Disk usage on agent machines (mitigated by rotation at 50MB)
- Con: Reconnection replay can be slow for very long sessions (mitigated by sending only recent output or using byte offsets)

**Critical implementation detail:** The scrollback writer goroutine and the stream relay goroutine both read from the same PTY master fd. Use a fan-out pattern: one goroutine reads the PTY and writes to both destinations (scrollback file and, if attached, the gRPC stream channel).

```go
// Agent-side: fan-out from PTY to scrollback + optional live stream
func (s *Session) ptyReadLoop() {
    buf := make([]byte, 32*1024)
    for {
        n, err := s.ptyMaster.Read(buf)
        if err != nil { break }
        chunk := buf[:n]

        // Always write to scrollback
        s.scrollback.Write(chunk)

        // If attached, also send to live stream
        if ch := s.liveChannel.Load(); ch != nil {
            select {
            case ch <- chunk:
            default:
                // Drop if channel full (backpressure)
            }
        }
    }
}
```

### Pattern 4: Flow Control Across the Multi-Hop Path

**What:** Terminal output can burst at high throughput (e.g., `cat` of a large file produces megabytes/second). The path is: PTY -> agent -> gRPC -> server -> WebSocket -> xterm.js. Each hop has different throughput capacity. Without flow control, buffers grow unbounded and the browser tab crashes.

**When to use:** Always, for any production terminal streaming system.

**Trade-offs:**
- Pro: Prevents OOM in browser and unbounded memory growth on server
- Con: Adds complexity to the streaming path
- Con: Flow control pauses can cause PTY to block, which pauses the underlying process (this is actually correct behavior -- it is how real terminals work)

**Implementation (three layers):**

1. **xterm.js -> WebSocket:** Use xterm.js write callback to track pending bytes. When pending exceeds 128KB (high water mark), send a `pause` message over WebSocket. Resume at 16KB (low water mark).

2. **WebSocket -> gRPC:** Server tracks per-session buffer depth on the gRPC side. If the agent is producing faster than the WebSocket can drain, apply backpressure by not reading from the gRPC stream (HTTP/2 flow control propagates this naturally).

3. **gRPC -> PTY:** On the agent, if the gRPC send buffer is full, the fan-out goroutine blocks, which blocks the PTY read, which blocks the underlying process via kernel PTY flow control. This is the correct end-to-end backpressure behavior.

## Data Flow

### Terminal I/O Flow (the critical path)

```
User types in browser
    │
    ▼
xterm.js onData callback
    │
    ▼
WebSocket.send(raw bytes)
    │
    ▼ (:8443 /ws/terminal/:sessionID)
Server HTTP handler
    │
    ▼
Session Router lookup (sessionID → agent stream)
    │
    ▼
gRPC send: InputDataCmd{session_id, data}
    │
    ▼ (:9443 mTLS bidirectional stream)
Agent event loop receives command
    │
    ▼
PTY master fd write (bytes go to Claude CLI stdin)
    │
    ▼
Claude CLI processes input, produces output
    │
    ▼
PTY master fd read (agent reads output)
    │
    ├──► Scrollback file write (always)
    │
    └──► gRPC send: SessionOutputEvent{session_id, data}
              │
              ▼
         Server receives event
              │
              ▼
         Session Router lookup (sessionID → WebSocket conn)
              │
              ▼
         WebSocket.WriteMessage(binary, data)
              │
              ▼
         xterm.js term.write(data) → terminal renders
```

### Session Lifecycle Flow

```
1. User clicks "New Session" in browser
       │
       ▼
2. POST /api/v1/sessions {machine_id, working_dir, ...}
       │
       ▼
3. Server creates session record in DB (status: "starting")
       │
       ▼
4. Server sends CreateSessionCmd to agent via gRPC stream
       │
       ▼
5. Agent allocates PTY, injects env vars, spawns `claude` CLI
       │
       ▼
6. Agent sends SessionStatusEvent{status: "running"} to server
       │
       ▼
7. Server updates DB, notifies frontend via REST response or event
       │
       ▼
8. Frontend opens WebSocket to /ws/terminal/:sessionID
       │
       ▼
9. Server sends AttachSessionCmd to agent
       │
       ▼
10. Agent starts streaming PTY output over gRPC
       │
       ▼
11. Server bridges gRPC output to WebSocket → xterm.js renders
```

### Reconnection Flow

```
1. User closes laptop (WebSocket drops)
       │
       ▼
2. Server detects WS close, sends DetachSessionCmd to agent
       │
       ▼
3. Agent stops streaming live output (scrollback writer continues)
       │
       ▼
       ... time passes, CLI keeps running ...
       │
       ▼
4. User reopens browser, navigates to session
       │
       ▼
5. Frontend opens new WebSocket to /ws/terminal/:sessionID
       │
       ▼
6. Server sends RequestScrollbackCmd to agent
       │
       ▼
7. Agent reads scrollback file, sends ScrollbackChunkEvents
       │
       ▼
8. Server forwards chunks over WebSocket, xterm.js replays them
       │
       ▼
9. Final chunk (is_final=true) arrives
       │
       ▼
10. Server sends AttachSessionCmd, live streaming resumes
```

### Key Data Flows

1. **Control plane (low frequency, high reliability):** Session CRUD, job management, machine status -- REST API over HTTPS. Standard request/response. TanStack Query on the frontend for caching and refetching.

2. **Terminal I/O (high frequency, low latency):** Raw bytes flowing through WebSocket -> server -> gRPC -> PTY and back. Binary messages, no JSON serialization. Latency target: under 50ms end-to-end for keystrokes.

3. **Health reporting (periodic, low priority):** Agent pushes CPU/memory/disk stats every 10 seconds over the gRPC stream. Server stores latest snapshot per machine. Frontend polls via REST or receives via server-sent events.

4. **Scrollback replay (burst, then done):** On reconnect, a burst of historical output flows from agent disk through gRPC through WebSocket to xterm.js. Can be megabytes. Must be chunked and flow-controlled.

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| 1-5 agents, 1-10 sessions | Current design is more than sufficient. Single server process, SQLite, all in-memory routing. |
| 10-50 agents, 50-200 sessions | Still fine. gRPC bidirectional streams are lightweight (~one goroutine pair per agent). WebSocket connections scale to thousands per Go process. SQLite WAL handles concurrent reads. |
| 50+ agents, 200+ concurrent streams | First bottleneck: server CPU from terminal I/O bridging. Consider: separate the HTTP/WS server from the gRPC server into different goroutine pools with isolated scheduling. Or run multiple server instances with a shared database (requires migrating from SQLite to Postgres). |

### Scaling Priorities

1. **First bottleneck: server memory from concurrent terminal streams.** Each active terminal relay holds buffers (~64KB per direction per session). At 200 concurrent streams, that is ~25MB -- negligible. This system will hit operational complexity limits (managing 50+ agents) long before it hits technical scaling limits.

2. **Second bottleneck: SQLite write contention.** Session status updates and audit log writes are the primary write load. WAL mode handles concurrent reads well but writes are serialized. At high job throughput (many concurrent runs creating/updating run_steps), a write-ahead approach with batched updates would help. Migration to Postgres is the escape hatch but is unlikely to be needed for a small team's control plane.

## Anti-Patterns

### Anti-Pattern 1: One gRPC Stream Per Session

**What people do:** Open a new bidirectional gRPC stream for each terminal session on an agent, rather than multiplexing all sessions over one persistent stream.

**Why it is wrong:** Each gRPC stream is a separate HTTP/2 stream, but they all share one TCP connection anyway. Multiple streams add connection management complexity (which stream died? reconnect which ones?), make it harder to maintain a single "agent is connected" status, and create race conditions during reconnection (some streams reconnect before others).

**Do this instead:** One persistent bidirectional stream per agent. Multiplex all sessions over it using `session_id` fields in every message. The agent connection is either UP or DOWN, never partially connected.

### Anti-Pattern 2: Polling for Terminal Output

**What people do:** Frontend polls a REST endpoint for new terminal output instead of using a persistent WebSocket connection.

**Why it is wrong:** Terminal I/O requires sub-100ms latency for responsive keystrokes. Polling at any reasonable interval (100ms, 500ms) feels laggy, wastes bandwidth, and creates jittery output rendering. Additionally, polling creates O(sessions * poll_rate) requests even when sessions are idle.

**Do this instead:** One WebSocket per attached terminal session. Binary messages. Persistent connection. Close when the user navigates away or detaches.

### Anti-Pattern 3: Storing Terminal Output in the Database

**What people do:** Write every terminal output chunk to SQLite/Postgres rows for persistence and replay.

**Why it is wrong:** Terminal output is high-volume, append-only binary data. Databases are optimized for structured, query-able, update-able records. Writing megabytes of raw terminal bytes to SQL rows causes write amplification, bloats the database, and makes replay slow (sequential reads from a flat file are 10-100x faster than scanning database rows).

**Do this instead:** Scrollback files on the agent's local filesystem (asciicast v2 format). The database stores metadata only (session ID, start time, scrollback byte count). Replay reads the file directly and streams it.

### Anti-Pattern 4: No Flow Control on the Streaming Path

**What people do:** Treat the terminal streaming path as unbounded -- read from PTY as fast as possible, push to gRPC, push to WebSocket, write to xterm.js.

**Why it is wrong:** A fast producer (e.g., `cat /dev/urandom | xxd`) can generate 100+ MB/s. xterm.js processes at 5-35 MB/s. Without backpressure, the browser accumulates a growing buffer in memory, eventually crashing the tab with an OOM error.

**Do this instead:** Implement three-layer flow control as described in Pattern 4 above. Use xterm.js write callbacks for the browser layer, HTTP/2 flow control for the gRPC layer, and kernel PTY flow control for the agent layer.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Claude CLI | Agent spawns as child process in PTY | Agent does not import Claude CLI code -- it is a black box process. Communication is purely through PTY stdin/stdout. |
| Anthropic API | Indirectly, via Claude CLI | API key injected as `ANTHROPIC_API_KEY` env var on the PTY process. Server stores encrypted, agent holds in memory only. |
| GitHub | Indirectly, via Claude CLI | `GITHUB_TOKEN` injected same way. Claude CLI uses it for code operations. |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| Frontend <-> Server (control) | REST over HTTPS | Standard JSON API. TanStack Query handles caching, refetching, optimistic updates. |
| Frontend <-> Server (terminal) | WebSocket over HTTPS | Binary messages (raw terminal bytes). One WS per attached session. |
| Server <-> Agent (all) | gRPC bidirectional stream over mTLS | Single persistent stream per agent. Protobuf messages. Agent always initiates. |
| Agent <-> Claude CLI | PTY fd (stdin/stdout/stderr) | No serialization -- raw terminal bytes. Agent treats CLI as opaque process. |
| Server <-> SQLite | `database/sql` | In-process. No network hop. WAL mode for concurrent read access. |

## Build Order (Dependencies Between Components)

The architecture has clear dependency chains that dictate build order:

### Phase 1: Foundation (no dependencies)
Build these first -- everything else depends on them:
1. **Protobuf definitions** -- the contract between server and agent
2. **mTLS / CA tooling** -- both binaries need cert loading
3. **Config file parsing** -- both binaries need configuration
4. **SQLite store + migrations** -- server needs persistence from day one

### Phase 2: Agent Core (depends on Phase 1)
The agent is simpler and more self-contained:
1. **PTY management** -- spawn CLI, read/write PTY fd
2. **Scrollback writer** -- write output to disk
3. **gRPC client + reconnection** -- connect to server, maintain stream
4. **Session manager** -- tie PTY + scrollback + stream relay together

### Phase 3: Server Core (depends on Phase 1)
Can be built in parallel with Phase 2:
1. **gRPC server** -- accept agent connections, manage streams
2. **Agent connection manager** -- track connected agents
3. **Session registry** -- track active sessions, link to agents
4. **REST API** -- CRUD endpoints for sessions, machines

### Phase 4: Terminal Streaming (depends on Phase 2 + 3)
This is the integration point -- it needs both agent and server working:
1. **WebSocket terminal handler** -- upgrade HTTP, manage WS lifecycle
2. **Session router (WS <-> gRPC bridge)** -- the critical bridging logic
3. **Attach/detach/reconnect flow** -- scrollback replay, live switch

### Phase 5: Frontend (depends on Phase 3 + 4)
1. **xterm.js terminal component** -- renders terminal, sends input
2. **Session management UI** -- create, list, attach, kill sessions
3. **Machine status view** -- see connected agents, health

### Phase 6: Job System (depends on Phase 4)
Most complex server component, but not needed for basic terminal access:
1. **Job/step CRUD** -- define jobs and steps
2. **DAG runner** -- dependency resolution, parallel execution
3. **Cron scheduler** -- time-based triggers
4. **Job UI** -- notebook-style interface for building and reviewing jobs

**Key insight:** Terminal streaming (Phase 4) is the architectural crux. It touches every component (frontend WS, server routing, agent PTY). Get this working end-to-end first, even with a minimal UI. The job system is layered on top and can be deferred without blocking the core value proposition.

## Sources

- [gRPC Core Concepts (official)](https://grpc.io/docs/what-is-grpc/core-concepts/)
- [gRPC Go Basics Tutorial (official)](https://grpc.io/docs/languages/go/basics/)
- [gRPC Keepalive Documentation (official)](https://grpc.io/docs/guides/keepalive/)
- [grpc-go keepalive package](https://pkg.go.dev/google.golang.org/grpc/keepalive)
- [xterm.js Flow Control Guide (official)](https://xtermjs.org/docs/guides/flowcontrol/)
- [xterm.js backpressure discussion (GitHub issue #2077)](https://github.com/xtermjs/xterm.js/issues/2077)
- [GoTTY webtty package -- PTY-to-WebSocket bridge pattern](https://pkg.go.dev/github.com/yudai/gotty/webtty)
- [Teleport terminal.go -- WebSocket terminal handler in Go](https://github.com/gravitational/teleport/blob/master/lib/web/terminal.go)
- [gRPC bidirectional streaming example (Go)](https://github.com/pahanini/go-grpc-bidirectional-streaming-example)
- [gRPC connection management best practices (2026)](https://oneuptime.com/blog/post/2026-01-30-grpc-connection-management/view)
- Existing project design documents: `docs/internal/product/backend_v1.md`, `frontend_v1.md`, `suplementary_v1.md`

---
*Architecture research for: Remote CLI session management control plane*
*Researched: 2026-03-11*
