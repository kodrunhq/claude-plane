# claude-plane: Supplementary Design — Remaining Systems

**Version:** 0.1.0-draft
**Author:** José / Claude (Opus)
**Date:** 2026-03-11
**Companion to:** Backend Architecture + Frontend Architecture documents

---

## Table of Contents

1. Workspace Isolation
2. Arena Mode
3. Settings & Credentials Management
4. Logging, Observability & Debugging
5. Security Hardening
6. Operational Concerns (Backup, Recovery, Migration)
7. Integration Points (Kodrun, CLI, Webhooks)
8. Job System Edge Cases
9. Agent Provisioning & Upgrades
10. Cost Tracking & Token Analytics
11. Phased Roadmap (V1 → V2 → V3 → V4)

---

## 1. Workspace Isolation

### The Problem

Two sessions (same user or different users) targeting the same machine and the same repository will collide. Git state is global to the working directory — one session checks out a branch, the other sees dirty state. File writes conflict. Build artifacts clobber each other.

### The Solution: Per-Session Workspace Directories

Every session gets its own workspace. The agent creates an isolated directory before spawning the Claude CLI.

#### Strategy: Copy-on-Session with Shared Base

```
/var/lib/claude-plane/workspaces/
├── repos/                          # Shared base clones (pulled periodically)
│   ├── kodrun/                     # git clone of kodrun
│   ├── spark-lens/                 # git clone of spark-lens
│   └── ...
└── sessions/
    ├── sess-abc/                   # Session workspace
    │   └── kodrun/                 # git worktree or copy from repos/kodrun
    ├── sess-def/
    │   └── kodrun/                 # Independent workspace, same repo
    └── ...
```

**How it works:**

1. **Base clones** live in `repos/`. These are bare or full clones of each repository, updated periodically (or on-demand before session creation). They're never used directly by a session.

2. **Session workspaces** are created per session using `git worktree` (preferred) or `cp -r` (fallback).

3. **`git worktree`** is the ideal approach — it creates a new working directory that shares the same `.git` object store as the base clone, but has its own index, HEAD, and working tree. Fast to create (no full copy), minimal disk usage, full git isolation.

```bash
# Agent creates a session workspace
cd /var/lib/claude-plane/workspaces/repos/kodrun
git fetch origin
git worktree add /var/lib/claude-plane/workspaces/sessions/sess-abc/kodrun origin/main
```

4. **Session cleanup:** When the session ends (or after a configurable retention period), the workspace is deleted:
```bash
cd /var/lib/claude-plane/workspaces/repos/kodrun
git worktree remove /var/lib/claude-plane/workspaces/sessions/sess-abc/kodrun --force
```

#### Agent-Side Implementation

```go
type WorkspaceManager struct {
    baseDir     string  // /var/lib/claude-plane/workspaces
    reposDir    string  // baseDir/repos
    sessionsDir string  // baseDir/sessions
}

func (w *WorkspaceManager) PrepareWorkspace(sessionID string, repoURL string, branch string) (string, error) {
    repoName := extractRepoName(repoURL)
    basePath := filepath.Join(w.reposDir, repoName)

    // Ensure base clone exists
    if !dirExists(basePath) {
        if err := exec.Command("git", "clone", "--bare", repoURL, basePath).Run(); err != nil {
            return "", fmt.Errorf("clone failed: %w", err)
        }
    }

    // Fetch latest
    cmd := exec.Command("git", "-C", basePath, "fetch", "origin")
    cmd.Run() // Best-effort; session can still work with stale data

    // Create session workspace via worktree
    workspacePath := filepath.Join(w.sessionsDir, sessionID, repoName)
    ref := "origin/" + branch
    if err := exec.Command("git", "-C", basePath, "worktree", "add", workspacePath, ref).Run(); err != nil {
        // Fallback: full copy if worktree fails (e.g., bare clone issues)
        return w.fallbackCopy(basePath, workspacePath)
    }

    return workspacePath, nil
}

func (w *WorkspaceManager) CleanupWorkspace(sessionID string, repoURL string) error {
    repoName := extractRepoName(repoURL)
    workspacePath := filepath.Join(w.sessionsDir, sessionID, repoName)
    basePath := filepath.Join(w.reposDir, repoName)

    // Remove worktree reference
    exec.Command("git", "-C", basePath, "worktree", "remove", workspacePath, "--force").Run()

    // Remove directory
    return os.RemoveAll(filepath.Join(w.sessionsDir, sessionID))
}
```

#### Changes to CreateSessionCmd

The `CreateSessionCmd` proto gets new fields:

```protobuf
message CreateSessionCmd {
    // ... existing fields ...
    WorkspaceConfig workspace = 8;
}

message WorkspaceConfig {
    string repo_url = 1;       // https://github.com/jose/kodrun.git
    string branch = 2;         // "main", "feature/v2", etc.
    string git_token = 3;      // Injected for private repos (transmitted over mTLS)
    bool skip_workspace = 4;   // If true, use working_dir directly (no isolation)
}
```

When `workspace` is set, the agent:
1. Calls `WorkspaceManager.PrepareWorkspace()`
2. Sets the session's working directory to the resulting workspace path
3. Injects `GIT_TOKEN` into the environment
4. Spawns Claude CLI in that directory

When `skip_workspace = true` (or workspace is nil), the agent uses `working_dir` directly — this is the "I know what I'm doing, just use this path" escape hatch.

#### Cleanup Policy

```toml
# In agent.toml
[workspace]
cleanup_on_session_end = true       # Delete workspace when session completes
retention_after_end = "1h"          # Keep for 1h after session ends (in case user wants to inspect)
max_workspace_disk_gb = 50          # Hard limit — refuse new sessions if exceeded
base_clone_refresh_interval = "1h"  # How often to fetch base clones
```

#### Multi-User Isolation

With this model, multi-user isolation is automatic:
- User A's session gets `sessions/sess-abc/kodrun/`
- User B's session gets `sessions/sess-def/kodrun/`
- Both are independent git worktrees from the same base
- No conflicting state, ever
- Each session has its own env vars (different API keys, git credentials)

#### Frontend Changes

The "New Session" modal and job step editor get a workspace section:

```
┌────────────────────────────────────────┐
│  Workspace                             │
│                                        │
│  Repository: [github.com/jose/kodrun▾] │
│  Branch:     [main               ▾]   │
│                                        │
│  ☐ Skip isolation (use raw directory)  │
│    Path: [/home/jose/repos/kodrun   ]  │
└────────────────────────────────────────┘
```

Repository dropdown populated from credentials store (GitHub token → list repos via API, cached). Branch dropdown populated from the base clone's remote refs.

---

## 2. Arena Mode

### Concept

Run the same task against multiple Claude CLI configurations in parallel, compare results side by side. Configurations can differ in: model, prompt, CLI flags, or any combination.

### Data Model

