# Phase 2: Session Injection — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the ability to push text into running sessions mid-flight via a REST API, with an in-memory delivery queue, audit logging, and a frontend inject panel in the session detail view.

**Architecture:** New `/inject` endpoint on sessions, in-memory per-session queue with drainer goroutines that forward text as `InputDataCmd` via gRPC. Queue lifecycle managed via event bus subscription. Audit records stored in `injections` table (text length only, not content). Frontend adds collapsible inject panel to session detail.

**Tech Stack:** Go (handler, queue, store), SQLite migration, React 19, TypeScript, TanStack Query

**Design Spec:** `docs/superpowers/specs/2026-03-14-v2-templates-injection-bridge-design.md` — Section 4

**Depends on:** Phase 1 (templates) must be complete — shares the modified session handler. Phase 1's migration is version 5; this phase uses version 6.

**Known limitation:** In Phase 2, `TerminateSession` still emits `TypeSessionExited` (not `TypeSessionTerminated`). The drainer subscribes to `session.*` which catches both. Drainers for terminated sessions will still exit correctly via the `session.exited` event. Phase 3 adds the distinct `TypeSessionTerminated` event type.

---

## File Map

### Backend — Create
| File | Responsibility |
|------|---------------|
| `internal/server/store/injections.go` | Injection audit store methods |
| `internal/server/store/injections_test.go` | Store tests |
| `internal/server/session/injection_queue.go` | In-memory injection queue + drainer goroutines |
| `internal/server/session/injection_queue_test.go` | Queue tests |

### Backend — Modify
| File | Change |
|------|--------|
| `internal/server/store/migrations.go` | Add migration v6: `injections` table |
| `internal/server/session/handler.go` | Add `InjectSession` and `ListInjections` handlers |
| `internal/server/api/router.go` | Register inject + injection history routes |
| `internal/server/event/bus.go` | Add `Subscriber` interface (narrow read-only interface) |

### Frontend — Create
| File | Responsibility |
|------|---------------|
| `web/src/types/injection.ts` | `Injection` TypeScript interface |
| `web/src/api/injections.ts` | API client for inject + list |
| `web/src/hooks/useInjections.ts` | TanStack Query hooks |
| `web/src/components/sessions/InjectPanel.tsx` | Collapsible inject panel below terminal |

### Frontend — Modify
| File | Change |
|------|--------|
| Session detail view (likely `web/src/views/SessionsPage.tsx` or session detail component) | Add InjectPanel below terminal |

---

## Chunk 1: Backend

### Task 1: Migration — Injections Table

**Files:**
- Modify: `internal/server/store/migrations.go`

- [ ] **Step 1: Add migration v6**

```go
{
    Version: 6,
    SQL: `
CREATE TABLE injections (
    injection_id TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES sessions(session_id),
    user_id      TEXT NOT NULL REFERENCES users(user_id),
    text_length  INTEGER NOT NULL,
    metadata     TEXT,
    source       TEXT NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at DATETIME
);
CREATE INDEX idx_injections_session ON injections(session_id, created_at DESC);
    `,
},
```

- [ ] **Step 2: Run migration tests**

Run: `go test -race ./internal/server/store/ -run TestMigrations -v`
Expected: PASS

- [ ] **Step 3: Commit**

```
git add internal/server/store/migrations.go
git commit -m "feat: add migration v6 for injections audit table"
```

---

### Task 2: Injection Store Methods

**Files:**
- Create: `internal/server/store/injections.go`
- Create: `internal/server/store/injections_test.go`

- [ ] **Step 1: Write failing tests**

Test cases:
- `CreateInjection`: insert with all fields, verify returned struct has generated ID and timestamp
- `UpdateInjectionDelivered`: set `delivered_at`, verify it persists
- `ListInjectionsBySession`: create 3 injections for session A, 1 for session B, list for A returns 3 ordered by `created_at DESC`

- [ ] **Step 2: Implement store methods**

In `injections.go`:

```go
type Injection struct {
    InjectionID string     `json:"injection_id"`
    SessionID   string     `json:"session_id"`
    UserID      string     `json:"user_id"`
    TextLength  int        `json:"text_length"`
    Metadata    string     `json:"metadata,omitempty"`
    Source      string     `json:"source"`
    CreatedAt   time.Time  `json:"created_at"`
    DeliveredAt *time.Time `json:"delivered_at,omitempty"`
}
```

