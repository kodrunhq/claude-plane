# claude-plane: Backend Architecture Design Document

**Version:** 0.1.0-draft
**Author:** José / Claude (Opus)
**Date:** 2026-03-11

---

## 1. System Overview

claude-plane is a self-hosted control plane for managing interactive Claude CLI sessions across distributed machines. It consists of two Go binaries:

- **`claude-plane-server`** — The control plane. Serves the frontend, manages sessions, orchestrates jobs, and accepts inbound connections from agents.
- **`claude-plane-agent`** — Runs on each worker machine. Manages Claude CLI processes in PTYs, buffers terminal output, and maintains a persistent connection back to the server.

### Design Principles

1. **Agents dial in, server never dials out.** Workers can be behind NATs, firewalls, corporate networks. The server is the only component that needs a reachable address.
2. **Sessions survive disconnection.** If the user closes their laptop, every CLI session keeps running. Reconnection replays what was missed.
3. **Jobs are interactive notebooks, not CI pipelines.** Users build and inspect jobs through the frontend, not YAML files.
4. **Single binary per role.** No runtime dependencies, no interpreters, no package managers. `scp` the binary, run it.

---

## 2. Security Architecture

### 2.1 Mutual TLS (mTLS) — Agent ↔ Server

All communication between agents and the server uses mTLS over gRPC. This provides:

- **Authentication in both directions.** Server verifies the agent is legitimate; agent verifies it's talking to the real server.
- **Encryption in transit.** All gRPC traffic is TLS-encrypted.
- **No passwords, no tokens to rotate.** Certificate-based identity.

#### Certificate Hierarchy

```
claude-plane CA (self-signed root)
├── Server certificate (CN=claude-plane-server)
│   └── Used by the server's gRPC listener
└── Agent certificates (CN=agent-<machine-id>)
    └── One per worker machine
```

#### Certificate Generation (built into the CLI)

```bash
# First time: generate the CA (do this once, store securely)
claude-plane-server ca init --out /etc/claude-plane/ca/

# Generate server cert
claude-plane-server ca issue-server \
  --ca-dir /etc/claude-plane/ca/ \
  --san "controlplane.local,10.0.1.50" \
  --out /etc/claude-plane/server-cert/

# Generate agent cert (one per worker)
claude-plane-server ca issue-agent \
  --ca-dir /etc/claude-plane/ca/ \
  --machine-id "nuc-01" \
  --out ./nuc-01-cert/
```

You `scp` the agent cert bundle (`ca.pem`, `agent.pem`, `agent-key.pem`) to the worker along with the agent binary. That's the entire provisioning step.

#### Why mTLS over alternatives