```sql
-- Arena definitions (reusable templates)
CREATE TABLE arenas (
    arena_id       TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    description    TEXT,
    base_prompt    TEXT NOT NULL,      -- The shared task prompt
    repo_url       TEXT,               -- Optional: shared repo context
    branch         TEXT,
    working_dir    TEXT,
    user_id        TEXT,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Arena contestants (different configurations to compare)
CREATE TABLE arena_contestants (
    contestant_id  TEXT PRIMARY KEY,
    arena_id       TEXT NOT NULL REFERENCES arenas(arena_id) ON DELETE CASCADE,
    name           TEXT NOT NULL,      -- Display label: "Opus", "Sonnet", "Opus + detailed prompt"
    command        TEXT DEFAULT 'claude',
    args           TEXT,               -- JSON array: ["--model", "opus"]
    prompt_override TEXT,              -- If set, overrides base_prompt for this contestant
    machine_id     TEXT REFERENCES machines(machine_id),  -- null = auto-assign
    sort_order     INTEGER NOT NULL
);

CREATE INDEX idx_contestants_arena ON arena_contestants(arena_id, sort_order);

-- Arena runs (a specific execution of an arena)
CREATE TABLE arena_runs (
    arena_run_id   TEXT PRIMARY KEY,
    arena_id       TEXT NOT NULL REFERENCES arenas(arena_id),
    status         TEXT NOT NULL DEFAULT 'pending',
    -- 'pending', 'running', 'completed', 'judging', 'judged'
    started_at     DATETIME,
    ended_at       DATETIME,
    winner_id      TEXT REFERENCES arena_contestants(contestant_id), -- Set after judging
    notes          TEXT,               -- User's comparison notes
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Arena run entries (one per contestant per run, links to sessions)
CREATE TABLE arena_run_entries (
    entry_id       TEXT PRIMARY KEY,
    arena_run_id   TEXT NOT NULL REFERENCES arena_runs(arena_run_id) ON DELETE CASCADE,
    contestant_id  TEXT NOT NULL REFERENCES arena_contestants(contestant_id),
    session_id     TEXT REFERENCES sessions(session_id),
    status         TEXT NOT NULL DEFAULT 'pending',
    -- 'pending', 'running', 'completed', 'failed'
    exit_code      INTEGER,
    duration_seconds REAL,
    started_at     DATETIME,
    ended_at       DATETIME
);

CREATE INDEX idx_arena_entries_run ON arena_run_entries(arena_run_id);
```

### Execution Flow

1. User defines an arena: base prompt, repo (optional), and 2–4 contestants with different configurations.
2. User clicks "Run Arena."
3. Server creates an `arena_run` and one `arena_run_entry` per contestant.
4. For each entry: creates a session on the assigned (or auto-selected) machine, with the contestant's config.
5. All sessions start simultaneously (or as close to it as machine availability allows).
6. Each session runs independently. The prompt is injected via synthetic keystrokes (same as jobs).
7. When all entries complete, the arena run status → `completed`.
8. User reviews results side by side, optionally picks a winner and writes notes.

### Arena Frontend

#### Arena List (`/arenas`)

```
┌─────────────────────────────────────────────────────────────┐
│  ARENAS                                    [+ New Arena]    │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  PRD Generation Shootout                              │   │
│  │  3 contestants: Opus / Sonnet / Opus (detailed)       │   │
│  │  Last run: 2h ago · Opus won (3/5 runs)               │   │
│  │                              [Run Now]  [Edit]        │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

#### Arena Editor (`/arenas/:id/edit`)

```
┌─────────────────────────────────────────────────────────────┐
│  ← Arenas   PRD Generation Shootout        [Save] [Run]    │
│                                                              │
│  Base prompt:                                                │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Analyze this codebase and generate a comprehensive   │   │
│  │ PRD covering architecture, requirements, and...      │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  Repository: [kodrun ▾]  Branch: [main ▾]                   │
│                                                              │
│  CONTESTANTS                                                 │
│  ┌────────────────────┐ ┌────────────────────┐              │
│  │  Opus              │ │  Sonnet            │ [+ Add]      │
│  │  --model opus      │ │  --model sonnet    │              │
│  │  Machine: auto     │ │  Machine: auto     │              │
│  │  Prompt: (base)    │ │  Prompt: (base)    │              │
│  │  [Edit] [Remove]   │ │  [Edit] [Remove]   │              │
│  └────────────────────┘ └────────────────────┘              │
└─────────────────────────────────────────────────────────────┘
```

#### Arena Run Result (`/arenas/:id/runs/:runId`)

This is the comparison view — the main UX payoff:

```
┌─────────────────────────────────────────────────────────────┐
│  ← Arena   PRD Shootout   Run #5           ● Completed      │
│                                                              │
│  ┌──────────────┬──────────────┬──────────────┐             │
│  │   Opus       │   Sonnet     │  Opus+Detail │             │
│  │   ✓ 8m 23s   │   ✓ 4m 12s   │  ✓ 12m 45s   │             │
│  ├──────────────┼──────────────┼──────────────┤             │
│  │              │              │              │             │
│  │  (terminal   │  (terminal   │  (terminal   │             │
│  │   replay)    │   replay)    │   replay)    │             │
│  │              │              │              │             │
│  │              │              │              │             │
│  │              │              │              │             │
│  │              │              │              │             │
│  │              │              │              │             │
│  │              │              │              │             │
│  └──────────────┴──────────────┴──────────────┘             │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  COMPARISON                                           │   │
│  │  Duration:  Sonnet (4m) < Opus (8m) < Opus+ (12m)    │   │
│  │                                                       │   │
│  │  Winner: [Select... ▾]   Notes: [                  ]  │   │
│  │                                   [Save judgment]     │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

Three (or more) terminal panels side by side, each with independent replay controls. Synchronized playback option: all three play at the same speed, aligned to the same relative timestamp. Or independent: scrub each one separately.

**Key metrics shown per contestant:**
- Duration (time to completion)
- Exit code
- Output file sizes (if expected_outputs are configured)
- Session recording size (proxy for "how much did it do")

**On live arena runs:** The terminals show live output. You watch all three working simultaneously. This is the most visually impressive feature of the entire product.

#### Responsive: Arena Runs

| Breakpoint | Layout |
|------------|--------|
| Desktop (≥1440px) | Side-by-side panels (2–4 columns) |
| Laptop (1024–1439px) | 2 columns, scroll for more |
| Tablet | Stacked vertically, one at a time with contestant tabs |
| Phone | Read-only: show winner + durations, link to replays |

### REST API Additions

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/arenas` | List arenas |
| POST | `/api/v1/arenas` | Create arena |
| GET | `/api/v1/arenas/:id` | Get arena with contestants |
| PUT | `/api/v1/arenas/:id` | Update arena |
| DELETE | `/api/v1/arenas/:id` | Delete arena |
| POST | `/api/v1/arenas/:id/contestants` | Add contestant |
| PUT | `/api/v1/arenas/:aid/contestants/:cid` | Update contestant |
| DELETE | `/api/v1/arenas/:aid/contestants/:cid` | Remove contestant |
| POST | `/api/v1/arenas/:id/runs` | Start an arena run |
| GET | `/api/v1/arenas/:id/runs` | List arena runs |
| GET | `/api/v1/arena-runs/:id` | Get arena run with entries |
| PUT | `/api/v1/arena-runs/:id/judge` | Set winner + notes |

### Frontend Navigation Addition

Sidebar gets a new item:
```
○ Arenas (between Runs and Machines)
```

Routes:
```
/arenas
/arenas/new
/arenas/:id
/arenas/:id/edit
/arenas/:id/runs/:runId
```

---

## 3. Settings & Credentials Management

### Settings Page (`/settings`)

```
┌─────────────────────────────────────────────────────────────┐
│  SETTINGS                                                    │
│                                                              │
│  ┌──────────┬───────────────────────────────────────────┐   │
│  │          │                                           │   │
│  │ General  │  GENERAL                                  │   │
│  │          │                                           │   │
│  │ Creds    │  Display name: [José                   ]  │   │
│  │          │  Timezone:     [Europe/Madrid          ▾]  │   │
│  │ Security │  Theme:        [Dark ▾]                   │   │
│  │          │  Terminal font: [JetBrains Mono        ▾]  │   │
│  │ Danger   │  Terminal font size: [14 ▾]               │   │
│  │          │                                           │   │
│  │          │  Default machine: [Auto (least loaded) ▾] │   │
│  │          │  Default working dir: [/home/jose/repos]  │   │
│  │          │                                           │   │
│  │          │                          [Save Changes]   │   │
│  └──────────┴───────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Credentials Page (`/settings/credentials`)

