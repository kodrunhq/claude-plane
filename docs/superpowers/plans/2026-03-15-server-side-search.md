# Server-Side Session Search Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the stubbed agent-side search with real server-side full-text search. Stream terminal output into SQLite FTS5 as it arrives, add configurable data retention with agent `.cast` file cleanup, and wire the settings page.

**Architecture:** Terminal output is teed from the existing gRPC `SessionOutputEvent` handler into a new `ContentIngestor` that strips ANSI, splits lines, and batch-inserts into a `session_lines` table backed by an FTS5 external-content index. A background retention cleaner prunes old content and notifies agents to delete `.cast` files.

**Tech Stack:** Go, SQLite FTS5, gRPC/protobuf, React/TypeScript, TanStack Query, Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-03-15-server-side-search-design.md`

---

## Chunk 1: Database Migration & Content Store

### Task 1: Add migration 12

**Files:**
- Modify: `internal/server/store/migrations.go`

- [ ] **Step 1: Add migration 12 to the migrations slice**

Append after migration 11 (the `run_job` fields migration, ending at line ~436):

```go
{
    Version:     12,
    Description: "session content search index, server settings, and pending cleanups",
    SQL: `
CREATE TABLE session_lines (
    rowid       INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL,
    line_number INTEGER NOT NULL,
    content     TEXT NOT NULL
);

CREATE INDEX idx_session_lines_session ON session_lines(session_id, line_number);

CREATE VIRTUAL TABLE session_content USING fts5(
    content,
    content='session_lines',
    content_rowid='rowid',
    tokenize='unicode61'
);

CREATE TRIGGER session_lines_ai AFTER INSERT ON session_lines BEGIN
    INSERT INTO session_content(rowid, content) VALUES (new.rowid, new.content);
END;

CREATE TRIGGER session_lines_ad AFTER DELETE ON session_lines BEGIN
    INSERT INTO session_content(session_content, rowid, content) VALUES('delete', old.rowid, old.content);
END;

CREATE TABLE session_content_meta (
    session_id  TEXT PRIMARY KEY REFERENCES sessions(session_id) ON DELETE CASCADE,
    line_count  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS server_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE pending_cleanups (
    cleanup_id  TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    machine_id  TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_pending_cleanups_machine ON pending_cleanups(machine_id);
`,
},
```

- [ ] **Step 2: Verify migration applies**

Run: `go build -o /dev/null ./cmd/server`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/server/store/migrations.go
git commit -m "feat: migration 12 — session content FTS5 index, server settings, pending cleanups"
```

### Task 2: Create content store

**Files:**
- Create: `internal/server/store/content.go`

- [ ] **Step 1: Create the content store file**

This file adds methods to the existing `Store` type for content operations:

