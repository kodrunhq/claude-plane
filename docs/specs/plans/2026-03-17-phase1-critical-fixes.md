# Phase 1: Critical Fixes — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 6 critical issues where the app is losing data, lying to users, or silently failing.

**Architecture:** Backend-first fixes (SQLite migrations, store methods, handler changes) followed by frontend alignment (event stream, types, UI). Each task is independent and can be committed separately.

**Tech Stack:** Go 1.25, SQLite (modernc.org/sqlite), React 19, TypeScript, TanStack Query, Vite

**Design spec:** `docs/specs/2026-03-17-product-audit-and-stabilization-design.md` — Phase 1 section

---

## File Map

| Task | Creates | Modifies |
|------|---------|----------|
| 1.1 Session metadata | — | `internal/server/store/migrations.go`, `internal/server/store/sessions.go`, `internal/server/session/handler.go`, `web/src/types/session.ts` |
| 1.2 Event type fix | `web/src/constants/eventTypes.ts` | `web/src/hooks/useEventStream.ts` |
| 1.3 Machine rename | `internal/server/handler/machines.go` | `internal/server/store/machines.go`, `internal/server/api/router.go`, `web/src/components/machines/MachineCard.tsx`, `web/src/api/machines.ts`, `web/src/hooks/useMachines.ts` |
| 1.4 Sessions race fix | — | `web/src/hooks/useSessions.ts` |
| 1.5 Encryption key | — | `internal/server/config/config.go`, `cmd/server/main.go` |
| 1.6 404 page | `web/src/views/NotFoundPage.tsx`, `web/src/components/shared/ErrorBoundary.tsx` | `web/src/App.tsx` |

---

## Task 1: Persist session metadata in SQLite (item 1.1)

The sessions table already has `args` and `initial_prompt` columns (migration 1, line 51-65 of `migrations.go`) but the Go code never reads/writes them. Additionally, `model`, `skip_permissions`, and `env_vars` have no columns at all.

**Files:**
- Modify: `internal/server/store/migrations.go:495` (append migration 14)
- Modify: `internal/server/store/sessions.go:10-22` (Session struct), `:25-41` (CreateSession), `:50-57` (GetSession), `:80-84` (ListSessions)
- Modify: `internal/server/session/handler.go:201-209` (CreateSession handler store call)
- Modify: `web/src/types/session.ts:6-15` (Session interface)
- Test: `internal/server/store/sessions_test.go`

### Steps

- [ ] **Step 1: Add migration 14 — new session columns**

Add after line 495 in `internal/server/store/migrations.go`, inside the `migrations` slice:

```go
{
    Version: 14,
    SQL: `ALTER TABLE sessions ADD COLUMN model TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN skip_permissions TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN env_vars TEXT DEFAULT '';`,
},
```

Note: `args` and `initial_prompt` already exist in the schema from migration 1. Only `model`, `skip_permissions`, `env_vars` are new.

- [ ] **Step 2: Add fields to Session struct**

In `internal/server/store/sessions.go`, update the Session struct (lines 10-22) to add:

```go
type Session struct {
	SessionID      string    `json:"session_id"`
	MachineID      string    `json:"machine_id"`
	UserID         string    `json:"user_id"`
	TemplateID     string    `json:"template_id,omitempty"`
	Command        string    `json:"command"`
	WorkingDir     string    `json:"working_dir"`
	Status         string    `json:"status"`
	Model          string    `json:"model,omitempty"`
	SkipPerms      string    `json:"skip_permissions,omitempty"`
	EnvVars        string    `json:"env_vars,omitempty"`
	Args           string    `json:"args,omitempty"`
	InitialPrompt  string    `json:"initial_prompt,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