Note: The narrow interfaces (`InjectionStoreIface`, `SessionStatusChecker`) are defined in the consuming package (`session/injection_queue.go`), NOT here in the store package. This follows the Go convention (interfaces in the consumer) and the pattern used by `connmgr.MachineStore`. The store package only provides the concrete methods on `*Store`.

Note: The existing `store.GetSession(id string)` does NOT accept `context.Context`. The `SessionStatusChecker` interface must match this signature: `GetSession(id string) (*Session, error)` — do NOT add ctx.
```

- [ ] **Step 3: Run tests**

Run: `go test -race ./internal/server/store/ -run TestInjection -v`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/server/store/injections.go internal/server/store/injections_test.go
git commit -m "feat: add injection audit store methods"
```

---

### Task 3: Subscriber Interface on Event Bus

**Files:**
- Modify: `internal/server/event/bus.go`

- [ ] **Step 1: Add Subscriber interface**

Add alongside the existing `Publisher` interface:

```go
// Subscriber is a narrow read-only interface for subscribing to events.
// This complements the write-only Publisher interface.
type Subscriber interface {
    Subscribe(pattern string, handler HandlerFunc, opts SubscriberOptions) (unsubscribe func())
}
```

Verify `*Bus` satisfies both `Publisher` and `Subscriber`:

```go
var _ Publisher = (*Bus)(nil)
var _ Subscriber = (*Bus)(nil)
```

- [ ] **Step 2: Run event tests**

Run: `go test -race ./internal/server/event/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```
git add internal/server/event/bus.go
git commit -m "feat: add Subscriber interface to event bus"
```

---

### Task 4: Injection Queue

**Files:**
- Create: `internal/server/session/injection_queue.go`
- Create: `internal/server/session/injection_queue_test.go`

- [ ] **Step 1: Write failing tests**

Test cases:
- `Enqueue` to a running session: item appears in queue, drainer sends `InputDataCmd` to mock agent
- `Enqueue` with `raw=false`: newline appended to data
- `Enqueue` with `delay_ms=100`: verify delay between deliveries (use short delays for test)
- `Enqueue` when session not running: returns error (409-equivalent)
- `Enqueue` when queue full (32 items): returns error (429-equivalent)
- `Enqueue` when agent disconnected: returns error (503-equivalent)
- Session exit event: drainer goroutine exits, no goroutine leak
- Idle drainer exit: set `idleTimeout` to 100ms in test, verify drainer exits after idle period (no goroutine leak)
- Race condition: session transitions to terminal between status check and drainer creation — verify inject returns 409 (drainer checks status before starting)
- Stale items (older than 5 min): silently dropped by drainer
- Audit record created with `delivered_at = nil`, updated after delivery

- [ ] **Step 2: Implement InjectionQueue**

In `injection_queue.go`:

```go
// Narrow interfaces defined in the consuming package (session), not in store.
// Follows the pattern used by connmgr.MachineStore.

// InjectionStoreIface is the narrow interface for injection audit.
type InjectionStoreIface interface {
    CreateInjection(ctx context.Context, inj *store.Injection) (*store.Injection, error)
    UpdateInjectionDelivered(ctx context.Context, injectionID string, deliveredAt time.Time) error
}

// InjectionListStore is a separate narrow interface for listing injections (used by handler, not queue).
type InjectionListStore interface {
    ListInjectionsBySession(ctx context.Context, sessionID string) ([]store.Injection, error)
}

// SessionStatusChecker matches the existing store.GetSession signature (no context param).
type SessionStatusChecker interface {
    GetSession(id string) (*store.Session, error)
}

type InjectionQueue struct {
    mu           sync.Mutex
    queues       map[string]*sessionQueue
    connMgr      *connmgr.ConnectionManager
    store        InjectionStoreIface     // narrow: Create + UpdateDelivered only
    sessionStore SessionStatusChecker    // matches existing store.GetSession(id) signature
    subscriber   event.Subscriber
    unsubscribe  func()
    idleTimeout  time.Duration           // configurable for testing (default 5 min)
    logger       *slog.Logger
}

