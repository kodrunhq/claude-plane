# Phase 3: Bridge Core + Telegram — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build server-side bridge support (API keys, connector config management, event feed, bridge control) and the bridge binary with Telegram connector. The bridge connects to external services via outbound-only protocols and translates events into claude-plane API calls.

**Architecture:** Three sub-systems: (1) Server-side — API keys auth, connector config CRUD with encrypted secrets, event feed endpoint, bridge restart control. (2) Bridge binary — `cmd/bridge/` with connector interface, bootstrap TOML, lifecycle management, state persistence. (3) Telegram connector — long-polling for inbound commands, event feed polling for outbound notifications.

**Tech Stack:** Go (server handlers, bridge binary, Cobra CLI), SQLite migration, HMAC-SHA256, AES-GCM, Telegram Bot API, React 19, TypeScript

**Design Spec:** `docs/superpowers/specs/2026-03-14-v2-templates-injection-bridge-design.md` — Sections 5, 6, 7

**Depends on:** Phase 2 (injection) must be complete — bridge uses inject endpoint.

---

## File Map

### Server-Side — Create
| File | Responsibility |
|------|---------------|
| `internal/server/store/apikeys.go` | API key store methods |
| `internal/server/store/apikeys_test.go` | Tests |
| `internal/server/store/bridge.go` | Bridge connector config + control store methods |
| `internal/server/store/bridge_test.go` | Tests |
| `internal/server/handler/apikeys.go` | API key REST handler |
| `internal/server/handler/apikeys_test.go` | Tests |
| `internal/server/handler/bridge.go` | Bridge connector config + control REST handler |
| `internal/server/handler/bridge_test.go` | Tests |

### Server-Side — Modify
| File | Change |
|------|--------|
| `internal/server/store/migrations.go` | Migration v7: `api_keys`, `bridge_connectors`, `bridge_control` tables |
| `internal/server/api/router.go` | Register API key + bridge routes; refactor to `HandlerSet` struct |
| `internal/server/api/middleware.go` | Add API key auth detection (`cpk_` prefix) alongside JWT |
| `internal/server/store/events.go` | Add `ListEventsAfter` cursor-based query |
| `internal/server/store/events_test.go` | Tests for cursor query |
| `internal/server/handler/events.go` | Add event feed endpoint |
| `internal/server/event/event.go` | Add `TypeSessionTerminated` constant |
| `internal/server/event/builders.go` | Add `NewSessionTerminatedEvent` builder with reason field |
| `internal/server/session/handler.go` | Change `TerminateSession` to emit `TypeSessionTerminated` instead of `TypeSessionExited` |

### Bridge Binary — Create
| File | Responsibility |
|------|---------------|
| `cmd/bridge/main.go` | Cobra CLI entry point, TOML loading, graceful shutdown |
| `internal/bridge/config/config.go` | Bootstrap TOML parsing |
| `internal/bridge/config/config_test.go` | Tests |
| `internal/bridge/connector/connector.go` | `Connector` interface definition |
| `internal/bridge/client/client.go` | Claude-plane REST API client |
| `internal/bridge/client/client_test.go` | Tests |
| `internal/bridge/state/state.go` | High-water mark + dedup state persistence |
| `internal/bridge/state/state_test.go` | Tests |
| `internal/bridge/bridge.go` | Bridge lifecycle (fetch config, start connectors, health, restart signal) |
| `internal/bridge/bridge_test.go` | Tests |
| `internal/bridge/connector/telegram/telegram.go` | Telegram connector |
| `internal/bridge/connector/telegram/telegram_test.go` | Tests |
| `internal/bridge/connector/telegram/formatter.go` | Event → Telegram message formatting |
| `internal/bridge/connector/telegram/commands.go` | Slash command parsing and dispatch |

### Frontend — Create
| File | Responsibility |
|------|---------------|
| `web/src/types/apikey.ts` | API key TypeScript interface |
| `web/src/types/connector.ts` | Connector config TypeScript interface |
| `web/src/api/apikeys.ts` | API key API client |
| `web/src/api/bridge.ts` | Bridge connector + control API client |
| `web/src/hooks/useApiKeys.ts` | TanStack Query hooks |
| `web/src/hooks/useBridge.ts` | TanStack Query hooks |
| `web/src/views/ApiKeysPage.tsx` | API key management page |
| `web/src/views/ConnectorsPage.tsx` | Connector configuration page |
| `web/src/components/connectors/ConnectorCard.tsx` | Connector status card |
| `web/src/components/connectors/TelegramForm.tsx` | Telegram connector config form |
| `web/src/components/connectors/AddConnectorModal.tsx` | Type picker modal |
| `web/src/components/apikeys/CreateKeyModal.tsx` | Create key + show plaintext modal |