```go
package store

import (
    "context"
    "database/sql"
    "fmt"
    "strings"

    "github.com/google/uuid"
)

// ContentLine represents a single line of session terminal output.
type ContentLine struct {
    SessionID  string
    LineNumber int
    Content    string
}

// ContentSearchResult represents a search match with context.
type ContentSearchResult struct {
    SessionID     string `json:"session_id"`
    MachineID     string `json:"machine_id"`
    Line          string `json:"line"`
    LineNumber    int    `json:"line_number"`
    ContextBefore string `json:"context_before"`
    ContextAfter  string `json:"context_after"`
    TimestampMs   int64  `json:"timestamp_ms"`
    SessionStatus string `json:"session_status,omitempty"`
}

// InsertContentLines bulk-inserts lines into session_lines.
// The FTS5 index is updated automatically via the session_lines_ai trigger.
func (s *Store) InsertContentLines(ctx context.Context, lines []ContentLine) error {
    if len(lines) == 0 {
        return nil
    }
    tx, err := s.writer.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback()

    stmt, err := tx.PrepareContext(ctx,
        "INSERT INTO session_lines (session_id, line_number, content) VALUES (?, ?, ?)")
    if err != nil {
        return fmt.Errorf("prepare insert: %w", err)
    }
    defer stmt.Close()

    for _, l := range lines {
        if _, err := stmt.ExecContext(ctx, l.SessionID, l.LineNumber, l.Content); err != nil {
            return fmt.Errorf("insert line: %w", err)
        }
    }
    return tx.Commit()
}

// UpsertContentMeta creates or updates the content metadata for a session.
func (s *Store) UpsertContentMeta(ctx context.Context, sessionID string, lineCount int) error {
    _, err := s.writer.ExecContext(ctx,
        `INSERT INTO session_content_meta (session_id, line_count)
         VALUES (?, ?)
         ON CONFLICT(session_id) DO UPDATE SET line_count = excluded.line_count`,
        sessionID, lineCount)
    if err != nil {
        return fmt.Errorf("upsert content meta: %w", err)
    }
    return nil
}

// SearchContent performs a full-text search across session terminal output.
func (s *Store) SearchContent(ctx context.Context, query string, limit, offset int) ([]ContentSearchResult, error) {
    rows, err := s.reader.QueryContext(ctx,
        `SELECT sl.session_id, sl.line_number, sl.content,
                s.machine_id, s.status, s.started_at
         FROM session_content sc
         JOIN session_lines sl ON sc.rowid = sl.rowid
         JOIN sessions s ON sl.session_id = s.session_id
         WHERE session_content MATCH ?
         ORDER BY sc.rank
         LIMIT ? OFFSET ?`,
        query, limit, offset)
    if err != nil {
        return nil, fmt.Errorf("search content: %w", err)
    }
    defer rows.Close()

    var results []ContentSearchResult
    for rows.Next() {
        var r ContentSearchResult
        var startedAt sql.NullTime
        if err := rows.Scan(&r.SessionID, &r.LineNumber, &r.Line,
            &r.MachineID, &r.SessionStatus, &startedAt); err != nil {
            return nil, fmt.Errorf("scan result: %w", err)
        }
        if startedAt.Valid {
            r.TimestampMs = startedAt.Time.UnixMilli()
        }
        results = append(results, r)
    }
    if results == nil {
        results = []ContentSearchResult{}
    }
    return results, rows.Err()
}

// FetchContextLines retrieves lines around a match for context display.
func (s *Store) FetchContextLines(ctx context.Context, sessionID string, lineNumber, before, after int) (contextBefore, contextAfter string, err error) {
    startLine := lineNumber - before
    if startLine < 0 {
        startLine = 0
    }
    endLine := lineNumber + after

    rows, err := s.reader.QueryContext(ctx,
        `SELECT line_number, content FROM session_lines
         WHERE session_id = ? AND line_number BETWEEN ? AND ?
         ORDER BY line_number`,
        sessionID, startLine, endLine)
    if err != nil {
        return "", "", fmt.Errorf("fetch context: %w", err)
    }
    defer rows.Close()

    var beforeLines, afterLines []string
    for rows.Next() {
        var ln int
        var content string
        if err := rows.Scan(&ln, &content); err != nil {
            return "", "", fmt.Errorf("scan context line: %w", err)
        }
        if ln < lineNumber {
            beforeLines = append(beforeLines, content)
        } else if ln > lineNumber {
            afterLines = append(afterLines, content)
        }
    }
    return strings.Join(beforeLines, "\n"), strings.Join(afterLines, "\n"), rows.Err()
}

// DeleteSessionContent removes all content for a session (lines + meta).
func (s *Store) DeleteSessionContent(ctx context.Context, sessionID string) error {
    tx, err := s.writer.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback()

    if _, err := tx.ExecContext(ctx, "DELETE FROM session_lines WHERE session_id = ?", sessionID); err != nil {
        return fmt.Errorf("delete lines: %w", err)
    }
    if _, err := tx.ExecContext(ctx, "DELETE FROM session_content_meta WHERE session_id = ?", sessionID); err != nil {
        return fmt.Errorf("delete meta: %w", err)
    }
    return tx.Commit()
}

// ListExpiredContentSessions returns sessions eligible for content cleanup.
func (s *Store) ListExpiredContentSessions(ctx context.Context, retentionDays int) ([]struct{ SessionID, MachineID string }, error) {
    rows, err := s.reader.QueryContext(ctx,
        `SELECT s.session_id, s.machine_id FROM sessions s
         JOIN session_content_meta m ON s.session_id = m.session_id
         WHERE s.status IN ('completed', 'failed', 'terminated')
         AND s.ended_at < datetime('now', '-' || ? || ' days')`,
        retentionDays)
    if err != nil {
        return nil, fmt.Errorf("list expired: %w", err)
    }
    defer rows.Close()

    var results []struct{ SessionID, MachineID string }
    for rows.Next() {
        var r struct{ SessionID, MachineID string }
        if err := rows.Scan(&r.SessionID, &r.MachineID); err != nil {
            return nil, fmt.Errorf("scan expired: %w", err)
        }
        results = append(results, r)
    }
    return results, rows.Err()
}

// OptimizeFTS runs FTS5 merge optimization after bulk deletes.
func (s *Store) OptimizeFTS(ctx context.Context) error {
    _, err := s.writer.ExecContext(ctx, "INSERT INTO session_content(session_content) VALUES('optimize')")
    if err != nil {
        return fmt.Errorf("optimize FTS: %w", err)
    }
    return nil
}

// --- Server Settings ---

// GetSetting reads a server setting by key. Returns empty string if not found.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
    var value string
    err := s.reader.QueryRowContext(ctx,
        "SELECT value FROM server_settings WHERE key = ?", key).Scan(&value)
    if err == sql.ErrNoRows {
        return "", nil
    }
    if err != nil {
        return "", fmt.Errorf("get setting %s: %w", key, err)
    }
    return value, nil
}

// SetSetting upserts a server setting.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
    _, err := s.writer.ExecContext(ctx,
        `INSERT INTO server_settings (key, value, updated_at)
         VALUES (?, ?, CURRENT_TIMESTAMP)
         ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
        key, value)
    if err != nil {
        return fmt.Errorf("set setting %s: %w", key, err)
    }
    return nil
}