```
┌─────────────────────────────────────────────────────────────┐
│  SETTINGS > CREDENTIALS                                      │
│                                                              │
│  These are injected as environment variables into every      │
│  Claude CLI session. Stored encrypted (AES-256-GCM).        │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  ANTHROPIC_API_KEY                                    │   │
│  │  sk-ant-•••••••••••••••••3kF                         │   │
│  │  Last updated: 3 days ago                             │   │
│  │                                     [Update] [Delete] │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │  GITHUB_TOKEN                                         │   │
│  │  ghp_•••••••••••••••••xY2                             │   │
│  │  Last updated: 2 weeks ago                            │   │
│  │                                     [Update] [Delete] │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │  OPENAI_API_KEY                                       │   │
│  │  sk-•••••••••••••••••p9Q                              │   │
│  │  Last updated: 1 month ago                            │   │
│  │                                     [Update] [Delete] │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  [+ Add Credential]                                          │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  ⚠ Credential Scope                                   │   │
│  │  Credentials marked "Global" are injected into every  │   │
│  │  session. You can also set per-job overrides in the   │   │
│  │  job editor.                                          │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Add/Update Credential Modal:**

```
┌────────────────────────────────────────┐
│  Add Credential                    ✕   │
│                                        │
│  Name (env var):                       │
│  [DEEPSEEK_API_KEY               ]     │
│                                        │
│  Value:                                │
│  [•••••••••••••••••••••••••••••••]     │
│  👁 Show                               │
│                                        │
│  Scope:                                │
│  ◉ Global (all sessions)              │
│  ○ Specific jobs only                  │
│    [Select jobs... ▾]                  │
│                                        │
│            [Cancel]  [Save]            │
└────────────────────────────────────────┘
```

**Key behaviors:**
- Values are never sent back to the frontend after creation. The API returns masked values (`sk-ant-•••3kF` — first 6 + last 3 chars).
- "Update" replaces the value entirely (no "edit" — you paste a new key).
- "Delete" requires confirmation dialog.
- Scope: Global credentials are injected into every session. Job-scoped credentials override globals for specific jobs (e.g., a different API key for arena runs to track cost separately).

### Security Settings (`/settings/security`)

```
┌──────────────────────────────────────────────────────────┐
│  SECURITY                                                 │
│                                                           │
│  Authentication                                           │
│  Mode: Basic Auth                                         │
│  [Change Password]                                        │
│                                                           │
│  Machine Allowlist                                        │
│  ┌────────────────────────────────────────────────────┐  │
│  │  nuc-01     ● Connected    [Revoke]                │  │
│  │  nuc-02     ● Connected    [Revoke]                │  │
│  │  zima-01    ○ Disconnected [Revoke]                │  │
│  └────────────────────────────────────────────────────┘  │
│  [+ Add Machine]                                          │
│                                                           │
│  Certificates                                             │
│  CA expires: 2028-03-11                                   │
│  [Regenerate Agent Cert]  [Download CA cert]              │
│                                                           │
│  Audit Log                                                │
│  [View full audit log →]                                  │
└──────────────────────────────────────────────────────────┘
```

### Credential Storage Backend

Already defined in the backend doc (AES-256-GCM, master key file). Adding the scoping logic:

```sql
-- Credential-to-job mapping (for job-scoped credentials)
CREATE TABLE credential_job_scope (
    credential_id  TEXT NOT NULL REFERENCES credentials(credential_id) ON DELETE CASCADE,
    job_id         TEXT NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    PRIMARY KEY (credential_id, job_id)
);
```

When building env vars for a session:
1. Start with all global credentials (those with no entries in `credential_job_scope`).
2. If the session is part of a job run, overlay job-scoped credentials (they override globals with the same name).
3. Inject into `CreateSessionCmd.env_vars`.

### REST API Additions

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/credentials` | List credentials (masked values) |
| POST | `/api/v1/credentials` | Create credential |
| PUT | `/api/v1/credentials/:id` | Update credential value |
| DELETE | `/api/v1/credentials/:id` | Delete credential |
| GET | `/api/v1/settings` | Get user settings |
| PUT | `/api/v1/settings` | Update user settings |
| PUT | `/api/v1/settings/password` | Change password |
| GET | `/api/v1/audit-log` | Paginated audit log |

---

## 4. Logging, Observability & Debugging

### 4.1 Log Architecture

Three log streams, all structured JSON:

#### Server Logs

```json
{
  "ts": "2026-03-11T09:00:01.234Z",
  "level": "info",
  "component": "orchestrator",
  "msg": "cron trigger fired",
  "job_id": "job-abc",
  "schedule_id": "sched-001",
  "cron_expr": "0 9 * * 1",
  "run_id": "run-xyz"
}
```

Components: `http`, `grpc`, `orchestrator`, `sessions`, `auth`, `db`, `ws`, `agents`.

#### Agent Logs

```json
{
  "ts": "2026-03-11T09:00:02.456Z",
  "level": "info",
  "component": "session",
  "msg": "session started",
  "machine_id": "nuc-01",
  "session_id": "sess-abc",
  "command": "claude",
  "working_dir": "/var/lib/claude-plane/workspaces/sessions/sess-abc/kodrun",
  "pid": 12345
}
```

Components: `client` (gRPC connection), `session`, `workspace`, `health`, `scrollback`.

#### Audit Log (already in DB)

Covered in the backend doc. This is the user-facing "who did what when" trail, stored in SQLite, accessible via API and UI.

### 4.2 Log Configuration

```toml
# Server or agent .toml
[logging]
level = "info"                  # debug, info, warn, error
format = "json"                 # json or text (text for local dev)
output = "stderr"               # stderr, file, both
file_path = "/var/log/claude-plane/server.log"
file_max_size_mb = 100          # Rotate at 100MB
file_max_backups = 5            # Keep 5 rotated files
file_max_age_days = 30          # Delete after 30 days
```

Use `zerolog` (Go) — zero-allocation structured logger. Fast, JSON by default, leveled.

### 4.3 Metrics (Prometheus-Compatible)

The server exposes a `/metrics` endpoint (Prometheus format). No extra infrastructure required — if you have Prometheus + Grafana, point them here. If not, the frontend dashboard covers the basics.

#### Server Metrics

```
# Agent connections
claude_plane_agents_connected{} 2
claude_plane_agent_last_seen_seconds{machine_id="nuc-01"} 2

# Sessions
claude_plane_sessions_active{machine_id="nuc-01"} 3
claude_plane_sessions_total{status="completed"} 142
claude_plane_sessions_total{status="failed"} 8
claude_plane_session_duration_seconds_bucket{le="60"} 12
claude_plane_session_duration_seconds_bucket{le="300"} 45
claude_plane_session_duration_seconds_bucket{le="3600"} 120

# Jobs and runs
claude_plane_runs_active{} 2
claude_plane_runs_total{status="completed",trigger="cron"} 30
claude_plane_runs_total{status="failed",trigger="cron"} 3

# WebSocket connections
claude_plane_ws_terminal_connections{} 4
claude_plane_ws_event_connections{} 2

# gRPC
claude_plane_grpc_stream_messages_sent_total{machine_id="nuc-01"} 15234
claude_plane_grpc_stream_messages_received_total{machine_id="nuc-01"} 89012

# API latency
claude_plane_http_request_duration_seconds_bucket{method="GET",path="/api/v1/sessions",le="0.01"} 500
```