```

- [ ] **Step 3: Update CreateSession to persist new fields**

In `internal/server/store/sessions.go`, update the CreateSession INSERT statement (lines 25-41) to include all metadata columns:

```go
func (s *Store) CreateSession(sess *Session) error {
	userID := sql.NullString{String: sess.UserID, Valid: sess.UserID != ""}
	templateID := sql.NullString{String: sess.TemplateID, Valid: sess.TemplateID != ""}
	_, err := s.writer.Exec(`
		INSERT INTO sessions (session_id, machine_id, user_id, template_id, command, working_dir, status, model, skip_permissions, env_vars, args, initial_prompt, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		sess.SessionID, sess.MachineID, userID, templateID,
		sess.Command, sess.WorkingDir, sess.Status,
		sess.Model, sess.SkipPerms, sess.EnvVars, sess.Args, sess.InitialPrompt,
	)
	return err
}
```

- [ ] **Step 4: Update GetSession to read new fields**

Update the SELECT and Scan in GetSession (lines 50-57) to include the new columns:

```go
func (s *Store) GetSession(id string) (*Session, error) {
	var sess Session
	var userID, templateID sql.NullString
	var endedAt sql.NullTime
	err := s.reader.QueryRow(`
		SELECT session_id, machine_id, COALESCE(user_id, ''), COALESCE(template_id, ''),
			COALESCE(command, 'claude'), COALESCE(working_dir, ''), status,
			COALESCE(model, ''), COALESCE(skip_permissions, ''), COALESCE(env_vars, ''),
			COALESCE(args, ''), COALESCE(initial_prompt, ''),
			started_at, ended_at
		FROM sessions WHERE session_id = ?`, id).Scan(
		&sess.SessionID, &sess.MachineID, &sess.UserID, &sess.TemplateID,
		&sess.Command, &sess.WorkingDir, &sess.Status,
		&sess.Model, &sess.SkipPerms, &sess.EnvVars,
		&sess.Args, &sess.InitialPrompt,
		&sess.CreatedAt, &endedAt,
	)
	if err != nil {
		return nil, err
	}
	if endedAt.Valid {
		sess.UpdatedAt = endedAt.Time
	}
	return &sess, nil
}
```

Apply the same pattern to ListSessions (lines 80-84) — add the same COALESCE columns and Scan fields.

- [ ] **Step 5: Update handler to pass metadata to store**

In `internal/server/session/handler.go`, update the store call at lines 201-209:

```go
sess := &store.Session{
	SessionID:     sessionID,
	MachineID:     req.MachineID,
	UserID:        userID,
	TemplateID:    templateID,
	Command:       req.Command,
	WorkingDir:    req.WorkingDir,
	Status:        store.StatusCreated,
	Model:         req.Model,
	SkipPerms:     req.SkipPermissions,
	EnvVars:       marshalJSON(req.EnvVars),
	Args:          marshalJSON(req.Args),
	InitialPrompt: req.InitialPrompt,
}
```

Add a helper if one doesn't exist:

```go
func marshalJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
```

Look at the `createSessionRequest` struct (handler.go lines 78-90) — it already has `Model string`, `Args []string`, `EnvVars map[string]string`, `InitialPrompt string` fields. Check if `SkipPermissions` exists; if not, add `SkipPermissions string \`json:"skip_permissions"\`` to the request struct.

- [ ] **Step 6: Update frontend Session type**

In `web/src/types/session.ts`, add the new optional fields:

```typescript
export interface Session {
  session_id: string;
  machine_id: string;
  user_id: string;
  template_id?: string;
  command: string;
  working_dir: string;
  status: 'created' | 'running' | 'completed' | 'failed' | 'terminated';
  model?: string;
  skip_permissions?: string;
  env_vars?: string;
  args?: string;
  initial_prompt?: string;
  created_at: string;
  updated_at: string;
}
```

- [ ] **Step 7: Write test for session metadata persistence**

Create or update `internal/server/store/sessions_test.go`:

```go
func TestCreateSession_PersistsMetadata(t *testing.T) {
	db := newTestStore(t)

	// Setup: create a machine first (FK requirement)
	err := db.UpsertMachine("test-machine", 10)
	require.NoError(t, err)

	sess := &Session{
		SessionID:     "sess-meta-test",
		MachineID:     "test-machine",
		Command:       "claude",
		Status:        "created",
		Model:         "opus",
		SkipPerms:     "true",
		EnvVars:       `{"FOO":"bar"}`,
		Args:          `["--verbose"]`,
		InitialPrompt: "Hello world",
	}
	err = db.CreateSession(sess)
	require.NoError(t, err)

	got, err := db.GetSession("sess-meta-test")
	require.NoError(t, err)
	assert.Equal(t, "opus", got.Model)
	assert.Equal(t, "true", got.SkipPerms)
	assert.Equal(t, `{"FOO":"bar"}`, got.EnvVars)
	assert.Equal(t, `["--verbose"]`, got.Args)
	assert.Equal(t, "Hello world", got.InitialPrompt)
}
```

- [ ] **Step 8: Run tests**

```bash
go test -race ./internal/server/store/ -run TestCreateSession_PersistsMetadata -v
```

Expected: PASS

- [ ] **Step 9: Run full backend test suite**

```bash
go test -race ./...
```

Expected: All existing tests still pass.

- [ ] **Step 10: Commit**

```bash
git add internal/server/store/migrations.go internal/server/store/sessions.go internal/server/store/sessions_test.go internal/server/session/handler.go web/src/types/session.ts
git commit -m "fix: persist session metadata (model, env_vars, args) in SQLite"
```

---

## Task 2: Fix frontend/backend event type mismatches (item 1.2)

Frontend listens for `machine.status`, `machine.health`, and `run.step.status` — none of these exist in the backend. Backend emits `machine.connected`, `machine.disconnected`, `run.step.completed`, `run.step.failed`. Payload fields use snake_case in backend but frontend reads camelCase.

**Files:**
- Create: `web/src/constants/eventTypes.ts`
- Modify: `web/src/hooks/useEventStream.ts:46-73`
- Test: `web/src/__tests__/hooks/useEventStream.test.ts`

### Steps

- [ ] **Step 1: Create event type constants file**

Create `web/src/constants/eventTypes.ts`:

```typescript
/**
 * Event type constants — must match backend internal/server/event/event.go.
 * Phase 2 will add CI validation to enforce sync.
 */

// Run lifecycle
export const RUN_CREATED = 'run.created';
export const RUN_STARTED = 'run.started';
export const RUN_COMPLETED = 'run.completed';
export const RUN_FAILED = 'run.failed';
export const RUN_CANCELLED = 'run.cancelled';

// Session lifecycle
export const SESSION_STARTED = 'session.started';
export const SESSION_EXITED = 'session.exited';
export const SESSION_TERMINATED = 'session.terminated';

// Machine connectivity
export const MACHINE_CONNECTED = 'machine.connected';
export const MACHINE_DISCONNECTED = 'machine.disconnected';

// Triggers
export const TRIGGER_CRON = 'trigger.cron';
export const TRIGGER_WEBHOOK = 'trigger.webhook';
export const TRIGGER_JOB_COMPLETED = 'trigger.job_completed';

// Templates
export const TEMPLATE_CREATED = 'template.created';
export const TEMPLATE_UPDATED = 'template.updated';
export const TEMPLATE_DELETED = 'template.deleted';

// Run steps
export const RUN_STEP_COMPLETED = 'run.step.completed';
export const RUN_STEP_FAILED = 'run.step.failed';

/** All known event types — used for validation and webhook selectors */
export const ALL_EVENT_TYPES = [
  RUN_CREATED, RUN_STARTED, RUN_COMPLETED, RUN_FAILED, RUN_CANCELLED,
  SESSION_STARTED, SESSION_EXITED, SESSION_TERMINATED,
  MACHINE_CONNECTED, MACHINE_DISCONNECTED,
  TRIGGER_CRON, TRIGGER_WEBHOOK, TRIGGER_JOB_COMPLETED,
  TEMPLATE_CREATED, TEMPLATE_UPDATED, TEMPLATE_DELETED,
  RUN_STEP_COMPLETED, RUN_STEP_FAILED,
] as const;
```

- [ ] **Step 2: Rewrite useEventStream event handler**

In `web/src/hooks/useEventStream.ts`, replace the switch/case block (lines 46-73) with corrected event types and snake_case payload fields. Import from the new constants file:

```typescript
import {
  SESSION_STARTED, SESSION_EXITED, SESSION_TERMINATED,
  MACHINE_CONNECTED, MACHINE_DISCONNECTED,
  RUN_STEP_COMPLETED, RUN_STEP_FAILED,
  RUN_CREATED, RUN_STARTED, RUN_COMPLETED, RUN_FAILED, RUN_CANCELLED,
  TEMPLATE_CREATED, TEMPLATE_UPDATED, TEMPLATE_DELETED,
} from '../constants/eventTypes';
```

Replace the switch block:

```typescript
switch (msg.event_type) {
  case SESSION_STARTED:
  case SESSION_EXITED:
  case SESSION_TERMINATED:
    queryClient.invalidateQueries({ queryKey: ['sessions'] });
    break;

  case MACHINE_CONNECTED:
  case MACHINE_DISCONNECTED:
    queryClient.invalidateQueries({ queryKey: ['machines'] });
    break;

  case RUN_CREATED:
  case RUN_STARTED:
  case RUN_COMPLETED:
  case RUN_FAILED:
  case RUN_CANCELLED:
    queryClient.invalidateQueries({ queryKey: ['runs'] });
    break;

  case TEMPLATE_CREATED:
  case TEMPLATE_UPDATED:
  case TEMPLATE_DELETED:
    queryClient.invalidateQueries({ queryKey: ['templates'] });
    break;

  case RUN_STEP_COMPLETED:
  case RUN_STEP_FAILED: {
    const payload = msg.payload as Record<string, unknown>;
    const runId = payload.run_id as string | undefined;
    const stepId = payload.step_id as string | undefined;
    const status = payload.status as string | undefined;
    if (runId && stepId && status) {
      useRunStore.getState().updateTaskStatus(runId, stepId, status);
    }
    queryClient.invalidateQueries({ queryKey: ['runs'] });
    break;
  }
}
```

Key changes:
- `machine.status` / `machine.health` → `machine.connected` / `machine.disconnected`
- `run.step.status` → `run.step.completed` / `run.step.failed`
- Payload fields: `runId` → `run_id`, `stepId` → `step_id` (snake_case)
- Added run and template invalidation (missing before)

- [ ] **Step 3: Write test for event stream handler**

Create `web/src/__tests__/hooks/useEventStream.test.ts` that validates the correct event types trigger correct query invalidation. Use a mock WebSocket and mock QueryClient.

- [ ] **Step 4: Run frontend tests and typecheck**

```bash
cd web && npx tsc --noEmit && npx vitest run
```

Expected: All pass, no type errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/constants/eventTypes.ts web/src/hooks/useEventStream.ts web/src/__tests__/hooks/useEventStream.test.ts
git commit -m "fix: align frontend event types with backend constants"
```

---

## Task 3: Fix machine display names — add rename endpoint (item 1.3)

Machines register with a `machine_id` only. `display_name` is always NULL because there's no update endpoint. Users see truncated UUIDs everywhere.

**Files:**
- Create: `internal/server/handler/machines.go`
- Modify: `internal/server/store/machines.go:27-37` (add UpdateMachineDisplayName)
- Modify: `internal/server/api/router.go:117-118` (register new route)
- Modify: `web/src/components/machines/MachineCard.tsx` (add rename action)
- Modify: `web/src/api/machines.ts` (add update function)
- Modify: `web/src/hooks/useMachines.ts` (add useUpdateMachine mutation)
- Test: `internal/server/store/machines_test.go`, `internal/server/handler/machines_test.go`

### Steps

- [ ] **Step 1: Add store method UpdateMachineDisplayName**

In `internal/server/store/machines.go`, add after the existing methods:

```go
func (s *Store) UpdateMachineDisplayName(machineID, displayName string) error {
	result, err := s.writer.Exec(
		`UPDATE machines SET display_name = ? WHERE machine_id = ?`,
		displayName, machineID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 2: Write test for store method**

In `internal/server/store/machines_test.go`:

```go
func TestUpdateMachineDisplayName(t *testing.T) {
	db := newTestStore(t)

	err := db.UpsertMachine("machine-1", 10)
	require.NoError(t, err)

	err = db.UpdateMachineDisplayName("machine-1", "Production Server")
	require.NoError(t, err)

	m, err := db.GetMachine("machine-1")
	require.NoError(t, err)
	assert.Equal(t, "Production Server", m.DisplayName)
}

func TestUpdateMachineDisplayName_NotFound(t *testing.T) {
	db := newTestStore(t)

	err := db.UpdateMachineDisplayName("nonexistent", "Name")
	assert.ErrorIs(t, err, ErrNotFound)
}
```

- [ ] **Step 3: Run store test**

```bash
go test -race ./internal/server/store/ -run TestUpdateMachineDisplayName -v
```

Expected: PASS

- [ ] **Step 4: Create machine handler**

Create `internal/server/handler/machines.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/kodrunhq/claude-plane/internal/server/httputil"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

type MachineHandler struct {
	store *store.Store
}

func NewMachineHandler(s *store.Store) *MachineHandler {
	return &MachineHandler{store: s}
}

type updateMachineRequest struct {
	DisplayName string `json:"display_name"`
}

func (h *MachineHandler) UpdateMachine(w http.ResponseWriter, r *http.Request) {
	machineID := chi.URLParam(r, "machineID")
	if machineID == "" {
		httputil.Error(w, http.StatusBadRequest, "machine_id is required")
		return
	}

	var req updateMachineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.store.UpdateMachineDisplayName(machineID, req.DisplayName); err != nil {
		if err == store.ErrNotFound {
			httputil.Error(w, http.StatusNotFound, "machine not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "failed to update machine")
		return
	}

	m, err := h.store.GetMachine(machineID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to fetch machine")
		return
	}

	httputil.JSON(w, http.StatusOK, m)
}
```

- [ ] **Step 5: Register route in router**

In `internal/server/api/router.go`, find the machine routes (lines 117-118) and add the PUT route. You'll need to check how `deps` is structured — if machines use the base handler `h`, wire the new `MachineHandler` through deps. Add:

```go
r.Put("/machines/{machineID}", machineHandler.UpdateMachine)
```

Ensure the `MachineHandler` is created in `cmd/server/main.go` and passed to the router deps.

- [ ] **Step 6: Write handler test**

Create `internal/server/handler/machines_test.go` testing the update endpoint returns 200 with updated machine, 404 for nonexistent machine, and 400 for invalid body.

- [ ] **Step 7: Run handler test**

```bash
go test -race ./internal/server/handler/ -run TestMachineHandler -v
```

Expected: PASS

- [ ] **Step 8: Add frontend API function**

In `web/src/api/machines.ts`, add:

```typescript
update: (machineId: string, data: { display_name: string }) =>
  request<Machine>(`/machines/${machineId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
```

- [ ] **Step 9: Add mutation hook**

In `web/src/hooks/useMachines.ts`, add:

```typescript
export function useUpdateMachine() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ machineId, displayName }: { machineId: string; displayName: string }) =>
      machinesApi.update(machineId, { display_name: displayName }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['machines'] });
    },
  });
}
```

- [ ] **Step 10: Add rename action to MachineCard**

In `web/src/components/machines/MachineCard.tsx`, add a "Rename" button that opens a small inline input or modal. On submit, call `useUpdateMachine().mutate({ machineId, displayName })`. Show current display_name as default value.

- [ ] **Step 11: Run full test suite**

```bash
go test -race ./... && cd web && npx tsc --noEmit && npx vitest run
```

Expected: All pass.

- [ ] **Step 12: Commit**

```bash
git add internal/server/handler/machines.go internal/server/handler/machines_test.go internal/server/store/machines.go internal/server/store/machines_test.go internal/server/api/router.go cmd/server/main.go web/src/api/machines.ts web/src/hooks/useMachines.ts web/src/components/machines/MachineCard.tsx
git commit -m "feat: add machine rename endpoint and UI"
```

---

## Task 4: Fix Command Center active sessions race condition (item 1.4)

Sessions appear empty on first render because `useSessions()` returns `undefined` before the API responds. The `useMemo` filter in CommandCenter treats `undefined` as empty array.

**Files:**
- Modify: `web/src/hooks/useSessions.ts:3-8`

### Steps

- [ ] **Step 1: Add placeholderData to useSessions**

In `web/src/hooks/useSessions.ts`, update the `useSessions` function:

```typescript
export function useSessions(filters?: Record<string, string>) {
  return useQuery({
    queryKey: ['sessions', filters],
    queryFn: () => sessionsApi.list(filters),
    refetchInterval: 30_000,
    placeholderData: [],
  });
}
```

This ensures `data` is always `Session[]` (never `undefined`), so the CommandCenter's `useMemo` filter works immediately.

- [ ] **Step 2: Verify typecheck passes**

```bash
cd web && npx tsc --noEmit
```

Expected: No errors. TanStack Query's `placeholderData` types should infer correctly.

- [ ] **Step 3: Commit**

```bash
git add web/src/hooks/useSessions.ts
git commit -m "fix: prevent empty flash on Command Center active sessions"
```

---

## Task 5: Auto-generate encryption key for credentials vault (item 1.5)

Credentials page crashes because encryption key requires manual config. Auto-generate on first boot.

**Files:**
- Modify: `internal/server/config/config.go:156-194` (ParseEncryptionKey)
- Modify: `cmd/server/main.go` (startup sequence)
- Test: `internal/server/config/config_test.go`

### Steps

- [ ] **Step 1: Add auto-generation logic to config**

In `internal/server/config/config.go`, modify the `ParseEncryptionKey` method. Currently it returns an error if no key is found. Change it to auto-generate when missing:

```go
func (c *SecretsConfig) ParseEncryptionKey(dataDir string) ([]byte, error) {
	// Existing resolution order: file → config → env var
	hexKey := c.resolveKeyString()

	if hexKey != "" {
		if len(hexKey) != 64 {
			return nil, fmt.Errorf("encryption key must be 64 hex characters (32 bytes), got %d", len(hexKey))
		}
		return hex.DecodeString(hexKey)
	}

	// No key configured — auto-generate
	keyPath := filepath.Join(dataDir, "encryption.key")

	// Try reading existing auto-generated key
	if data, err := os.ReadFile(keyPath); err == nil {
		hexKey = strings.TrimSpace(string(data))
		if len(hexKey) == 64 {
			return hex.DecodeString(hexKey)
		}
	}

	// Generate new key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}

	hexKey = hex.EncodeToString(key)
	if err := os.WriteFile(keyPath, []byte(hexKey+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("failed to write encryption key to %s: %w", keyPath, err)
	}

	slog.Info("generated encryption key", "path", keyPath)
	return key, nil
}

func (c *SecretsConfig) resolveKeyString() string {
	if c.EncryptionKeyFile != "" {
		if data, err := os.ReadFile(c.EncryptionKeyFile); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	if c.EncryptionKey != "" {
		return c.EncryptionKey
	}
	if envKey := os.Getenv("CLAUDE_PLANE_ENCRYPTION_KEY"); envKey != "" {
		return envKey
	}
	return ""
}
```

- [ ] **Step 2: Update server startup to pass dataDir**

In `cmd/server/main.go`, find where `ParseEncryptionKey` is called (around line 331-337). Update the call to pass the database directory:

```go
dataDir := filepath.Dir(cfg.Database.Path) // derive from DB path
encKey, err := cfg.Secrets.ParseEncryptionKey(dataDir)
if err != nil {
    return fmt.Errorf("encryption key: %w", err)
}
```

Remove any fallback that returns 503 when key is missing — it should always succeed now.

- [ ] **Step 3: Write test for auto-generation**

In `internal/server/config/config_test.go`:

```go
func TestParseEncryptionKey_AutoGenerate(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := SecretsConfig{} // no key configured
	key, err := cfg.ParseEncryptionKey(tmpDir)
	require.NoError(t, err)
	assert.Len(t, key, 32)

	// Verify key file was created
	keyPath := filepath.Join(tmpDir, "encryption.key")
	data, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	assert.Len(t, strings.TrimSpace(string(data)), 64) // 32 bytes = 64 hex chars

	// Verify same key is returned on second call
	key2, err := cfg.ParseEncryptionKey(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, key, key2)
}

func TestParseEncryptionKey_ExplicitKey(t *testing.T) {
	cfg := SecretsConfig{
		EncryptionKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	key, err := cfg.ParseEncryptionKey(t.TempDir())
	require.NoError(t, err)
	assert.Len(t, key, 32)
}
```

- [ ] **Step 4: Run tests**

```bash
go test -race ./internal/server/config/ -run TestParseEncryptionKey -v
```

Expected: PASS

- [ ] **Step 5: Run full backend test suite**

```bash
go test -race ./...
```

Expected: All pass. No existing tests should break since we only changed the no-key code path.

- [ ] **Step 6: Commit**

```bash
git add internal/server/config/config.go internal/server/config/config_test.go cmd/server/main.go
git commit -m "fix: auto-generate encryption key on first boot"
```

---

## Task 6: Add 404 page and React error boundary (item 1.6)

Invalid URLs render blank pages. Component crashes propagate to white screens.

**Files:**
- Create: `web/src/views/NotFoundPage.tsx`
- Create: `web/src/components/shared/ErrorBoundary.tsx`
- Modify: `web/src/App.tsx:87-115` (add catch-all route and error boundary)
- Test: `web/src/__tests__/views/NotFoundPage.test.tsx`

### Steps

- [ ] **Step 1: Create NotFoundPage component**

Create `web/src/views/NotFoundPage.tsx`:

```tsx
import { FileQuestion } from 'lucide-react';
import { Link } from 'react-router-dom';

export function NotFoundPage() {
  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh] px-4">
      <FileQuestion size={64} className="text-text-secondary mb-4" />
      <h1 className="text-2xl font-bold text-text-primary mb-2">Page not found</h1>
      <p className="text-text-secondary mb-6 text-center">
        The page you're looking for doesn't exist or has been moved.
      </p>
      <Link
        to="/"
        className="px-4 py-2 bg-accent-primary text-white rounded-lg hover:bg-accent-primary/80 transition-colors"
      >
        Go to Command Center
      </Link>
    </div>
  );
}
```

- [ ] **Step 2: Create ErrorBoundary component**

Create `web/src/components/shared/ErrorBoundary.tsx`:

```tsx
import { Component, type ReactNode } from 'react';
import { AlertTriangle } from 'lucide-react';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex flex-col items-center justify-center min-h-[60vh] px-4">
          <AlertTriangle size={64} className="text-status-error mb-4" />
          <h1 className="text-2xl font-bold text-text-primary mb-2">Something went wrong</h1>
          <p className="text-text-secondary mb-2 text-center">
            An unexpected error occurred. Try refreshing the page.
          </p>
          <p className="text-xs text-text-secondary mb-6 font-mono max-w-md text-center truncate">
            {this.state.error?.message}
          </p>
          <button
            onClick={() => {
              this.setState({ hasError: false, error: null });
              window.location.href = '/';
            }}
            className="px-4 py-2 bg-accent-primary text-white rounded-lg hover:bg-accent-primary/80 transition-colors"
          >
            Go Home
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}
```

- [ ] **Step 3: Add to App.tsx**

In `web/src/App.tsx`, import both components and wrap the Routes in ErrorBoundary. Add catch-all route at the end:

```tsx
import { NotFoundPage } from './views/NotFoundPage';
import { ErrorBoundary } from './components/shared/ErrorBoundary';
```

Wrap the `<Routes>` in `<ErrorBoundary>`:

```tsx
<ErrorBoundary>
  <Routes>
    {/* ... all existing routes ... */}
    <Route path="*" element={<NotFoundPage />} />
  </Routes>