func NewInjectionQueue(
    connMgr *connmgr.ConnectionManager,
    injStore InjectionStoreIface,
    sessionStore SessionStatusChecker,
    subscriber event.Subscriber,
    logger *slog.Logger,
) *InjectionQueue
```

Key implementation details:
- `Enqueue(ctx, sessionID, machineID, data []byte, delayMs int, injectionID string) error`
  - Checks session status via `sessionStore.GetSession()` — returns error if terminal
  - Gets or creates `sessionQueue` (lazy creation)
  - Tries non-blocking send to channel — returns queue-full error if can't
- Drainer goroutine per session:
  - Reads from channel
  - Checks `QueuedAt` — drops if older than 5 minutes
  - Sleeps `delay_ms`
  - Sends `InputDataCmd` via `connMgr.GetAgent(machineID).SendCommand()`
  - Updates `store.UpdateInjectionDelivered()`
  - Exits on `done` channel close or 5 minutes idle
- `handleSessionEvent`: on `session.exited` or `session.terminated`, close `done` channel for that session
- `Close()` method: calls `unsubscribe()`, closes all queues

- [ ] **Step 3: Run tests**

Run: `go test -race ./internal/server/session/ -run TestInjection -v`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/server/session/injection_queue.go internal/server/session/injection_queue_test.go
git commit -m "feat: add in-memory injection queue with drainer goroutines"
```

---

### Task 5: Inject + History Handlers

**Files:**
- Modify: `internal/server/session/handler.go`

- [ ] **Step 1: Write failing tests**

Test cases:
- `POST /api/v1/sessions/{id}/inject` — valid inject returns 202 with `injection_id`
- Inject to non-existent session returns 404
- Inject to terminated session returns 409
- Inject when agent disconnected returns 503
- Inject by non-owner returns 404
- `GET /api/v1/sessions/{id}/injections` — returns list of injections

- [ ] **Step 2: Add InjectionQueue and InjectionListStore dependencies to SessionHandler**

Add two new fields to `SessionHandler`:
```go
injectionQueue *InjectionQueue
injListStore   InjectionListStore  // for ListInjections handler
```

Update `NewSessionHandler` constructor to accept these (or use setter methods like `SetPublisher`). Wire in `cmd/server/` main:
1. Create `InjectionQueue` with `NewInjectionQueue(connMgr, store, store, eventBus, logger)`
2. Pass it to session handler: `sessionHandler.SetInjectionQueue(injQueue)`
3. Pass `store` (which satisfies `InjectionListStore`) for listing

- [ ] **Step 3: Implement InjectSession handler**

```go
func (h *SessionHandler) InjectSession(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionID")

    // Auth + ownership check
    sess, err := h.store.GetSession(sessionID)
    if err != nil { ... 404 }
    if !h.authorizeSession(r, sess) { ... 404 }

    // Check session status
    if isTerminalStatus(sess.Status) { ... 409 }

    // Check agent connected
    agent := h.connMgr.GetAgent(sess.MachineID)
    if agent == nil { ... 503 }

    // Parse request
    var req injectRequest
    json.NewDecoder(r.Body).Decode(&req)
    if req.Text == "" { ... 400 }

    // Prepare data
    data := []byte(req.Text)
    if !req.Raw {
        data = append(data, '\n')
    }

    // Create audit record
    injectionID := uuid.New().String()
    metadata, _ := json.Marshal(req.Metadata)
    inj := &store.Injection{
        InjectionID: injectionID,
        SessionID:   sessionID,
        UserID:      claims.UserID,
        TextLength:  len(req.Text),
        Metadata:    string(metadata),
        Source:      "api", // or "manual" if from UI
    }
    h.injStore.CreateInjection(r.Context(), inj)

    // Enqueue
    err = h.injectionQueue.Enqueue(r.Context(), sessionID, sess.MachineID, data, req.DelayMs, injectionID)
    // Handle 429 (queue full), 503 (agent disconnected)

    // 202 Accepted
    httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
        "injection_id": injectionID,
        "queued_at":    time.Now().UTC(),
    })
}
```

- [ ] **Step 4: Implement ListInjections handler**

```go
func (h *SessionHandler) ListInjections(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionID")
    // Auth + ownership check (same as GetSession)
    injections, err := h.injStore.ListInjectionsBySession(r.Context(), sessionID)
    httputil.WriteJSON(w, http.StatusOK, injections)
}
```

- [ ] **Step 5: Run tests**

