# Verified Bug Fixes Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 14 verified bugs (B01–B20) from codebase audit in a single branch.

**Architecture:** Fixes are grouped by subsystem to minimize context switching. Each task is self-contained with its own tests and commit. The P0 fixes (B01, B02, B03) are first because they address data loss and security issues.

**Tech Stack:** Go 1.25, SQLite, Chi router, gRPC, Telegram Bot API

**Branch:** `fix/verified-bug-fixes`

**Spec:** `docs/superpowers/specs/2026-03-14-verified-bug-fixes-design.md`

---

## Chunk 1: P0 Critical Fixes

### Task 1: B01 — Injection queue graceful shutdown

**Files:**
- Modify: `internal/server/session/injection_queue.go`
- Create: `internal/server/session/injection_queue_test.go` (or add to existing)

- [ ] **Step 1: Add `UpdateInjectionFailed` to the `InjectionAuditStore` interface**

```go
// In injection_queue.go, update the interface:
type InjectionAuditStore interface {
	UpdateInjectionDelivered(ctx context.Context, injectionID string, deliveredAt time.Time) error
	UpdateInjectionFailed(ctx context.Context, injectionID string, reason string) error
}
```

- [ ] **Step 2: Add `sync.WaitGroup` to `InjectionQueue` struct**

```go
type InjectionQueue struct {
	mu           sync.Mutex
	queues       map[string]*sessionQueue
	wg           sync.WaitGroup // tracks drainer goroutines
	connMgr      *connmgr.ConnectionManager
	auditStore   InjectionAuditStore
	sessionStore SessionStatusChecker
	subscriber   event.Subscriber
	unsubscribe  func()
	idleTimeout  time.Duration
	logger       *slog.Logger
}
```

- [ ] **Step 3: Increment WaitGroup in `getOrCreateQueue`, defer Done in `drainSession`**

In `getOrCreateQueue`, before `go q.drainSession(...)`:
```go
q.wg.Add(1)
go q.drainSession(sessionID, sq)
```

At top of `drainSession`:
```go
func (q *InjectionQueue) drainSession(sessionID string, sq *sessionQueue) {
	defer q.wg.Done()
	defer q.removeQueue(sessionID)
	// ... rest unchanged until the done case
```

- [ ] **Step 4: Add shutdown drain loop in `drainSession`**

Replace the `case <-sq.done: return` with:
```go
case <-sq.done:
	// Drain remaining buffered items — mark as failed, don't attempt delivery.
	for {
		select {
		case item, ok := <-sq.items:
			if !ok {
				return
			}
			q.logger.Warn("injection undelivered at shutdown",
				"session_id", sessionID,
				"injection_id", item.InjectionID,
			)
			if err := q.auditStore.UpdateInjectionFailed(
				context.Background(), item.InjectionID, "queue shutdown",
			); err != nil {
				q.logger.Error("failed to mark injection failed",
					"injection_id", item.InjectionID, "error", err)
			}
		default:
			return
		}
	}
```

- [ ] **Step 5: Update `Close()` to wait for drainers**

```go
func (q *InjectionQueue) Close() {
	if q.unsubscribe != nil {
		q.unsubscribe()
	}

	q.mu.Lock()
	queues := make(map[string]*sessionQueue, len(q.queues))
	for k, v := range q.queues {
		queues[k] = v
	}
	q.mu.Unlock()

	for _, sq := range queues {
		sq.closeOnce.Do(func() { close(sq.done) })
	}

	q.wg.Wait()
}
```

- [ ] **Step 6: Write tests**

Test 1: `Close()` blocks until drainers exit — create a queue, enqueue items, call Close(), verify it returns only after drainer finishes.

Test 2: Buffered items at shutdown are marked failed — enqueue items, close immediately, verify `UpdateInjectionFailed` was called for each buffered item.

Run: `go test -race ./internal/server/session/ -run TestInjectionQueue -v`

- [ ] **Step 7: Implement `UpdateInjectionFailed` in store**

In `internal/server/store/injections.go` (or wherever injection store methods live), add:
```go
func (s *Store) UpdateInjectionFailed(ctx context.Context, injectionID string, reason string) error {
	_, err := s.writer.ExecContext(ctx,
		`UPDATE injections SET delivered_at = NULL, metadata = json_set(COALESCE(metadata, '{}'), '$.failure_reason', ?) WHERE injection_id = ?`,
		reason, injectionID,
	)
	return err
}
```