// GetAllSettings returns all server settings as a map.
func (s *Store) GetAllSettings(ctx context.Context) (map[string]string, error) {
    rows, err := s.reader.QueryContext(ctx, "SELECT key, value FROM server_settings")
    if err != nil {
        return nil, fmt.Errorf("get all settings: %w", err)
    }
    defer rows.Close()

    settings := make(map[string]string)
    for rows.Next() {
        var k, v string
        if err := rows.Scan(&k, &v); err != nil {
            return nil, fmt.Errorf("scan setting: %w", err)
        }
        settings[k] = v
    }
    return settings, rows.Err()
}

// --- Pending Cleanups ---

// AddPendingCleanup records a cleanup for offline agents.
func (s *Store) AddPendingCleanup(ctx context.Context, sessionID, machineID string) error {
    id := uuid.New().String()
    _, err := s.writer.ExecContext(ctx,
        `INSERT INTO pending_cleanups (cleanup_id, session_id, machine_id) VALUES (?, ?, ?)`,
        id, sessionID, machineID)
    if err != nil {
        return fmt.Errorf("add pending cleanup: %w", err)
    }
    return nil
}

// ListPendingCleanups returns pending cleanups for a machine.
func (s *Store) ListPendingCleanups(ctx context.Context, machineID string) ([]string, error) {
    rows, err := s.reader.QueryContext(ctx,
        "SELECT session_id FROM pending_cleanups WHERE machine_id = ?", machineID)
    if err != nil {
        return nil, fmt.Errorf("list pending cleanups: %w", err)
    }
    defer rows.Close()

    var sessionIDs []string
    for rows.Next() {
        var id string
        if err := rows.Scan(&id); err != nil {
            return nil, fmt.Errorf("scan cleanup: %w", err)
        }
        sessionIDs = append(sessionIDs, id)
    }
    return sessionIDs, rows.Err()
}

// DeletePendingCleanups removes pending cleanups for a machine.
func (s *Store) DeletePendingCleanups(ctx context.Context, machineID string) error {
    _, err := s.writer.ExecContext(ctx,
        "DELETE FROM pending_cleanups WHERE machine_id = ?", machineID)
    if err != nil {
        return fmt.Errorf("delete pending cleanups: %w", err)
    }
    return nil
}
```

- [ ] **Step 2: Verify build**

Run: `go build -o /dev/null ./cmd/server`

- [ ] **Step 3: Commit**

```bash
git add internal/server/store/content.go
git commit -m "feat: content store — FTS5 search, line insert, settings, cleanup queries"
```

---

## Chunk 2: Content Ingestor

### Task 3: Create ANSI stripper and content ingestor

**Files:**
- Create: `internal/server/ingest/content.go`

- [ ] **Step 1: Create the ingest package**

```go
package ingest

import (
    "bytes"
    "context"
    "log/slog"
    "regexp"
    "sync"
    "time"

    "github.com/kodrunhq/claude-plane/internal/server/store"
)

// ansiPattern matches ANSI escape sequences:
// - CSI sequences: \x1b[ ... (any params) ... final byte
// - OSC sequences: \x1b] ... ST (or BEL)
// - Single-char escapes: \x1b followed by one character
// - C0 control chars except \n, \r, \t
var ansiPattern = regexp.MustCompile(
    `\x1b\[[0-9;?]*[a-zA-Z]` + // CSI sequences
    `|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)` + // OSC sequences
    `|\x1b[()][0-9A-B]` + // Character set selection
    `|\x1b[^[\]()0-9A-B]` + // Other single-char escapes
    `|[\x00-\x08\x0b\x0c\x0e-\x1f]`, // C0 controls (keep \n \r \t)
)

// stripANSI removes all ANSI escape sequences and control characters from data,
// keeping only printable text, newlines, carriage returns, and tabs.
func stripANSI(data []byte) []byte {
    return ansiPattern.ReplaceAll(data, nil)
}

// ContentStore is the interface needed by the ingestor.
type ContentStore interface {
    InsertContentLines(ctx context.Context, lines []store.ContentLine) error
    UpsertContentMeta(ctx context.Context, sessionID string, lineCount int) error
}

const (
    batchFlushInterval = 500 * time.Millisecond
    batchFlushSize     = 100
)

// ContentIngestor receives raw terminal output, strips ANSI, splits into lines,
// and batch-inserts into the content store for full-text search.
type ContentIngestor struct {
    store   ContentStore
    logger  *slog.Logger
    buffers sync.Map // map[string]*lineBuffer
    done    chan struct{}
    wg      sync.WaitGroup
}

type lineBuffer struct {
    mu        sync.Mutex
    lineCount int
    partial   []byte          // incomplete line waiting for newline
    batch     []store.ContentLine // buffered complete lines pending flush
    flushing  bool            // set during FlushSession to block ticker
    sessionID string
}

// NewContentIngestor creates a new ingestor and starts the background flush ticker.
func NewContentIngestor(st ContentStore, logger *slog.Logger) *ContentIngestor {
    if logger == nil {
        logger = slog.Default()
    }
    ci := &ContentIngestor{
        store:  st,
        logger: logger,
        done:   make(chan struct{}),
    }
    ci.wg.Add(1)
    go ci.flushLoop()
    return ci
}

