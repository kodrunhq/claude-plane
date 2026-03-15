# Server-Side Session Content Persistence & Search — Design Spec

**Date:** 2026-03-15
**Status:** Approved

## Overview

Replace the stubbed agent-side search fan-out with server-side session content persistence. Terminal output is ingested into SQLite FTS5 as it streams through the server, enabling instant full-text search across all sessions regardless of agent connectivity. Includes data retention with configurable cleanup of both server content and agent `.cast` files.

---

## 1. Data Flow

Terminal output already flows through the server via gRPC `SessionOutputEvent` for WebSocket fanout. A new `ContentIngestor` component tees off this stream:

```
Agent PTY → SessionOutputEvent (gRPC) → Server
                                          ├→ WebSocket fanout (existing, unchanged)
                                          └→ ContentIngestor (NEW)
                                               ├→ Strip ANSI escape sequences
                                               ├→ Split into lines
                                               └→ INSERT into session_lines + FTS5
```

**ContentIngestor responsibilities:**
- Receives raw terminal output bytes per session
- Strips ANSI escape sequences using a comprehensive VT100/xterm state machine (CSI, OSC, DCS, APC, mouse reporting, alternate screen buffer sequences)
- Maintains a per-session line buffer for partial lines (output that hasn't hit a newline yet)
- On each complete line, inserts into `session_lines` table (regular) and `session_content` FTS5 table
- Thread-safe — called from gRPC goroutines
- Flushes remaining partial buffer when a session ends (via session status change)
- `FlushSession` acquires the per-session mutex, drains the batch buffer, then flushes any remaining partial line. The background batch ticker skips sessions that are being flushed (checked via mutex trylock or a `flushing` flag).

**ANSI stripping implementation:** Use the `github.com/acarl005/stripansi` Go library, which handles CSI, OSC, and other ANSI escape sequences comprehensively. If we want zero dependencies, implement a state machine covering: CSI sequences (`\x1b[...`), OSC sequences (`\x1b]...ST`), single-character escapes (`\x1b[A-Z]`), and C0/C1 control characters. The state machine approach is preferred for correctness.

**Batching:** To avoid one INSERT per line, batch lines and flush every 500ms or 100 lines (whichever comes first). Uses a per-session batch buffer flushed by a background ticker. Both the batch buffer and the partial-line buffer are protected by the same per-session mutex.

**What stays unchanged:**
- Agent `.cast` file writing (still used for scrollback replay on reconnect)
- `ScrollbackChunkEvent` gRPC flow (still used for attach/replay)
- Agent `scrollback.go` (still active)
- WebSocket fanout of raw terminal data to browsers

---

## 2. Database Schema

### Migration 12

```sql
-- Regular table storing one row per line of terminal output.
-- Used for context-line lookups (adjacent lines around search matches).
-- FTS5 UNINDEXED columns cannot be efficiently filtered, so we use a
-- separate indexed table and FTS5 external content.
CREATE TABLE session_lines (
    rowid       INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL,
    line_number INTEGER NOT NULL,
    content     TEXT NOT NULL
);

CREATE INDEX idx_session_lines_session ON session_lines(session_id, line_number);

-- FTS5 virtual table backed by session_lines (external content).
-- Only the 'content' column is indexed for full-text search.
-- Queries join back to session_lines for metadata.
CREATE VIRTUAL TABLE session_content USING fts5(
    content,
    content='session_lines',
    content_rowid='rowid',
    tokenize='unicode61'
);

-- Triggers to keep FTS5 index in sync with session_lines.
CREATE TRIGGER session_lines_ai AFTER INSERT ON session_lines BEGIN
    INSERT INTO session_content(rowid, content) VALUES (new.rowid, new.content);
END;

CREATE TRIGGER session_lines_ad AFTER DELETE ON session_lines BEGIN
    INSERT INTO session_content(session_content, rowid, content) VALUES('delete', old.rowid, old.content);
END;

-- Tracks which sessions have ingested content, for efficient cleanup.
CREATE TABLE session_content_meta (
    session_id  TEXT PRIMARY KEY REFERENCES sessions(session_id) ON DELETE CASCADE,
    line_count  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Server-wide settings (retention config, etc).
CREATE TABLE IF NOT EXISTS server_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Tracks cleanup commands that couldn't be delivered because the agent was offline.
CREATE TABLE pending_cleanups (
    cleanup_id  TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    machine_id  TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_pending_cleanups_machine ON pending_cleanups(machine_id);
```

**Key design decision:** FTS5 with external content (`content='session_lines'`). The `session_lines` regular table has proper indexes for context-line lookups (`WHERE session_id = ? AND line_number BETWEEN ? AND ?`), while the FTS5 table only handles full-text matching. Inserts go to `session_lines` and the trigger syncs FTS5 automatically. Deletes cascade through the trigger as well.

**Storage characteristics:**
- FTS5 uses ~2-3x the raw text size for its index
- A typical Claude session produces 10K-50K lines
- At 30-day retention with moderate usage, expect 10-100MB of content data

---

## 3. Content Ingestor

**New package:** `internal/server/ingest/`

**File:** `content.go`

```go
type ContentIngestor struct {
    store   ContentStore
    buffers sync.Map  // map[sessionID]*lineBuffer
    done    chan struct{}
}

type lineBuffer struct {
    mu        sync.Mutex
    lineCount int
    partial   []byte           // incomplete line waiting for newline
    batch     []contentLine    // buffered complete lines pending flush
    flushing  bool             // set during FlushSession to prevent ticker conflict
}

type contentLine struct {
    sessionID  string
    lineNumber int
    content    string
}
```

**Interface:**
- `Ingest(sessionID string, data []byte)` — called from gRPC server on each `SessionOutputEvent`
- `FlushSession(sessionID string)` — called when session ends, flushes batch + partial line buffer, updates `session_content_meta.line_count`
- `Close()` — stops background ticker, flushes all buffers on server shutdown

**`session_content_meta` maintenance:** The ingestor creates a meta row on first insert for a session (`INSERT OR IGNORE`). On each batch flush, it updates `line_count` with `UPDATE session_content_meta SET line_count = ? WHERE session_id = ?`. On `FlushSession`, the final line count is written.

---

## 4. Search Handler

**File:** `internal/server/handler/search.go` (replace stub)

**Endpoint:** `GET /api/v1/search/sessions?q=<query>&limit=50&offset=0`

Parameters:
- `q` (required) — FTS5 match expression. Supports prefix (`error*`), phrase (`"connection refused"`), and boolean (`error AND database`). Invalid syntax returns 400.
- `limit` (optional, default 50, max 200)
- `offset` (optional, default 0) — for pagination

**FTS5 query:**
```sql
SELECT sl.session_id, sl.line_number, sl.content,
       s.machine_id, s.status, s.started_at
FROM session_content sc
JOIN session_lines sl ON sc.rowid = sl.rowid
JOIN sessions s ON sl.session_id = s.session_id
WHERE session_content MATCH ?
ORDER BY sc.rank
LIMIT ? OFFSET ?
```

**Context lines:** For each match, fetch 2 lines before and 2 lines after from the same session using the indexed `session_lines` table:
```sql
SELECT content FROM session_lines
WHERE session_id = ? AND line_number BETWEEN ? AND ?
ORDER BY line_number
```

Context lines are concatenated into a single string (newline-separated) to match the existing frontend `SearchResult` type.

**Response shape** (matches existing frontend `SearchResult` type at `web/src/api/search.ts`):

The endpoint returns a flat `SearchResult[]` array (not wrapped in an envelope). Each result:

```typescript
interface SearchResult {
  session_id: string;
  machine_id: string;
  line: string;              // the matched line content
  context_before: string;    // 2 preceding lines, newline-joined
  context_after: string;     // 2 following lines, newline-joined
  timestamp_ms: number;      // session started_at as unix millis
  session_status: string;    // session status (running, completed, etc.)
}
```

**Note on `timestamp_ms`:** This is the session's `started_at` timestamp, not a per-line timestamp. Per-line timestamps are not available since we strip ANSI and store plain text without timing data. The session timestamp provides sufficient context for identifying when the output occurred. The frontend already uses this for "relative time" display on result cards.

---

## 5. Data Retention

### Retention Cleaner

**New package:** `internal/server/retention/`

**File:** `cleaner.go`

**Behavior:**
1. Background goroutine, runs every hour
2. Reads retention period: check `server_settings` table for key `retention_days`, fall back to TOML config `[retention] days = 30`, fall back to hardcoded default 30
3. Query sessions eligible for cleanup:
   ```sql
   SELECT s.session_id, s.machine_id FROM sessions s
   JOIN session_content_meta m ON s.session_id = m.session_id
   WHERE s.status IN ('completed', 'failed', 'terminated')
   AND s.ended_at < datetime('now', '-' || ? || ' days')
   ```
   The join with `session_content_meta` naturally limits the scan to sessions that have content. The `ended_at` filter on a small result set is efficient enough without a dedicated index.
4. For each eligible session:
   - Delete from `session_lines` WHERE `session_id = ?` (triggers cascade-delete the FTS5 rows)
   - Delete from `session_content_meta` WHERE `session_id = ?`
   - If agent is connected: send `CleanupScrollbackCmd` via gRPC
   - If agent is offline: insert into `pending_cleanups` table
5. On agent reconnect: query `pending_cleanups` by `machine_id` (indexed), send commands, delete fulfilled entries
6. After each cleanup sweep, run FTS5 optimization:
   ```sql
   INSERT INTO session_content(session_content) VALUES('optimize');
   ```
   This merges FTS5 index segments for consistent query performance after bulk deletes.

**Running sessions are never pruned** regardless of age.

### Agent-Side Cleanup

**Proto addition** to `agent.proto`:
```protobuf
message CleanupScrollbackCmd {
    string session_id = 1;
}
```

Added as a new command type in the existing `ServerCommand.command` oneof.

**Agent handler** in `session_manager.go`:
- Receives `CleanupScrollbackCmd`
- Deletes `{dataDir}/{session_id}.cast` if it exists
- Logs the cleanup at info level
- Best-effort: no error is sent back to the server. If deletion fails (permissions, disk error), the `.cast` file remains and will be retried on the next retention sweep only if the server re-creates a `pending_cleanups` entry (it won't — the server considers the cleanup delivered once the command is sent). **Known limitation:** agent-side file deletion failures are not retried. This is acceptable because `.cast` files are not user-facing after content is deleted from the server, and manual cleanup is trivial.

### TOML Config

Add to server config:
```toml
[retention]
days = 30  # default retention period for session content
```

---

## 6. Settings Page

### Backend

**New handler:** `internal/server/handler/settings.go`

**Endpoints:**
- `GET /api/v1/settings` — returns all server settings as a JSON object: `{"retention_days": "30"}`
- `PUT /api/v1/settings` — accepts a JSON object with one or more key-value pairs to upsert: `{"retention_days": "90"}`. Admin-only. Values are strings. Returns the updated settings object.

Validation for `retention_days`: must be a positive integer or `0` (meaning unlimited). Valid presets: 7, 30, 90, 365, 0.

### Frontend

**Modify:** `web/src/views/SettingsPage.tsx`

Add a "Data Retention" section:
- Label: "Session content retention"
- Dropdown with options: 7 days, 30 days, 90 days, 1 year, Unlimited
- Shows current value from API
- On change, PUTs the new value
- Tooltip/help text: "Terminal output older than this is deleted from the server and agent machines. Running sessions are never affected."
- Admin-only (consistent with settings page access)

**New files:**
- `web/src/api/settings.ts` — API client for settings CRUD
- `web/src/hooks/useSettings.ts` — TanStack Query hooks

---

## 7. Dead Code Removal

### Code to remove

| File | What | Why |
|------|------|-----|
| `internal/server/handler/search.go` | TODO comments about fan-out to agents | Replaced by FTS5 search |
| `internal/server/handler/search.go` | Stub empty-result return logic | Replaced by real implementation |

### Code to audit for dead references

After implementation, perform a dedicated sweep for:
- Any remaining references to `SearchScrollbackCmd` or `SearchScrollbackResultEvent` concepts (these were never implemented, only referenced in comments)
- Unused imports in modified files (`search.go`, `grpc/server.go`, `session_manager.go`)
- Any helper functions in `search.go` that were scaffolded for the fan-out approach but never used
- Frontend search-related code that assumed the fan-out response shape — verify `SearchResult` type in `web/src/api/search.ts` still matches the new backend response
- Stale comments in `grpc/server.go` referencing "future search" or agent-side search plans
- Any test fixtures or mocks set up for the old search approach
- Run `go vet ./...` and check for unused variables/imports across all modified packages
- Grep the entire codebase for `SearchScrollback`, `fan.out`, `fan-out` to catch stray references

### Code to keep (NOT dead)

| File | What | Why still needed |
|------|------|------------------|
| `internal/agent/scrollback.go` | `.cast` file writer | Used for scrollback replay on attach |
| `internal/agent/session.go` | Scrollback integration | Drives `.cast` writing during session |
| `internal/server/grpc/server.go` | `ScrollbackChunkEvent` handling | Used for attach/replay flow |
| `internal/server/grpc/server.go` | `parseAsciicastData` | Parses `.cast` chunks for replay |

---

## 8. Files Created/Modified

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/server/ingest/content.go` | ContentIngestor — ANSI strip, line split, batch insert |
| Create | `internal/server/store/content.go` | Content store methods (insert lines, search, cleanup, meta) |
| Create | `internal/server/retention/cleaner.go` | Background retention sweep goroutine |
| Create | `internal/server/handler/settings.go` | Server settings CRUD handler |
| Create | `web/src/api/settings.ts` | Settings API client |
| Create | `web/src/hooks/useSettings.ts` | TanStack Query hooks for settings |
| Modify | `internal/server/store/migrations.go` | Migration 12 — session_lines, FTS5, meta, settings, pending_cleanups |
| Modify | `internal/server/handler/search.go` | Replace stub with real FTS5 search |
| Modify | `internal/server/grpc/server.go` | Tee SessionOutputEvent to ContentIngestor |
| Modify | `internal/server/config/config.go` | Add `[retention]` TOML section |
| Modify | `proto/claudeplane/v1/agent.proto` | Add `CleanupScrollbackCmd` to command oneof |
| Modify | `internal/agent/session_manager.go` | Handle `CleanupScrollbackCmd` — delete `.cast` file |
| Modify | `cmd/server/main.go` | Wire ContentIngestor, retention cleaner, settings handler |
| Modify | `web/src/views/SettingsPage.tsx` | Add data retention UI section |