| Approach | Pros | Cons |
|----------|------|------|
| **mTLS** | Mutual auth, no secrets to rotate, battle-tested | Cert management overhead (mitigated by built-in CA tooling) |
| Bearer tokens | Simple to implement | Tokens can leak, need rotation, no server verification |
| SSH keys | Familiar | Requires server to dial out (breaks principle #1) |
| WireGuard/Tailscale | Great for networking | Adds infrastructure dependency, overkill for app-level auth |

mTLS is the right call. It's what Kubernetes, Istio, and every serious service mesh uses for internal comms.

### 2.2 Frontend ↔ Server Authentication

The frontend (browser) connects to the server over HTTPS (standard TLS, not mTLS — browsers don't do client certs well).

#### Auth Options (pick based on deployment)

**Personal/homelab deployment:**
- HTTP Basic Auth over TLS, or a simple session cookie with a password.
- Simplest possible. One user, one password, done.

**Multi-user deployment:**
- OIDC / OAuth2 (e.g., GitHub, Google, Keycloak).
- Server issues a JWT on login, frontend sends it as `Authorization: Bearer <token>` on every request and WebSocket upgrade.
- JWT contains `user_id`, `display_name`, `roles`.

**For V1:** Start with Basic Auth + TLS. Add OIDC in V2 when multi-user matters.

### 2.3 Agent Identity & Machine Authorization

Each agent cert embeds a `machine-id` in the certificate's Common Name (CN) or a custom SAN field. When an agent connects, the server:

1. Validates the cert chain against the CA.
2. Extracts the `machine-id` from the cert.
3. Checks it against a server-side allowlist (config file or DB).
4. If not in the allowlist → reject connection.
5. If valid → register the agent in the connection manager.

This means a stolen agent cert is useless unless the machine-id is also in the server's allowlist. Revocation is instant: remove from allowlist, agent gets rejected on next reconnect.

### 2.4 Credential Injection for Sessions

When a user spawns a Claude CLI session on a remote machine, the server tells the agent to inject environment variables before exec:

```
ANTHROPIC_API_KEY=sk-ant-...
GITHUB_TOKEN=ghp_...
GIT_AUTHOR_NAME=José
GIT_AUTHOR_EMAIL=jose@example.com
```

These credentials are:
- Stored **encrypted at rest** in the server's database (AES-256-GCM, key from server config).
- Transmitted **over the mTLS channel** to the agent (already encrypted in transit).
- Set as **environment variables** on the PTY process (not written to disk on the agent).
- **Never logged.** Agent must explicitly exclude env vars from any debug output.

The agent holds these in memory for the lifetime of the process, then discards them. If the agent restarts, it re-fetches from the server on next session creation.

---

## 3. Agent Binary (`claude-plane-agent`)

### 3.1 Responsibilities

1. **Connect to the server** on startup and maintain a persistent gRPC bidirectional stream.
2. **Spawn and manage Claude CLI processes** in isolated PTYs.
3. **Buffer all terminal output** to disk (scrollback files).
4. **Stream I/O** to the server in real-time when a user is attached.
5. **Report health** (CPU, memory, running session count, disk space).
6. **Clean up** terminated sessions (archive scrollback, release resources).

### 3.2 Process Model

```
claude-plane-agent (PID 1 of its context)
├── gRPC client goroutine (persistent connection to server)
├── Health reporter goroutine (periodic stats)
├── Session manager
│   ├── Session "sess-abc" 
│   │   ├── PTY master fd
│   │   ├── Claude CLI process (PID 12345)
│   │   ├── Scrollback writer goroutine (PTY → file)
│   │   └── Stream relay goroutine (PTY ↔ gRPC, when attached)
│   ├── Session "sess-def"
│   │   └── ...
│   └── ...
└── Signal handler (graceful shutdown)
```

### 3.3 Session Lifecycle on the Agent

```
                           ┌─────────────────┐
     CreateSession RPC     │                 │
    ──────────────────────►│    STARTING     │
                           │                 │
                           └────────┬────────┘
                                    │ PTY allocated, CLI spawned
                                    ▼
                           ┌─────────────────┐
                           │                 │
                      ┌────│    RUNNING       │────┐
                      │    │                 │    │
                      │    └────────┬────────┘    │
                 User │             │              │ User
              attaches│             │ CLI exits    │ detaches
                      │             │              │
                      ▼             ▼              ▼
               ┌────────────┐ ┌──────────┐ ┌────────────┐
               │  ATTACHED  │ │COMPLETED │ │  DETACHED  │
               │ (streaming)│ │          │ │(buffering) │
               └──────┬─────┘ └──────────┘ └─────┬──────┘
                      │                           │
                      └───────────────────────────┘
                        (user disconnect/reconnect)
```

**Key states:**
- **STARTING:** PTY being allocated, env vars injected, CLI being spawned.
- **RUNNING/DETACHED:** CLI is running, output goes to scrollback file only. No user is watching.
- **RUNNING/ATTACHED:** CLI is running, output goes to both scrollback file AND is streamed via gRPC to the server (and from there to the user's browser).
- **COMPLETED:** CLI process exited (exit code 0 or otherwise). Scrollback file is finalized.

**The scrollback file is ALWAYS being written, regardless of attachment state.** This is what enables the "close your laptop, come back tomorrow" behavior.

### 3.4 Scrollback Format

Each session produces a scrollback file at: `<agent-data-dir>/sessions/<session-id>/scrollback.cast`

Format: [asciicast v2](https://docs.asciinema.org/manual/asciicast/v2/) — a newline-delimited JSON format with timestamps:

```jsonl
{"version": 2, "width": 120, "height": 40, "timestamp": 1710180000}
[0.000000, "o", "$ claude\r\n"]
[0.523000, "o", "Welcome to Claude CLI...\r\n"]
[1.100000, "o", "\u001b[32m❯\u001b[0m "]
[5.200000, "i", "analyze this codebase\r"]
[5.201000, "o", "analyze this codebase\r\n"]
[6.800000, "o", "I'll start by examining...\r\n"]
```

- `"o"` = output (from CLI to terminal)
- `"i"` = input (from user to CLI)
- Timestamps are seconds since session start (float64)

**Why asciicast v2:**
- Human-readable (JSON lines, easy to debug)
- Ecosystem support (asciinema player can render them natively)
- Compact enough (gzip if storage matters)
- Captures timing for faithful replay

**Scrollback rotation:** For very long sessions (hours/days), rotate files at a configurable size (default 50MB). Index file maps time ranges to scrollback chunks.

### 3.5 Agent Configuration

```toml
# /etc/claude-plane/agent.toml

[server]
address = "controlplane.example.com:9443"
# Agent keeps trying to reconnect with exponential backoff
reconnect_min_interval = "1s"
reconnect_max_interval = "60s"

[tls]
ca_cert = "/etc/claude-plane/certs/ca.pem"
agent_cert = "/etc/claude-plane/certs/agent.pem"
agent_key = "/etc/claude-plane/certs/agent-key.pem"

[agent]
machine_id = "nuc-01"            # Must match the cert CN and server allowlist
data_dir = "/var/lib/claude-plane"  # Scrollback files, session state
max_sessions = 5                 # Max concurrent Claude CLI sessions on this machine
claude_cli_path = "/usr/local/bin/claude"  # Path to the Claude CLI binary

[health]
report_interval = "10s"          # How often to push health stats to server
```

### 3.6 Agent gRPC Client Behavior

On startup:
1. Load certs, dial the server with mTLS.
2. Call `Register()` RPC — sends machine-id, capabilities (max sessions, available resources).
3. Open a **bidirectional stream** (`AgentStream()`) — this is the persistent connection.
4. Enter event loop: receive commands from server (create session, attach, detach, kill), send events back (session started, output chunk, session completed, health stats).

On disconnect:
1. All running sessions **continue running** (they're local processes, unaffected by gRPC state).
2. Scrollback files keep being written.
3. Agent retries connection with exponential backoff (1s, 2s, 4s, ... 60s cap).
4. On reconnect: re-register, report current session states, resume streaming for any attached sessions.

---

## 4. Server Binary (`claude-plane-server`)

### 4.1 Responsibilities

1. **Serve the frontend** (static files — React build — or reverse proxy to dev server).
2. **Accept agent connections** over gRPC with mTLS.
3. **Manage user sessions** — route terminal I/O between browser WebSockets and agent gRPC streams.
4. **Run the job orchestrator** — DAG execution, cron scheduling, dependency resolution.
5. **Persist state** — sessions, jobs, runs, user config — in embedded database.
6. **Expose a REST API** for the frontend (session list, job management, machine status, etc.).

### 4.2 Internal Architecture

```
claude-plane-server
│
├── HTTP Server (net/http or chi router)
│   ├── /api/v1/sessions/*      — REST: list, get, create, kill sessions
│   ├── /api/v1/jobs/*          — REST: CRUD jobs and steps
│   ├── /api/v1/runs/*          — REST: list runs, get run status
│   ├── /api/v1/machines/*      — REST: list machines, health
│   ├── /api/v1/credentials/*   — REST: manage user credentials
│   ├── /ws/terminal/:sessionID — WebSocket: terminal I/O
│   └── /* (static)             — Frontend assets
│
├── gRPC Server (mTLS, port 9443)
│   ├── AgentService.Register()
│   ├── AgentService.AgentStream() — bidirectional persistent stream
│   └── Interceptor: extract machine-id from cert, validate allowlist
│
├── Agent Connection Manager
│   ├── Tracks connected agents (machine-id → gRPC stream)
│   ├── Detects disconnections, marks machines unhealthy
│   ├── Routes commands to specific agents
│   └── Aggregates health stats
│
├── Session Registry
│   ├── In-memory map of active sessions
│   ├── Backed by DB for persistence across server restarts
│   └── Links: session ↔ agent ↔ user ↔ WebSocket ↔ job step (if applicable)
│
├── Job Orchestrator
│   ├── DAG Runner (dependency resolution)
│   ├── Cron Scheduler (time-based triggers)
│   ├── Run Manager (tracks execution state per run)
│   └── Step Executor (translates steps into session creation commands)
│
└── Database (SQLite via mattn/go-sqlite3, or embedded Postgres via embedded-postgres-go)
    └── Tables: machines, sessions, jobs, steps, runs, run_steps, credentials, audit_log
```

### 4.3 Server Configuration

```toml
# /etc/claude-plane/server.toml

[http]
listen = "0.0.0.0:8443"
tls_cert = "/etc/claude-plane/server-cert/server.pem"
tls_key = "/etc/claude-plane/server-cert/server-key.pem"

[grpc]
listen = "0.0.0.0:9443"

[tls]
ca_cert = "/etc/claude-plane/ca/ca.pem"
server_cert = "/etc/claude-plane/server-cert/server.pem"
server_key = "/etc/claude-plane/server-cert/server-key.pem"

[auth]
# V1: basic auth
mode = "basic"
username = "jose"
password_hash = "$argon2id$..."  # argon2id hash of password

# V2: OIDC
# mode = "oidc"
# issuer = "https://github.com/login/oauth"
# client_id = "..."
# client_secret_encrypted = "..."

[database]
path = "/var/lib/claude-plane/server.db"
# SQLite WAL mode for concurrent reads

[encryption]
# AES-256-GCM key for encrypting credentials at rest
# Generate: claude-plane-server generate-key
master_key_file = "/etc/claude-plane/master.key"

[machines]
# Allowlist of machine-ids that can connect as agents
allowed = ["nuc-01", "nuc-02", "zimaboard-01"]
```

---

## 5. gRPC Protocol Definition

### 5.1 Service Definition

```protobuf
syntax = "proto3";
package claudeplane;

import "google/protobuf/timestamp.proto";

service AgentService {
  // Agent calls this once on startup to register itself
  rpc Register(RegisterRequest) returns (RegisterResponse);

  // Persistent bidirectional stream — the main communication channel
  // Agent sends: events (session output, status changes, health)
  // Server sends: commands (create session, attach, detach, kill, resize)
  rpc AgentStream(stream AgentEvent) returns (stream ServerCommand);
}
```

### 5.2 Registration

```protobuf
message RegisterRequest {
  string machine_id = 1;          // Must match cert CN
  int32 max_sessions = 2;         // Agent's configured limit
  ResourceInfo resources = 3;     // CPU cores, RAM, disk
  repeated SessionState existing_sessions = 4;  // Sessions still running from before reconnect
}

message RegisterResponse {
  bool accepted = 1;
  string reject_reason = 2;       // If not accepted
  string server_version = 3;
}

message ResourceInfo {
  int32 cpu_cores = 1;
  int64 memory_bytes = 2;
  int64 disk_free_bytes = 3;
}

message SessionState {
  string session_id = 1;
  string status = 2;              // "running", "completed", "failed"
  google.protobuf.Timestamp started_at = 3;
  optional google.protobuf.Timestamp ended_at = 4;
  optional int32 exit_code = 5;
}
```

### 5.3 Server → Agent Commands

```protobuf
message ServerCommand {
  oneof command {
    CreateSessionCmd create_session = 1;
    AttachSessionCmd attach_session = 2;
    DetachSessionCmd detach_session = 3;
    KillSessionCmd kill_session = 4;
    ResizeTerminalCmd resize_terminal = 5;
    InputDataCmd input_data = 6;
    RequestScrollbackCmd request_scrollback = 7;
  }
}

message CreateSessionCmd {
  string session_id = 1;            // Server-generated UUID
  string working_dir = 2;           // Absolute path on the remote machine
  map<string, string> env_vars = 3; // ANTHROPIC_API_KEY, GITHUB_TOKEN, etc.
  string command = 4;               // Default: "claude" — can be customized
  repeated string args = 5;         // CLI args (e.g., "--model", "opus")
  string initial_prompt = 6;        // If set, piped as first input (for jobs)
  TerminalSize terminal_size = 7;
}

message AttachSessionCmd {
  string session_id = 1;
  // Server wants to start receiving live output for this session
}

message DetachSessionCmd {
  string session_id = 1;
  // Server no longer needs live output (user disconnected or switched tabs)
}

message KillSessionCmd {
  string session_id = 1;
  string signal = 2;   // "SIGTERM", "SIGKILL"
}

message ResizeTerminalCmd {
  string session_id = 1;
  TerminalSize size = 2;
}

message InputDataCmd {
  string session_id = 1;
  bytes data = 2;   // Raw terminal input (keystrokes from xterm.js)
}

message RequestScrollbackCmd {
  string session_id = 1;
  // Agent should start streaming the full scrollback file, then switch to live
}

message TerminalSize {
  uint32 cols = 1;
  uint32 rows = 2;
}
```

### 5.4 Agent → Server Events

```protobuf
message AgentEvent {
  oneof event {
    SessionOutputEvent session_output = 1;
    SessionStatusEvent session_status = 2;
    HealthEvent health = 3;
    ScrollbackChunkEvent scrollback_chunk = 4;
    SessionExitEvent session_exit = 5;
  }
}

message SessionOutputEvent {
  string session_id = 1;
  bytes data = 2;            // Raw terminal output bytes (ANSI included)
  double timestamp = 3;      // Seconds since session start
}

message SessionStatusEvent {
  string session_id = 1;
  string status = 2;         // "running", "attached", "detached"
}

message SessionExitEvent {
  string session_id = 1;
  int32 exit_code = 2;
  google.protobuf.Timestamp exited_at = 3;
  int64 scrollback_bytes = 4;   // Total size of session recording
}

message HealthEvent {
  float cpu_usage_percent = 1;
  int64 memory_used_bytes = 2;
  int64 disk_free_bytes = 3;
  int32 active_sessions = 4;
  int32 max_sessions = 5;
}

message ScrollbackChunkEvent {
  string session_id = 1;
  bytes data = 2;              // Chunk of the scrollback file
  bool is_final = 3;           // True = scrollback replay complete, switch to live
  int64 total_bytes = 4;       // For progress indication
  int64 offset = 5;            // Byte offset of this chunk in the full file
}
```

### 5.5 Connection Lifecycle Sequence

```
Agent                          Server
  │                              │
  │──── TLS Handshake ──────────►│  (mTLS: both verify certs)
  │                              │
  │──── Register() ─────────────►│  (machine-id, capabilities,
  │                              │   any surviving sessions)
  │◄─── RegisterResponse ────────│  (accepted / rejected)
  │                              │
  │◄═══ AgentStream() ══════════►│  (bidirectional, persistent)
  │                              │
  │  ... time passes ...         │
  │                              │
  │◄─── CreateSessionCmd ────────│  (user clicked "new session")
  │                              │
  │──── SessionStatusEvent ─────►│  (status: "running")
  │──── SessionOutputEvent ─────►│  (streaming terminal output)
  │──── SessionOutputEvent ─────►│
  │                              │
  │◄─── InputDataCmd ────────────│  (user typed something)
  │◄─── InputDataCmd ────────────│
  │                              │
  │  ... user closes laptop ...  │
  │                              │
  │◄─── DetachSessionCmd ────────│  (WebSocket closed, stop streaming)
  │                              │
  │  ... CLI keeps running ...   │
  │  ... scrollback keeps writing│
  │                              │
  │  ... user reconnects ...     │
  │                              │
  │◄─── RequestScrollbackCmd ───│  (user wants to see what happened)
  │──── ScrollbackChunkEvent ──►│  (replay buffered output)
  │──── ScrollbackChunkEvent ──►│  (more chunks...)
  │──── ScrollbackChunkEvent ──►│  (is_final=true)
  │◄─── AttachSessionCmd ───────│  (now stream live again)
  │──── SessionOutputEvent ────►│  (back to live streaming)
  │                              │
  │  ... CLI exits ...           │
  │                              │
  │──── SessionExitEvent ───────►│  (exit_code, final scrollback size)
```

---

## 6. Database Schema

SQLite with WAL mode for V1. Migrate to Postgres if you ever need multi-server HA.

```sql
-- Machines (populated from config + enriched by agent registration)
CREATE TABLE machines (
    machine_id     TEXT PRIMARY KEY,
    display_name   TEXT,
    status         TEXT NOT NULL DEFAULT 'disconnected',
    -- 'connected', 'disconnected', 'draining' (no new sessions)
    max_sessions   INTEGER NOT NULL DEFAULT 5,
    last_health    TEXT,       -- JSON blob of last HealthEvent
    last_seen_at   DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Sessions (every terminal session, whether manual or job-triggered)
CREATE TABLE sessions (
    session_id     TEXT PRIMARY KEY,   -- UUID
    machine_id     TEXT NOT NULL REFERENCES machines(machine_id),
    user_id        TEXT,               -- Null for V1 single-user
    status         TEXT NOT NULL DEFAULT 'starting',
    -- 'starting', 'running', 'completed', 'failed', 'killed'
    command        TEXT NOT NULL DEFAULT 'claude',
    args           TEXT,               -- JSON array
    working_dir    TEXT,
    initial_prompt TEXT,               -- For job-triggered sessions
    exit_code      INTEGER,
    started_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at       DATETIME,
    scrollback_bytes INTEGER DEFAULT 0,

    -- Link to job system (nullable — manual sessions have no run_step)
    run_step_id    TEXT REFERENCES run_steps(run_step_id)
);

CREATE INDEX idx_sessions_machine ON sessions(machine_id, status);
CREATE INDEX idx_sessions_run_step ON sessions(run_step_id);

-- Jobs (the template / definition — reusable)
CREATE TABLE jobs (
    job_id         TEXT PRIMARY KEY,   -- UUID
    name           TEXT NOT NULL,
    description    TEXT,
    user_id        TEXT,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Steps (belong to a job — the template for what to execute)
CREATE TABLE steps (
    step_id        TEXT PRIMARY KEY,   -- UUID
    job_id         TEXT NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    prompt         TEXT NOT NULL,       -- The instruction for Claude CLI
    machine_id     TEXT REFERENCES machines(machine_id), -- Target machine (null = auto-assign)
    working_dir    TEXT,
    command        TEXT DEFAULT 'claude',
    args           TEXT,               -- JSON array of CLI args
    sort_order     INTEGER NOT NULL,   -- Display order in the UI
    timeout_seconds INTEGER,           -- Max time before auto-kill (0 = unlimited)
    expected_outputs TEXT,             -- JSON array of file paths to check
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_steps_job ON steps(job_id, sort_order);

-- Step dependencies (edges in the DAG)
CREATE TABLE step_dependencies (
    step_id        TEXT NOT NULL REFERENCES steps(step_id) ON DELETE CASCADE,
    depends_on     TEXT NOT NULL REFERENCES steps(step_id) ON DELETE CASCADE,
    PRIMARY KEY (step_id, depends_on),
    CHECK (step_id != depends_on)  -- No self-loops
);

-- Runs (a specific execution of a job)
CREATE TABLE runs (
    run_id         TEXT PRIMARY KEY,   -- UUID
    job_id         TEXT NOT NULL REFERENCES jobs(job_id),
    status         TEXT NOT NULL DEFAULT 'pending',
    -- 'pending', 'running', 'completed', 'failed', 'cancelled'
    trigger_type   TEXT NOT NULL,      -- 'manual', 'cron', 'dependency', 'api'
    trigger_detail TEXT,               -- Cron expression, parent run_id, etc.
    started_at     DATETIME,
    ended_at       DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_runs_job ON runs(job_id, created_at DESC);
CREATE INDEX idx_runs_status ON runs(status);

-- Run steps (instance of a step within a specific run)
CREATE TABLE run_steps (
    run_step_id    TEXT PRIMARY KEY,   -- UUID
    run_id         TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    step_id        TEXT NOT NULL REFERENCES steps(step_id),
    status         TEXT NOT NULL DEFAULT 'pending',
    -- 'pending', 'waiting', 'running', 'completed', 'failed', 'skipped', 'cancelled'
    machine_id     TEXT REFERENCES machines(machine_id),
    session_id     TEXT,               -- Set when session is created
    exit_code      INTEGER,
    started_at     DATETIME,
    ended_at       DATETIME,
    error_message  TEXT                -- If failed, why
);

CREATE INDEX idx_run_steps_run ON run_steps(run_id);
CREATE INDEX idx_run_steps_status ON run_steps(status);

-- Cron schedules (jobs that run on a schedule)
CREATE TABLE cron_schedules (
    schedule_id    TEXT PRIMARY KEY,   -- UUID
    job_id         TEXT NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    cron_expr      TEXT NOT NULL,      -- Standard cron: "0 9 * * 1-5" (weekdays at 9am)
    timezone       TEXT NOT NULL DEFAULT 'UTC',
    enabled        BOOLEAN NOT NULL DEFAULT 1,
    next_run_at    DATETIME,           -- Pre-computed next fire time
    last_run_id    TEXT REFERENCES runs(run_id),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_cron_next ON cron_schedules(enabled, next_run_at);

-- Cross-job triggers (a completed job triggers another job)
CREATE TABLE job_triggers (
    trigger_id     TEXT PRIMARY KEY,   -- UUID
    source_job_id  TEXT NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    target_job_id  TEXT NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    condition      TEXT NOT NULL DEFAULT 'on_success',
    -- 'on_success', 'on_failure', 'on_completion' (either)
    enabled        BOOLEAN NOT NULL DEFAULT 1,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (source_job_id != target_job_id)
);

CREATE INDEX idx_job_triggers_source ON job_triggers(source_job_id, enabled);

-- Encrypted credentials (per user in multi-user, global in single-user)
CREATE TABLE credentials (
    credential_id  TEXT PRIMARY KEY,
    user_id        TEXT,               -- Null = global
    name           TEXT NOT NULL,      -- "ANTHROPIC_API_KEY", "GITHUB_TOKEN", etc.
    encrypted_value BLOB NOT NULL,     -- AES-256-GCM encrypted
    nonce          BLOB NOT NULL,      -- GCM nonce
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Audit log (who did what)
CREATE TABLE audit_log (
    log_id         INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id        TEXT,
    action         TEXT NOT NULL,
    -- 'session.create', 'session.kill', 'job.create', 'job.run',
    -- 'run.cancel', 'credential.update', 'machine.drain', etc.
    resource_type  TEXT,               -- 'session', 'job', 'run', 'machine'
    resource_id    TEXT,
    detail         TEXT                -- JSON with extra context
);

CREATE INDEX idx_audit_time ON audit_log(timestamp DESC);
```

---

## 7. Job Orchestrator

The job orchestrator is the most complex server component. It handles three types of triggers and manages the lifecycle of job runs.

### 7.1 Trigger Types

#### Manual Trigger
User clicks "Run" on a job in the frontend. Simple REST call:

```
POST /api/v1/jobs/{job_id}/runs
{ "trigger_type": "manual" }
```

#### Cron Trigger
Time-based scheduling. The server runs a cron evaluator goroutine.

```go
// Cron evaluator — runs every second (or every 10s and pre-computes)
func (o *Orchestrator) cronLoop(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    for {
        select {
        case <-ctx.Done():
            return
        case now := <-ticker.C:
            schedules := o.db.GetDueCronSchedules(now)
            for _, sched := range schedules {
                runID := o.createRun(sched.JobID, "cron", sched.CronExpr)
                o.db.UpdateCronNextRun(sched.ScheduleID, sched.ComputeNext(now))
                go o.executeRun(ctx, runID)
            }
        }
    }
}
```

**Cron expression support:** Standard 5-field cron (`minute hour day-of-month month day-of-week`). Use a Go cron library like `robfig/cron/v3` for parsing and next-time computation.

**Timezone handling:** Each schedule has an explicit timezone. `next_run_at` is stored as UTC. Cron evaluator compares against UTC.

**Missed fires:** If the server was down and a cron fire was missed, check on startup: if `next_run_at < now`, fire immediately (configurable: fire-once-then-catchup vs. skip-missed).

#### Cross-Job Dependency Trigger
When a job run completes, check if any other jobs should be triggered:

```go
func (o *Orchestrator) onRunCompleted(runID string, status string) {
    run := o.db.GetRun(runID)

    // Check for cross-job triggers
    triggers := o.db.GetJobTriggers(run.JobID)
    for _, t := range triggers {
        if !t.Enabled {
            continue
        }
        shouldFire := false
        switch t.Condition {
        case "on_success":
            shouldFire = (status == "completed")
        case "on_failure":
            shouldFire = (status == "failed")
        case "on_completion":
            shouldFire = true
        }
        if shouldFire {
            newRunID := o.createRun(t.TargetJobID, "dependency", runID)
            go o.executeRun(context.Background(), newRunID)
        }
    }
}
```

This means you can chain jobs across different scopes:
- Job A: "Generate PRD for repo X" → on success → Job B: "Generate TRD from PRD"
- Nightly Job: "Run test suite" → on failure → Alert Job: "Post failure summary"

### 7.2 DAG Execution Within a Run

When a run starts, the orchestrator:

1. **Instantiates run_steps** — one `run_step` row per `step` in the job.
2. **Resolves the DAG** — identifies steps with no dependencies (roots).
3. **Executes ready steps in parallel** — each step spawns a session on its target machine.
4. **Monitors completion** — when a step's session exits, check if downstream steps are now unblocked.
5. **Handles failures** — configurable per step: `fail_run`, `skip_dependents`, `continue`.

```go
type DAGRunner struct {
    mu        sync.Mutex
    runID     string
    steps     map[string]*RunStep       // step_id → run step state
    deps      map[string][]string       // step_id → list of step_ids it depends on
    dependents map[string][]string      // step_id → list of step_ids that depend on it
    executor  StepExecutor
}

func (d *DAGRunner) Start(ctx context.Context) {
    // Find all root steps (no dependencies)
    for stepID, depList := range d.deps {
        if len(depList) == 0 {
            go d.executeStep(ctx, stepID)
        }
    }
}

func (d *DAGRunner) OnStepCompleted(stepID string, exitCode int) {
    d.mu.Lock()
    defer d.mu.Unlock()

    step := d.steps[stepID]
    if exitCode == 0 {
        step.Status = "completed"
    } else {
        step.Status = "failed"
        // TODO: handle failure policy
    }

    // Check dependents
    for _, depID := range d.dependents[stepID] {
        if d.allDependenciesMet(depID) {
            go d.executeStep(context.Background(), depID)
        }
    }

    // Check if entire run is done
    if d.allStepsTerminal() {
        d.finalizeRun()
    }
}

func (d *DAGRunner) allDependenciesMet(stepID string) bool {
    for _, depID := range d.deps[stepID] {
        if d.steps[depID].Status != "completed" {
            return false
        }
    }
    return true
}
```

### 7.3 Step Execution Flow

When a step becomes runnable:

```go
func (d *DAGRunner) executeStep(ctx context.Context, stepID string) {
    step := d.steps[stepID]
    stepDef := d.db.GetStep(stepID) // Get the template

    // 1. Pick a machine
    machineID := stepDef.MachineID
    if machineID == "" {
        machineID = d.scheduler.PickMachine() // Least-loaded available machine
    }

    // 2. Build the session creation command
    cmd := &CreateSessionCmd{
        SessionID:     uuid.New().String(),
        WorkingDir:    stepDef.WorkingDir,
        EnvVars:       d.getCredentials(), // Decrypt and inject
        Command:       stepDef.Command,
        Args:          stepDef.Args,
        InitialPrompt: stepDef.Prompt,
        TerminalSize:  &TerminalSize{Cols: 120, Rows: 40},
    }

    // 3. Send to agent
    agent := d.agentMgr.GetAgent(machineID)
    agent.SendCommand(cmd)

    // 4. Update state
    step.Status = "running"
    step.SessionID = cmd.SessionID
    step.MachineID = machineID
    d.db.UpdateRunStep(step)

    // 5. If there's a timeout, start a watchdog
    if stepDef.TimeoutSeconds > 0 {
        go d.watchTimeout(ctx, stepID, cmd.SessionID, stepDef.TimeoutSeconds)
    }
}
```

### 7.4 Initial Prompt Delivery

When a job step has a prompt, it needs to be "typed" into the Claude CLI after it starts. Two strategies:

**Strategy A: Pipe on exec**
```bash
echo "analyze this codebase and generate a PRD" | claude
```
Problem: Claude CLI might not support piped input well, or it enters non-interactive mode.

**Strategy B: Synthetic keystrokes**
Agent waits for the CLI to be ready (detects the prompt marker in output), then writes the prompt text into the PTY's stdin, followed by a newline. This is indistinguishable from a human typing.

```go
// In the agent, after spawning the CLI process:
func (s *Session) injectPrompt(prompt string) {
    // Wait for CLI to show its prompt (e.g., scan output for "❯" or "> ")
    <-s.readySignal

    // Type the prompt as if a user typed it
    s.ptyMaster.Write([]byte(prompt + "\n"))
}
```

**Strategy B is better** — it keeps the CLI in full interactive mode, and the prompt appears in the terminal output naturally (user can see exactly what was sent).

### 7.5 Machine Scheduling

When a step has no explicit machine target (`machine_id` is null), the scheduler picks one:

```go
func (s *Scheduler) PickMachine() string {
    s.mu.RLock()
    defer s.mu.RUnlock()

    var best string
    var bestScore float64 = -1

    for id, machine := range s.machines {
        if machine.Status != "connected" {
            continue
        }
        available := machine.MaxSessions - machine.ActiveSessions
        if available <= 0 {
            continue
        }
        // Score: higher = better candidate
        // Favor machines with more free capacity and lower CPU usage
        score := float64(available) * (1.0 - machine.CPUUsage)
        if score > bestScore {
            bestScore = score
            best = id
        }
    }
    return best // Empty string if no machines available
}
```

If no machine is available, the step stays in `waiting` state. The scheduler re-evaluates when:
- An agent reconnects.
- A session completes (freeing a slot).
- Health stats arrive showing reduced load.

### 7.6 DAG Cycle Detection

When creating or updating a job's steps and dependencies, validate there are no cycles:

```go
func validateDAG(steps []Step, deps []StepDependency) error {
    // Kahn's algorithm (topological sort)
    inDegree := make(map[string]int)
    adj := make(map[string][]string)

    for _, s := range steps {
        inDegree[s.StepID] = 0
    }
    for _, d := range deps {
        adj[d.DependsOn] = append(adj[d.DependsOn], d.StepID)
        inDegree[d.StepID]++
    }

    queue := []string{}
    for id, deg := range inDegree {
        if deg == 0 {
            queue = append(queue, id)
        }
    }

    visited := 0
    for len(queue) > 0 {
        node := queue[0]
        queue = queue[1:]
        visited++
        for _, next := range adj[node] {
            inDegree[next]--
            if inDegree[next] == 0 {
                queue = append(queue, next)
            }
        }
    }

    if visited != len(steps) {
        return fmt.Errorf("cycle detected in job DAG")
    }
    return nil
}
```

Reject any job save that introduces a cycle. Same validation for cross-job triggers (build a graph of all job-to-job edges and check for cycles).

---

## 8. WebSocket ↔ gRPC Bridge (Terminal I/O)

The server sits between the browser's WebSocket and the agent's gRPC stream. It's a two-way relay with session awareness.

### 8.1 Flow: User opens a terminal session in the browser

```
Browser                    Server                          Agent
  │                          │                               │
  │── WS upgrade ───────────►│                               │
  │   /ws/terminal/sess-abc  │                               │
  │                          │                               │
  │                          │── Is sess-abc active? ───────►│
  │                          │   (check session registry)    │
  │                          │                               │
  │                          │── RequestScrollbackCmd ──────►│
  │                          │                               │
  │◄── scrollback data ──────│◄── ScrollbackChunkEvent ──────│
  │◄── scrollback data ──────│◄── ScrollbackChunkEvent ──────│
  │◄── (replay complete) ────│◄── ScrollbackChunk(final) ────│
  │                          │                               │
  │                          │── AttachSessionCmd ──────────►│
  │                          │                               │
  │◄── live output ──────────│◄── SessionOutputEvent ────────│
  │── keystrokes ────────────│── InputDataCmd ──────────────►│
  │                          │                               │
  │── WS close ──────────────│                               │
  │                          │── DetachSessionCmd ──────────►│
  │                          │   (agent keeps session alive) │
```

### 8.2 Server-side WebSocket Handler

```go
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionID")

    // Auth check
    user, err := s.authenticate(r)
    if err != nil {
        http.Error(w, "unauthorized", 401)
        return
    }

    // Find the session
    session, err := s.sessionRegistry.Get(sessionID)
    if err != nil {
        http.Error(w, "session not found", 404)
        return
    }

    // Upgrade to WebSocket
    conn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    // Get the agent for this session's machine
    agent := s.agentMgr.GetAgent(session.MachineID)
    if agent == nil {
        conn.WriteMessage(websocket.TextMessage,
            []byte(`{"error":"agent disconnected"}`))
        return
    }

    // Request scrollback replay
    agent.SendCommand(&RequestScrollbackCmd{SessionID: sessionID})

    // Create a channel for this session's output
    outputCh := s.sessionRegistry.Subscribe(sessionID)
    defer s.sessionRegistry.Unsubscribe(sessionID, outputCh)

    ctx, cancel := context.WithCancel(r.Context())
    defer cancel()

    // Relay: agent output → WebSocket
    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            case data := <-outputCh:
                conn.WriteMessage(websocket.BinaryMessage, data)
            }
        }
    }()

    // Relay: WebSocket → agent input
    for {
        _, msg, err := conn.ReadMessage()
        if err != nil {
            // WebSocket closed — detach but don't kill session
            agent.SendCommand(&DetachSessionCmd{SessionID: sessionID})
            return
        }
        agent.SendCommand(&InputDataCmd{
            SessionID: sessionID,
            Data:      msg,
        })
    }
}
```

---

## 9. REST API Summary

### Sessions

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/sessions` | List all sessions (filterable by status, machine, user) |
| GET | `/api/v1/sessions/:id` | Get session details |
| POST | `/api/v1/sessions` | Create a new manual session (pick machine + working dir) |
| DELETE | `/api/v1/sessions/:id` | Kill a session (sends SIGTERM → SIGKILL after 10s) |

### Jobs

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/jobs` | List all jobs |
| GET | `/api/v1/jobs/:id` | Get job details (including steps + dependencies) |
| POST | `/api/v1/jobs` | Create a new job |
| PUT | `/api/v1/jobs/:id` | Update a job (validates DAG) |
| DELETE | `/api/v1/jobs/:id` | Delete a job (cascades to steps, deps, schedules) |
| POST | `/api/v1/jobs/:id/runs` | Trigger a run of this job |

### Steps (nested under jobs)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/jobs/:id/steps` | Add a step to a job |
| PUT | `/api/v1/jobs/:jid/steps/:sid` | Update a step |
| DELETE | `/api/v1/jobs/:jid/steps/:sid` | Remove a step |
| POST | `/api/v1/jobs/:jid/steps/:sid/deps` | Add a dependency edge |
| DELETE | `/api/v1/jobs/:jid/steps/:sid/deps/:dep_id` | Remove a dependency edge |

### Runs

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/runs` | List runs (filterable by job, status, trigger type) |
| GET | `/api/v1/runs/:id` | Get run details (including run_steps and their session IDs) |
| POST | `/api/v1/runs/:id/cancel` | Cancel a running run (kills active sessions, skips pending steps) |

### Schedules

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/jobs/:id/schedules` | List cron schedules for a job |
| POST | `/api/v1/jobs/:id/schedules` | Create a cron schedule |
| PUT | `/api/v1/schedules/:id` | Update schedule (expr, timezone, enabled) |
| DELETE | `/api/v1/schedules/:id` | Delete schedule |

### Triggers

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/jobs/:id/triggers` | List cross-job triggers where this job is the source |
| POST | `/api/v1/jobs/:id/triggers` | Create a trigger (this job → target job) |
| DELETE | `/api/v1/triggers/:id` | Delete trigger |

### Machines

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/machines` | List all machines + health + session count |
| PUT | `/api/v1/machines/:id` | Update display name, drain status |
| POST | `/api/v1/machines/:id/drain` | Drain machine (finish active sessions, accept no new ones) |
| POST | `/api/v1/machines/:id/undrain` | Remove drain status |

---

## 10. Failure Modes & Recovery

### Agent disconnects (network failure, machine crash)

1. Server detects gRPC stream break.
2. Machine status → `disconnected`.
3. All sessions on that machine → status `unknown` (not failed yet — agent might come back).
4. If agent reconnects within grace period (configurable, default 5 min):
   - Agent sends `existing_sessions` in `Register()`.
   - Server reconciles: sessions that are still running get status updated back to `running`.
5. If grace period expires:
   - Sessions → `failed` (with error: "agent disconnected").
   - Any runs with failed steps → trigger failure handling (skip dependents or fail run).

### Server restarts

1. On startup, server loads all state from SQLite.
2. Agents reconnect (they retry automatically).
3. Active sessions are reconciled via the `Register()` RPC.
4. Cron evaluator checks for missed fires.
5. In-progress runs resume: re-build DAGRunners from `run_steps` table, re-evaluate what's runnable.

### Claude CLI crashes inside a session

1. Agent detects PTY process exit with non-zero code.
2. Sends `SessionExitEvent` with exit code.
3. If session is part of a job step:
   - Run step → `failed`.
   - DAG runner evaluates failure policy.
4. Scrollback file captures everything up to the crash — user can replay and see what happened.

### Concurrent user access to the same session

Multiple browser tabs can connect to the same session (same user). Both get the output stream. Both can send input. This is intentional — it's like two people in the same `tmux` session. If it causes problems, add a "primary / observer" mode later where only one can type and others are read-only.

---

## 11. Build & Deployment

### Binary Build

```makefile
# Makefile
VERSION := $(shell git describe --tags --always)

build-server:
	CGO_ENABLED=1 go build -ldflags "-X main.version=$(VERSION)" \
		-o bin/claude-plane-server ./cmd/server

build-agent:
	CGO_ENABLED=0 go build -ldflags "-X main.version=$(VERSION)" \
		-o bin/claude-plane-agent ./cmd/agent

# Cross-compile agent for multiple architectures
build-agent-all:
	GOOS=linux GOARCH=amd64 go build -o bin/claude-plane-agent-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 go build -o bin/claude-plane-agent-linux-arm64 ./cmd/agent
	GOOS=darwin GOARCH=arm64 go build -o bin/claude-plane-agent-darwin-arm64 ./cmd/agent
```

**Note:** Server needs `CGO_ENABLED=1` for SQLite (mattn/go-sqlite3). Agent is pure Go, no CGO needed. Alternatively, use `modernc.org/sqlite` for a pure-Go SQLite driver (no CGO, easier cross-compilation, slightly slower).

### Docker Images

```dockerfile
# Server
FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o claude-plane-server ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/claude-plane-server /usr/local/bin/
COPY --from=builder /build/frontend/dist /var/www/claude-plane/
EXPOSE 8443 9443
ENTRYPOINT ["claude-plane-server"]

# Agent
FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -o claude-plane-agent ./cmd/agent

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
# Claude CLI must be installed separately or mounted
COPY --from=builder /build/claude-plane-agent /usr/local/bin/
ENTRYPOINT ["claude-plane-agent"]
```

### Minimal Deployment Steps

```bash
# On the control plane machine:
claude-plane-server ca init --out /etc/claude-plane/ca/
claude-plane-server ca issue-server --ca-dir /etc/claude-plane/ca/ --out /etc/claude-plane/server-cert/
claude-plane-server ca issue-agent --ca-dir /etc/claude-plane/ca/ --machine-id nuc-01 --out ./nuc-01-certs/
# Edit /etc/claude-plane/server.toml
claude-plane-server serve

# On each worker:
# scp agent binary + certs
# Edit /etc/claude-plane/agent.toml
claude-plane-agent run
```

---

## 12. Go Project Structure

```
claude-plane/
├── cmd/
│   ├── server/
│   │   └── main.go              # Server entrypoint + CLI commands
│   └── agent/
│       └── main.go              # Agent entrypoint + CLI commands
├── proto/
│   └── claudeplane.proto        # gRPC service definition
├── pkg/
│   ├── server/
│   │   ├── http.go              # HTTP server + REST handlers
│   │   ├── ws.go                # WebSocket terminal handler
│   │   ├── grpc.go              # gRPC server + agent connection manager
│   │   ├── sessions.go          # Session registry
│   │   ├── orchestrator.go      # Job orchestrator (DAG runner, cron, triggers)
│   │   ├── scheduler.go         # Machine selection logic
│   │   ├── db.go                # Database access layer
│   │   └── auth.go              # Authentication middleware
│   ├── agent/
│   │   ├── client.go            # gRPC client + reconnection logic
│   │   ├── session.go           # PTY management + scrollback
│   │   ├── health.go            # Health stats collection
│   │   └── commands.go          # Command handlers (create, attach, kill, etc.)
│   ├── tls/
│   │   ├── ca.go                # CA management + cert generation
│   │   └── config.go            # TLS config helpers
│   ├── crypto/
│   │   └── encrypt.go           # AES-256-GCM for credential encryption
│   └── models/
│       └── types.go             # Shared types (Session, Job, Step, Run, etc.)
├── frontend/                    # React app (separate build)
│   ├── src/
│   └── package.json
├── Makefile
├── go.mod
└── go.sum
```

---

## Appendix A: Full Lifecycle Example

**Scenario:** José creates a 3-step job (PRD → TRD → Implementation Plan) with a cron schedule to run every Monday at 9am, and a cross-job trigger to run a "Notify" job on completion.

1. **Job creation** (via frontend):
   - POST `/api/v1/jobs` → creates job "Kodrun V2 Planning"
   - POST steps: PRD (no deps), TRD (depends on PRD), Plan (depends on TRD)
   - POST schedule: `0 9 * * 1` (Monday 9am), timezone `Europe/Madrid`
   - POST trigger: on_success → "Notify Slack" job

2. **Monday 9:00 AM** — cron evaluator fires:
   - Creates run (trigger_type: "cron")
   - Instantiates 3 run_steps, all "pending"
   - DAG runner finds PRD has no deps → executes

3. **PRD step executes:**
   - Scheduler picks nuc-01 (least loaded)
   - Server sends CreateSessionCmd to nuc-01 agent
   - Agent spawns PTY, starts `claude` CLI, injects prompt
   - Claude works autonomously (José is asleep or at work)
   - Agent writes scrollback to disk
   - Claude finishes, CLI exits with code 0
   - Agent sends SessionExitEvent → server marks run_step "completed"

4. **TRD step unlocked:**
   - DAG runner detects PRD completed → TRD is now runnable
   - Same flow: pick machine, create session, inject prompt, execute

5. **Plan step** runs after TRD completes.

6. **All steps done** → run status: "completed"
   - Cross-job trigger fires → "Notify Slack" job starts a new run

7. **José opens claude-plane at 11am:**
   - Sees the completed run in the dashboard
   - Clicks into PRD step → replays the full terminal recording
   - Clicks into TRD step → sees exactly what Claude generated
   - Reviews, opens a new manual session to iterate on the Plan

---

## Appendix B: Cron Expression Reference

| Expression | Meaning |
|------------|---------|
| `0 9 * * 1-5` | Weekdays at 9:00 AM |
| `*/30 * * * *` | Every 30 minutes |
| `0 0 * * 0` | Sunday at midnight |
| `0 9 1 * *` | First of every month at 9 AM |
| `0 */6 * * *` | Every 6 hours |

Standard 5-field cron. No seconds field (not needed for this use case). Extended expressions (e.g., `@daily`, `@weekly`) supported via `robfig/cron/v3`.