// Ingest processes raw terminal output for a session.
// Called from the gRPC server goroutine on each SessionOutputEvent.
func (ci *ContentIngestor) Ingest(sessionID string, data []byte) {
    if len(data) == 0 {
        return
    }
    stripped := stripANSI(data)
    if len(stripped) == 0 {
        return
    }

    val, _ := ci.buffers.LoadOrStore(sessionID, &lineBuffer{sessionID: sessionID})
    buf := val.(*lineBuffer)

    buf.mu.Lock()
    defer buf.mu.Unlock()

    // Combine partial line with new data
    combined := append(buf.partial, stripped...)
    buf.partial = nil

    // Split on newlines
    for {
        idx := bytes.IndexByte(combined, '\n')
        if idx < 0 {
            // No more newlines — save remainder as partial
            if len(combined) > 0 {
                buf.partial = make([]byte, len(combined))
                copy(buf.partial, combined)
            }
            break
        }
        line := combined[:idx]
        combined = combined[idx+1:]

        // Strip carriage returns
        line = bytes.TrimRight(line, "\r")

        // Skip empty lines
        content := string(bytes.TrimSpace(line))
        if content == "" {
            continue
        }

        buf.lineCount++
        buf.batch = append(buf.batch, store.ContentLine{
            SessionID:  sessionID,
            LineNumber: buf.lineCount,
            Content:    content,
        })
    }

    // Flush if batch is large enough
    if len(buf.batch) >= batchFlushSize {
        ci.flushBuffer(buf)
    }
}

// FlushSession flushes all buffered content for a session (call when session ends).
func (ci *ContentIngestor) FlushSession(sessionID string) {
    val, ok := ci.buffers.Load(sessionID)
    if !ok {
        return
    }
    buf := val.(*lineBuffer)

    buf.mu.Lock()
    buf.flushing = true

    // Flush any remaining partial line
    if len(buf.partial) > 0 {
        content := string(bytes.TrimSpace(buf.partial))
        if content != "" {
            buf.lineCount++
            buf.batch = append(buf.batch, store.ContentLine{
                SessionID:  sessionID,
                LineNumber: buf.lineCount,
                Content:    content,
            })
        }
        buf.partial = nil
    }

    ci.flushBuffer(buf)
    lineCount := buf.lineCount
    buf.mu.Unlock()

    // Update meta with final line count
    if lineCount > 0 {
        if err := ci.store.UpsertContentMeta(context.Background(), sessionID, lineCount); err != nil {
            ci.logger.Warn("failed to update content meta", "error", err, "session_id", sessionID)
        }
    }

    ci.buffers.Delete(sessionID)
}

// Close stops the background flush ticker and flushes all remaining buffers.
func (ci *ContentIngestor) Close() {
    close(ci.done)
    ci.wg.Wait()

    // Flush all remaining buffers
    ci.buffers.Range(func(key, val any) bool {
        buf := val.(*lineBuffer)
        buf.mu.Lock()
        ci.flushBuffer(buf)
        buf.mu.Unlock()
        return true
    })
}

// flushLoop runs the periodic batch flush.
func (ci *ContentIngestor) flushLoop() {
    defer ci.wg.Done()
    ticker := time.NewTicker(batchFlushInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ci.done:
            return
        case <-ticker.C:
            ci.buffers.Range(func(key, val any) bool {
                buf := val.(*lineBuffer)
                buf.mu.Lock()
                if !buf.flushing && len(buf.batch) > 0 {
                    ci.flushBuffer(buf)
                }
                buf.mu.Unlock()
                return true
            })
        }
    }
}

// flushBuffer writes the batch to the store. Caller must hold buf.mu.
func (ci *ContentIngestor) flushBuffer(buf *lineBuffer) {
    if len(buf.batch) == 0 {
        return
    }
    batch := buf.batch
    buf.batch = nil

    if err := ci.store.InsertContentLines(context.Background(), batch); err != nil {
        ci.logger.Warn("failed to insert content lines",
            "error", err,
            "session_id", buf.sessionID,
            "line_count", len(batch),
        )
    }
}
```

- [ ] **Step 2: Verify build**

Run: `go build -o /dev/null ./cmd/server`

- [ ] **Step 3: Commit**

```bash
git add internal/server/ingest/content.go
git commit -m "feat: content ingestor — ANSI strip, line split, batch insert into FTS5"
```

---

## Chunk 3: Search Handler, Settings Handler, and Wiring

### Task 4: Replace search handler stub

**Files:**
- Modify: `internal/server/handler/search.go`

- [ ] **Step 1: Rewrite search.go**

Replace the entire file. The new handler uses the content store instead of the connection manager:

```go
package handler

import (
    "net/http"
    "strconv"
    "strings"

    "github.com/go-chi/chi/v5"

    "github.com/kodrunhq/claude-plane/internal/server/store"
)