</ErrorBoundary>
```

- [ ] **Step 4: Write test**

Create `web/src/__tests__/views/NotFoundPage.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { NotFoundPage } from '../../views/NotFoundPage';

describe('NotFoundPage', () => {
  it('renders page not found message', () => {
    render(
      <MemoryRouter>
        <NotFoundPage />
      </MemoryRouter>
    );
    expect(screen.getByText('Page not found')).toBeInTheDocument();
    expect(screen.getByText('Go to Command Center')).toBeInTheDocument();
  });
});
```

- [ ] **Step 5: Run frontend tests and typecheck**

```bash
cd web && npx tsc --noEmit && npx vitest run
```

Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add web/src/views/NotFoundPage.tsx web/src/components/shared/ErrorBoundary.tsx web/src/App.tsx web/src/__tests__/views/NotFoundPage.test.tsx
git commit -m "fix: add 404 page and error boundary for graceful error handling"
```

---

## Final Verification

- [ ] **Run complete CI check**

```bash
# Backend
go vet ./...
go test -race ./...

# Frontend
cd web && npx tsc --noEmit && npx vitest run && npx vite build
```

All must pass before creating the PR.

- [ ] **Create PR for Phase 1**

Branch name: `fix/phase1-critical-fixes`
PR title: "fix: Phase 1 — Critical fixes (session metadata, event types, machine rename, 404)"

