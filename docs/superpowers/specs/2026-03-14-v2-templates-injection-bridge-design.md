# V2 Technical Design: Session Templates, Injection & Bridge

**Date:** 2026-03-14
**Status:** Approved
**Depends on:** backend_v1.md, frontend_v1.md

---

## 1. Overview

Three features that compose into an event-driven automation platform:

- **Session Templates** — reusable execution contexts (what to run, with what parameters)
- **Session Injection** — push text into running sessions mid-flight via REST API
- **Polling Bridge** — separate binary with modular connectors (Telegram, GitHub) that translates external events into claude-plane API calls

Together: an external event (PR opened, CI failed, Telegram command) triggers a template-based session, and injection feeds context into it mid-flight.

### Design Principles

1. **No inbound exposure.** Server stays behind NATs/firewalls. External integrations use outbound-only polling.
2. **Composability.** Each feature is useful standalone. Templates work without the bridge. Injection works without templates.
3. **Backend + Frontend together.** Every backend feature ships with its complete frontend counterpart. No stubs, no deferred UI.
4. **Templates are execution contexts, not deployment targets.** Machine assignment belongs to jobs/steps or is chosen at launch time.

---

## 2. Decisions Log

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | All 4 phases designed and planned together | Full picture needed for coherent architecture |
| 2 | Open questions from original doc: go with all "leaning" positions | Except event feed — see #3 |
| 3 | Event feed uses existing `events` table + cursor-based query | `events` table already has `event_type`, timestamps, and indexed queries. No need to modify `audit_log` |
| 4 | Bridge is a separate binary in same repo (`cmd/bridge/`) | Shares proto types, single goreleaser config, keeps API contract in sync |
| 5 | `env_vars` on templates: literal values only, no `$cred:` resolution | CLIs on agents are already configured. `env_vars` is for workflow variables, not secrets |
| 6 | Drop `max_cost_usd` entirely | Subscription-based CLI usage — cost is external, not tracked |
| 7 | Drop `machine_id` from templates | Templates are execution contexts, not deployment targets. Machine is chosen at launch time or set in job steps |
| 8 | Keep `timeout_seconds` on templates | Useful for preventing runaway sessions |
| 9 | Bridge connector pattern: Go interface, per-connector packages | Each service has unique semantics. Clean interface without premature abstraction |
| 10 | `initial_prompt` stays in `CreateSessionCmd` (agent-side) | Already working. Injection is for mid-flight context only |
| 11 | Injection queue: in-memory with DB audit trail | Fire-and-deliver semantics. Queue state doesn't need persistence |
| 12 | Bridge connector config stored in server DB, managed via UI | Non-technical users configure connectors from the frontend, not TOML files |
| 13 | Bridge TOML is bootstrap-only (server URL + API key) | One-time setup, never touched again |
| 14 | Bridge restart via pull-based signal | Server sets `restart_requested_at`, bridge detects on next poll and exits cleanly for process manager to restart |

---

## 3. Session Templates

### 3.1 Data Model

**New table: `session_templates`**

```sql
CREATE TABLE session_templates (
    template_id     TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(user_id),
    name            TEXT NOT NULL,
    description     TEXT,
    command         TEXT,
    args            TEXT,           -- JSON array: ["--model", "opus"]
    working_dir     TEXT,
    env_vars        TEXT,           -- JSON object: {"KEY": "value"}
    initial_prompt  TEXT,
    terminal_rows   INTEGER NOT NULL DEFAULT 24,
    terminal_cols   INTEGER NOT NULL DEFAULT 80,
    tags            TEXT,           -- JSON array: ["ci", "review"]
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    deleted_at      DATETIME,       -- Soft delete
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);
CREATE INDEX idx_templates_user ON session_templates(user_id, deleted_at);
```

**Modified table: `sessions`**

```sql
ALTER TABLE sessions ADD COLUMN template_id TEXT REFERENCES session_templates(template_id);
```

### 3.2 Go Struct

```go
type SessionTemplate struct {
    TemplateID     string            `json:"template_id"`
    UserID         string            `json:"user_id"`
    Name           string            `json:"name"`
    Description    string            `json:"description,omitempty"`
    Command        string            `json:"command,omitempty"`
    Args           []string          `json:"args,omitempty"`
    WorkingDir     string            `json:"working_dir,omitempty"`
    EnvVars        map[string]string `json:"env_vars,omitempty"`
    InitialPrompt  string            `json:"initial_prompt,omitempty"`
    TerminalRows   int               `json:"terminal_rows"`
    TerminalCols   int               `json:"terminal_cols"`
    Tags           []string          `json:"tags,omitempty"`
    TimeoutSeconds int               `json:"timeout_seconds"`
    DeletedAt      *time.Time        `json:"deleted_at,omitempty"`
    CreatedAt      time.Time         `json:"created_at"`
    UpdatedAt      time.Time         `json:"updated_at"`
}
```