// SearchHandler handles REST endpoints for searching session content.
type SearchHandler struct {
    store *store.Store
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(s *store.Store) *SearchHandler {
    return &SearchHandler{store: s}
}

// RegisterSearchRoutes mounts all search routes on the given router.
func RegisterSearchRoutes(r chi.Router, h *SearchHandler) {
    r.Get("/api/v1/search/sessions", h.SearchSessions)
}

// SearchSessions handles GET /api/v1/search/sessions?q=<query>&limit=50&offset=0.
func (h *SearchHandler) SearchSessions(w http.ResponseWriter, r *http.Request) {
    query := r.URL.Query().Get("q")
    if query == "" {
        writeError(w, http.StatusBadRequest, "q parameter is required")
        return
    }

    limit := 50
    if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
        limit = l
    }

    offset := 0
    if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
        offset = o
    }

    results, err := h.store.SearchContent(r.Context(), query, limit, offset)
    if err != nil {
        if strings.Contains(err.Error(), "fts5: syntax error") ||
           strings.Contains(err.Error(), "no such column") {
            writeError(w, http.StatusBadRequest, "invalid search query syntax")
            return
        }
        writeError(w, http.StatusInternalServerError, "search failed")
        return
    }

    // Fetch context lines for each result
    for i := range results {
        before, after, err := h.store.FetchContextLines(r.Context(),
            results[i].SessionID, results[i].LineNumber, 2, 2)
        if err != nil {
            continue // skip context on error, still return the match
        }
        results[i].ContextBefore = before
        results[i].ContextAfter = after
    }

    writeJSON(w, http.StatusOK, results)
}
```

Note: The `SearchResult` struct from the old file is replaced by `store.ContentSearchResult`. The `SearchHandler` now takes `*store.Store` instead of `*connmgr.ConnectionManager`. The `ContextBefore`/`ContextAfter` fields and `Line` field on `ContentSearchResult` map directly to the frontend's `SearchResult` type.

- [ ] **Step 2: Verify build**

Run: `go build -o /dev/null ./cmd/server`
Expected: Will fail — `main.go` still passes `connMgr` to `NewSearchHandler`. Fix in Task 6.

- [ ] **Step 3: Commit**

```bash
git add internal/server/handler/search.go
git commit -m "feat: replace search stub with FTS5 full-text search"
```

### Task 5: Create settings handler

**Files:**
- Create: `internal/server/handler/settings.go`

- [ ] **Step 1: Create the settings handler**

```go
package handler

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"

    "github.com/go-chi/chi/v5"

    "github.com/kodrunhq/claude-plane/internal/server/store"
)

// validRetentionDays are the allowed values for the retention_days setting.
// 0 means unlimited.
var validRetentionDays = map[int]bool{7: true, 30: true, 90: true, 365: true, 0: true}

// SettingsHandler handles REST endpoints for server settings.
type SettingsHandler struct {
    store     *store.Store
    getClaims ClaimsGetter
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(s *store.Store, getClaims ClaimsGetter) *SettingsHandler {
    return &SettingsHandler{store: s, getClaims: getClaims}
}

// RegisterSettingsRoutes mounts settings routes.
func RegisterSettingsRoutes(r chi.Router, h *SettingsHandler) {
    r.Get("/api/v1/settings", h.GetSettings)
    r.Put("/api/v1/settings", h.UpdateSettings)
}

// GetSettings returns all server settings.
func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
    settings, err := h.store.GetAllSettings(r.Context())
    if err != nil {
        writeError(w, http.StatusInternalServerError, "failed to load settings")
        return
    }
    writeJSON(w, http.StatusOK, settings)
}

// UpdateSettings upserts server settings. Admin-only.
func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
    claims := h.getClaims(r)
    if claims == nil || claims.Role != "admin" {
        writeError(w, http.StatusForbidden, "admin access required")
        return
    }

    var body map[string]string
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    // Validate known settings
    if val, ok := body["retention_days"]; ok {
        days, err := strconv.Atoi(val)
        if err != nil || !validRetentionDays[days] {
            writeError(w, http.StatusBadRequest,
                fmt.Sprintf("retention_days must be one of: 7, 30, 90, 365, 0 (unlimited)"))
            return
        }
    }

    for k, v := range body {
        if err := h.store.SetSetting(r.Context(), k, v); err != nil {
            writeError(w, http.StatusInternalServerError, "failed to update settings")
            return
        }
    }

    // Return updated settings
    settings, err := h.store.GetAllSettings(r.Context())
    if err != nil {
        writeError(w, http.StatusInternalServerError, "failed to load settings")
        return
    }
    writeJSON(w, http.StatusOK, settings)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/server/handler/settings.go
git commit -m "feat: settings handler — GET/PUT /api/v1/settings for retention config"
```

### Task 6: Add retention config and wire everything in main.go

**Files:**
- Modify: `internal/server/config/config.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Add retention config**

In `config.go`, add `Retention` field to `ServerConfig` struct (after `Events`):

```go
Retention RetentionConfig `toml:"retention"`
```

Add the struct:

```go
// RetentionConfig controls session content retention.
type RetentionConfig struct {
    Days int `toml:"days"`
}

// GetRetentionDays returns the configured retention period.
// Defaults to 30 days when not set or zero.
func (r *RetentionConfig) GetRetentionDays() int {
    if r.Days <= 0 {
        return 30
    }
    return r.Days
}
```

- [ ] **Step 2: Update main.go wiring**

In `cmd/server/main.go`:

1. Import the ingest package:
   ```go
   "github.com/kodrunhq/claude-plane/internal/server/ingest"
   ```