### Frontend — Modify
| File | Change |
|------|--------|
| `web/src/App.tsx` | Add `/connectors`, `/api-keys` routes |
| `web/src/components/layout/Sidebar.tsx` | Add "Connectors" and "API Keys" nav items |

---

## Chunk 1: Server — API Keys

### Task 1: Migration v7

**Files:**
- Modify: `internal/server/store/migrations.go`

- [ ] **Step 1: Add migration**

```go
{
    Version: 7,
    SQL: `
CREATE TABLE api_keys (
    key_id       TEXT PRIMARY KEY,
    key_hmac     TEXT NOT NULL,
    user_id      TEXT NOT NULL REFERENCES users(user_id),
    name         TEXT NOT NULL,
    scopes       TEXT,
    expires_at   DATETIME,
    last_used_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_api_keys_user ON api_keys(user_id);

CREATE TABLE bridge_connectors (
    connector_id   TEXT PRIMARY KEY,
    connector_type TEXT NOT NULL,
    name           TEXT NOT NULL,
    enabled        INTEGER NOT NULL DEFAULT 1,
    config         TEXT NOT NULL,
    config_secret  BLOB,
    config_nonce   BLOB,
    created_by     TEXT NOT NULL REFERENCES users(user_id),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE bridge_control (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
    `,
},
```

- [ ] **Step 2: Run tests**

Run: `go test -race ./internal/server/store/ -run TestMigrations -v`
Expected: PASS

- [ ] **Step 3: Commit**

```
git add internal/server/store/migrations.go
git commit -m "feat: add migration v7 for api_keys, bridge_connectors, bridge_control"
```

---

### Task 2: API Key Store

**Files:**
- Create: `internal/server/store/apikeys.go`
- Create: `internal/server/store/apikeys_test.go`

- [ ] **Step 1: Write failing tests**

Test cases:
- `CreateAPIKey`: returns plaintext key starting with `cpk_`, key_id, and stores HMAC hash
- `GetAPIKeyByID`: returns APIKey struct (without hash in JSON)
- `ValidateAPIKey`: given plaintext key, computes HMAC, finds and verifies match
- `ListAPIKeys`: returns user's keys; admin sees all
- `DeleteAPIKey`: removes key; subsequent validate fails
- `UpdateAPIKeyLastUsed`: updates timestamp
- Expired key: validate returns error

- [ ] **Step 2: Implement**

```go
type APIKey struct {
    KeyID      string     `json:"key_id"`
    UserID     string     `json:"user_id"`
    Name       string     `json:"name"`
    Scopes     []string   `json:"scopes,omitempty"`
    ExpiresAt  *time.Time `json:"expires_at,omitempty"`
    LastUsedAt *time.Time `json:"last_used_at,omitempty"`
    CreatedAt  time.Time  `json:"created_at"`
}

func (s *Store) CreateAPIKey(ctx context.Context, userID, name string, scopes []string, signingKey []byte) (plaintextKey string, keyID string, err error)
func (s *Store) GetAPIKeyByID(ctx context.Context, keyID string) (*APIKey, error)
func (s *Store) ValidateAPIKey(ctx context.Context, plaintextKey string, signingKey []byte) (*APIKey, error)
func (s *Store) ListAPIKeys(ctx context.Context, userID string) ([]APIKey, error)
func (s *Store) DeleteAPIKey(ctx context.Context, keyID string) error
func (s *Store) UpdateAPIKeyLastUsed(ctx context.Context, keyID string) error
```

Key format: `cpk_{8-char-hex-keyid}_{32-byte-random-base64url}`. HMAC computed over full key using `crypto/hmac` + `crypto/sha256`. Constant-time comparison via `hmac.Equal`.

- [ ] **Step 3: Run tests**

Run: `go test -race ./internal/server/store/ -run TestAPIKey -v`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/server/store/apikeys.go internal/server/store/apikeys_test.go
git commit -m "feat: add API key store with HMAC-SHA256 validation"
```

---

### Task 3: API Key Auth Middleware

**Files:**
- Modify: `internal/server/api/middleware.go` (or wherever `JWTAuthMiddleware` is defined)

- [ ] **Step 1: Write failing tests**

Test cases:
- Request with `Authorization: Bearer cpk_...` — middleware validates via API key store, sets user claims in context
- Request with regular JWT Bearer token — existing behavior unchanged
- Request with `cpk_` prefix but invalid key — 401
- Request with expired API key — 401
- `last_used_at` updated asynchronously after successful auth

- [ ] **Step 2: Modify JWTAuthMiddleware to detect cpk_ prefix**

Before JWT parsing, check if token starts with `cpk_`. If so:
1. Call `store.ValidateAPIKey(token, signingKey)`
2. Look up user by `apiKey.UserID`
3. Build claims from user record
4. Set claims in context (same key as JWT path)
5. Fire-and-forget `store.UpdateAPIKeyLastUsed(keyID)` in a goroutine

- [ ] **Step 3: Run tests**

Run: `go test -race ./internal/server/api/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/server/api/
git commit -m "feat: add API key authentication in auth middleware"
```

---

### Task 4: API Key Handler

**Files:**
- Create: `internal/server/handler/apikeys.go`
- Create: `internal/server/handler/apikeys_test.go`

- [ ] **Step 1: Write failing tests**

- `POST /api/v1/api-keys` — creates key, returns plaintext once (201)
- `POST` with missing name — 400
- `GET /api/v1/api-keys` — lists user's keys (no hash/hmac in response)
- `DELETE /api/v1/api-keys/{keyID}` — deletes key (204)
- Non-owner delete — 404

- [ ] **Step 2: Implement handler**

```go
type APIKeyHandler struct {
    store      *store.Store
    signingKey []byte
    getClaims  ClaimsGetter
}

func RegisterAPIKeyRoutes(r chi.Router, h *APIKeyHandler) {
    r.Post("/api/v1/api-keys", h.Create)
    r.Get("/api/v1/api-keys", h.List)
    r.Delete("/api/v1/api-keys/{keyID}", h.Delete)
}
```

Create response includes `key` field (plaintext, shown once) and `key_id`.

- [ ] **Step 3: Run tests**

Run: `go test -race ./internal/server/handler/ -run TestAPIKey -v`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/server/handler/apikeys.go internal/server/handler/apikeys_test.go
git commit -m "feat: add API key handler with CRUD endpoints"
```

---

## Chunk 2: Server — Bridge Config + Event Feed

### Task 5: Bridge Connector Config Store

**Files:**
- Create: `internal/server/store/bridge.go`
- Create: `internal/server/store/bridge_test.go`

- [ ] **Step 1: Write failing tests**

Test cases:
- `CreateConnector`: insert with config + encrypted secret, verify stored
- `GetConnector`: returns config; with `decrypt=true` flag returns decrypted secrets
- `ListConnectors`: returns all connectors
- `UpdateConnector`: updates config + re-encrypts secrets
- `DeleteConnector`: removes connector
- `SetBridgeControl`: sets key-value pair
- `GetBridgeControl`: retrieves by key

- [ ] **Step 2: Implement**

```go
type BridgeConnector struct {
    ConnectorID   string    `json:"connector_id"`
    ConnectorType string    `json:"connector_type"`
    Name          string    `json:"name"`
    Enabled       bool      `json:"enabled"`
    Config        string    `json:"config"`         // JSON, non-sensitive fields
    CreatedBy     string    `json:"created_by"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}
```

Sensitive fields (tokens) encrypted/decrypted using existing `crypto.go` `Encrypt`/`Decrypt` functions with server encryption key.

- [ ] **Step 3: Run tests and commit**

```
git add internal/server/store/bridge.go internal/server/store/bridge_test.go
git commit -m "feat: add bridge connector config and control store"
```

---

### Task 6: Event Feed Endpoint + Missing Event Types

**Files:**
- Modify: `internal/server/store/events.go`
- Modify: `internal/server/handler/events.go`
- Modify: `internal/server/event/event.go`
- Modify: `internal/server/event/builders.go`
- Modify: `internal/server/session/handler.go`

- [ ] **Step 1: Add ListEventsAfter store method**

Cursor-based query using compound `(timestamp, event_id)`:
```go
func (s *Store) ListEventsAfter(ctx context.Context, afterTimestamp time.Time, afterEventID string, limit int) ([]Event, error)
```

- [ ] **Step 2: Add event feed handler**

In `handler/events.go`, add:
```go
func (h *EventHandler) Feed(w http.ResponseWriter, r *http.Request) {
    cursor := r.URL.Query().Get("after")
    // Parse compound cursor: "2026-03-14T10:00:00Z|event-uuid"
    // Query store
    // Return events + next_cursor
}
```

Register route: `r.Get("/api/v1/events/feed", eventHandler.Feed)`

- [ ] **Step 3: Add TypeSessionTerminated event**

In `event.go`: `TypeSessionTerminated = "session.terminated"`

In `builders.go`:
```go
func NewSessionTerminatedEvent(sessionID, machineID, reason string) Event {
    return newEvent(TypeSessionTerminated, "session", map[string]any{
        "session_id": sessionID,
        "machine_id": machineID,
        "reason":     reason,
    })
}
```

- [ ] **Step 4: Fix TerminateSession to use new event type**

In `session/handler.go` line 269, change:
```go
// Before:
h.publishEvent(r.Context(), event.NewSessionEvent(event.TypeSessionExited, sessionID, sess.MachineID))
// After:
h.publishEvent(r.Context(), event.NewSessionTerminatedEvent(sessionID, sess.MachineID, "user"))
```

- [ ] **Step 5: Run tests**

Run: `go test -race ./internal/server/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```
git add internal/server/store/events.go internal/server/handler/events.go internal/server/event/ internal/server/session/handler.go
git commit -m "feat: add event feed endpoint and session.terminated event type"
```

---

### Task 7: Bridge Handler + Router Refactor

**Files:**
- Create: `internal/server/handler/bridge.go`
- Create: `internal/server/handler/bridge_test.go`
- Modify: `internal/server/api/router.go`

- [ ] **Step 1: Implement bridge handler**

```go
type BridgeHandler struct {
    store     *store.Store
    getClaims ClaimsGetter
    encKey    []byte
}

func RegisterBridgeRoutes(r chi.Router, h *BridgeHandler) {
    r.Post("/api/v1/bridge/connectors", h.CreateConnector)
    r.Get("/api/v1/bridge/connectors", h.ListConnectors)
    r.Get("/api/v1/bridge/connectors/{connectorID}", h.GetConnector)
    r.Put("/api/v1/bridge/connectors/{connectorID}", h.UpdateConnector)
    r.Delete("/api/v1/bridge/connectors/{connectorID}", h.DeleteConnector)
    r.Post("/api/v1/bridge/restart", h.Restart)
    r.Get("/api/v1/bridge/status", h.Status)
}
```

`ListConnectors` and `GetConnector`: detect auth type (API key vs JWT). If API key auth, return decrypted secrets. If JWT auth, redact sensitive fields.

- [ ] **Step 2: Refactor NewRouter to use HandlerSet struct**

Replace the growing parameter list with:
```go
type HandlerSet struct {
    Session    *session.SessionHandler
    Job        *handler.JobHandler
    Run        *handler.RunHandler
    Event      *handler.EventHandler
    Webhook    *handler.WebhookHandler
    Trigger    *handler.TriggerHandler
    Ingest     *handler.IngestHandler
    Schedule   *handler.ScheduleHandler
    User       *handler.UserHandler
    Credential *handler.CredentialHandler
    Template   *handler.TemplateHandler
    APIKey     *handler.APIKeyHandler
    Bridge     *handler.BridgeHandler
}

func NewRouter(h *Handlers, hs HandlerSet, wsHandler, eventsWSHandler http.HandlerFunc) chi.Router
```

Update all callers.

- [ ] **Step 3: Run tests and commit**

```
git add internal/server/handler/bridge.go internal/server/handler/bridge_test.go internal/server/api/router.go
git commit -m "feat: add bridge handler and refactor router to HandlerSet"
```

---

## Chunk 3: Bridge Binary

### Task 8: Bridge Config + Client + State

**Files:**
- Create: `cmd/bridge/main.go`
- Create: `internal/bridge/config/config.go`
- Create: `internal/bridge/client/client.go`
- Create: `internal/bridge/state/state.go`
- Create: `internal/bridge/connector/connector.go`
- Create tests for each

- [ ] **Step 1: Bootstrap TOML config**

```go
type Config struct {
    ClaudePlane struct {
        APIURL string `toml:"api_url"`
        APIKey string `toml:"api_key"`
    } `toml:"claude_plane"`
    State struct {
        Path string `toml:"path"`
    } `toml:"state"`
    Health struct {
        Address string `toml:"address"`
    } `toml:"health"`
}
```

- [ ] **Step 2: Claude-plane API client**

HTTP client with all methods from spec section 5.5. Uses `Authorization: Bearer cpk_...` header. Each method maps to one REST call.

- [ ] **Step 3: State persistence**

JSON file with cursors and processed dedup maps. Thread-safe reads/writes. Prune method removes entries older than 7 days.

- [ ] **Step 4: Connector interface**

```go
type Connector interface {
    Name() string
    Start(ctx context.Context) error
    Healthy() bool
}
```

- [ ] **Step 5: Cobra CLI entry point**

`cmd/bridge/main.go` with `serve` subcommand:
- Load TOML config
- Create API client
- Fetch connector configs from server
- Instantiate enabled connectors
- Start bridge lifecycle

- [ ] **Step 6: Run tests and commit**

```
git add cmd/bridge/ internal/bridge/
git commit -m "feat: add bridge binary scaffold with config, client, state, connector interface"
```

---

### Task 9: Bridge Lifecycle + Health

**Files:**
- Create: `internal/bridge/bridge.go`
- Create: `internal/bridge/bridge_test.go`

- [ ] **Step 1: Implement Bridge struct**

```go
type Bridge struct {
    client     *client.Client
    state      *state.Store
    connectors []connector.Connector
    health     *healthServer
    logger     *slog.Logger
    bootTime   time.Time
}

func (b *Bridge) Run(ctx context.Context) error {
    // 1. Fetch connector configs
    // 2. Instantiate enabled connectors
    // 3. Validate auth per connector (log warnings, don't exit)
    // 4. Start health endpoint
    // 5. Start each connector in goroutine
    // 6. Poll for restart signal every 15s
    // 7. On ctx cancel or restart signal: graceful shutdown (10s)
}
```

Health endpoint: `GET /healthz` returns JSON with per-connector status.

- [ ] **Step 2: Test lifecycle**

Test: start bridge with mock connector, verify it starts. Cancel context, verify graceful shutdown. Set restart signal, verify bridge exits.

- [ ] **Step 3: Commit**

```
git add internal/bridge/bridge.go internal/bridge/bridge_test.go
git commit -m "feat: add bridge lifecycle with health endpoint and restart signal"
```

---

### Task 10: Telegram Connector

**Files:**
- Create: `internal/bridge/connector/telegram/telegram.go`
- Create: `internal/bridge/connector/telegram/formatter.go`
- Create: `internal/bridge/connector/telegram/commands.go`
- Create: `internal/bridge/connector/telegram/telegram_test.go`

- [ ] **Step 1: Implement event formatter**

In `formatter.go`: function that takes an `Event` and returns formatted Telegram message string with markdown. Handle each event type per spec section 7.1.

- [ ] **Step 2: Write failing tests for formatter**

Test each event type produces expected message format.

- [ ] **Step 3: Implement command parser**

In `commands.go`: parse Telegram message text into command struct:
```go
type Command struct {
    Name   string            // "start", "list", "status", "kill", "inject", "machines", "help"
    Args   []string
    Vars   map[string]string // for /start with | VAR=val
}

func ParseCommand(text string) (*Command, error)
```

- [ ] **Step 4: Write failing tests for command parser**

Test: `/start review-pr nuc-01 | PR_URL=https://... PR_TITLE=Fix bug` parses correctly. Test `/kill abc-123`. Test invalid commands.

- [ ] **Step 5: Implement Telegram connector**

In `telegram.go`:
```go
type Telegram struct {
    botToken       string
    groupID        int64
    eventsTopicID  int
    commandsTopicID int
    pollTimeout    int
    eventTypes     []string
    client         *client.Client
    state          *state.Store
    logger         *slog.Logger
}
```

Two goroutines in `Start()`:
1. **Events poller**: polls `client.PollEvents()`, formats events, sends to Telegram Events topic via `sendMessage` API
2. **Commands poller**: long-polls Telegram `getUpdates`, parses commands, dispatches to claude-plane API, replies with result

- [ ] **Step 6: Run tests**

Run: `go test -race ./internal/bridge/connector/telegram/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```
git add internal/bridge/connector/telegram/
git commit -m "feat: add Telegram connector with event formatting and command parsing"
```

---

## Chunk 4: Frontend

### Task 11: API Keys Page

**Files:**
- Create: `web/src/types/apikey.ts`
- Create: `web/src/api/apikeys.ts`
- Create: `web/src/hooks/useApiKeys.ts`
- Create: `web/src/views/ApiKeysPage.tsx`
- Create: `web/src/components/apikeys/CreateKeyModal.tsx`

- [ ] **Step 1: Types + API + hooks** (follow existing patterns)

- [ ] **Step 2: CreateKeyModal**

Modal with:
- Name input (required)
- Scopes multi-select (optional, empty = full access)
- Expiration date picker (optional)
- On create: shows plaintext key in a read-only input with copy button
- Warning: "This key will only be shown once"
- "Done" button closes modal

- [ ] **Step 3: ApiKeysPage**

Table with: name, key_id (prefix), scopes, created_at, last_used_at, expires_at, delete button.
"Create API Key" button opens modal.

- [ ] **Step 4: Commit**

```
git add web/src/types/apikey.ts web/src/api/apikeys.ts web/src/hooks/useApiKeys.ts web/src/views/ApiKeysPage.tsx web/src/components/apikeys/
git commit -m "feat: add API keys management page"
```

---

### Task 12: Connectors Page + Telegram Form

**Files:**
- Create: `web/src/types/connector.ts`
- Create: `web/src/api/bridge.ts`
- Create: `web/src/hooks/useBridge.ts`
- Create: `web/src/views/ConnectorsPage.tsx`
- Create: `web/src/components/connectors/ConnectorCard.tsx`
- Create: `web/src/components/connectors/AddConnectorModal.tsx`
- Create: `web/src/components/connectors/TelegramForm.tsx`

- [ ] **Step 1: Types + API + hooks**

Include bridge status and restart API calls.

- [ ] **Step 2: ConnectorCard**

Card showing: connector type icon, name, enabled toggle, status indicator, edit/delete actions.

- [ ] **Step 3: AddConnectorModal**

Type picker with cards for Telegram and GitHub (GitHub disabled/grayed until Phase 4).

- [ ] **Step 4: TelegramForm**

Form fields: bot token (password), group ID, events topic ID, commands topic ID, poll timeout, event type filter checkboxes. "Test Connection" button.

- [ ] **Step 5: ConnectorsPage**

- Empty state with "Add Connector" button
- Connector card grid
- Banner when config changed: "Configuration saved. Restart bridges to apply." with "Apply & Restart" button
- "Apply & Restart" calls `POST /api/v1/bridge/restart`

- [ ] **Step 6: Commit**

```
git add web/src/types/connector.ts web/src/api/bridge.ts web/src/hooks/useBridge.ts web/src/views/ConnectorsPage.tsx web/src/components/connectors/
git commit -m "feat: add connectors page with Telegram form"
```

---

### Task 13: Routes + Navigation

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/components/layout/Sidebar.tsx`

- [ ] **Step 1: Add routes**

```typescript
<Route path="/connectors" element={<ConnectorsPage />} />
<Route path="/api-keys" element={<ApiKeysPage />} />
```

- [ ] **Step 2: Add sidebar items**

Add "Connectors" and "API Keys" nav items in the admin/settings section of the sidebar.

- [ ] **Step 3: Run lint + tests**

Run: `cd web && npx vitest run && npm run lint`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add web/src/App.tsx web/src/components/layout/Sidebar.tsx
git commit -m "feat: add connector and API key routes to sidebar navigation"
```

---

## Chunk 5: Verification

### Task 14: Full Verification

- [ ] **Step 1: Run all Go tests**

Run: `go test -race ./...`
Expected: PASS

- [ ] **Step 2: Build bridge binary**

Run: `go build -o claude-plane-bridge ./cmd/bridge`
Expected: binary builds successfully

- [ ] **Step 3: Run all frontend tests**

Run: `cd web && npx vitest run && npm run lint`
Expected: PASS

- [ ] **Step 4: Manual smoke test — server side**

1. Create API key via UI
2. Verify key works: `curl -H "Authorization: Bearer cpk_..." http://localhost:8080/api/v1/machines`
3. Create Telegram connector via UI
4. Verify config stored (list connectors)
5. Click "Apply & Restart"

- [ ] **Step 5: Manual smoke test — bridge**

1. Create bootstrap TOML with server URL + API key
2. Run bridge: `./claude-plane-bridge serve --config bridge.toml`
3. Verify bridge connects, fetches config, starts Telegram connector
4. Send `/status` in Telegram Commands topic
5. Verify bridge replies with active sessions
6. Create a session → verify notification in Events topic

- [ ] **Step 6: Commit any fixes**

```
git commit -m "fix: address integration issues from Phase 3 smoke test"
```