JSON arrays/objects serialized in the store layer (same pattern as `steps.args`).

### 3.3 Store Methods

```go
CreateTemplate(ctx, template) (*SessionTemplate, error)
GetTemplate(ctx, templateID) (*SessionTemplate, error)
GetTemplateByName(ctx, userID, name) (*SessionTemplate, error)
ListTemplates(ctx, userID, opts ListTemplateOptions) ([]SessionTemplate, error)
UpdateTemplate(ctx, templateID, template) (*SessionTemplate, error)
DeleteTemplate(ctx, templateID) error                   // soft delete
CloneTemplate(ctx, templateID) (*SessionTemplate, error) // copy with "-copy" suffix
```

### 3.4 REST API

| Method | Endpoint | Notes |
|--------|----------|-------|
| POST | `/api/v1/templates` | Validates name uniqueness per user |
| GET | `/api/v1/templates` | Filterable: `?tag=ci&name=review-pr`. Caller's templates only (admin sees all). `name` filter enables lookup by name for bridge/CLI |
| GET | `/api/v1/templates/{templateID}` | Standard get. Returns 404 for soft-deleted templates |
| PUT | `/api/v1/templates/{templateID}` | Full replacement. Returns 404 for soft-deleted templates |
| DELETE | `/api/v1/templates/{templateID}` | Soft delete |
| POST | `/api/v1/templates/{templateID}/clone` | Copy with `-copy` suffix. On name collision, retries with `-copy-2`, `-copy-3`, up to 10 attempts |

Note: Template lookup by name uses `?name=` query parameter on the list endpoint (not a separate `/by-name/{name}` route) to avoid Chi wildcard routing conflicts.

### 3.5 Template-Aware Session Creation

Existing `POST /api/v1/sessions` gains new optional fields:

```go
type createSessionRequest struct {
    // Existing fields...
    MachineID    string            `json:"machine_id"`
    Command      string            `json:"command"`
    Args         []string          `json:"args"`
    WorkingDir   string            `json:"working_dir"`
    // New fields...
    TemplateID   string            `json:"template_id"`
    TemplateName string            `json:"template_name"`
    Variables    map[string]string `json:"variables"`
}
```

**Merge order:** template defaults -> request body overrides -> variable substitution in `initial_prompt`.

Variable substitution uses `${VAR_NAME}` delimiters (not `{{.VarName}}` to avoid collision with Go template syntax in prompts). `strings.ReplaceAll` for each `${VAR_NAME}` -> value. Unresolved placeholders left as-is. Variable names must match `[A-Z][A-Z0-9_]*` (validated on template creation).

**Critical wire-up:** `session/handler.go` must be modified to populate `EnvVars` and `InitialPrompt` fields in the `CreateSessionCmd` proto message. These fields exist in the proto but the current handler does not pass them through. Without this change, templates with `initial_prompt` or `env_vars` will silently have no effect.

### 3.6 Handler

```go
type TemplateHandler struct {
    store     store.TemplateStoreIface
    getClaims handler.ClaimsGetter  // reuse existing type from handler package
}
```

**`TemplateStoreIface` definition** (in `store/templates.go`):

```go
type TemplateStoreIface interface {
    CreateTemplate(ctx context.Context, t *SessionTemplate) (*SessionTemplate, error)
    GetTemplate(ctx context.Context, templateID string) (*SessionTemplate, error)
    GetTemplateByName(ctx context.Context, userID, name string) (*SessionTemplate, error)
    ListTemplates(ctx context.Context, userID string, opts ListTemplateOptions) ([]SessionTemplate, error)
    UpdateTemplate(ctx context.Context, templateID string, t *SessionTemplate) (*SessionTemplate, error)
    DeleteTemplate(ctx context.Context, templateID string) error
    CloneTemplate(ctx context.Context, templateID string) (*SessionTemplate, error)
}
```

Follows existing handler patterns: constructor injection, `authorizeTemplate` guard (owner or admin, checks `deleted_at IS NULL`), `writeJSON`/`writeError` responses.

### 3.7 Frontend

**TypeScript types:**