2. After registry creation (line ~108), create the content ingestor:
   ```go
   contentIngestor := ingest.NewContentIngestor(s, slog.Default())
   defer contentIngestor.Close()
   ```

3. Set ingestor on gRPC server (after `grpcSrv.SetTaskValueStore(s)` at line ~114):
   ```go
   grpcSrv.SetContentIngestor(contentIngestor)
   ```

4. Change `searchHandler` creation (line ~310) from:
   ```go
   searchHandler := handler.NewSearchHandler(connMgr)
   ```
   to:
   ```go
   searchHandler := handler.NewSearchHandler(s)
   ```

5. Add settings handler creation (near other handlers):
   ```go
   settingsHandler := handler.NewSettingsHandler(s, handlerClaimsGetter)
   ```

6. Add settings routes (in a JWT-protected group):
   ```go
   router.Group(func(r chi.Router) {
       r.Use(api.JWTAuthMiddleware(authSvc, apiKeyAuth))
       handler.RegisterSettingsRoutes(r, settingsHandler)
   })
   ```

- [ ] **Step 3: Add SetContentIngestor to gRPC server**

In `internal/server/grpc/server.go`, add a field and setter:

```go
// In the GRPCServer struct, add:
ingestor *ingest.ContentIngestor

// Add setter method:
func (s *GRPCServer) SetContentIngestor(ci *ingest.ContentIngestor) {
    s.ingestor = ci
}
```

Then in the `CommandStream` function, after the existing `registry.Publish` for `SessionOutputEvent` (line ~294), add the ingestor tee:

```go
if out := res.event.GetSessionOutput(); out != nil {
    s.registry.Publish(out.GetSessionId(), out.GetData())
    // Tee output to content ingestor for search indexing
    if s.ingestor != nil {
        s.ingestor.Ingest(out.GetSessionId(), out.GetData())
    }
}
```

Also add flush on session exit (after the `SessionExitEvent` handling):

```go
if exit := res.event.GetSessionExit(); exit != nil {
    // ... existing exit handling ...
    // Flush content ingestor for this session
    if s.ingestor != nil {
        s.ingestor.FlushSession(exit.GetSessionId())
    }
}
```

- [ ] **Step 4: Verify build**

Run: `go vet ./... && go build -o /dev/null ./cmd/server`

- [ ] **Step 5: Run tests**

Run: `go test -race github.com/kodrunhq/claude-plane/internal/server/...`

- [ ] **Step 6: Commit**

```bash
git add internal/server/config/config.go internal/server/grpc/server.go cmd/server/main.go
git commit -m "feat: wire content ingestor, search handler, and settings into server"
```

---

## Chunk 4: Retention Cleaner & Agent Cleanup

### Task 7: Create retention cleaner

**Files:**
- Create: `internal/server/retention/cleaner.go`

- [ ] **Step 1: Create the retention package**

```go
package retention

import (
    "context"
    "log/slog"
    "time"

    "github.com/kodrunhq/claude-plane/internal/server/connmgr"
    "github.com/kodrunhq/claude-plane/internal/server/store"
    pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// Cleaner periodically removes expired session content and notifies agents.
type Cleaner struct {
    store           *store.Store
    connMgr         *connmgr.ConnectionManager
    logger          *slog.Logger
    defaultDays     int
    done            chan struct{}
}

// NewCleaner creates a new retention cleaner.
func NewCleaner(st *store.Store, cm *connmgr.ConnectionManager, logger *slog.Logger, defaultDays int) *Cleaner {
    if logger == nil {
        logger = slog.Default()
    }
    return &Cleaner{
        store:       st,
        connMgr:     cm,
        logger:      logger,
        defaultDays: defaultDays,
        done:        make(chan struct{}),
    }
}

// Start begins the hourly cleanup sweep in a background goroutine.
func (c *Cleaner) Start() {
    go c.loop()
}

// Stop halts the cleaner.
func (c *Cleaner) Stop() {
    close(c.done)
}

func (c *Cleaner) loop() {
    // Run once on startup after a short delay
    timer := time.NewTimer(30 * time.Second)
    defer timer.Stop()

    for {
        select {
        case <-c.done:
            return
        case <-timer.C:
            c.sweep()
            timer.Reset(1 * time.Hour)
        }
    }
}

func (c *Cleaner) sweep() {
    ctx := context.Background()

    // Determine retention period
    days := c.defaultDays
    if val, err := c.store.GetSetting(ctx, "retention_days"); err == nil && val != "" {
        if d := parseInt(val); d > 0 {
            days = d
        } else if val == "0" {
            return // unlimited retention
        }
    }

    expired, err := c.store.ListExpiredContentSessions(ctx, days)
    if err != nil {
        c.logger.Warn("retention sweep: failed to list expired sessions", "error", err)
        return
    }

    if len(expired) == 0 {
        return
    }

    c.logger.Info("retention sweep: cleaning expired sessions", "count", len(expired), "retention_days", days)

    for _, s := range expired {
        if err := c.store.DeleteSessionContent(ctx, s.SessionID); err != nil {
            c.logger.Warn("retention sweep: failed to delete content",
                "error", err, "session_id", s.SessionID)
            continue
        }

        // Notify agent to delete .cast file
        agent := c.connMgr.GetAgent(s.MachineID)
        if agent != nil {
            cmd := &pb.ServerCommand{
                Command: &pb.ServerCommand_CleanupScrollback{
                    CleanupScrollback: &pb.CleanupScrollbackCmd{
                        SessionId: s.SessionID,
                    },
                },
            }
            if err := agent.SendCommand(cmd); err != nil {
                c.logger.Warn("retention sweep: failed to send cleanup to agent",
                    "error", err, "machine_id", s.MachineID, "session_id", s.SessionID)
                // Queue for later
                _ = c.store.AddPendingCleanup(ctx, s.SessionID, s.MachineID)
            }
        } else {
            // Agent offline — queue for reconnect
            _ = c.store.AddPendingCleanup(ctx, s.SessionID, s.MachineID)
        }
    }

    // Optimize FTS5 index after bulk deletes
    if err := c.store.OptimizeFTS(ctx); err != nil {
        c.logger.Warn("retention sweep: FTS optimize failed", "error", err)
    }
}

func parseInt(s string) int {
    n := 0
    for _, c := range s {
        if c < '0' || c > '9' {
            return 0
        }
        n = n*10 + int(c-'0')
    }
    return n
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/server/retention/cleaner.go
git commit -m "feat: retention cleaner — hourly sweep with agent cleanup notification"
```