#### Agent Metrics (reported to server via health events, also exposed locally)

```
# Machine resources
claude_plane_agent_cpu_usage_percent{machine_id="nuc-01"} 34.5
claude_plane_agent_memory_used_bytes{machine_id="nuc-01"} 4294967296
claude_plane_agent_disk_free_bytes{machine_id="nuc-01"} 107374182400

# Session processes
claude_plane_agent_sessions_active{machine_id="nuc-01"} 3
claude_plane_agent_pty_read_bytes_total{session_id="sess-abc"} 1048576
claude_plane_agent_scrollback_bytes{session_id="sess-abc"} 524288

# Workspace
claude_plane_agent_workspace_disk_bytes{machine_id="nuc-01"} 5368709120
claude_plane_agent_workspace_count{machine_id="nuc-01"} 3
```

### 4.4 Health Check Endpoints

```
GET /healthz          → 200 OK (server is running)
GET /readyz           → 200 OK if DB is accessible and at least 1 agent connected
                        503 if DB unreachable or 0 agents
```

Agent has a similar local endpoint (configurable port):
```
GET http://localhost:9444/healthz  → 200 OK
```

### 4.5 Frontend: Observability View

Accessible from Settings or as a dedicated debug page (`/settings/observability`):

```
┌──────────────────────────────────────────────────────────┐
│  OBSERVABILITY                                            │
│                                                           │
│  System Health                                            │
│  Server uptime: 14 days 3 hours                           │
│  DB size: 24 MB                                           │
│  Active WebSockets: 4 (2 terminal, 2 event)               │
│  Active gRPC streams: 2                                   │
│                                                           │
│  ┌──────────────────────────────────────────────────┐    │
│  │  Agent Health (live)                              │    │
│  │                                                   │    │
│  │  Machine    Status    CPU   RAM   Disk  Sessions  │    │
│  │  nuc-01     ● Up      34%   48%   22%   3/5      │    │
│  │  nuc-02     ● Up      12%   31%   45%   1/5      │    │
│  │  zima-01    ○ Down    —     —     —     —        │    │
│  └──────────────────────────────────────────────────┘    │
│                                                           │
│  ┌──────────────────────────────────────────────────┐    │
│  │  Recent Server Logs (tail -f style, live)         │    │
│  │                                                   │    │
│  │  09:01:23 INFO  orchestrator  Step completed      │    │
│  │  09:01:22 INFO  grpc          Output chunk sent   │    │
│  │  09:01:20 INFO  sessions      Session attached    │    │
│  │  09:01:15 WARN  agents        Health check late   │    │
│  │                                                   │    │
│  │  Level: [All ▾]  Component: [All ▾]  [Filter]    │    │
│  └──────────────────────────────────────────────────┘    │
│                                                           │
│  ┌──────────────────────────────────────────────────┐    │
│  │  Session Duration (last 7 days)                   │    │
│  │  ┌─────────────────────────────────────────┐     │    │
│  │  │         ╱╲                               │     │    │
│  │  │   ╱╲  ╱    ╲     ╱╲                     │     │    │
│  │  │  ╱  ╲╱      ╲╱╲╱  ╲                    │     │    │
│  │  │ ╱                    ╲───                │     │    │
│  │  └─────────────────────────────────────────┘     │    │
│  │  Mon  Tue  Wed  Thu  Fri  Sat  Sun               │    │
│  └──────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────┘
```

**Live server logs** are streamed via the event WebSocket (new event type: `log`). Only in this view — not broadcasted when you're on other pages (to save bandwidth).

### 4.6 Debugging Common Issues

Built-in diagnostics exposed via CLI:

```bash
# Server-side
claude-plane-server diagnose agents    # List agents, connection state, cert expiry
claude-plane-server diagnose sessions  # List active sessions, PIDs, scrollback sizes
claude-plane-server diagnose db        # DB integrity check, table sizes, WAL size
claude-plane-server diagnose certs     # Validate CA chain, check expirations

# Agent-side
claude-plane-agent diagnose connection  # Test gRPC connection to server
claude-plane-agent diagnose sessions    # List local sessions, PTY state, scrollback files
claude-plane-agent diagnose workspace   # List workspaces, disk usage, stale worktrees
claude-plane-agent diagnose claude      # Verify Claude CLI is installed and callable
```

---

## 5. Security Hardening

### 5.1 HTTP Security Headers

```go
func securityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-XSS-Protection", "0")  // Modern browsers, CSP is better
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        w.Header().Set("Content-Security-Policy",
            "default-src 'self'; "+
            "script-src 'self'; "+
            "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
            "font-src 'self' https://fonts.gstatic.com; "+
            "connect-src 'self' wss:; "+  // WebSocket connections
            "img-src 'self' data:; "+
            "frame-ancestors 'none'")
        w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
        next.ServeHTTP(w, r)
    })
}
```

### 5.2 Rate Limiting

```go
// Per-endpoint rate limits
rateLimits := map[string]rate.Limit{
    "/api/v1/sessions":    rate.Every(time.Second),     // 1 session creation per second
    "/api/v1/credentials": rate.Every(5 * time.Second), // 1 credential op per 5s
    "/api/v1/*/runs":      rate.Every(2 * time.Second), // 1 run trigger per 2s
    "default":             rate.Every(100 * time.Millisecond), // 10 req/s general
}

// Login endpoint: strict rate limit to prevent brute force
"/api/v1/auth/login": rate.Every(3 * time.Second) // Max ~20 attempts per minute
// After 10 consecutive failures: lock for 5 minutes
```

### 5.3 CORS Policy

```go
cors := cors.New(cors.Options{
    AllowedOrigins:   []string{"https://localhost:8443"}, // Only the server itself
    AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE"},
    AllowedHeaders:   []string{"Authorization", "Content-Type"},
    AllowCredentials: true,
    MaxAge:           3600,
})
```

In production with a custom domain, set `AllowedOrigins` to that domain. For single-binary serving (frontend embedded in Go), CORS isn't strictly necessary since everything is same-origin — but it's good defense in depth.

### 5.4 Session Token Management

```go
type AuthToken struct {
    UserID    string    `json:"uid"`
    IssuedAt  time.Time `json:"iat"`
    ExpiresAt time.Time `json:"exp"`
}

const (
    tokenExpiry   = 24 * time.Hour  // Token valid for 24h
    refreshWindow = 1 * time.Hour   // Can refresh in last hour
)
```

Tokens are HMAC-SHA256 signed (not JWT to avoid complexity — simple custom tokens). Stored in an httpOnly, secure, sameSite=strict cookie.

**Refresh flow:** If the token is within the refresh window (last hour before expiry), any API request automatically returns a new token in the `Set-Cookie` header. No explicit refresh endpoint needed.

### 5.5 Agent Binary Without Cert

If someone obtains the agent binary but not a valid certificate:
- The binary is useless — it can't connect to the server without a cert signed by the server's CA.
- The agent binary contains no secrets. All configuration is external (TOML file + cert files).
- Even if they reverse-engineer the binary, they can't forge a cert without the CA private key.

If a cert is compromised:
1. Remove the machine-id from the server's allowlist → immediate rejection on next reconnect.
2. Regenerate a new cert for that machine via `claude-plane-server ca issue-agent`.
3. The compromised cert is now useless (not in allowlist) even if it's technically valid against the CA.

For extra protection, add a Certificate Revocation List (CRL) check — but for V1 with 2-5 machines, the allowlist approach is sufficient.

### 5.6 Input Validation & Sanitization