Run: `go test -race ./internal/server/session/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/server/session/injection_queue.go internal/server/session/injection_queue_test.go internal/server/store/injections.go
git commit -m "fix: injection queue Close() waits for drainers and marks buffered items as failed (B01)"
```

---

### Task 2: B02 — Enforce API key scopes for connector secrets

**Files:**
- Modify: `internal/server/auth/jwt.go` (add Scopes to Claims)
- Modify: `internal/server/api/middleware.go` (propagate scopes, add RequireScope)
- Modify: `internal/server/handler/bridge.go` (gate secrets on scope)
- Create: `internal/server/api/middleware_test.go` (scope tests)

- [ ] **Step 1: Add Scopes field to `auth.Claims`**

In `internal/server/auth/jwt.go`:
```go
type Claims struct {
	jwt.RegisteredClaims
	UserID string   `json:"uid"`
	Email  string   `json:"email"`
	Role   string   `json:"role"`
	Scopes []string `json:"scopes,omitempty"`
}

// HasScope reports whether the claims include the given scope.
func (c *Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Propagate scopes in `validateAPIKeyToken`**

In `internal/server/api/middleware.go`, update `validateAPIKeyToken` return:
```go
return &auth.Claims{
	UserID: user.UserID,
	Email:  user.Email,
	Role:   user.Role,
	Scopes: apiKey.Scopes,
}
```

- [ ] **Step 3: Gate secrets in bridge handler on scope**

In `internal/server/handler/bridge.go`, update `ListConnectors`:
```go
// Replace: withSecrets := httputil.IsAPIKeyAuth(r)
c := h.getClaims(r)
withSecrets := httputil.IsAPIKeyAuth(r) && c != nil && c.HasScope("connectors:read_secret")
```

Same pattern in `GetConnector`:
```go
// Replace: if httputil.IsAPIKeyAuth(r) {
c := h.getClaims(r)
if httputil.IsAPIKeyAuth(r) && c != nil && c.HasScope("connectors:read_secret") {
```

- [ ] **Step 4: Write tests for scope enforcement**

Test 1: API key with `connectors:read_secret` scope → secrets returned.
Test 2: API key without scope → secrets masked.
Test 3: JWT auth → secrets masked (existing behavior preserved).
Test 4: `HasScope` unit test.

Run: `go test -race ./internal/server/handler/ -run TestBridge -v`
Run: `go test -race ./internal/server/auth/ -run TestHasScope -v`

- [ ] **Step 5: Commit**

```bash
git add internal/server/auth/jwt.go internal/server/api/middleware.go internal/server/handler/bridge.go internal/server/handler/bridge_test.go internal/server/auth/jwt_test.go
git commit -m "fix: enforce API key scopes for connector secret access (B02)"
```

---

### Task 3: B03 — Dedicated persist delivery path in event bus

**Files:**
- Modify: `internal/server/event/bus.go`
- Modify: `internal/server/event/persist.go`
- Modify: `cmd/server/main.go` (wiring)
- Create/Modify: `internal/server/event/bus_test.go`

- [ ] **Step 1: Add `persistHandler` field to `Bus`**

```go
type Bus struct {
	mu             sync.RWMutex
	subscribers    []*subscriber
	logger         *slog.Logger
	closed         bool
	persistHandler func(context.Context, Event) error // synchronous persist, called outside lock
}
```

- [ ] **Step 2: Add `SetPersistHandler` method**

```go
// SetPersistHandler registers a synchronous handler that is called for every
// event before the non-blocking fan-out. Must be called before any Publish.
func (b *Bus) SetPersistHandler(fn func(context.Context, Event) error) {
	b.persistHandler = fn
}
```

- [ ] **Step 3: Update `Publish` to call persist outside the lock**

```go
func (b *Bus) Publish(ctx context.Context, event Event) error {
	// Synchronous persist — called outside the lock to avoid blocking
	// concurrent publishers on SQLite writes.
	if b.persistHandler != nil {
		if err := b.persistHandler(ctx, event); err != nil {
			b.logger.Error("persist handler failed",
				"event_id", event.EventID,
				"event_type", event.Type,
				"error", err,
			)
		}
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil
	}

	for _, sub := range b.subscribers {
		if !MatchPattern(sub.pattern, event.Type) {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			b.logger.Warn("event dropped: subscriber buffer full",
				"event_id", event.EventID,
				"event_type", event.Type,
				"pattern", sub.pattern,
			)
		}
	}
	return nil
}
```

- [ ] **Step 4: Update wiring in `cmd/server/main.go`**

Replace the `PersistSubscriber.Subscribe(bus)` call with:
```go
persistSub := event.NewPersistSubscriber(store, logger)
bus.SetPersistHandler(persistSub.Handler())
// Remove: persistUnsub := persistSub.Subscribe(bus)
```

- [ ] **Step 5: Write tests**

Test 1: Persist handler is called for every published event.
Test 2: Persist handler error doesn't block fan-out to other subscribers.
Test 3: Other subscribers still get non-blocking delivery (drops on full buffer).

Run: `go test -race ./internal/server/event/ -run TestBus -v`

- [ ] **Step 6: Commit**

```bash
git add internal/server/event/bus.go internal/server/event/persist.go internal/server/event/bus_test.go cmd/server/main.go
git commit -m "fix: dedicated synchronous persist path in event bus prevents audit trail gaps (B03)"
```

---

## Chunk 2: P1 Bug Fixes

### Task 4: B05 — `/start` fallback to template's default machine_id

**Files:**
- Modify: `internal/server/store/migrations.go` (migration 8)
- Modify: `internal/server/store/templates.go` (add MachineID field + queries)
- Modify: `internal/server/handler/templates.go` (accept machine_id in create/update)
- Modify: `internal/bridge/client/client.go` (add MachineID to Template)
- Modify: `internal/bridge/connector/telegram/telegram.go` (handleStart fallback)
- Create/Modify: `internal/bridge/connector/telegram/telegram_test.go`

- [ ] **Step 1: Add migration 8 — `machine_id` column**

In `internal/server/store/migrations.go`, append to `migrations` slice:
```go
{
	Version:     8,
	Description: "add machine_id to session templates",
	SQL: `ALTER TABLE session_templates ADD COLUMN machine_id TEXT NOT NULL DEFAULT '';`,
},
```

Note: SQLite `ALTER TABLE ADD COLUMN` is idempotent-safe (errors if column exists, but migration version table prevents re-runs).

- [ ] **Step 2: Add `MachineID` to `SessionTemplate` struct**

In `internal/server/store/templates.go`:
```go
type SessionTemplate struct {
	TemplateID     string            `json:"template_id"`
	UserID         string            `json:"user_id"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	MachineID      string            `json:"machine_id,omitempty"`
	Command        string            `json:"command,omitempty"`
	// ... rest unchanged
}
```

Update all SQL queries in the template store methods (CreateTemplate, GetTemplate, GetTemplateByName, ListTemplates, UpdateTemplate, CloneTemplate) to include the `machine_id` column in SELECT lists and INSERT/UPDATE statements.

**Critical:** Also update both `scanTemplate` (line 278) and `scanTemplateRow` (line 332) to scan the new column. Both currently scan 16 columns into fixed variables. Add `machine_id` between `name` and `description` in the scan order (matching the SELECT column order). Example for `scanTemplate`:
```go
err := row.Scan(
	&tmpl.TemplateID, &tmpl.UserID, &tmpl.Name, &tmpl.MachineID, &desc, &cmd,
	&argsJSON, &workDir, &envJSON, &prompt,
	&tmpl.TerminalRows, &tmpl.TerminalCols, &tagsJSON,
	&tmpl.TimeoutSeconds, &deletedAt, &tmpl.CreatedAt, &tmpl.UpdatedAt,
)
```

Apply the same change to `scanTemplateRow`. The scan order must exactly match the SELECT column order in every query.

- [ ] **Step 3: Accept `machine_id` in handler request types**

In `internal/server/handler/templates.go`, add to both request structs:
```go
type createTemplateRequest struct {
	Name      string `json:"name"`
	MachineID string `json:"machine_id,omitempty"`
	// ... rest unchanged
}

type updateTemplateRequest struct {
	Name      string `json:"name"`
	MachineID string `json:"machine_id,omitempty"`
	// ... rest unchanged
}
```

Wire `MachineID` through in `Create` and `Update` handlers.

- [ ] **Step 4: Add `MachineID` to bridge client's `Template` struct**

In `internal/bridge/client/client.go`:
```go
type Template struct {
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
	MachineID  string `json:"machine_id,omitempty"`
}
```

- [ ] **Step 5: Update `handleStart` to fallback to template's machine_id**

Note: This step uses `ListTemplates` + linear scan. Task 9 (B14) will replace this with the by-name endpoint. The machine_id fallback logic survives both versions.

In `internal/bridge/connector/telegram/telegram.go`:
```go
func (t *Telegram) handleStart(ctx context.Context, cmd *Command) string {
	if len(cmd.Args) < 1 {
		return "❌ Usage: /start <template_name> [machine_id] [| VAR=val …]"
	}
	templateName := cmd.Args[0]

	// Machine ID is optional — falls back to template default.
	var machineID string
	if len(cmd.Args) >= 2 {
		machineID = cmd.Args[1]
	}

	// Resolve template name to ID.
	templates, err := t.apiClient.ListTemplates(ctx)
	if err != nil {
		return fmt.Sprintf("❌ Failed to list templates: %s", err.Error())
	}
	var matched *client.Template
	for i, tmpl := range templates {
		if tmpl.Name == templateName {
			matched = &templates[i]
			break
		}
	}
	if matched == nil {
		return fmt.Sprintf("❌ Template %q not found.", templateName)
	}

	// Fall back to template's default machine.
	if machineID == "" {
		machineID = matched.MachineID
	}
	if machineID == "" {
		return "❌ Template has no default machine. Usage: /start <template> <machine>"
	}

	req := client.CreateSessionRequest{
		MachineID:  machineID,
		TemplateID: matched.TemplateID,
		Variables:  cmd.Vars,
	}
	session, err := t.apiClient.CreateSession(ctx, req)
	if err != nil {
		return fmt.Sprintf("❌ Failed to create session: %s", err.Error())
	}
	return fmt.Sprintf("✅ Session started\nID: `%s`\nMachine: `%s`", session.SessionID, session.MachineID)
}
```

- [ ] **Step 6: Update help text**

```go
func helpText() string {
	return `*claude-plane bot commands*

/start <template> [machine] [| VAR=val …] — Start a session from a template
/list — List active sessions
/machines — List connected machines
/status — Bridge status
/kill <session_id> — Kill a session
/inject <session_id> <text> — Inject text into a session
/help — Show this message`
}
```

- [ ] **Step 7: Write tests**

Test 1: `/start template1 machine1` — explicit machine used.
Test 2: `/start template1` — falls back to template's default machine.
Test 3: `/start template1` with no default machine — error message.

Run: `go test -race ./internal/bridge/connector/telegram/ -v`

- [ ] **Step 8: Verify migration and store changes compile**

Run: `go build ./...`
Run: `go test -race ./internal/server/store/ -v`

- [ ] **Step 9: Commit**

```bash
git add internal/server/store/migrations.go internal/server/store/templates.go internal/server/handler/templates.go internal/bridge/client/client.go internal/bridge/connector/telegram/telegram.go internal/bridge/connector/telegram/telegram_test.go
git commit -m "feat: /start command falls back to template's default machine_id (B05)"
```

---

### Task 5: B06 — Defensive SQL in migrations 5, 6, 7

**Files:**
- Modify: `internal/server/store/migrations.go`

- [ ] **Step 1: Add `IF NOT EXISTS` to migration 5**

Lines 283 and 303:
```sql
CREATE TABLE IF NOT EXISTS session_templates (
```
```sql
CREATE INDEX IF NOT EXISTS idx_templates_user ON session_templates(user_id, deleted_at);
```

- [ ] **Step 2: Add `IF NOT EXISTS` to migration 6**

Lines 312 and 322:
```sql
CREATE TABLE IF NOT EXISTS injections (
```
```sql
CREATE INDEX IF NOT EXISTS idx_injections_session ON injections(session_id, created_at DESC);
```

- [ ] **Step 3: Add `IF NOT EXISTS` to migration 7**

Lines 329, 339, 341, 354:
```sql
CREATE TABLE IF NOT EXISTS api_keys (
```
```sql
CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);
```
```sql
CREATE TABLE IF NOT EXISTS bridge_connectors (
```
```sql
CREATE TABLE IF NOT EXISTS bridge_control (
```

- [ ] **Step 4: Verify**

Run: `go test -race ./internal/server/store/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/server/store/migrations.go
git commit -m "fix: add IF NOT EXISTS to migrations 5, 6, 7 for defensive idempotency (B06)"
```

---

### Task 6: B08 — Webhook retry jitter

**Files:**
- Modify: `internal/server/event/webhook_delivery.go`
- Create/Modify: `internal/server/event/webhook_delivery_test.go`

- [ ] **Step 1: Write failing test for jitter**

```go
func TestRetryBackoff_HasJitter(t *testing.T) {
	seen := make(map[time.Duration]bool)
	for i := 0; i < 20; i++ {
		d := retryBackoff(1)
		seen[d] = true
		if d < 1*time.Second || d > 1500*time.Millisecond {
			t.Fatalf("attempt 1 backoff %v out of range [1s, 1.5s]", d)
		}
	}
	if len(seen) < 2 {
		t.Fatal("expected jitter to produce varying durations")
	}
}
```

Run: `go test -race ./internal/server/event/ -run TestRetryBackoff -v`
Expected: FAIL (currently returns fixed 1s every time)

- [ ] **Step 2: Add jitter to `retryBackoff`**

```go
func retryBackoff(attempt int) time.Duration {
	var base time.Duration
	switch attempt {
	case 1:
		base = 1 * time.Second
	case 2:
		base = 5 * time.Second
	default:
		base = 30 * time.Second
	}
	jitter := time.Duration(rand.Int64N(int64(base / 2)))
	return base + jitter
}
```

Add `"math/rand/v2"` to imports (or use `math/rand` with `rand.Int64N` available in Go 1.22+).

- [ ] **Step 3: Run test**

Run: `go test -race ./internal/server/event/ -run TestRetryBackoff -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/server/event/webhook_delivery.go internal/server/event/webhook_delivery_test.go
git commit -m "fix: add jitter to webhook retry backoff to prevent thundering herd (B08)"
```

---

## Chunk 3: P2 Quality Fixes

### Task 7: B12 — Enqueue TOCTOU mitigation

**Files:**
- Modify: `internal/server/session/injection_queue.go`
- Modify: `internal/server/session/injection_queue_test.go`

- [ ] **Step 1: Add session status re-check in `processItem`**

After the stale item check (line 197), before the delay:
```go
// Re-check session status before delivery (mitigates Enqueue TOCTOU race).
sess, err := q.sessionStore.GetSession(sessionID)
if err != nil {
	q.logger.Warn("failed to check session status before delivery",
		"session_id", sessionID, "injection_id", item.InjectionID, "error", err)
	return
}
if isTerminalStatus(sess.Status) {
	q.logger.Info("session terminated before injection delivery",
		"session_id", sessionID, "injection_id", item.InjectionID)
	if err := q.auditStore.UpdateInjectionFailed(
		context.Background(), item.InjectionID, "session terminated",
	); err != nil {
		q.logger.Error("failed to mark injection failed",
			"injection_id", item.InjectionID, "error", err)
	}
	return
}
```

- [ ] **Step 2: Write test**

Test: Enqueue an item, then mark session as terminated in the mock store. Verify `processItem` calls `UpdateInjectionFailed` instead of `SendCommand`.

Run: `go test -race ./internal/server/session/ -run TestProcessItem_TerminalSession -v`

- [ ] **Step 3: Commit**

```bash
git add internal/server/session/injection_queue.go internal/server/session/injection_queue_test.go
git commit -m "fix: re-check session status in processItem before injection delivery (B12)"
```

---

### Task 8: B13 — Configurable event retention

**Files:**
- Modify: `internal/server/config/config.go`
- Modify: `internal/server/event/retention.go`
- Modify: `cmd/server/main.go`
- Create/Modify: `internal/server/event/retention_test.go`

- [ ] **Step 1: Add `EventsConfig` to server config**

In `internal/server/config/config.go`:
```go
type ServerConfig struct {
	HTTP      HTTPConfig      `toml:"http"`
	GRPC      GRPCConfig      `toml:"grpc"`
	TLS       TLSConfig       `toml:"tls"`
	Database  DatabaseConfig  `toml:"database"`
	Auth      AuthConfig      `toml:"auth"`
	Shutdown  ShutdownConfig  `toml:"shutdown"`
	Webhooks  WebhooksConfig  `toml:"webhooks"`
	Provision ProvisionConfig `toml:"provision"`
	CA        CAConfig        `toml:"ca"`
	Secrets   SecretsConfig   `toml:"secrets"`
	Events    EventsConfig    `toml:"events"`
}

// EventsConfig configures event retention behavior.
type EventsConfig struct {
	RetentionDays int `toml:"retention_days"`
}

// GetRetentionDays returns the configured retention period, defaulting to 7.
func (e *EventsConfig) GetRetentionDays() int {
	if e.RetentionDays <= 0 {
		return 7
	}
	return e.RetentionDays
}
```

- [ ] **Step 2: Update `NewRetentionCleaner` to accept maxAge**

In `internal/server/event/retention.go`:
```go
func NewRetentionCleaner(store RetentionStore, maxAge time.Duration, logger *slog.Logger) *RetentionCleaner {
	if logger == nil {
		logger = slog.Default()
	}
	if maxAge <= 0 {
		maxAge = defaultMaxAge
	}
	return &RetentionCleaner{
		store:  store,
		period: defaultRetentionPeriod,
		maxAge: maxAge,
		logger: logger,
	}
}
```

- [ ] **Step 3: Update wiring in `cmd/server/main.go`**

Find the `NewRetentionCleaner` call and update:
```go
retentionDays := cfg.Events.GetRetentionDays()
maxAge := time.Duration(retentionDays) * 24 * time.Hour
retentionCleaner := event.NewRetentionCleaner(store, maxAge, logger)
```

- [ ] **Step 4: Write tests**

Test 1: `GetRetentionDays()` returns 7 for zero/negative values.
Test 2: `NewRetentionCleaner` uses provided maxAge.
Test 3: `NewRetentionCleaner` falls back to default when maxAge <= 0.

Run: `go test -race ./internal/server/event/ -run TestRetention -v`
Run: `go test -race ./internal/server/config/ -run TestEventsConfig -v`

- [ ] **Step 5: Commit**

```bash
git add internal/server/config/config.go internal/server/event/retention.go cmd/server/main.go internal/server/event/retention_test.go internal/server/config/config_test.go
git commit -m "feat: configurable event retention period via [events] config section (B13)"
```

---

### Task 9: B14 — By-name template endpoint

**Files:**
- Modify: `internal/server/handler/templates.go` (add route + handler)
- Modify: `internal/bridge/client/client.go` (add GetTemplateByName)
- Modify: `internal/bridge/connector/telegram/telegram.go` (use new endpoint in handleStart)
- Create/Modify: `internal/server/handler/templates_test.go`

- [ ] **Step 1: Add route in `RegisterTemplateRoutes`**

```go
func RegisterTemplateRoutes(r chi.Router, h *TemplateHandler) {
	r.Post("/api/v1/templates", h.Create)
	r.Get("/api/v1/templates", h.List)
	r.Get("/api/v1/templates/by-name/{name}", h.GetByName)
	r.Get("/api/v1/templates/{templateID}", h.Get)
	r.Put("/api/v1/templates/{templateID}", h.Update)
	r.Delete("/api/v1/templates/{templateID}", h.Delete)
	r.Post("/api/v1/templates/{templateID}/clone", h.Clone)
}
```

Note: `by-name/{name}` must come before `{templateID}` to avoid Chi treating "by-name" as a template ID.

- [ ] **Step 2: Add `GetByName` handler**

```go
// GetByName handles GET /api/v1/templates/by-name/{name}.
func (h *TemplateHandler) GetByName(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	userID := ""
	if c := h.claims(r); c != nil {
		userID = c.UserID
	}

	tmpl, err := h.store.GetTemplateByName(r.Context(), userID, name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !h.authorizeTemplate(w, r, tmpl) {
		return
	}
	writeJSON(w, http.StatusOK, tmpl)
}
```

- [ ] **Step 3: Add `GetTemplateByName` to bridge client**

In `internal/bridge/client/client.go`:
```go
// GetTemplateByName returns a single template by name, or an error if not found.
func (c *Client) GetTemplateByName(ctx context.Context, name string) (*Template, error) {
	var tmpl Template
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/templates/by-name/"+url.PathEscape(name), nil, &tmpl); err != nil {
		return nil, fmt.Errorf("get template by name %q: %w", name, err)
	}
	return &tmpl, nil
}
```

- [ ] **Step 4: Update Telegram `handleStart` to use by-name endpoint**

Replace the `ListTemplates` + loop from Task 4 (B05) with the direct lookup. Keep the machine_id fallback logic:
```go
// Replace the ListTemplates + loop block with:
matched, err := t.apiClient.GetTemplateByName(ctx, templateName)
if err != nil {
	return fmt.Sprintf("❌ Template %q not found.", templateName)
}

// Fall back to template's default machine (from B05).
if machineID == "" {
	machineID = matched.MachineID
}
if machineID == "" {
	return "❌ Template has no default machine. Usage: /start <template> <machine>"
}
```

- [ ] **Step 5: Write tests**

Test 1: `GET /api/v1/templates/by-name/my-template` → 200 + template.
Test 2: `GET /api/v1/templates/by-name/nonexistent` → 404.

Run: `go test -race ./internal/server/handler/ -run TestTemplate -v`

- [ ] **Step 6: Commit**

```bash
git add internal/server/handler/templates.go internal/bridge/client/client.go internal/bridge/connector/telegram/telegram.go internal/server/handler/templates_test.go
git commit -m "feat: add GET /api/v1/templates/by-name/{name} endpoint (B14)"
```

---

### Task 10: B15 — Update PRD variable syntax documentation

**Files:**
- Modify: `docs/internal/product/backend_v1.md` (or wherever `{{.VarName}}` appears)

- [ ] **Step 1: Find and update all PRD references**

Search for `{{.VarName}}` or `{{.` in `docs/internal/product/`:
```bash
grep -rn '{{\\.' docs/internal/product/
```

Replace `{{.VarName}}` with `${VAR_NAME}` in all matching locations. The code uses `${VAR_NAME}` syntax per `handler/templates.go:20`.

- [ ] **Step 2: Commit**

```bash
git add docs/internal/product/
git commit -m "docs: update PRD to match ${VAR_NAME} template variable syntax (B15)"
```

---

## Chunk 4: P3 Nice-to-Have Fixes

### Task 11: B18 — Telegram 429 handling

**Files:**
- Modify: `internal/bridge/connector/telegram/telegram.go`
- Create/Modify: `internal/bridge/connector/telegram/telegram_test.go`

- [ ] **Step 1: Add 429 response struct**

```go
// telegramRateLimitResponse is parsed when the API returns HTTP 429.
type telegramRateLimitResponse struct {
	OK         bool `json:"ok"`
	Parameters struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}
```

- [ ] **Step 2: Add `checkRateLimit` helper**

```go
// checkRateLimit inspects the HTTP response for 429 status. On rate limit,
// it waits for the retry_after duration (context-aware) and returns a retriable error.
// For other non-2xx statuses, it returns an error with the status code.
// For 2xx, it returns nil.
func checkRateLimit(ctx context.Context, resp *http.Response, rawBody []byte) error {
	if resp.StatusCode == http.StatusTooManyRequests {
		var rl telegramRateLimitResponse
		retryAfter := 5 // default 5 seconds
		if json.Unmarshal(rawBody, &rl) == nil && rl.Parameters.RetryAfter > 0 {
			retryAfter = rl.Parameters.RetryAfter
		}
		select {
		case <-time.After(time.Duration(retryAfter) * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
		return fmt.Errorf("telegram rate limited, waited %ds", retryAfter)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram API error: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}
	return nil
}
```

- [ ] **Step 3: Integrate into `sendMessage` and `getUpdates`**

In `sendMessage`, after `resp.Body.Close()` and `io.ReadAll`:
```go
if err := checkRateLimit(ctx, resp, raw); err != nil {
	return err
}
```

Same in `getUpdates`.

- [ ] **Step 4: Write test**

Test: Mock HTTP server returns 429 with `retry_after: 1`. Verify `checkRateLimit` waits and returns error.

Run: `go test -race ./internal/bridge/connector/telegram/ -run TestCheckRateLimit -v`

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/connector/telegram/telegram.go internal/bridge/connector/telegram/telegram_test.go
git commit -m "fix: handle Telegram 429 rate limit with context-aware retry_after wait (B18)"
```

---

### Task 12: B19 — Switch Telegram to MarkdownV2

**Files:**
- Modify: `internal/bridge/connector/telegram/telegram.go` (parse_mode)
- Modify: `internal/bridge/connector/telegram/formatter.go` (escape helper + update FormatEvent)
- Create/Modify: `internal/bridge/connector/telegram/formatter_test.go`

- [ ] **Step 1: Write failing test for escape function**

```go
func TestEscapeMarkdownV2(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"hello.world", "hello\\.world"},
		{"test-123", "test\\-123"},
		{"a_b*c[d](e)~f>g#h+i=j|k{l}m!n", "a\\_b\\*c\\[d\\]\\(e\\)\\~f\\>g\\#h\\+i\\=j\\|k\\{l\\}m\\!n"},
	}
	for _, tt := range tests {
		got := escapeMarkdownV2(tt.input)
		if got != tt.want {
			t.Errorf("escapeMarkdownV2(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

Run: `go test -race ./internal/bridge/connector/telegram/ -run TestEscapeMarkdownV2 -v`
Expected: FAIL (function doesn't exist)

- [ ] **Step 2: Implement `escapeMarkdownV2` in `formatter.go`**

```go
// escapeMarkdownV2 escapes special characters for Telegram MarkdownV2 parse mode.
var mdv2Replacer = strings.NewReplacer(
	"_", "\\_",
	"*", "\\*",
	"[", "\\[",
	"]", "\\]",
	"(", "\\(",
	")", "\\)",
	"~", "\\~",
	"`", "\\`",
	">", "\\>",
	"#", "\\#",
	"+", "\\+",
	"-", "\\-",
	"=", "\\=",
	"|", "\\|",
	"{", "\\{",
	"}", "\\}",
	".", "\\.",
	"!", "\\!",
)

func escapeMarkdownV2(s string) string {
	return mdv2Replacer.Replace(s)
}
```

Run: `go test -race ./internal/bridge/connector/telegram/ -run TestEscapeMarkdownV2 -v`
Expected: PASS

- [ ] **Step 3: Update `FormatEvent` to use MarkdownV2 escaping**

In `formatter.go`, update the `str` helper inside `FormatEvent`:
```go
str := func(key string) string {
	v, _ := e.Payload[key]
	if v == nil {
		return ""
	}
	return escapeMarkdownV2(fmt.Sprintf("%v", v))
}
```

- [ ] **Step 4: Update all message builders in `telegram.go`**

In `handleList`, `handleMachines`, `handleKill`, `handleInject`, `handleStart`: escape dynamic values (session IDs, machine IDs, error messages) with `escapeMarkdownV2()`. Keep formatting markers (`*bold*`, backtick code) as-is.

Example for `handleList`:
```go
sb.WriteString(fmt.Sprintf("• `%s` — %s \\(%s\\)\n",
	s.SessionID,
	escapeMarkdownV2(s.MachineID),
	escapeMarkdownV2(s.Status)))
```

Note: In MarkdownV2, parentheses must be escaped outside of inline links.

- [ ] **Step 5: Change parse_mode in `sendMessage`**

```go
"parse_mode": "MarkdownV2",
```

- [ ] **Step 6: Run all telegram tests**

Run: `go test -race ./internal/bridge/connector/telegram/ -v`

- [ ] **Step 7: Commit**

```bash
git add internal/bridge/connector/telegram/telegram.go internal/bridge/connector/telegram/formatter.go internal/bridge/connector/telegram/formatter_test.go
git commit -m "fix: switch Telegram connector to MarkdownV2 with proper escaping (B19)"
```

---

### Task 13: B20 — Health endpoint address validation

**Files:**
- Modify: `internal/bridge/bridge.go`
- Modify: `internal/bridge/bridge_test.go`

- [ ] **Step 1: Add address validation in `RunWithInterval`**

Before `healthSrv := b.startHealthServer()`:
```go
if b.healthAddr == "" {
	b.healthAddr = "localhost:9091"
	b.logger.Warn("health address not configured, defaulting to localhost:9091")
}
```

- [ ] **Step 2: Propagate health server startup failure**

Update `startHealthServer` to return an error channel:
```go
func (b *Bridge) startHealthServer() (*http.Server, <-chan error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", b.handleHealthz)

	srv := &http.Server{
		Addr:    b.healthAddr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("health server failed to start on %s: %w", b.healthAddr, err)
		}
	}()

	return srv, errCh
}
```

In `RunWithInterval`, check the error channel briefly:
```go
healthSrv, healthErrCh := b.startHealthServer()

// Give the health server a moment to bind.
select {
case err := <-healthErrCh:
	return err
case <-time.After(100 * time.Millisecond):
	b.logger.Info("health server started", "addr", b.healthAddr)
}
```

- [ ] **Step 3: Write tests**

Test 1: Empty address defaults to `localhost:9091`.
Test 2: Invalid address (e.g. `not-a-valid-addr:-1`) returns startup error.

Run: `go test -race ./internal/bridge/ -run TestBridge -v`

- [ ] **Step 4: Commit**

```bash
git add internal/bridge/bridge.go internal/bridge/bridge_test.go
git commit -m "fix: validate health endpoint address and propagate startup failures (B20)"
```

---

## Chunk 5: Final Verification

### Task 14: Full build and test verification

- [ ] **Step 1: Build all binaries**

```bash
go build ./cmd/server && go build ./cmd/agent && go build ./cmd/bridge
```

- [ ] **Step 2: Run all Go tests**

```bash
go vet ./...
go test -race ./...
```

- [ ] **Step 3: Run frontend tests (ensure nothing broken)**

```bash
cd web && npx tsc --noEmit && npm run lint && npx vitest run && cd ..
```

- [ ] **Step 4: Review all commits**

```bash
git log --oneline fix/verified-bug-fixes..HEAD
```

Verify 13 commits (Tasks 1-13), each self-contained.