```typescript
export interface SessionTemplate {
  template_id: string;
  user_id: string;
  name: string;
  description?: string;
  command?: string;
  args?: string[];
  working_dir?: string;
  env_vars?: Record<string, string>;
  initial_prompt?: string;
  terminal_rows: number;
  terminal_cols: number;
  tags?: string[];
  timeout_seconds: number;
  deleted_at?: string;
  created_at: string;
  updated_at: string;
}
```

**API client:** `templatesApi` with list, get, getByName, create, update, delete, clone.

**TanStack Query hooks:** `useTemplates()`, `useTemplate(id)`, mutation hooks.

**UI components:**

- **Command Center — Template Cards:** card grid below machine list. Each card shows name, description, tags (badges), "Launch" button. If template has `${VAR_NAME}` placeholders, Launch opens a modal with input fields for each variable + machine selector.
- **Template Editor (`/templates/{id}/edit` and `/templates/new`):** form with all fields. `initial_prompt` textarea with visual highlighting for `${VAR_NAME}` placeholders. `env_vars` key-value editor. `args` list editor. `tags` tag input. Save/Cancel.
- **Session Creation Modal:** "From Template" dropdown at top. Selecting a template pre-fills fields. User can override before confirming. Variable inputs appear if placeholders detected.
- **Sidebar Navigation:** Add "Templates" nav item (below "Jobs"). Add "Connectors" and "API Keys" nav items (in admin/settings section) in Phase 3.
- **Routes:** `/templates`, `/templates/new`, `/templates/{id}/edit`.

---

## 4. Session Injection

### 4.1 REST Endpoint

```
POST /api/v1/sessions/{sessionID}/inject
```

**Request:**

```go
type injectRequest struct {
    Text     string         `json:"text"`      // Required
    Raw      bool           `json:"raw"`       // Default false; if false, appends \n
    DelayMs  int            `json:"delay_ms"`  // Default 0
    Metadata map[string]any `json:"metadata"`  // Stored in audit, not sent to session
}
```

**Responses:**

| Status | Meaning |
|--------|---------|
| 202 Accepted | Injection queued. Body: `{"injection_id": "...", "queued_at": "..."}` |
| 404 Not Found | Session doesn't exist or caller lacks access |
| 409 Conflict | Session not in `running` state |
| 429 Too Many Requests | Queue full (32 items) |
| 503 Service Unavailable | Agent disconnected |

### 4.2 In-Memory Injection Queue

```go
// InjectionStoreIface — narrow interface for testability (follows existing pattern)
type InjectionStoreIface interface {
    CreateInjection(ctx context.Context, inj *Injection) (*Injection, error)
    UpdateInjectionDelivered(ctx context.Context, injectionID string, deliveredAt time.Time) error
}

type InjectionQueue struct {
    mu       sync.Mutex
    queues   map[string]*sessionQueue  // sessionID -> queue
    connMgr  *connmgr.ConnectionManager
    store    InjectionStoreIface       // narrow interface, not concrete *store.Store
    bus      *event.Bus                // subscribe to session lifecycle events
    logger   *slog.Logger
}

type sessionQueue struct {
    items chan queueItem  // capacity 32
    done  chan struct{}
}

type queueItem struct {
    InjectionID string
    Data        []byte
    DelayMs     int
    QueuedAt    time.Time
}
```

**Behavior:**
- Per-session channel, capacity 32
- Single drainer goroutine per session, created lazily on first injection
- Drainer sends each item via `connMgr.GetAgent(machineID).SendCommand(InputDataCmd)`
- Respects `delay_ms` between items
- Items older than 5 minutes silently dropped (stale context)
- Drainer exits when session ends or after 5 minutes idle
- Agent temporarily disconnected: drainer pauses, retries on reconnect

**Drainer lifecycle management:** The `InjectionQueue` subscribes to `session.*` events on the event bus at initialization. When a `session.exited` or `session.terminated` event is received, the queue closes the corresponding `sessionQueue.done` channel, which signals the drainer goroutine to exit. This prevents goroutine leaks on long-running deployments.

### 4.3 Audit Table

```sql
CREATE TABLE injections (
    injection_id TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES sessions(session_id),
    user_id      TEXT NOT NULL REFERENCES users(user_id),
    text_length  INTEGER NOT NULL,
    metadata     TEXT,            -- JSON
    source       TEXT NOT NULL,   -- 'manual', 'api', 'bridge-telegram', 'bridge-github'
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at DATETIME
);
CREATE INDEX idx_injections_session ON injections(session_id, created_at DESC);
```