```go
// All user inputs validated before use
func validateSessionCreate(req CreateSessionRequest) error {
    if !isValidPath(req.WorkingDir) {
        return errors.New("invalid working directory path")
    }
    if req.WorkingDir != "" && strings.Contains(req.WorkingDir, "..") {
        return errors.New("path traversal not allowed")
    }
    if len(req.Command) > 256 {
        return errors.New("command too long")
    }
    // Allowlist for commands — only claude-related binaries
    allowedCommands := []string{"claude", "/usr/local/bin/claude"}
    if !slices.Contains(allowedCommands, req.Command) {
        return errors.New("command not in allowlist")
    }
    return nil
}
```

**Command allowlist** is critical. Without it, the session creation API becomes an RCE vector. Only `claude` (or configured paths) are allowed as session commands.

### 5.7 Env Var Safety

Credentials injected as env vars must not leak:

```go
// Agent: strip env vars from any logging
func sanitizeForLog(env map[string]string) map[string]string {
    safe := make(map[string]string, len(env))
    sensitiveKeys := []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL"}
    for k, v := range env {
        isSensitive := false
        for _, s := range sensitiveKeys {
            if strings.Contains(strings.ToUpper(k), s) {
                isSensitive = true
                break
            }
        }
        if isSensitive {
            safe[k] = v[:min(6, len(v))] + "•••" + v[max(0, len(v)-3):]
        } else {
            safe[k] = v
        }
    }
    return safe
}
```

---

## 6. Operational Concerns

### 6.1 Database Backup

SQLite with WAL mode. Backup strategy:

```go
// Built into the server — daily automated backup
func (s *Server) backupLoop(ctx context.Context) {
    ticker := time.NewTicker(24 * time.Hour)
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.backupDB()
        }
    }
}

func (s *Server) backupDB() error {
    backupPath := fmt.Sprintf("%s/backups/server-%s.db",
        s.config.DataDir,
        time.Now().Format("2006-01-02"))

    // SQLite online backup API — safe while server is running
    src, _ := sql.Open("sqlite3", s.config.DBPath)
    dst, _ := sql.Open("sqlite3", backupPath)

    srcConn, _ := src.Conn(context.Background())
    dstConn, _ := dst.Conn(context.Background())

    return sqlite3.BackupDB(dstConn, "main", srcConn, "main")
}
```

Also exposed via CLI:
```bash
claude-plane-server backup --output /path/to/backup.db
```

**Retention:** Keep 7 daily backups, 4 weekly backups. Configurable.

**External backup:** The backup file is a standard SQLite database. Copy it off-machine with `scp`, rsync, or your existing backup system.

### 6.2 Disaster Recovery

**Server dies, data intact (disk survives):**
1. Install `claude-plane-server` on new machine (or same machine).
2. Point config at the existing data directory.
3. Start server. Agents reconnect automatically.
4. All sessions, jobs, runs, credentials are intact.

**Server dies, data lost:**
1. Install `claude-plane-server` on new machine.
2. Restore from latest backup: `claude-plane-server restore --from /path/to/backup.db`
3. Re-generate server cert (new machine, new identity): `claude-plane-server ca issue-server`
4. Update agents' config with new server address (if it changed).
5. Agents reconnect. Active sessions on agents are still running — server reconciles via `Register()`.