---

## Review Corrections

The following corrections were identified during plan review and MUST be applied during implementation:

### Task 1 (Session metadata)

1. **Add `Model` and `SkipPermissions` to `createSessionRequest` struct** in `session/handler.go:78-90`. These fields don't exist on the request struct yet — add `Model string \`json:"model"\`` and `SkipPermissions string \`json:"skip_permissions"\``.

2. **Preserve `fmt.Errorf` wrapping** in CreateSession. The existing code wraps errors: `return fmt.Errorf("create session: %w", err)`. Keep this pattern.

3. **Keep `sql.NullString` handling** in GetSession/ListSessions. The existing code scans `userID` and `templateID` as `sql.NullString` then conditionally assigns. Keep this pattern — don't replace it with direct COALESCE scanning, as it's used consistently across the codebase.

4. **Update ListSessions and scanSessions separately.** Step 4 bundles ListSessions into GetSession's step. Treat ListSessions (and the shared `scanSessions` helper if one exists) as a separate sub-step.

5. **Add frontend UI for session metadata display.** The plan updates the type but doesn't add UI. Add a sub-step: modify `SessionCard.tsx` to show a model badge when `session.model` is set. The session detail header (3.5 in Phase 3) will show the full metadata — keep that in Phase 3.

### Task 2 (Event types)