Stores `text_length`, not the text itself (may contain sensitive content).

### 4.4 Store Methods

```go
CreateInjection(ctx, injection) (*Injection, error)
UpdateInjectionDelivered(ctx, injectionID, deliveredAt) error
ListInjectionsBySession(ctx, sessionID) ([]Injection, error)
```

### 4.5 Injection History Endpoint

```
GET /api/v1/sessions/{sessionID}/injections
```

Returns injection records for the session, ordered by `created_at DESC`. Same authorization as session access.

### 4.6 Data Flow

```
Caller (bridge / UI / curl)
  |
  POST /api/v1/sessions/{id}/inject
  |
  +-- Validate JWT, session ownership, session status == running
  +-- Write audit record (delivered_at = NULL)
  +-- Append to in-memory session queue
  +-- Return 202 with injection_id
  |
  v
Drainer goroutine
  |
  +-- Wait delay_ms
  +-- Append \n if raw=false
  +-- SendCommand(InputDataCmd{data})
  +-- Update injections: delivered_at = now
  |
  v
Agent PTY stdin -> Claude reads as user input
```

### 4.7 Frontend

**Session detail view — collapsible "Inject" panel below the terminal:**

- Textarea for input text
- "Send" button (also Ctrl+Enter)
- Toggle for `raw` mode (default off)
- Injection history list below textarea:
  - Each row: timestamp, source badge, text length, delivery status
  - Sourced from `GET /api/v1/sessions/{id}/injections`

---

## 5. Polling Bridge

### 5.1 Binary Structure

```
cmd/bridge/
    main.go                          // Cobra CLI, config loading, graceful shutdown

internal/bridge/
    config/
        config.go                    // Bootstrap TOML parsing
    connector/
        connector.go                 // Connector interface
        telegram/
            telegram.go
        github/
            github.go
    client/
        client.go                    // claude-plane REST API client
    state/
        state.go                     // High-water mark persistence
    bridge.go                        // Lifecycle: fetch config, start connectors, health
```

### 5.2 Connector Interface

```go
type Connector interface {
    Name() string
    Start(ctx context.Context) error  // blocks until ctx cancelled
    Healthy() bool
}
```

Each connector receives at construction:
- Parsed config from server (via `GET /api/v1/bridge/connectors`)
- `client.Client` for claude-plane API calls
- `state.Store` for high-water marks
- `*slog.Logger`

### 5.3 Bridge Lifecycle

```go
type Bridge struct {
    connectors []Connector
    health     *healthServer
    logger     *slog.Logger
}

func (b *Bridge) Run(ctx context.Context) error {
    // 1. Fetch connector configs from server
    // 2. Instantiate enabled connectors
    // 3. Start health endpoint (localhost:9090/healthz)
    // 4. Start each connector in its own goroutine
    // 5. Poll for restart signal periodically
    // 6. On ctx cancellation or restart signal: graceful shutdown (10s timeout)
}
```

- Each connector runs independently; one failing doesn't stop others
- Startup validates authentication (Telegram `getMe`, GitHub `GET /user`)
- Failed auth logs warning, doesn't block other connectors

### 5.4 Bootstrap TOML (minimal, one-time setup)

```toml
[claude_plane]
api_url = "http://localhost:8080"
api_key = "cpk_..."

[state]
path = "~/.claude-plane-bridge/state.json"

[health]
address = "localhost:9090"
```

This is the only file the bridge reads. All connector configuration comes from the server API.

### 5.5 Claude-Plane API Client

```go
type Client struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

func (c *Client) ListTemplates(ctx) ([]SessionTemplate, error)
func (c *Client) CreateSession(ctx, req) (*Session, error)
func (c *Client) InjectSession(ctx, sessionID, req) (*InjectionResult, error)
func (c *Client) ListSessions(ctx, opts) ([]Session, error)
func (c *Client) GetSession(ctx, sessionID) (*Session, error)
func (c *Client) KillSession(ctx, sessionID) error
func (c *Client) ListMachines(ctx) ([]Machine, error)
func (c *Client) PollEvents(ctx, afterCursor) ([]Event, string, error)
func (c *Client) GetConnectorConfigs(ctx) ([]ConnectorConfig, error)
func (c *Client) CheckRestartSignal(ctx, bootTime) (bool, error)
```

### 5.6 State Persistence