**Agent dies:**
1. Sessions on that agent are lost (processes died with the machine).
2. Server marks sessions as `failed` after grace period.
3. Scrollback files for those sessions are lost (they lived on the agent's disk).
4. Fix the machine, restart the agent. It reconnects and is ready for new sessions.

**Data you cannot recover:**
- Scrollback files live on agents, not the server. If an agent's disk dies, session recordings are gone.
- Mitigation for V2: agents can optionally stream scrollback files to the server for archival.

### 6.3 Server Migration

```bash
# On old server
claude-plane-server backup --output /tmp/migration.db
tar czf /tmp/claude-plane-data.tar.gz /etc/claude-plane/ /var/lib/claude-plane/

# On new server
tar xzf /tmp/claude-plane-data.tar.gz -C /
claude-plane-server restore --from /tmp/migration.db
# Update DNS/IP if server address changed
# Update agent configs if needed
claude-plane-server serve
```

If the server address changes, agents need their config updated (`server.address` in agent.toml). This is the one manual step — everything else is portable.

---

## 7. Integration Points

### 7.1 Kodrun Integration

claude-plane and Kodrun are complementary:

- **Kodrun**: Autonomous agent sessions. Fire-and-forget. No terminal UI.
- **claude-plane**: Interactive/semi-interactive sessions. Terminal-first. Human-in-the-loop.

**Integration point:** Kodrun can trigger claude-plane jobs (and vice versa) via the REST API.

```bash
# Kodrun session finishes, triggers a claude-plane review job
curl -X POST https://controlplane:8443/api/v1/jobs/review-job/runs \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"trigger_type": "api", "trigger_detail": "kodrun-run-xyz"}'
```

**Deeper integration (V3+):** claude-plane could display Kodrun sessions alongside native sessions. Kodrun would need to expose session output in a compatible format (asciicast or similar). This is a significant effort — park it.

### 7.2 claude-plane CLI

A lightweight CLI for interacting with the control plane from your terminal:

```bash
# List active sessions
claude-plane sessions list

# Create a new session
claude-plane sessions create --machine nuc-01 --dir /repos/kodrun

# Attach to a session (opens a local terminal, proxied through the server)
claude-plane sessions attach sess-abc

# Run a job
claude-plane jobs run "Kodrun V2 Planning"

# Check run status
claude-plane runs list --active

# Trigger from scripts
claude-plane jobs run "Nightly Tests" --wait --exit-code
# Exits with the run's final status code (0 = all steps succeeded)
```

**Implementation:** The CLI is a third Go binary (`claude-plane-cli`) that talks to the server's REST API. For `sessions attach`, it opens a WebSocket and bridges it to the local terminal's stdin/stdout — you get a real terminal experience without the browser.

```go
// cmd/cli/main.go
func main() {
    app := &cli.App{
        Name: "claude-plane",
        Commands: []*cli.Command{
            {Name: "sessions", Subcommands: []*cli.Command{
                {Name: "list", Action: sessionsList},
                {Name: "create", Action: sessionsCreate},
                {Name: "attach", Action: sessionsAttach},
                {Name: "kill", Action: sessionsKill},
            }},
            {Name: "jobs", Subcommands: []*cli.Command{
                {Name: "list", Action: jobsList},
                {Name: "run", Action: jobsRun},
            }},
            {Name: "runs", Subcommands: []*cli.Command{
                {Name: "list", Action: runsList},
                {Name: "status", Action: runsStatus},
            }},
            {Name: "machines", Subcommands: []*cli.Command{
                {Name: "list", Action: machinesList},
                {Name: "drain", Action: machinesDrain},
            }},
        },
    }
    app.Run(os.Args)
}
```

### 7.3 Webhooks

Outbound webhooks for external integrations. Configurable per event type.

```sql
CREATE TABLE webhooks (
    webhook_id     TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    url            TEXT NOT NULL,             -- Target URL
    secret         BLOB,                      -- HMAC secret for signature verification
    events         TEXT NOT NULL,             -- JSON array of event types to subscribe to
    -- ["run.completed", "run.failed", "session.failed", "machine.disconnected"]
    enabled        BOOLEAN NOT NULL DEFAULT 1,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**Webhook payload:**

```json
{
  "event": "run.completed",
  "timestamp": "2026-03-11T09:15:00Z",
  "data": {
    "run_id": "run-xyz",
    "job_id": "job-abc",
    "job_name": "Kodrun V2 Planning",
    "status": "completed",
    "trigger_type": "cron",
    "duration_seconds": 2700,
    "steps": [
      {"name": "Generate PRD", "status": "completed", "duration_seconds": 720},
      {"name": "Generate TRD", "status": "completed", "duration_seconds": 900},
      {"name": "Impl Plan", "status": "completed", "duration_seconds": 1080}
    ]
  }
}
```

**Signature:** HMAC-SHA256 of the payload, sent in `X-Claude-Plane-Signature` header. Receiver can verify authenticity.

**Use cases:**
- Slack notification on run failure
- GitHub status check on run completion
- PagerDuty alert on machine disconnection
- Custom dashboards consuming events

**Retry policy:** 3 retries with exponential backoff (1s, 5s, 30s). After 3 failures, mark delivery as failed (visible in webhook logs UI).

**Frontend: Webhook management** (`/settings/webhooks`):
```
┌──────────────────────────────────────────────────────────┐
│  WEBHOOKS                                [+ Add Webhook]  │
│                                                           │
│  Slack Alerts                                             │
│  https://hooks.slack.com/services/T.../B.../...           │
│  Events: run.failed, machine.disconnected                 │
│  Last delivery: 2h ago ✓                                  │
│                                          [Edit] [Delete]  │
└──────────────────────────────────────────────────────────┘
```

---

## 8. Job System Edge Cases

### 8.1 Editing a Job While a Run Is In Progress

**Rule:** A running run uses a **snapshot** of the job definition at the time it was created. Edits to the job do not affect in-progress runs.

**Implementation:** When a run is created, copy the step definitions into `run_steps` with all config fields (prompt, machine, working_dir, args, etc.). The run references these copies, not the live job/step records.

```sql
-- run_steps already has the relevant fields, but make them explicit copies:
ALTER TABLE run_steps ADD COLUMN prompt_snapshot TEXT;
ALTER TABLE run_steps ADD COLUMN machine_id_snapshot TEXT;
ALTER TABLE run_steps ADD COLUMN working_dir_snapshot TEXT;
ALTER TABLE run_steps ADD COLUMN command_snapshot TEXT;
ALTER TABLE run_steps ADD COLUMN args_snapshot TEXT;
```

When creating a run:
```go
func (o *Orchestrator) createRunSteps(runID string, jobID string) {
    steps := o.db.GetSteps(jobID)
    for _, step := range steps {
        o.db.InsertRunStep(RunStep{
            RunStepID:         uuid.New().String(),
            RunID:             runID,
            StepID:            step.StepID,
            Status:            "pending",
            PromptSnapshot:    step.Prompt,
            MachineIDSnapshot: step.MachineID,
            WorkingDirSnapshot: step.WorkingDir,
            CommandSnapshot:   step.Command,
            ArgsSnapshot:      step.Args,
        })
    }
}
```

**UX:** The job editor shows a warning banner if there's an active run: "A run is in progress. Changes will apply to future runs only."

### 8.2 Re-Running a Single Failed Step

**Supported.** From the run detail view, a failed step has a "Retry" button.

```
POST /api/v1/runs/:runId/steps/:stepId/retry
```

This:
1. Resets the run_step status to `pending`.
2. Re-evaluates DAG (the step's dependencies are already `completed`, so it's immediately runnable).
3. Creates a new session with the same config (from the snapshot).
4. Does NOT re-run upstream steps.

**What about downstream steps?** If the failed step had dependents that were `skipped` or `cancelled`, they become `pending` again and will run when the retried step completes.

### 8.3 Cloning / Templating Jobs

**Clone:** Duplicate a job with all its steps and dependencies, under a new name.

```
POST /api/v1/jobs/:id/clone
{ "name": "Kodrun V2 Planning (copy)" }
```

Returns a new job with identical steps and deps, no schedules or triggers (those are intentionally not cloned to avoid duplicate cron fires).

**Templates (V2):** A job can be marked as a template. Templates appear in a "New Job from Template" flow. Useful for standardized workflows: "PRD → TRD → Plan" template that you instantiate for different repos.

### 8.4 Job-Level Failure Policies

Each step can have a failure policy:

```sql
ALTER TABLE steps ADD COLUMN on_failure TEXT NOT NULL DEFAULT 'fail_run';
-- 'fail_run': Mark the entire run as failed, cancel pending steps
-- 'skip_dependents': Mark this step failed, skip everything downstream, continue parallel branches
-- 'continue': Mark this step failed, but still trigger dependents (they might handle it)
-- 'retry_once': Retry the step once automatically before applying the above
```

**Frontend:** Dropdown in the step editor: "On failure: [Fail entire run ▾]"

### 8.5 Manual Approval Gates

A step can be configured to require manual approval before executing:

```sql
ALTER TABLE steps ADD COLUMN requires_approval BOOLEAN NOT NULL DEFAULT 0;
```

When the DAG runner encounters an approval-gated step:
1. Set status to `waiting_approval`.
2. Send a notification (WebSocket event, optional webhook).
3. The step stays blocked until a user approves it via the UI or API.

```
POST /api/v1/runs/:runId/steps/:stepId/approve
POST /api/v1/runs/:runId/steps/:stepId/reject
```

**Frontend:** The run detail shows an approval button on waiting steps:
```
┌───────────────┐
│ ⏸ Deploy      │
│ Waiting for   │
│ approval      │
│ [Approve] [✕] │
└───────────────┘
```

This is powerful for production-facing workflows: "Generate the code → wait for my review → deploy."

---

## 9. Agent Provisioning & Upgrades

### 9.1 Initial Provisioning

The server generates a one-liner provisioning script:

```bash
# On the server, generate a provisioning command for a new agent
claude-plane-server provision agent \
  --machine-id "nuc-03" \
  --arch "linux-amd64" \
  --output-script /tmp/provision-nuc-03.sh
```

The script contains:
1. Download the agent binary (from the server itself, served at `/dl/agent/<os>-<arch>`).
2. Create the cert directory.
3. Write the agent cert + key + CA cert (embedded in the script, base64 encoded).
4. Write the agent.toml config.
5. Install systemd service.
6. Start the agent.

```bash
#!/bin/bash
# Auto-generated provisioning script for nuc-03
set -euo pipefail

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/claude-plane"
DATA_DIR="/var/lib/claude-plane"

# Download agent binary
curl -fSL https://controlplane.local:8443/dl/agent/linux-amd64 -o $INSTALL_DIR/claude-plane-agent
chmod +x $INSTALL_DIR/claude-plane-agent

# Install certificates
mkdir -p $CONFIG_DIR/certs
echo "BASE64_CA_CERT" | base64 -d > $CONFIG_DIR/certs/ca.pem
echo "BASE64_AGENT_CERT" | base64 -d > $CONFIG_DIR/certs/agent.pem
echo "BASE64_AGENT_KEY" | base64 -d > $CONFIG_DIR/certs/agent-key.pem
chmod 600 $CONFIG_DIR/certs/agent-key.pem

# Write config
cat > $CONFIG_DIR/agent.toml << 'EOF'
[server]
address = "controlplane.local:9443"
[tls]
ca_cert = "/etc/claude-plane/certs/ca.pem"
agent_cert = "/etc/claude-plane/certs/agent.pem"
agent_key = "/etc/claude-plane/certs/agent-key.pem"
[agent]
machine_id = "nuc-03"
data_dir = "/var/lib/claude-plane"
max_sessions = 5
claude_cli_path = "/usr/local/bin/claude"
EOF

# Create data directory
mkdir -p $DATA_DIR

# Install systemd service
cat > /etc/systemd/system/claude-plane-agent.service << 'EOF'
[Unit]
Description=claude-plane Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/claude-plane-agent run
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now claude-plane-agent

echo "Agent provisioned and started."
```

**SSH into the machine, paste the script, done.** One command to provision a new agent.

### 9.2 Upgrades

**Agent upgrades:**

The server can push updates to agents. When you upgrade the server binary, it includes the latest agent binary (embedded).

```bash
# Server CLI
claude-plane-server upgrade agents --version latest
```

This:
1. For each connected agent, sends an `UpgradeCmd` via gRPC.
2. Agent downloads the new binary from the server (`/dl/agent/<os>-<arch>?version=latest`).
3. Agent replaces its own binary (writes to a temp file, renames atomically).
4. Agent restarts itself (exec syscall, or systemd restart).
5. Active sessions are NOT affected — the agent restarts, sessions keep running in their PTYs.

Wait — sessions surviving agent restart needs care. The agent would need to re-adopt orphaned PTY processes. This is complex.

**Simpler V1 approach:**
1. Drain the machine (finish active sessions, no new ones).
2. Replace the agent binary.
3. Restart the agent.
4. Undrain.

The `drain` command in the frontend or CLI does this gracefully.

**Server upgrades:**
1. Stop the server.
2. Replace the binary.
3. Start the server.
4. Agents reconnect automatically (they retry with backoff).
5. Frontend is embedded, so it's updated too.

### 9.3 Version Compatibility

The `Register()` RPC includes `server_version` in the response. The agent should check compatibility:

```go
// Agent checks server version on register
if !isCompatible(response.ServerVersion, agentVersion) {
    log.Warn().
        Str("server_version", response.ServerVersion).
        Str("agent_version", agentVersion).
        Msg("version mismatch — consider upgrading agent")
}
```

**Compatibility policy:** Server is always backward-compatible with agents up to 2 minor versions behind. Major version changes require agent upgrades.

---

## 10. Cost Tracking & Token Analytics

### 10.1 Data Collection

Claude CLI doesn't expose token usage in a machine-readable way (as of now). Two approaches:

**Approach A: Parse CLI output**

Claude CLI may print token usage in its output (e.g., at the end of a conversation). The agent can scan the scrollback for patterns like:

```
Token usage: 15,234 input / 8,456 output
```

This is fragile — it depends on CLI output format, which can change.

**Approach B: API-level tracking (better)**

If sessions use an Anthropic API key, the Anthropic API returns token usage in response headers or response bodies. But we don't intercept API calls — the Claude CLI makes them directly.

**Approach C: Proxy the API (most reliable)**

Run a lightweight HTTP proxy on each agent that sits between the Claude CLI and the Anthropic API. The proxy:
1. Forwards all requests to `api.anthropic.com`.
2. Reads response headers/body for token usage.
3. Logs usage per session.
4. Reports to the server.

```toml
# Agent config
[proxy]
enabled = true
listen = "127.0.0.1:9445"
# CLI uses ANTHROPIC_BASE_URL=http://127.0.0.1:9445 instead of the real API
```

The agent injects `ANTHROPIC_BASE_URL=http://127.0.0.1:9445` into the session's env vars.

**This is the cleanest approach** — it works regardless of CLI output format, captures exact token counts, and can track cost per model.

### 10.2 Data Model

```sql
CREATE TABLE token_usage (
    usage_id       INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id     TEXT NOT NULL REFERENCES sessions(session_id),
    machine_id     TEXT NOT NULL,
    timestamp      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    model          TEXT NOT NULL,          -- "claude-opus-4", "claude-sonnet-4", etc.
    input_tokens   INTEGER NOT NULL,
    output_tokens  INTEGER NOT NULL,
    cache_read_tokens INTEGER DEFAULT 0,
    cache_write_tokens INTEGER DEFAULT 0,
    cost_usd       REAL                    -- Computed from model pricing table
);

CREATE INDEX idx_usage_session ON token_usage(session_id);
CREATE INDEX idx_usage_time ON token_usage(timestamp);

-- Model pricing (admin-managed)
CREATE TABLE model_pricing (
    model          TEXT PRIMARY KEY,
    input_per_mtok REAL NOT NULL,          -- Cost per million input tokens (USD)
    output_per_mtok REAL NOT NULL,         -- Cost per million output tokens (USD)
    cache_read_per_mtok REAL,
    cache_write_per_mtok REAL,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Pre-populate with known pricing
INSERT INTO model_pricing VALUES
    ('claude-opus-4', 15.0, 75.0, 1.5, 3.75, CURRENT_TIMESTAMP),
    ('claude-sonnet-4', 3.0, 15.0, 0.3, 3.75, CURRENT_TIMESTAMP);
```

### 10.3 Cost Analytics Frontend

New section on the Command Center and a dedicated page (`/analytics`):

**Command Center widget:**

```
┌──────────────────────────────────────────┐
│  COST (Last 7 Days)                      │
│                                          │
│  Total: $47.23                           │
│  ┌──────────────────────────────────┐    │
│  │  ██████  Opus      $38.50 (81%) │    │
│  │  ██      Sonnet    $8.73  (19%) │    │
│  └──────────────────────────────────┘    │
│  Tokens: 2.1M input / 890K output       │
│                              [Details →] │
└──────────────────────────────────────────┘
```

**Analytics page (`/analytics`):**

```
┌─────────────────────────────────────────────────────────────┐
│  ANALYTICS                      Period: [Last 7 days ▾]     │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Daily Cost                                           │   │
│  │  $12 ┤                        ╱╲                     │   │
│  │   $8 ┤              ╱╲      ╱    ╲                   │   │
│  │   $4 ┤    ╱╲      ╱    ╲  ╱      ╲──                │   │
│  │   $0 ┤──╱    ╲──╱                                    │   │
│  │       Mon  Tue  Wed  Thu  Fri  Sat  Sun              │   │
│  │       [■ Opus  ■ Sonnet]                             │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌────────────────────────┐  ┌────────────────────────┐     │
│  │  BY MODEL              │  │  BY JOB                 │     │
│  │  Opus:    $38.50       │  │  Kodrun V2:   $22.10   │     │
│  │  Sonnet:  $8.73        │  │  Nightly:     $5.30    │     │
│  │                        │  │  Manual:      $19.83   │     │
│  └────────────────────────┘  └────────────────────────┘     │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  MOST EXPENSIVE SESSIONS                              │   │
│  │  sess-abc  Kodrun V2 / PRD   Opus   45m   $8.50     │   │
│  │  sess-def  Manual             Opus   2h    $6.20     │   │
│  │  sess-ghi  Arena / Shootout   Opus   12m   $3.40     │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## 11. Phased Roadmap

Based on everything designed above, here's the full scope broken into phases. Every feature is placed where it makes sense for dependencies and incremental value.

### V1 — "It works for me" (Personal, Single-User)

**Goal:** A functional control plane José can use across his homelab machines.

**Backend:**
- [ ] Server binary with embedded frontend
- [ ] Agent binary with PTY management and scrollback
- [ ] mTLS with built-in CA tooling
- [ ] gRPC protocol (register, create/attach/detach/kill session, resize, I/O streaming)
- [ ] Scrollback buffering (asciicast v2)
- [ ] Session reconnection with scrollback replay
- [ ] SQLite database (sessions, machines)
- [ ] REST API: sessions, machines
- [ ] Basic auth (single user, password hash)
- [ ] Server health endpoints (`/healthz`, `/readyz`)
- [ ] Structured JSON logging (zerolog)
- [ ] Machine allowlist
- [ ] Agent provisioning script generator
- [ ] CLI diagnose commands (server + agent)

**Frontend:**
- [ ] App shell (sidebar, top bar, status bar)
- [ ] Command center (machines, active sessions, activity feed)
- [ ] Sessions list + terminal view (xterm.js, WebSocket, live I/O)
- [ ] Session replay (asciicast player with playback controls)
- [ ] New session modal (machine picker, working dir)
- [ ] Machines view (health bars, session list)
- [ ] Real-time event WebSocket (live status updates)
- [ ] Keyboard shortcuts (Cmd+K, Cmd+N, Cmd+W)
- [ ] Dark theme

**Scope OUT of V1:**
- Jobs, runs, DAG system
- Arena mode
- Credentials management (hardcode in agent env for now)
- Workspace isolation (use raw directories)
- Cost tracking
- Webhooks
- CLI tool
- Multi-user
- Metrics endpoint
- Responsive mobile view (desktop/laptop only)

---

### V2 — "Jobs and isolation" (Productive Daily Driver)

**Goal:** Job orchestration, workspace isolation, proper credential management.

**Backend:**
- [ ] Job + step + dependency data model
- [ ] DAG runner with parallel execution
- [ ] Cron scheduler
- [ ] Cross-job triggers
- [ ] Run management (create, cancel, retry step)
- [ ] Workspace isolation (git worktree per session)
- [ ] Credential storage (AES-256-GCM encrypted)
- [ ] Credential injection into sessions
- [ ] Job snapshot on run creation (edits don't affect active runs)
- [ ] Manual approval gates
- [ ] Step failure policies (fail_run, skip_dependents, continue, retry_once)
- [ ] REST API: jobs, steps, runs, schedules, triggers, credentials
- [ ] Agent: workspace manager
- [ ] DB backup (daily automated + CLI command)
- [ ] Prometheus metrics endpoint

**Frontend:**
- [ ] Jobs list + job editor (ReactFlow DAG canvas + step editor)
- [ ] Cron input with human-readable preview
- [ ] Cross-job trigger builder
- [ ] Runs list + run detail (DAG status + embedded terminals)
- [ ] Step retry from run detail
- [ ] Approval gate UI
- [ ] Settings page (general settings)
- [ ] Credentials management page
- [ ] Command palette (Cmd+Shift+P)
- [ ] Session replay improvements (seek, speed control)
- [ ] Responsive: laptop layout

---

### V3 — "Arena and analytics" (Power Features)

**Goal:** Model competition, cost awareness, external integrations.

**Backend:**
- [ ] Arena data model (arenas, contestants, arena_runs, entries)
- [ ] Arena execution engine (parallel session spawning)
- [ ] Cost tracking proxy (agent-side HTTP proxy between CLI and API)
- [ ] Token usage data model + cost computation
- [ ] Webhooks (outbound, HMAC-signed, retry logic)
- [ ] claude-plane CLI tool (third binary)
- [ ] Agent self-upgrade support (drain → replace → restart)
- [ ] Observability: live log streaming to frontend

**Frontend:**
- [ ] Arena list + editor + run comparison view
- [ ] Side-by-side terminal replay (synchronized playback)
- [ ] Analytics page (cost charts, token usage, by-model/by-job breakdowns)
- [ ] Command center cost widget
- [ ] Webhook management in settings
- [ ] Observability page (agent health, live logs, metrics)
- [ ] Responsive: tablet + phone (read-only status views)

---

### V4 — "Team-ready" (Multi-User, Polish)

**Goal:** Multiple users, enterprise-ish features, production hardening.

**Backend:**
- [ ] OIDC / OAuth2 authentication (GitHub, Google, Keycloak)
- [ ] Multi-user: per-user credentials, session isolation, audit per user
- [ ] RBAC (admin, operator, viewer roles)
- [ ] Job templates (create-from-template flow)
- [ ] Scrollback archival (agents → server for long-term storage)
- [ ] Rate limiting on all API endpoints
- [ ] CRL (Certificate Revocation List) for agent certs
- [ ] Server HA (optional Postgres backend, multi-server)
- [ ] Agent: directory listing RPC (for workspace browser in frontend)

**Frontend:**
- [ ] User management page
- [ ] Role-based UI (viewers can't create sessions, operators can, admins manage everything)
- [ ] Job template library
- [ ] Light theme option
- [ ] Workspace browser (tree view of remote directories)
- [ ] Session sharing (multiple users viewing same terminal)
- [ ] Export: session recordings as .cast files, run reports as PDF
- [ ] Onboarding flow (first-time setup wizard)

---

## Appendix: Updated Navigation Structure (All Phases)

```
Sidebar (V1 → V4 progression):

V1:
  ○ Command Center
  ○ Sessions
  ○ Machines

V2 adds:
  ○ Jobs
  ○ Runs

V3 adds:
  ○ Arenas
  ○ Analytics

V4 adds:
  ○ Users (admin only)

Bottom:
  ⚙ Settings
```

## Appendix: Updated Database Schema (All Tables)

For reference, the complete list of tables across all phases:

**V1:** machines, sessions, audit_log
**V2 adds:** jobs, steps, step_dependencies, runs, run_steps, cron_schedules, job_triggers, credentials, credential_job_scope
**V3 adds:** arenas, arena_contestants, arena_runs, arena_run_entries, token_usage, model_pricing, webhooks
**V4 adds:** users, roles, user_roles

## Appendix: Full REST API Surface (All Phases)

**V1:**
- `GET/POST /api/v1/sessions`
- `GET/DELETE /api/v1/sessions/:id`
- `GET /api/v1/sessions/:id/recording`
- `GET /api/v1/machines`
- `PUT /api/v1/machines/:id`
- `POST /api/v1/machines/:id/drain`
- `POST /api/v1/machines/:id/undrain`
- `GET /healthz`
- `GET /readyz`
- `WS /ws/terminal/:sessionId`
- `WS /ws/events`

**V2 adds:**
- `GET/POST /api/v1/jobs`
- `GET/PUT/DELETE /api/v1/jobs/:id`
- `POST /api/v1/jobs/:id/clone`
- `POST /api/v1/jobs/:id/runs`
- `POST/PUT/DELETE /api/v1/jobs/:id/steps`
- `POST/DELETE /api/v1/jobs/:jid/steps/:sid/deps`
- `GET/POST /api/v1/jobs/:id/schedules`
- `PUT/DELETE /api/v1/schedules/:id`
- `GET/POST /api/v1/jobs/:id/triggers`
- `DELETE /api/v1/triggers/:id`
- `GET /api/v1/runs`
- `GET /api/v1/runs/:id`
- `POST /api/v1/runs/:id/cancel`
- `POST /api/v1/runs/:rid/steps/:sid/retry`
- `POST /api/v1/runs/:rid/steps/:sid/approve`
- `POST /api/v1/runs/:rid/steps/:sid/reject`
- `GET/POST/PUT/DELETE /api/v1/credentials`
- `GET/PUT /api/v1/settings`
- `PUT /api/v1/settings/password`
- `GET /api/v1/audit-log`
- `GET /metrics`

**V3 adds:**
- `GET/POST /api/v1/arenas`
- `GET/PUT/DELETE /api/v1/arenas/:id`
- `POST/PUT/DELETE /api/v1/arenas/:aid/contestants`
- `POST /api/v1/arenas/:id/runs`
- `GET /api/v1/arenas/:id/runs`
- `GET /api/v1/arena-runs/:id`
- `PUT /api/v1/arena-runs/:id/judge`
- `GET /api/v1/analytics/cost`
- `GET /api/v1/analytics/tokens`
- `GET/POST/PUT/DELETE /api/v1/webhooks`

**V4 adds:**
- `GET/POST/PUT/DELETE /api/v1/users`
- `GET/PUT /api/v1/users/:id/roles`
- `GET /api/v1/job-templates`
- `POST /api/v1/job-templates/:id/instantiate`