6. **Preserve 4th argument to `updateTaskStatus`.** The existing code passes `sessionId` as the 4th argument: `updateTaskStatus(runId, stepId, status, sessionId)`. Add `const sessionId = payload.session_id as string | undefined;` and pass it: `updateTaskStatus(runId, stepId, status, sessionId)`.

7. **Write actual test code for Step 3.** The plan says "Create test..." but provides no code. Write a test using a mock WebSocket (or `vi.fn()` for the query client) that verifies: `machine.connected` event invalidates `['machines']` query, `run.step.completed` event calls `updateTaskStatus` with snake_case fields.

### Task 3 (Machine rename)

8. **Add `UpdateMachine` to the existing `api` package, not a new `handler/machines.go` file.** The existing machine handlers (`ListMachines`, `GetMachine`) are methods on the `Handlers` struct in the `api` package (registered in `router.go:117-118`). Add `UpdateMachine` as a method on this same struct. Do NOT create a separate `handler/machines.go`.

9. **Use correct error variable.** The machines store uses `ErrMachineNotFound`, not `ErrNotFound`. Update the not-found check accordingly.

10. **Use correct httputil function names.** The codebase uses `httputil.WriteError()` and `httputil.WriteJSON()`, not `httputil.Error()` and `httputil.JSON()`. Check the actual function signatures in `internal/server/httputil/` or `internal/server/api/`.