```go
type Store struct {
    path string
    mu   sync.Mutex
    data StateData
}

type StateData struct {
    Cursors   map[string]string `json:"cursors"`    // high-water marks
    Processed map[string]int64  `json:"processed"`  // deduplication: key -> Unix epoch seconds when processed
}

func (s *Store) GetCursor(key string) string
func (s *Store) SetCursor(key, value string) error
func (s *Store) MarkProcessed(key string) error
func (s *Store) IsProcessed(key string) bool
func (s *Store) Prune(olderThan time.Duration) int  // removes entries > 7 days
```

File: `~/.claude-plane-bridge/state.json` (configurable).

---

## 6. Server-Side Bridge Support

### 6.1 API Keys

**New table:**

```sql
CREATE TABLE api_keys (
    key_id       TEXT PRIMARY KEY,       -- public identifier (first 8 bytes of random portion)
    key_hmac     TEXT NOT NULL,           -- HMAC-SHA256(full_key, server_signing_key)
    user_id      TEXT NOT NULL REFERENCES users(user_id),
    name         TEXT NOT NULL,
    scopes       TEXT,                    -- JSON array or NULL for full access
    expires_at   DATETIME,
    last_used_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_api_keys_user ON api_keys(user_id);
```

**Auth middleware:** detects `cpk_` prefix on Bearer token. Extracts the `key_id` portion (fixed-length prefix after `cpk_`), looks up the DB row, then computes `HMAC-SHA256(presented_key, server_signing_key)` and compares against `key_hmac` using constant-time comparison. This avoids storing any part of the plaintext key and is appropriate for high-entropy random API keys (unlike bcrypt which is designed for low-entropy passwords). Updates `last_used_at` asynchronously.

**Key format:** `cpk_{key_id}_{random_bytes}` — the `key_id` is an 8-character hex string used for DB lookup, the remainder is the secret verified via HMAC.

**Store methods:**

```go
CreateAPIKey(ctx, userID, name, scopes, signingKey) (plaintextKey, keyID string, error)
GetAPIKeyByID(ctx, keyID) (*APIKey, error)
ListAPIKeys(ctx, userID) ([]APIKey, error)    // never returns hmac
DeleteAPIKey(ctx, keyID) error
UpdateAPIKeyLastUsed(ctx, keyID) error
```

**REST endpoints:**

| Method | Endpoint | Notes |
|--------|----------|-------|
| POST | `/api/v1/api-keys` | Creates key, returns plaintext once |
| GET | `/api/v1/api-keys` | Lists user's keys (admin sees all) |
| DELETE | `/api/v1/api-keys/{keyID}` | Revoke key |

**Frontend:** API Keys management page. Create key -> modal shows plaintext once with copy button and "won't be shown again" warning.

### 6.2 Connector Configuration (DB-managed)

**New table:**