### Task 8: Add CleanupScrollbackCmd to proto and agent

**Files:**
- Modify: `proto/claudeplane/v1/agent.proto`
- Modify: `internal/agent/session_manager.go`

- [ ] **Step 1: Add proto message and command**

In `agent.proto`, add the message (after `RequestScrollbackCmd`):

```protobuf
message CleanupScrollbackCmd {
  string session_id = 1;
}
```

Add to `ServerCommand` oneof (after `request_scrollback = 7`):

```protobuf
CleanupScrollbackCmd cleanup_scrollback = 8;
```

- [ ] **Step 2: Regenerate proto**

Run: `buf generate`

- [ ] **Step 3: Add handler in session_manager.go**

In `HandleCommand`, add a new case (after `RequestScrollback`):

```go
case *pb.ServerCommand_CleanupScrollback:
    sm.handleCleanupScrollback(c.CleanupScrollback)
```

Add the handler method:

```go
func (sm *SessionManager) handleCleanupScrollback(cmd *pb.CleanupScrollbackCmd) {
    sessionID := cmd.GetSessionId()
    castPath := filepath.Join(sm.dataDir, sessionID+".cast")
    if err := os.Remove(castPath); err != nil {
        if !os.IsNotExist(err) {
            sm.logger.Warn("failed to delete scrollback file",
                slog.String("session_id", sessionID),
                slog.String("path", castPath),
                slog.String("error", err.Error()),
            )
        }
        return
    }
    sm.logger.Info("deleted scrollback file",
        slog.String("session_id", sessionID),
        slog.String("path", castPath),
    )
}
```

- [ ] **Step 4: Wire retention cleaner in main.go**

In `cmd/server/main.go`, after the content ingestor creation:

```go
retentionCleaner := retention.NewCleaner(s, connMgr, slog.Default(), cfg.Retention.GetRetentionDays())
retentionCleaner.Start()
defer retentionCleaner.Stop()
```

Add import for the retention package.

- [ ] **Step 5: Send pending cleanups on agent reconnect**

In `internal/server/grpc/server.go`, in the `CommandStream` function, after successful agent registration (the `Register` RPC response handling), add:

```go
// Send pending cleanups for this agent
go func() {
    cleanups, err := s.store.ListPendingCleanups(context.Background(), machineID)
    if err != nil || len(cleanups) == 0 {
        return
    }
    for _, sessionID := range cleanups {
        cmd := &pb.ServerCommand{
            Command: &pb.ServerCommand_CleanupScrollback{
                CleanupScrollback: &pb.CleanupScrollbackCmd{SessionId: sessionID},
            },
        }
        if err := agent.SendCommand(cmd); err != nil {
            s.logger.Warn("failed to send pending cleanup", "error", err, "machine_id", machineID)
            return
        }
    }
    _ = s.store.DeletePendingCleanups(context.Background(), machineID)
    s.logger.Info("sent pending cleanups to agent", "machine_id", machineID, "count", len(cleanups))
}()
```

This requires adding a `store` field to `GRPCServer` and a `SetStore` setter, or reusing the existing session store interface. Check what store interface is already available on the gRPC server and use it, or add a new `ContentCleanupStore` interface with `ListPendingCleanups` and `DeletePendingCleanups`.

- [ ] **Step 6: Verify build and tests**

Run: `go vet ./... && go build -o /dev/null ./cmd/server && go build -o /dev/null ./cmd/agent`
Run: `go test -race github.com/kodrunhq/claude-plane/internal/...`

- [ ] **Step 7: Commit**

```bash
git add proto/ internal/agent/session_manager.go internal/server/retention/ internal/server/grpc/server.go cmd/server/main.go
git commit -m "feat: retention cleaner with agent .cast cleanup via CleanupScrollbackCmd"
```