11. **Add `max_sessions` to the update request.** The spec says the endpoint accepts both `display_name` and `max_sessions`. Update `updateMachineRequest` to include `MaxSessions *int32 \`json:"max_sessions,omitempty"\``.

12. **Add display_name length validation.** Validate `len(req.DisplayName) <= 255` in the handler.

13. **Agent registration display_name is deferred.** The spec mentions agent-side config changes for `display_name` during `Register()` RPC. This is a deeper change touching the proto file and agent binary. Defer to a follow-up — the machine rename UI is the priority.

14. **Write actual handler test code.** The plan says "Create test..." but provides no code. Write tests covering: 200 with valid update, 404 for nonexistent machine, 400 for empty body, 400 for display_name > 255 chars.

### Task 5 (Encryption key)

15. **Breaking signature change.** `ParseEncryptionKey()` → `ParseEncryptionKey(dataDir string)` changes the method signature. Grep for all callers (`grep -r "ParseEncryptionKey" --include="*.go"`) and update each one. The plan only mentions `cmd/server/main.go` — verify there are no other callers.

16. **Don't silently swallow EncryptionKeyFile errors.** The existing code returns an error if `EncryptionKeyFile` is set but unreadable. The plan's `resolveKeyString()` silently falls through. Keep the error behavior: if `EncryptionKeyFile` is explicitly configured and can't be read, return an error (don't fall through to auto-generate).

17. **Handle corrupted key file.** If `encryption.key` exists but has invalid hex content, return a clear error: `"existing encryption key file at %s has invalid content — expected 64 hex characters, got %d"`.

### Task 6 (404 page)

18. **Inline 404 states for API errors deferred to Phase 3.** The spec says invalid entity IDs should show inline error states. The plan only adds the catch-all route. This is fine — inline API 404 handling is part of Phase 3 (session detail header, job editor, etc.). Note this in the plan for traceability.