Run: `go test -race ./internal/server/session/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```
git add internal/server/session/handler.go internal/server/session/handler_test.go
git commit -m "feat: add inject and injection history endpoints"
```

---

### Task 6: Register Inject Routes

**Files:**
- Modify: `internal/server/api/router.go`

- [ ] **Step 1: Add inject routes**

Inside the JWT-protected session routes block:
```go
r.Post("/sessions/{sessionID}/inject", sessionHandler.InjectSession)
r.Get("/sessions/{sessionID}/injections", sessionHandler.ListInjections)
```

- [ ] **Step 2: Run full Go tests**

Run: `go test -race ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```
git add internal/server/api/router.go
git commit -m "feat: register injection routes in API router"
```

---

## Chunk 2: Frontend

### Task 7: Frontend Types + API + Hooks

**Files:**
- Create: `web/src/types/injection.ts`
- Create: `web/src/api/injections.ts`
- Create: `web/src/hooks/useInjections.ts`

- [ ] **Step 1: Create types**

```typescript
export interface Injection {
  injection_id: string;
  session_id: string;
  user_id: string;
  text_length: number;
  metadata?: string;
  source: string;
  created_at: string;
  delivered_at?: string;
}
```

- [ ] **Step 2: Create API client**

```typescript
export const injectionsApi = {
  inject: (sessionId: string, params: { text: string; raw?: boolean; delay_ms?: number; metadata?: Record<string, unknown> }) =>
    request<{ injection_id: string; queued_at: string }>(
      `/sessions/${encodeURIComponent(sessionId)}/inject`,
      { method: 'POST', body: JSON.stringify(params) },
    ),
  list: (sessionId: string) =>
    request<Injection[]>(`/sessions/${encodeURIComponent(sessionId)}/injections`),
};
```

- [ ] **Step 3: Create hooks**

- `useInjections(sessionId)` — query for injection history
- `useInjectSession()` — mutation that invalidates injection list

- [ ] **Step 4: Commit**

```
git add web/src/types/injection.ts web/src/api/injections.ts web/src/hooks/useInjections.ts
git commit -m "feat: add injection types, API client, and hooks"
```

---

### Task 8: Inject Panel Component

**Files:**
- Create: `web/src/components/sessions/InjectPanel.tsx`
- Modify: Session detail view

- [ ] **Step 1: Create InjectPanel**

Collapsible panel with:
- Textarea for input text
- "Send" button + Ctrl+Enter keyboard shortcut
- Toggle for "Raw mode" (default off) — small checkbox or switch
- On submit: calls `useInjectSession()` with text and raw flag
- Success feedback: brief toast or inline success indicator

- [ ] **Step 2: Add injection history list**

Below the textarea, render injection history:
- Use `useInjections(sessionId)` to fetch
- Each row: timestamp (relative), source badge (styled: `manual` green, `api` blue, `bridge-*` purple), text length, delivery status (pending/delivered)
- Auto-refresh every 5 seconds (or use event stream invalidation)

- [ ] **Step 3: Integrate into session detail view**

Find the component that renders the terminal for a session. Add `<InjectPanel sessionId={id} />` below the terminal, wrapped in a collapsible section.

Only show the panel when session status is `running`.

- [ ] **Step 4: Write InjectPanel tests**

Create `web/src/components/sessions/InjectPanel.test.tsx` with test cases:
- Renders textarea and send button
- Submit sends inject API call with text
- Ctrl+Enter keyboard shortcut triggers submit
- Raw mode toggle changes request payload
- Error states: 503 (agent disconnected), 409 (session not running), 429 (queue full) — display appropriate messages
- Injection history list renders with timestamp, source badge, text length
- Panel hidden when session status is not `running`

- [ ] **Step 5: Run frontend lint + tests**

Run: `cd web && npx vitest run && npm run lint`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add web/src/components/sessions/InjectPanel.tsx
git commit -m "feat: add inject panel with history to session detail view"
```

---

## Chunk 3: Verification

### Task 9: End-to-End Verification

- [ ] **Step 1: Run full test suites**

Run: `go test -race ./...`
Run: `cd web && npx vitest run && npm run lint`
Expected: PASS

- [ ] **Step 2: Manual smoke test**

1. Start server + agent
2. Create a session
3. Inject text via API: `POST /api/v1/sessions/{id}/inject` with `{"text": "hello"}`
4. Verify text appears in terminal output
5. Verify injection history via `GET /api/v1/sessions/{id}/injections`
6. Inject from UI inject panel
7. Verify 409 when injecting into terminated session
8. Verify 429 when flooding the queue

- [ ] **Step 3: Commit any fixes**

```
git commit -m "fix: address integration issues from Phase 2 smoke test"
```