---

## Chunk 5: Frontend Settings & Dead Code Cleanup

### Task 9: Add frontend settings API and retention UI

**Files:**
- Create: `web/src/api/settings.ts`
- Create: `web/src/hooks/useSettings.ts`
- Modify: `web/src/views/SettingsPage.tsx`

- [ ] **Step 1: Create settings API client**

```typescript
// web/src/api/settings.ts
import { request } from './client.ts';

export type ServerSettings = Record<string, string>;

export const settingsApi = {
  get: () => request<ServerSettings>('/settings'),
  update: (settings: Partial<ServerSettings>) =>
    request<ServerSettings>('/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    }),
};
```

- [ ] **Step 2: Create settings hooks**

```typescript
// web/src/hooks/useSettings.ts
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { settingsApi } from '../api/settings.ts';
import type { ServerSettings } from '../api/settings.ts';

export function useServerSettings() {
  return useQuery({
    queryKey: ['server-settings'],
    queryFn: () => settingsApi.get(),
  });
}

export function useUpdateServerSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (settings: Partial<ServerSettings>) => settingsApi.update(settings),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['server-settings'] }),
  });
}
```

- [ ] **Step 3: Add Data Retention tab to SettingsPage**

In `web/src/views/SettingsPage.tsx`:

Add to the TABS array:
```typescript
{ id: 'retention', label: 'Data Retention', icon: Database },
```

Import `Database` from lucide-react.

Add a `DataRetentionTab` component (can be inline or a separate file):

```tsx
function DataRetentionTab() {
  const { data: settings, isLoading } = useServerSettings();
  const updateSettings = useUpdateServerSettings();
  const currentValue = settings?.retention_days ?? '30';

  const options = [
    { value: '7', label: '7 days' },
    { value: '30', label: '30 days' },
    { value: '90', label: '90 days' },
    { value: '365', label: '1 year' },
    { value: '0', label: 'Unlimited' },
  ];

  const handleChange = async (value: string) => {
    await updateSettings.mutateAsync({ retention_days: value });
  };

  if (isLoading) return <div className="text-text-secondary text-sm">Loading...</div>;

  return (
    <div className="space-y-4">
      <div>
        <label className="block text-sm font-medium text-text-primary mb-1">
          Session content retention
        </label>
        <p className="text-xs text-text-secondary mb-2">
          Terminal output older than this is deleted from the server and agent machines.
          Running sessions are never affected.
        </p>
        <select
          value={currentValue}
          onChange={(e) => handleChange(e.target.value)}
          disabled={updateSettings.isPending}
          className="w-48 px-3 py-2 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary"
        >
          {options.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </div>
    </div>
  );
}
```

Add the tab content rendering in the switch/conditional:
```tsx
{activeTab === 'retention' && <DataRetentionTab />}
```

Import `useServerSettings` and `useUpdateServerSettings`.

- [ ] **Step 4: Update search API types**

In `web/src/api/search.ts`, add `offset` parameter support:

```typescript
export const searchApi = {
  sessions: (q: string, limit = 50, offset = 0) =>
    request<SearchResult[]>(
      `/search/sessions?q=${encodeURIComponent(q)}&limit=${limit}&offset=${offset}`
    ),
};
```

- [ ] **Step 5: Verify frontend**

Run: `cd web && npx tsc --noEmit && npm run lint && npx vitest run`

- [ ] **Step 6: Commit**

```bash
git add web/src/api/settings.ts web/src/hooks/useSettings.ts web/src/views/SettingsPage.tsx web/src/api/search.ts
git commit -m "feat: data retention settings UI and search pagination support"
```

### Task 10: Dead code audit and cleanup

**Files:**
- Audit across entire codebase

- [ ] **Step 1: Search for dead references**

Run these searches and fix any findings:

```bash
# Search for old search fan-out references
grep -r "SearchScrollback" --include="*.go" --include="*.ts" --include="*.tsx"
grep -r "fan.out\|fan-out" --include="*.go" --include="*.ts"

# Search for stale search comments
grep -r "TODO.*search\|TODO.*Search" --include="*.go"

# Check for unused imports
go vet ./...

# Frontend lint
cd web && npm run lint
```

- [ ] **Step 2: Remove any found dead code**

Expected removals:
- TODO comments in `search.go` (already replaced in Task 4)
- Any `SearchResult` type re-exports that reference the old struct
- Stale comments in `grpc/server.go` about "future search"
- The old `connmgr` import in `search.go` (already removed in Task 4)

- [ ] **Step 3: Verify nothing references the old SearchHandler constructor signature**

The old `NewSearchHandler(connMgr)` signature is replaced with `NewSearchHandler(s)`. Verify no tests or other files reference the old signature.

- [ ] **Step 4: Run full verification**

```bash
go vet ./...
go test -race github.com/kodrunhq/claude-plane/internal/...
go build -o /dev/null ./cmd/server
go build -o /dev/null ./cmd/agent
go build -o /dev/null ./cmd/bridge
cd web && npx tsc --noEmit && npm run lint && npx vitest run && npm run build
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: dead code cleanup — remove old search stub references"
```