```sql
CREATE TABLE bridge_connectors (
    connector_id   TEXT PRIMARY KEY,
    connector_type TEXT NOT NULL,          -- 'telegram', 'github'
    name           TEXT NOT NULL,
    enabled        INTEGER NOT NULL DEFAULT 1,
    config         TEXT NOT NULL,          -- JSON, type-specific (non-sensitive fields)
    config_secret  BLOB,                   -- AES-GCM encrypted JSON of sensitive fields (tokens)
    config_nonce   BLOB,                   -- per-connector nonce (consistent with credentials table pattern)
    created_by     TEXT NOT NULL REFERENCES users(user_id),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Sensitive fields (bot tokens, GitHub PATs) are stored separately in `config_secret` + `config_nonce`, encrypted with AES-GCM using the server's encryption key (same pattern as `credentials` table).

**Two response modes for `GET /api/v1/bridge/connectors`:**
- **UI access (JWT auth):** returns `config` with sensitive fields redacted (e.g., `"bot_token": "***set***"`). Never exposes decrypted secrets to the browser.
- **Bridge access (API key auth):** returns `config` with sensitive fields decrypted and merged. The server detects API key auth (vs JWT) and includes the plaintext values. This is a separate internal path, not a query parameter.

**REST endpoints:**

| Method | Endpoint | Notes |
|--------|----------|-------|
| POST | `/api/v1/bridge/connectors` | Create connector config |
| GET | `/api/v1/bridge/connectors` | List all connectors (bridge uses this to fetch config) |
| GET | `/api/v1/bridge/connectors/{connectorID}` | Get single connector |
| PUT | `/api/v1/bridge/connectors/{connectorID}` | Update config |
| DELETE | `/api/v1/bridge/connectors/{connectorID}` | Remove connector |

### 6.3 Bridge Control

**New table:**

```sql
CREATE TABLE bridge_control (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- Predefined keys:
--   'restart_requested_at' -> RFC3339 timestamp string
```

**REST endpoints:**

| Method | Endpoint | Notes |
|--------|----------|-------|
| POST | `/api/v1/bridge/restart` | Sets key `restart_requested_at` to current RFC3339 timestamp |
| GET | `/api/v1/bridge/status` | Returns `{"restart_requested_at": "2026-03-14T10:30:00Z", "healthy": true}`. `healthy` is populated by proxying bridge health URL if configured in server TOML (`bridge_health_url`) |

**Bridge restart flow:**
1. User saves connector config in UI -> banner: "Configuration saved. Restart bridges to apply."
2. User clicks "Apply & Restart" -> `POST /api/v1/bridge/restart`
3. Bridge polls `GET /api/v1/bridge/status` every 10-30s
4. Detects `restart_requested_at` > boot time -> exits cleanly
5. Process manager (systemd/launchd/Docker) restarts bridge
6. Bridge fetches fresh config on startup

### 6.4 Event Feed Endpoint

```
GET /api/v1/events/feed?after={cursor}&limit=50
```

Built on existing `events` table. Returns events after cursor with `next_cursor` for efficient polling.

**Cursor format:** compound `"{ISO8601_timestamp}|{event_id}"` string. The query uses `WHERE (timestamp, event_id) > (?, ?) ORDER BY timestamp ASC, event_id ASC`. This is deterministic regardless of UUID ordering (existing UUIDs are v4/random, not time-ordered). The `next_cursor` in the response is the last row's compound value. An empty `after` parameter returns the most recent events.

**New store method:**

```go
ListEventsAfter(ctx context.Context, afterTimestamp time.Time, afterEventID string, limit int) ([]Event, error)
```

The handler parses the cursor string into its two components before calling the store.

### 6.5 Missing Event Types to Add

- `session.terminated` — distinct from `session.exited`. The existing `TypeSessionExited` covers both process exit and user-initiated termination. Add `TypeSessionTerminated = "session.terminated"` and emit it from `TerminateSession` handler. The payload includes `"reason"` field: `"user"`, `"timeout"`, or `"admin"`. Keep `session.exited` for process-initiated exits only.
- `job.run.step_completed` — individual step completion
- `job.run.step_failed` — individual step failure

Emitted by adding `publisher.Publish()` calls in existing orchestrator/executor code. New constants added to `event/types.go` and builders to `event/builders.go`.

### 6.6 Frontend: Connectors Page

**Route:** `/connectors`

**Empty state:** "No connectors configured" with "Add Connector" button.

**Add Connector flow:** type picker (cards for Telegram, GitHub with icons).

**Telegram form:**
- Bot Token (password input) + "How to create a bot" help link
- Group ID (text input)
- Events Topic ID, Commands Topic ID (number inputs)
- Poll Timeout (number, default 30s)
- Event Filters (multi-select checkboxes for event types)
- "Test Connection" button

**GitHub form:**
- GitHub Token (password input) + scope requirements help text
- "Test Connection" button
- Watches section (add/remove):
  - Repository (`owner/repo`)
  - Template (dropdown from user's templates)
  - Poll Interval (select: 30s, 60s, 120s, 300s)
  - Triggers (expandable per type, enable/disable toggle, filter fields)

**Connector cards:** existing connectors show as cards with status indicator, enable/disable toggle, edit/delete.

**Frontend: API Keys page** under admin/settings area.

---

## 7. Telegram Connector

### 7.1 Outbound (claude-plane -> Telegram Events topic)

Polls `GET /api/v1/events/feed?after={cursor}` and formats events into Telegram messages.

| Event Type | Message Format |
|-----------|----------------|
| `session.started` | `Session {name} started on {machine}` |
| `session.exited` (exit 0) | `Session {id} completed -- {duration}` |
| `session.exited` (exit != 0) | `Session {id} failed (exit {code}) -- {duration}` |
| `session.terminated` | `Session {id} killed -- {reason}` |
| `machine.connected` | `Agent {machine} connected` |
| `machine.disconnected` | `Agent {machine} disconnected` |
| `run.completed` | `Job {name} run #{n} completed -- {passed}/{total} steps` |
| `run.failed` | `Job {name} run #{n} failed at step {step}` |

Messages include inline buttons where actionable: "View Session" (deep link), "Kill" (confirmation).

### 7.2 Inbound (Telegram Commands topic -> claude-plane)

Uses `getUpdates` long-polling. Parses slash commands:

| Command | Maps to |
|---------|---------|
| `/start <template> [machine] \| VAR=val` | `POST /api/v1/sessions` with template + variables |
| `/list` | `GET /api/v1/templates` |
| `/status` | `GET /api/v1/sessions?status=running` |
| `/status <session-id>` | `GET /api/v1/sessions/{id}` |
| `/kill <session-id>` | Confirmation button -> `DELETE /api/v1/sessions/{id}` |
| `/inject <session-id> <text>` | `POST /api/v1/sessions/{id}/inject` |
| `/machines` | `GET /api/v1/machines` |
| `/help` | Local |

### 7.3 Error Handling

- API unreachable -> reply: "Control plane unreachable. Retrying..."
- Command fails -> reply with API error
- Telegram 429 -> back off using `retry_after`
- Destructive actions -> inline keyboard confirmation

### 7.4 Config (stored in `bridge_connectors.config`)

```json
{
  "bot_token": "...",
  "group_id": -1001234567890,
  "events_topic_id": 2,
  "commands_topic_id": 3,
  "poll_timeout_seconds": 30,
  "event_types": ["session.*", "machine.*", "run.*"]
}
```

---

## 8. GitHub Connector

### 8.1 Polling

Per configured watch, polls GitHub REST API:

| Trigger | Endpoint | Default Interval |
|---------|----------|-----------------|
| PR opened/updated | `GET /repos/{owner}/{repo}/pulls?state=open&sort=updated` | 60s |
| CI check completed | `GET /repos/{owner}/{repo}/check-runs?filter=latest` | 60s |
| Issue labeled | `GET /repos/{owner}/{repo}/issues?labels={label}&sort=updated` | 120s |

Tracks `updated_at` high-water mark per repo+trigger. Monitors `X-RateLimit-Remaining` and backs off below 10%.

### 8.2 Event Processing

1. Match against trigger rules + filters
2. Check deduplication (`state.Store.IsProcessed`)
3. Fetch additional context (PR diff URL, CI log)
4. Build `variables` map
5. `POST /api/v1/sessions` with `template_name` + `variables`
6. Mark processed in state store

**Variables per trigger:**

| Trigger | Variables |
|---------|-----------|
| `pull_request.opened/updated` | `PR_URL`, `PR_TITLE`, `PR_BODY`, `PR_AUTHOR`, `PR_BRANCH`, `PR_BASE`, `PR_NUMBER`, `PR_DIFF_URL`, `REPO_FULL_NAME` |
| `check_run.completed` | `CHECK_NAME`, `CHECK_STATUS`, `CHECK_CONCLUSION`, `CHECK_URL`, `CHECK_OUTPUT` (4KB max), `PR_URL`, `REPO_FULL_NAME` |
| `issue.labeled` | `ISSUE_URL`, `ISSUE_TITLE`, `ISSUE_BODY`, `ISSUE_AUTHOR`, `ISSUE_LABELS`, `ISSUE_NUMBER`, `REPO_FULL_NAME` |

### 8.3 Filters

All AND-combined:

| Filter | Description |
|--------|-------------|
| `branches` | PRs targeting these base branches |
| `labels` | PRs/issues with these labels |
| `check_names` | These check run names only |
| `conclusions` | These conclusions only (default: `["failure", "timed_out"]`) |
| `paths` | Changed files matching globs (extra API call) |
| `authors_ignore` | Skip these authors |

### 8.4 Config (stored in `bridge_connectors.config`)

```json
{
  "token": "ghp_...",
  "watches": [
    {
      "repo": "kodrunhq/kodrun",
      "template": "review-pr",
      "poll_interval": "60s",
      "triggers": {
        "pull_request_opened": {
          "enabled": true,
          "branches": ["main"],
          "labels": [],
          "authors_ignore": ["dependabot[bot]"],
          "paths": []
        },
        "check_run_completed": {
          "enabled": true,
          "check_names": ["CI / test", "CI / lint"],
          "conclusions": ["failure", "timed_out"]
        }
      }
    }
  ]
}
```

---

## 9. Implementation Phases

### Phase 1: Session Templates

**Backend:**
- Migration: `session_templates` table + `sessions.template_id` column
- Store: template CRUD methods
- Handler: `TemplateHandler` with full REST API
- Modified session creation: template merge logic, variable substitution
- Events: `template.created`, `template.updated`, `template.deleted`

**Frontend:**
- Types, API client, TanStack Query hooks
- Template card grid on Command Center
- Template editor page (`/templates/new`, `/templates/{id}/edit`)
- "From Template" integration in session creation modal
- Launch modal with variable inputs + machine selector

**Tests:** template CRUD, merge semantics, variable substitution, authorization, frontend components

### Phase 2: Session Injection

**Backend:**
- Migration: `injections` audit table
- Store: injection CRUD methods
- Handler: inject endpoint on session handler + injection history endpoint
- In-memory injection queue with drainer goroutines

**Frontend:**
- Inject panel in session detail view
- Injection history list
- Types, API client, hooks

**Tests:** injection delivery, queue ordering, delay, authorization, rate limiting (429), session state checks (409)

### Phase 3: Bridge Core + Telegram

**Backend (server-side):**
- Migration: `api_keys` table, `bridge_connectors` table, `bridge_control` table
- Store + handler: API key CRUD, connector config CRUD, bridge control endpoints
- Auth middleware: `cpk_` prefix detection, HMAC-SHA256 validation
- Event feed endpoint (`GET /api/v1/events/feed`) with compound cursor
- Missing event types: `session.terminated`, step events
- Refactor `api.NewRouter` to accept a `HandlerSet` struct instead of growing parameter list (currently 13+ params, would reach 17+ with new handlers)

**Bridge binary:**
- `cmd/bridge/main.go` — Cobra CLI, bootstrap TOML, graceful shutdown
- Connector interface + bridge lifecycle
- claude-plane API client
- State persistence
- Telegram connector: outbound events + inbound commands
- Restart signal detection

**Frontend:**
- Connectors page (`/connectors`) with add/edit/delete
- Telegram connector form
- API Keys management page
- Bridge status widget + "Apply & Restart" button

**Tests:** API key auth, connector config CRUD, event feed cursor, Telegram message formatting, command parsing

### Phase 4: GitHub Connector

**Bridge:**
- GitHub connector: polling, event processing, variable extraction
- Filter evaluation engine
- Deduplication via state store
- Rate limit monitoring

**Frontend:**
- GitHub connector form with watches sub-section
- Trigger configuration UI with filter fields

**Tests:** GitHub event parsing, filter logic, deduplication, rate limit handling, variable extraction

---

## 10. New Database Tables Summary

| Table | Phase | Purpose |
|-------|-------|---------|
| `session_templates` | 1 | Template definitions |
| `injections` | 2 | Injection audit trail |
| `api_keys` | 3 | Long-lived API keys for bridge |
| `bridge_connectors` | 3 | Connector configurations (managed via UI) |
| `bridge_control` | 3 | Bridge restart signals |

**Modified tables:**
| Table | Phase | Change |
|-------|-------|--------|
| `sessions` | 1 | Add `template_id` FK |

---

## 11. New API Endpoints Summary

| Phase | Method | Endpoint |
|-------|--------|----------|
| 1 | POST | `/api/v1/templates` |
| 1 | GET | `/api/v1/templates` |
| 1 | GET | `/api/v1/templates/{templateID}` |
| 1 | PUT | `/api/v1/templates/{templateID}` |
| 1 | DELETE | `/api/v1/templates/{templateID}` |
| 1 | POST | `/api/v1/templates/{templateID}/clone` |
| 2 | POST | `/api/v1/sessions/{sessionID}/inject` |
| 2 | GET | `/api/v1/sessions/{sessionID}/injections` |
| 3 | POST | `/api/v1/api-keys` |
| 3 | GET | `/api/v1/api-keys` |
| 3 | DELETE | `/api/v1/api-keys/{keyID}` |
| 3 | POST | `/api/v1/bridge/connectors` |
| 3 | GET | `/api/v1/bridge/connectors` |
| 3 | GET | `/api/v1/bridge/connectors/{connectorID}` |
| 3 | PUT | `/api/v1/bridge/connectors/{connectorID}` |
| 3 | DELETE | `/api/v1/bridge/connectors/{connectorID}` |
| 3 | POST | `/api/v1/bridge/restart` |
| 3 | GET | `/api/v1/bridge/status` |
| 3 | GET | `/api/v1/events/feed` |

---

## 12. New Frontend Routes Summary

| Phase | Route | Purpose |
|-------|-------|---------|
| 1 | `/templates` | Template list |
| 1 | `/templates/new` | Create template |
| 1 | `/templates/{id}/edit` | Edit template |
| 3 | `/connectors` | Connector configuration |
| 3 | `/api-keys` | API key management |
