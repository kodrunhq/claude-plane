# Bugs & Features Batch Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the double-step job creation bug, add skip-permissions/model/delay to steps, add trigger filter evaluation, build a settings page, and add session log search.

**Architecture:** Backend-first — migration, store, handlers, executor, then frontend. Each feature shares the same preference resolution pattern: step override > user preference > system default. Session search uses the existing CommandStream bidirectional gRPC, not a new RPC.

**Tech Stack:** Go 1.25, Chi router, SQLite, gRPC/protobuf, React 19, TypeScript, Zustand, TanStack Query, Tailwind CSS.

**Spec:** `docs/superpowers/specs/2026-03-14-bugs-features-batch-design.md`

---

## Chunk 1: Database Migration + Store Layer

### Task 1: Add Migration Version 9

**Files:**
- Modify: `internal/server/store/migrations.go:365` (insert before closing `}` of migrations slice at line 366)

- [ ] **Step 1: Write the migration SQL**

Add migration 9 after the existing migration 8 entry (line 364). Insert before the closing `}` of the migrations slice at line 366. Note: migration struct requires both `Version` and `Description` fields.

```go
// In the migrations slice, after migration 8 (line 364), before closing } at line 366:
{
    Version:     9,
    Description: "add step preferences, run_step snapshots, and user_preferences table",
    SQL: `
        ALTER TABLE steps ADD COLUMN skip_permissions INTEGER;
        ALTER TABLE steps ADD COLUMN model TEXT DEFAULT '';
        ALTER TABLE steps ADD COLUMN delay_seconds INTEGER DEFAULT 0;

        ALTER TABLE run_steps ADD COLUMN skip_permissions_snapshot INTEGER;
        ALTER TABLE run_steps ADD COLUMN model_snapshot TEXT DEFAULT '';
        ALTER TABLE run_steps ADD COLUMN delay_seconds_snapshot INTEGER DEFAULT 0;
        ALTER TABLE run_steps ADD COLUMN on_failure_snapshot TEXT DEFAULT 'fail_run';
        ALTER TABLE run_steps ADD COLUMN timeout_seconds_snapshot INTEGER DEFAULT 0;

        CREATE TABLE IF NOT EXISTS user_preferences (
            user_id    TEXT PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
            preferences TEXT NOT NULL DEFAULT '{}',
            updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        );
    `,
},
```

- [ ] **Step 2: Run tests to verify migration applies cleanly**

Run: `go test -race ./internal/server/store/ -run TestMigrations -v`
Expected: PASS (or if no migration test exists, run `go test -race ./internal/server/store/...`)

- [ ] **Step 3: Commit**

```bash
git add internal/server/store/migrations.go
git commit -m "feat: add migration 9 — step fields, run_steps snapshots, user_preferences table"
```

---

### Task 2: Update Step Struct and Store Functions

**Files:**
- Modify: `internal/server/store/jobs.go:107-120` (Step struct)
- Modify: `internal/server/store/jobs.go:16-28` (CreateStepParams)
- Modify: `internal/server/store/jobs.go:30-42` (UpdateStepParams)
- Modify: `internal/server/store/jobs.go:345-369` (CreateStep SQL)
- Modify: `internal/server/store/jobs.go:371-386` (UpdateStep SQL)

- [ ] **Step 1: Write failing test for new Step fields**

Create test or extend existing test verifying that CreateStep and UpdateStep round-trip the new fields (`SkipPermissions`, `Model`, `DelaySeconds`).

```go
func TestCreateStep_NewFields(t *testing.T) {
    s := newTestStore(t)
    // Create job first
    job, err := s.CreateJob(ctx, store.CreateJobParams{Name: "test"})
    require.NoError(t, err)

    skipPerms := 1
    step, err := s.CreateStep(ctx, store.CreateStepParams{
        JobID:           job.JobID,
        Name:            "step1",
        Prompt:          "do stuff",
        SkipPermissions: &skipPerms,
        Model:           "sonnet",
        DelaySeconds:    30,
    })
    require.NoError(t, err)
    assert.Equal(t, &skipPerms, step.SkipPermissions)
    assert.Equal(t, "sonnet", step.Model)
    assert.Equal(t, 30, step.DelaySeconds)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/server/store/ -run TestCreateStep_NewFields -v`
Expected: FAIL — fields don't exist on struct yet

- [ ] **Step 3: Add fields to Step struct**

In `internal/server/store/jobs.go`, add three fields after `OnFailure` in the `Step` struct (line 119). Note: `SkipPermissions` uses `*int` (nullable) because SQLite NULL must be scannable via pointer:

```go
    // Add after OnFailure string `json:"on_failure"` (line 119):
    SkipPermissions *int   `json:"skip_permissions"`
    Model           string `json:"model"`
    DelaySeconds    int    `json:"delay_seconds"`
```

- [ ] **Step 4: Add fields to CreateStepParams and UpdateStepParams**

```go
type CreateStepParams struct {
    JobID           string
    Name            string
    Prompt          string
    MachineID       string
    WorkingDir      string
    Command         string
    Args            string
    TimeoutSeconds  int
    SortOrder       int
    OnFailure       string
    SkipPermissions *int
    Model           string
    DelaySeconds    int
}

type UpdateStepParams struct {
    StepID          string
    Name            string
    Prompt          string
    MachineID       string
    WorkingDir      string
    Command         string
    Args            string
    TimeoutSeconds  int
    SortOrder       int
    OnFailure       string
    SkipPermissions *int
    Model           string
    DelaySeconds    int
}
```

- [ ] **Step 5: Update CreateStep SQL**

In the `CreateStep` function (line 345), add the three new columns to the INSERT:

```go
_, err := s.writer.ExecContext(ctx,
    `INSERT INTO steps (step_id, job_id, name, prompt, machine_id, working_dir, command, args, timeout_seconds, sort_order, on_failure, skip_permissions, model, delay_seconds)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    stepID, p.JobID, p.Name, p.Prompt, p.MachineID, p.WorkingDir, p.Command, p.Args, p.TimeoutSeconds, p.SortOrder, p.OnFailure, p.SkipPermissions, p.Model, p.DelaySeconds,
)
```

Note: `CreateStep` returns a struct literal directly (not a SELECT after INSERT). Update the struct literal that's returned (around lines 356-368) to include the new fields from the params.

- [ ] **Step 6: Update UpdateStep SQL**

In the `UpdateStep` function (line 371), add the three new columns to the UPDATE SET clause:

```go
_, err := s.writer.ExecContext(ctx,
    `UPDATE steps SET name=?, prompt=?, machine_id=?, working_dir=?, command=?, args=?, timeout_seconds=?, sort_order=?, on_failure=?, skip_permissions=?, model=?, delay_seconds=?
     WHERE step_id=?`,
    p.Name, p.Prompt, p.MachineID, p.WorkingDir, p.Command, p.Args, p.TimeoutSeconds, p.SortOrder, p.OnFailure, p.SkipPermissions, p.Model, p.DelaySeconds, p.StepID,
)
```

- [ ] **Step 7: Update all SELECT queries that scan Step structs**

Find all `rows.Scan` or `row.Scan` calls that populate Step structs and add the three new fields. Check: `GetStepsForJob`, `GetStep`, `GetStepsWithDeps`, and any other queries returning Step rows.

- [ ] **Step 8: Run test to verify it passes**

Run: `go test -race ./internal/server/store/ -run TestCreateStep_NewFields -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/server/store/jobs.go
git commit -m "feat: add skip_permissions, model, delay_seconds to Step struct and store functions"
```

---

### Task 3: Update RunStep Struct and Snapshot Logic

**Files:**
- Modify: `internal/server/store/jobs.go:140-157` (RunStep struct)
- Modify: `internal/server/store/jobs.go:506-529` (InsertRunSteps)
- Modify: `internal/server/orchestrator/orchestrator.go:82-91` (OnFailure population)

- [ ] **Step 1: Write failing test for RunStep snapshots**

```go
func TestInsertRunSteps_SnapshotsNewFields(t *testing.T) {
    s := newTestStore(t)
    // Setup: create job, steps with new fields, create run
    skipPerms := 1
    step := store.Step{
        StepID: "s1", JobID: "j1", Name: "step1", Prompt: "p",
        SkipPermissions: &skipPerms, Model: "opus", DelaySeconds: 10,
        OnFailure: "continue", TimeoutSeconds: 300,
    }
    err := s.InsertRunSteps(ctx, "run1", []store.Step{step})
    require.NoError(t, err)

    // Verify snapshots
    runSteps, err := s.GetRunSteps(ctx, "run1")
    require.NoError(t, err)
    assert.Equal(t, &skipPerms, runSteps[0].SkipPermissionsSnapshot)
    assert.Equal(t, "opus", runSteps[0].ModelSnapshot)
    assert.Equal(t, 10, runSteps[0].DelaySecondsSnapshot)
    assert.Equal(t, "continue", runSteps[0].OnFailureSnapshot)
    assert.Equal(t, 300, runSteps[0].TimeoutSecondsSnapshot)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/server/store/ -run TestInsertRunSteps_SnapshotsNewFields -v`
Expected: FAIL

- [ ] **Step 3: Add snapshot fields to RunStep struct**

Add after `ArgsSnapshot` (line 155) in the existing `RunStep` struct. Do NOT redefine the whole struct — only add the new fields. The existing struct uses `*int` for ExitCode and `*time.Time` for timestamps — preserve those types:

```go
    // Add after ArgsSnapshot string (line 155), before OnFailure string (line 156):
    SkipPermissionsSnapshot *int   `json:"skip_permissions_snapshot"`
    ModelSnapshot           string `json:"model_snapshot"`
    DelaySecondsSnapshot    int    `json:"delay_seconds_snapshot"`
    OnFailureSnapshot       string `json:"on_failure_snapshot"`
    TimeoutSecondsSnapshot  int    `json:"timeout_seconds_snapshot"`
```

- [ ] **Step 4: Update InsertRunSteps to write snapshot columns**

In `InsertRunSteps` (line 506), update the INSERT statement to include the 5 new snapshot columns:

```go
_, err := tx.ExecContext(ctx,
    `INSERT INTO run_steps (run_step_id, run_id, step_id, status, machine_id,
     prompt_snapshot, machine_id_snapshot, working_dir_snapshot, command_snapshot, args_snapshot,
     skip_permissions_snapshot, model_snapshot, delay_seconds_snapshot, on_failure_snapshot, timeout_seconds_snapshot)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    runStepID, runID, step.StepID, "pending", step.MachineID,
    step.Prompt, step.MachineID, step.WorkingDir, step.Command, step.Args,
    step.SkipPermissions, step.Model, step.DelaySeconds, step.OnFailure, step.TimeoutSeconds,
)
```

- [ ] **Step 5: Update orchestrator to read OnFailure from snapshot**

In `internal/server/orchestrator/orchestrator.go` lines 82-91, there's a manual loop that populates `RunStep.OnFailure` from the `steps` table after `InsertRunSteps`. Now that `on_failure_snapshot` is stored in `run_steps`, this loop can be removed. Instead:

1. In the store, find the `GetRunWithSteps` query (or whichever query loads RunSteps for a run). Update its SELECT to include `on_failure_snapshot` and scan it into `RunStep.OnFailureSnapshot`.
2. After scanning, set `RunStep.OnFailure = RunStep.OnFailureSnapshot` (the DAG runner reads `OnFailure`).
3. Remove the manual population loop at orchestrator.go lines 82-91.
4. The same pattern applies to the other snapshot fields — ensure `GetRunWithSteps` SELECTs and scans all new snapshot columns.

- [ ] **Step 6: Update all RunStep SELECT queries to scan new columns**

Find all queries that scan RunStep rows and add the 5 new snapshot columns.

- [ ] **Step 7: Run test to verify it passes**

Run: `go test -race ./internal/server/store/ -run TestInsertRunSteps_SnapshotsNewFields -v`
Expected: PASS

- [ ] **Step 8: Run full store tests**

Run: `go test -race ./internal/server/store/...`
Expected: ALL PASS

- [ ] **Step 9: Commit**

```bash
git add internal/server/store/jobs.go internal/server/orchestrator/orchestrator.go
git commit -m "feat: snapshot skip_permissions, model, delay, on_failure, timeout into run_steps"
```

---

### Task 4: User Preferences Store

**Files:**
- Create: `internal/server/store/preferences.go`
- Create: `internal/server/store/preferences_test.go`

- [ ] **Step 1: Write failing test for preferences CRUD**

```go
// preferences_test.go
func TestPreferences_CRUD(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    // Create user first
    user := createTestUser(t, s)

    // Get returns empty defaults when no prefs exist
    prefs, err := s.GetUserPreferences(ctx, user.UserID)
    require.NoError(t, err)
    assert.Equal(t, "{}", prefs.Preferences)

    // Upsert
    err = s.UpsertUserPreferences(ctx, user.UserID, `{"skip_permissions":true,"machine_overrides":{}}`)
    require.NoError(t, err)

    // Get returns updated
    prefs, err = s.GetUserPreferences(ctx, user.UserID)
    require.NoError(t, err)
    assert.Contains(t, prefs.Preferences, `"skip_permissions":true`)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/server/store/ -run TestPreferences_CRUD -v`
Expected: FAIL

- [ ] **Step 3: Implement preferences store**

```go
// preferences.go
package store

import (
    "context"
    "time"
)

type UserPreferences struct {
    UserID      string    `json:"user_id"`
    Preferences string    `json:"preferences"`
    UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Store) GetUserPreferences(ctx context.Context, userID string) (*UserPreferences, error) {
    row := s.reader.QueryRowContext(ctx,
        `SELECT user_id, preferences, updated_at FROM user_preferences WHERE user_id = ?`, userID)

    var p UserPreferences
    err := row.Scan(&p.UserID, &p.Preferences, &p.UpdatedAt)
    if err != nil {
        // Return defaults if no row
        return &UserPreferences{UserID: userID, Preferences: "{}", UpdatedAt: time.Now()}, nil
    }
    return &p, nil
}

func (s *Store) UpsertUserPreferences(ctx context.Context, userID, preferences string) error {
    _, err := s.writer.ExecContext(ctx,
        `INSERT INTO user_preferences (user_id, preferences, updated_at)
         VALUES (?, ?, CURRENT_TIMESTAMP)
         ON CONFLICT(user_id) DO UPDATE SET preferences = excluded.preferences, updated_at = CURRENT_TIMESTAMP`,
        userID, preferences)
    return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./internal/server/store/ -run TestPreferences_CRUD -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/store/preferences.go internal/server/store/preferences_test.go
git commit -m "feat: add user preferences store with Get and Upsert"
```

---

## Chunk 2: Handler Layer + Executor

### Task 5: Update Job Step Handlers

**Files:**
- Modify: `internal/server/handler/jobs.go:210-220` (addStepRequest)
- Modify: `internal/server/handler/jobs.go:256-266` (updateStepRequest)
- Modify: `internal/server/handler/jobs.go:223-253` (AddStep handler)
- Modify: `internal/server/handler/jobs.go:269-308` (UpdateStep handler)

- [ ] **Step 1: Write failing test for new step fields in handler**

Test that POST `/jobs/{jobId}/steps` with `skip_permissions`, `model`, `delay_seconds` persists them correctly. Also test validation: model must be empty or one of `opus`, `sonnet`, `haiku`; delay must be 0-86400.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/server/handler/ -run TestJobHandler_AddStep_NewFields -v`
Expected: FAIL

- [ ] **Step 3: Add new fields to request structs**

In `internal/server/handler/jobs.go`:

```go
type addStepRequest struct {
    Name            string `json:"name"`
    Prompt          string `json:"prompt"`
    MachineID       string `json:"machine_id"`
    WorkingDir      string `json:"working_dir"`
    Command         string `json:"command"`
    Args            string `json:"args"`
    TimeoutSeconds  int    `json:"timeout_seconds"`
    SortOrder       int    `json:"sort_order"`
    OnFailure       string `json:"on_failure"`
    SkipPermissions *int   `json:"skip_permissions"`
    Model           string `json:"model"`
    DelaySeconds    int    `json:"delay_seconds"`
}

type updateStepRequest struct {
    Name            string `json:"name"`
    Prompt          string `json:"prompt"`
    MachineID       string `json:"machine_id"`
    WorkingDir      string `json:"working_dir"`
    Command         string `json:"command"`
    Args            string `json:"args"`
    TimeoutSeconds  int    `json:"timeout_seconds"`
    SortOrder       int    `json:"sort_order"`
    OnFailure       string `json:"on_failure"`
    SkipPermissions *int   `json:"skip_permissions"`
    Model           string `json:"model"`
    DelaySeconds    int    `json:"delay_seconds"`
}
```

- [ ] **Step 4: Add validation in AddStep and UpdateStep handlers**

After decoding the request body, add validation:

```go
// Model validation
validModels := map[string]bool{"": true, "opus": true, "sonnet": true, "haiku": true}
if !validModels[req.Model] {
    httputil.Error(w, http.StatusBadRequest, "model must be one of: opus, sonnet, haiku")
    return
}

// Delay validation
if req.DelaySeconds < 0 || req.DelaySeconds > 86400 {
    httputil.Error(w, http.StatusBadRequest, "delay_seconds must be between 0 and 86400")
    return
}
```

- [ ] **Step 5: Pass new fields through to CreateStepParams/UpdateStepParams**

In the handler, when building the store params, include:

```go
store.CreateStepParams{
    // ...existing fields...
    SkipPermissions: req.SkipPermissions,
    Model:           req.Model,
    DelaySeconds:    req.DelaySeconds,
}
```

- [ ] **Step 6: Add backend default step name**

In the `AddStep` handler, when `req.Name` is empty, query the current step count for the job and default to `Step {count+1}`:

```go
if req.Name == "" {
    steps, err := h.store.GetStepsForJob(ctx, jobID)
    if err == nil {
        req.Name = fmt.Sprintf("Step %d", len(steps)+1)
    } else {
        req.Name = "Step 1"
    }
}
```

- [ ] **Step 7: Run test to verify it passes**

Run: `go test -race ./internal/server/handler/ -run TestJobHandler_AddStep_NewFields -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/server/handler/jobs.go
git commit -m "feat: add skip_permissions, model, delay_seconds to step handler with validation"
```

---

### Task 6: Preferences Handler

**Files:**
- Create: `internal/server/handler/preferences.go`
- Create: `internal/server/handler/preferences_test.go`
- Modify: `internal/server/api/router.go` (add routes)

- [ ] **Step 1: Write failing test for preferences endpoints**

Test GET, PUT, PATCH for `/api/v1/users/me/preferences`. Test validation: invalid model rejects, negative timeout rejects.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/server/handler/ -run TestPreferencesHandler -v`
Expected: FAIL

- [ ] **Step 3: Implement PreferencesHandler**

```go
// preferences.go
package handler

import (
    "encoding/json"
    "net/http"

    "github.com/kodrunhq/claude-plane/internal/server/auth"
    "github.com/kodrunhq/claude-plane/internal/server/httputil"
    "github.com/kodrunhq/claude-plane/internal/server/store"
)

type PreferencesHandler struct {
    store     *store.Store
    getClaims func(r *http.Request) *auth.Claims
}

func NewPreferencesHandler(s *store.Store, getClaims func(r *http.Request) *auth.Claims) *PreferencesHandler {
    return &PreferencesHandler{store: s, getClaims: getClaims}
}

// Preferences JSON structure for validation
type PreferencesPayload struct {
    SkipPermissions       *bool                       `json:"skip_permissions,omitempty"`
    DefaultSessionTimeout *int                        `json:"default_session_timeout,omitempty"`
    DefaultStepTimeout    *int                        `json:"default_step_timeout,omitempty"`
    DefaultStepDelay      *int                        `json:"default_step_delay,omitempty"`
    DefaultEnvVars        map[string]string            `json:"default_env_vars,omitempty"`
    Notifications         *NotificationPrefs           `json:"notifications,omitempty"`
    UI                    *UIPrefs                     `json:"ui,omitempty"`
    MachineOverrides      map[string]MachineOverride   `json:"machine_overrides,omitempty"`
}

type NotificationPrefs struct {
    Events []string `json:"events"`
}

type UIPrefs struct {
    Theme              string   `json:"theme"`
    TerminalFontSize   int      `json:"terminal_font_size"`
    AutoAttachSession  bool     `json:"auto_attach_session"`
    CommandCenterCards []string `json:"command_center_cards"`
}

type MachineOverride struct {
    WorkingDir            string            `json:"working_dir"`
    Model                 string            `json:"model"`
    EnvVars               map[string]string `json:"env_vars"`
    MaxConcurrentSessions int               `json:"max_concurrent_sessions"`
}

func (h *PreferencesHandler) Get(w http.ResponseWriter, r *http.Request) {
    claims := h.getClaims(r)
    prefs, err := h.store.GetUserPreferences(r.Context(), claims.UserID)
    if err != nil {
        httputil.Error(w, http.StatusInternalServerError, "failed to get preferences")
        return
    }
    httputil.JSON(w, http.StatusOK, json.RawMessage(prefs.Preferences))
}

func (h *PreferencesHandler) Put(w http.ResponseWriter, r *http.Request) {
    claims := h.getClaims(r)
    var payload PreferencesPayload
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        httputil.Error(w, http.StatusBadRequest, "invalid JSON")
        return
    }
    if err := validatePreferences(payload); err != nil {
        httputil.Error(w, http.StatusBadRequest, err.Error())
        return
    }
    data, _ := json.Marshal(payload)
    if err := h.store.UpsertUserPreferences(r.Context(), claims.UserID, string(data)); err != nil {
        httputil.Error(w, http.StatusInternalServerError, "failed to save preferences")
        return
    }
    httputil.JSON(w, http.StatusOK, payload)
}

func (h *PreferencesHandler) Patch(w http.ResponseWriter, r *http.Request) {
    claims := h.getClaims(r)
    // Get existing
    existing, err := h.store.GetUserPreferences(r.Context(), claims.UserID)
    if err != nil {
        httputil.Error(w, http.StatusInternalServerError, "failed to get preferences")
        return
    }
    // Parse existing into map for shallow merge
    var base map[string]json.RawMessage
    json.Unmarshal([]byte(existing.Preferences), &base)
    if base == nil {
        base = make(map[string]json.RawMessage)
    }
    // Parse patch
    var patch map[string]json.RawMessage
    if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
        httputil.Error(w, http.StatusBadRequest, "invalid JSON")
        return
    }
    // Shallow merge: top-level keys from patch replace base
    for k, v := range patch {
        base[k] = v
    }
    // Validate merged result
    merged, _ := json.Marshal(base)
    var payload PreferencesPayload
    if err := json.Unmarshal(merged, &payload); err != nil {
        httputil.Error(w, http.StatusBadRequest, "invalid preferences structure")
        return
    }
    if err := validatePreferences(payload); err != nil {
        httputil.Error(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := h.store.UpsertUserPreferences(r.Context(), claims.UserID, string(merged)); err != nil {
        httputil.Error(w, http.StatusInternalServerError, "failed to save preferences")
        return
    }
    httputil.JSON(w, http.StatusOK, payload)
}

func validatePreferences(p PreferencesPayload) error {
    validModels := map[string]bool{"": true, "opus": true, "sonnet": true, "haiku": true}
    for machineID, mo := range p.MachineOverrides {
        if !validModels[mo.Model] {
            return fmt.Errorf("machine %s: model must be one of: opus, sonnet, haiku", machineID)
        }
        if mo.MaxConcurrentSessions < 0 {
            return fmt.Errorf("machine %s: max_concurrent_sessions must be non-negative", machineID)
        }
    }
    if p.DefaultSessionTimeout != nil && *p.DefaultSessionTimeout < 0 {
        return fmt.Errorf("default_session_timeout must be non-negative")
    }
    if p.DefaultStepTimeout != nil && *p.DefaultStepTimeout < 0 {
        return fmt.Errorf("default_step_timeout must be non-negative")
    }
    if p.DefaultStepDelay != nil && (*p.DefaultStepDelay < 0 || *p.DefaultStepDelay > 86400) {
        return fmt.Errorf("default_step_delay must be between 0 and 86400")
    }
    validThemes := map[string]bool{"": true, "light": true, "dark": true, "system": true}
    if p.UI != nil && !validThemes[p.UI.Theme] {
        return fmt.Errorf("theme must be one of: light, dark, system")
    }
    return nil
}
```

- [ ] **Step 4: Register routes in router**

In `internal/server/api/router.go`, add within the authenticated group:

```go
r.Route("/api/v1/users/me/preferences", func(r chi.Router) {
    r.Get("/", prefsHandler.Get)
    r.Put("/", prefsHandler.Put)
    r.Patch("/", prefsHandler.Patch)
})
```

Wire `PreferencesHandler` in `cmd/server/main.go` with the store and claims getter.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -race ./internal/server/handler/ -run TestPreferencesHandler -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/handler/preferences.go internal/server/handler/preferences_test.go internal/server/api/router.go cmd/server/main.go
git commit -m "feat: add preferences REST API (GET/PUT/PATCH /users/me/preferences)"
```

---

### Task 7: Executor Preference Resolution

**Files:**
- Modify: `internal/server/executor/session_executor.go:88-143` (ExecuteStep)
- Create: `internal/server/executor/preference_resolver.go`
- Create: `internal/server/executor/preference_resolver_test.go`

- [ ] **Step 1: Write failing test for preference resolution**

```go
func TestResolveSkipPermissions(t *testing.T) {
    tests := []struct {
        name        string
        stepVal     *int
        prefVal     *bool
        wantInject  bool
    }{
        {"step=1 overrides pref=false", intPtr(1), boolPtr(false), true},
        {"step=0 overrides pref=true", intPtr(0), boolPtr(true), false},
        {"step=nil uses pref=true", nil, boolPtr(true), true},
        {"step=nil uses pref=false", nil, boolPtr(false), false},
        {"both nil defaults true", nil, nil, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := resolveSkipPermissions(tt.stepVal, tt.prefVal)
            assert.Equal(t, tt.wantInject, got)
        })
    }
}

func TestResolveModel(t *testing.T) {
    tests := []struct {
        name      string
        stepModel string
        prefModel string
        want      string
    }{
        {"step wins", "opus", "sonnet", "opus"},
        {"pref fallback", "", "haiku", "haiku"},
        {"both empty", "", "", ""},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := resolveModel(tt.stepModel, tt.prefModel)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/server/executor/ -run TestResolve -v`
Expected: FAIL

- [ ] **Step 3: Implement preference resolver**

```go
// preference_resolver.go
package executor

import (
    "encoding/json"
)

type resolvedPreferences struct {
    SkipPermissions bool
    Model           string
}

// PreferencesPayload mirrors the handler struct for parsing
type preferencesPayload struct {
    SkipPermissions *bool                     `json:"skip_permissions"`
    MachineOverrides map[string]machinePrefs  `json:"machine_overrides"`
}

type machinePrefs struct {
    Model string `json:"model"`
}

func parsePreferences(raw string) preferencesPayload {
    var p preferencesPayload
    json.Unmarshal([]byte(raw), &p)
    return p
}

func resolveSkipPermissions(stepVal *int, prefVal *bool) bool {
    if stepVal != nil {
        return *stepVal == 1
    }
    if prefVal != nil {
        return *prefVal
    }
    return true // default: skip permissions for jobs
}

func resolveModel(stepModel, prefModel string) string {
    if stepModel != "" {
        return stepModel
    }
    return prefModel
}
```

- [ ] **Step 4: Integrate into ExecuteStep**

In `session_executor.go` `ExecuteStep` function, after parsing args (line 109) and before building `CreateSessionCmd` (line 129):

```go
// Load user preferences (cached per run via context or passed in)
userPrefs := e.getPreferencesForRun(ctx, runStep)
parsed := parsePreferences(userPrefs.Preferences)

// Resolve machine-specific prefs
var machineModel string
if mo, ok := parsed.MachineOverrides[runStep.MachineIDSnapshot]; ok {
    machineModel = mo.Model
}

// Resolve skip_permissions
if resolveSkipPermissions(runStep.SkipPermissionsSnapshot, parsed.SkipPermissions) {
    args = append([]string{"--dangerously-skip-permissions"}, args...)
}

// Resolve model
model := resolveModel(runStep.ModelSnapshot, machineModel)
if model != "" {
    args = append(args, "--model", model)
}
```

The `getPreferencesForRun` method loads preferences once per run and caches them. It needs the user ID from the run record — update the `Run` struct or pass user context through.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -race ./internal/server/executor/ -run TestResolve -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/executor/preference_resolver.go internal/server/executor/preference_resolver_test.go internal/server/executor/session_executor.go
git commit -m "feat: executor resolves skip_permissions and model from step snapshot + user prefs"
```

---

### Task 8: Step Delay in DAG Runner

**Files:**
- Modify: `internal/server/orchestrator/dag_runner.go:106-129` (Start)
- Modify: `internal/server/orchestrator/dag_runner.go:131-231` (OnStepCompleted)

- [ ] **Step 1: Write failing test for step delay**

Test that a step with `delay_seconds_snapshot > 0` delays before execution. Use a mock executor that records timestamps.

```go
func TestDAGRunner_StepDelay(t *testing.T) {
    // Setup: step with delay_seconds_snapshot = 1
    // Record time when executor.ExecuteStep is called
    // Assert: execution started at least 1s after step became ready
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/server/orchestrator/ -run TestDAGRunner_StepDelay -v`
Expected: FAIL

- [ ] **Step 3: Add delay logic to step launch**

In `dag_runner.go`, extract step launching into a helper function that respects delay:

```go
func (d *DAGRunner) launchStep(ctx context.Context, rs store.RunStep) {
    delay := time.Duration(rs.DelaySecondsSnapshot) * time.Second
    if delay > 0 {
        go func() {
            select {
            case <-time.After(delay):
                d.executor.ExecuteStep(ctx, rs, d.OnStepCompleted)
            case <-ctx.Done():
                // Run cancelled during delay — mark step as cancelled
                d.handleStepCancelled(rs.StepID)
            }
        }()
    } else {
        go d.executor.ExecuteStep(ctx, rs, d.OnStepCompleted)
    }
}
```

Replace direct `go d.executor.ExecuteStep(...)` calls in `Start()` and `OnStepCompleted()` with `d.launchStep(ctx, rs)`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./internal/server/orchestrator/ -run TestDAGRunner_StepDelay -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/orchestrator/dag_runner.go internal/server/orchestrator/dag_runner_test.go
git commit -m "feat: support delay_seconds on job steps with context cancellation"
```

---

## Chunk 3: Trigger Filter Evaluation + DAG Tests

### Task 9: Trigger Filter Evaluation

**Files:**
- Modify: `internal/server/event/trigger_subscriber.go:54-95` (Handler)
- Modify: `internal/server/event/trigger_subscriber_test.go`

- [ ] **Step 1: Write failing tests for filter matching**

```go
func TestTriggerSubscriber_FilterMatching(t *testing.T) {
    tests := []struct {
        name       string
        filter     string
        payload    map[string]any
        shouldFire bool
    }{
        {"empty filter matches all", "", map[string]any{"job_id": "j1"}, true},
        {"matching filter fires", `{"job_id":"j1"}`, map[string]any{"job_id": "j1", "status": "completed"}, true},
        {"mismatched value skips", `{"job_id":"j2"}`, map[string]any{"job_id": "j1"}, false},
        {"missing key skips", `{"status":"completed"}`, map[string]any{"job_id": "j1"}, false},
        {"multi-key all must match", `{"job_id":"j1","status":"completed"}`, map[string]any{"job_id": "j1", "status": "completed"}, true},
        {"multi-key partial mismatch skips", `{"job_id":"j1","status":"failed"}`, map[string]any{"job_id": "j1", "status": "completed"}, false},
        {"invalid JSON filter skips", `{bad json`, map[string]any{"job_id": "j1"}, false},
    }
    // For each test case, set up trigger with filter and publish event with payload.
    // Assert: orchestrator.CreateRun called (or not) based on shouldFire.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/server/event/ -run TestTriggerSubscriber_FilterMatching -v`
Expected: FAIL

- [ ] **Step 3: Add matchesFilter function**

IMPORTANT: `Event.Payload` is `map[string]any` (not `map[string]string`). The function must coerce payload values to strings for comparison using `fmt.Sprintf`:

```go
// In trigger_subscriber.go
func matchesFilter(filter string, payload map[string]any) bool {
    if filter == "" {
        return true
    }
    var filterMap map[string]string
    if err := json.Unmarshal([]byte(filter), &filterMap); err != nil {
        return false // invalid filter never matches
    }
    for k, v := range filterMap {
        payloadVal, ok := payload[k]
        if !ok {
            return false
        }
        if fmt.Sprintf("%v", payloadVal) != v {
            return false
        }
    }
    return true
}
```

- [ ] **Step 4: Integrate filter check into Handler**

In the Handler function, after the glob pattern match (line 69) and loop prevention check (line 74), add:

```go
if !matchesFilter(trigger.Filter, e.Payload) {
    continue
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -race ./internal/server/event/ -run TestTriggerSubscriber_FilterMatching -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/event/trigger_subscriber.go internal/server/event/trigger_subscriber_test.go
git commit -m "feat: evaluate trigger filters as flat key-value match against event payload"
```

---

### Task 10: DAG Correctness Tests

**Files:**
- Modify: `internal/server/orchestrator/dag_runner_test.go`

- [ ] **Step 1: Write comprehensive DAG tests**

Add test cases for:

```go
func TestDAGRunner_DiamondDependency(t *testing.T) {
    // A -> B, A -> C, B+C -> D
    // All succeed: D runs after both B and C complete
}

func TestDAGRunner_FailurePropagation_FailRun(t *testing.T) {
    // A -> B -> C, A fails with on_failure=fail_run
    // B and C should be skipped, run should be failed
}

func TestDAGRunner_FailurePropagation_Continue(t *testing.T) {
    // A -> B, A -> C (independent), A fails with on_failure=continue
    // B should be skipped (depends on A), C should still run (independent)
    // Run continues but A's dependents are skipped
}

func TestDAGRunner_SkipCascade(t *testing.T) {
    // A -> B -> C, A fails with on_failure=continue
    // B is skipped, C (depends on B) should also be skipped
}

func TestDAGRunner_DiamondWithFailure(t *testing.T) {
    // A -> B, A -> C, B+C -> D
    // B fails (on_failure=continue), C succeeds
    // D should be skipped (one dependency failed)
}
```

- [ ] **Step 2: Run tests**

Run: `go test -race ./internal/server/orchestrator/ -run TestDAGRunner -v`
Expected: ALL PASS (fix any bugs found)

- [ ] **Step 3: Fix any bugs discovered**

If any test fails, fix the DAG runner logic. The most likely issue is skip cascading not propagating correctly through multi-level chains.

- [ ] **Step 4: Commit**

```bash
git add internal/server/orchestrator/dag_runner_test.go internal/server/orchestrator/dag_runner.go
git commit -m "test: comprehensive DAG runner tests for diamond deps, failure propagation, skip cascade"
```

---

## Chunk 4: Bug Fix + Session Search Proto

### Task 11: Fix Double Step Bug

**Files:**
- Modify: `web/src/views/JobEditor.tsx:88-121` (ensureJobCreated + handleAddStep)

- [ ] **Step 1: Read the current JobEditor code**

Read `web/src/views/JobEditor.tsx` to understand exact current state.

- [ ] **Step 2: Refactor ensureJobCreated to not navigate**

Remove `navigate()` from `ensureJobCreated()`. It should only create the job and return the ID:

```typescript
const ensureJobCreated = async (): Promise<string | null> => {
  if (effectiveJobId) return effectiveJobId;
  if (!jobName.trim()) {
    // show validation toast/message
    return null;
  }
  const job = await createJob.mutateAsync({ name: jobName.trim(), description: jobDescription });
  setJobId(job.job_id);
  return job.job_id;
};
```

- [ ] **Step 3: Refactor handleAddStep to navigate after step creation**

```typescript
const handleAddStep = async () => {
  const jid = await ensureJobCreated();
  if (!jid) return;

  const step = await addStep.mutateAsync({
    jobId: jid,
    params: { name: '', machine_id: '' }, // backend defaults name to "Step N"
  });

  selectStep(step.step_id);

  // Navigate only after both operations succeed
  if (isNew) {
    navigate(`/jobs/${jid}`, { replace: true });
  }
};
```

- [ ] **Step 4: Verify the fix**

Run: `cd web && npx vitest run src/views/JobEditor`
Expected: PASS (or write a test if none exists)

- [ ] **Step 5: Commit**

```bash
git add web/src/views/JobEditor.tsx
git commit -m "fix: prevent double step creation by deferring navigation until after step is created"
```

---

### Task 12: Proto Changes for Session Search

**Files:**
- Modify: `proto/claudeplane/v1/agent.proto`

- [ ] **Step 1: Add SearchScrollbackCmd to ServerCommand oneof**

After `RequestScrollbackCmd request_scrollback = 7;` (field 7), add:

```protobuf
SearchScrollbackCmd search_scrollback = 8;
```

- [ ] **Step 2: Add SearchScrollbackResultEvent to AgentEvent oneof**

After `SessionExitEvent session_exit = 5;` (field 5), add:

```protobuf
SearchScrollbackResultEvent search_scrollback_result = 6;
```

- [ ] **Step 3: Add the new message types**

```protobuf
message SearchScrollbackCmd {
  string request_id = 1;
  string query = 2;
  int32 max_results = 3;
  repeated string session_ids = 4;
}

message SearchScrollbackResultEvent {
  string request_id = 1;
  repeated ScrollbackMatch matches = 2;
}

message ScrollbackMatch {
  string session_id = 1;
  string line = 2;
  int64 timestamp_ms = 3;
  string context_before = 4;
  string context_after = 5;
}
```

- [ ] **Step 4: Regenerate proto stubs**

Run: `buf generate`
Expected: Clean generation, new Go files updated in `internal/shared/proto/`

- [ ] **Step 5: Commit**

```bash
git add proto/ internal/shared/proto/
git commit -m "feat: add SearchScrollbackCmd and SearchScrollbackResultEvent to proto"
```

---

### Task 13: Agent-Side Scrollback Search

**Files:**
- Create: `internal/agent/search.go`
- Create: `internal/agent/search_test.go`
- Modify: `internal/agent/session_manager.go` (handle SearchScrollbackCmd)

- [ ] **Step 1: Write failing test for scrollback search**

```go
func TestSearchScrollback(t *testing.T) {
    // Create a temp .cast file with known content
    castContent := `{"version":2,"width":80,"height":24}
[0.1,"o","Hello world\r\n"]
[0.5,"o","Error: file not found\r\n"]
[1.0,"o","Done processing\r\n"]`

    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "sess1.cast"), []byte(castContent), 0644)

    matches, err := searchScrollbackFiles(dir, "Error", nil, 10)
    require.NoError(t, err)
    require.Len(t, matches, 1)
    assert.Equal(t, "sess1", matches[0].SessionID)
    assert.Contains(t, matches[0].Line, "Error: file not found")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/agent/ -run TestSearchScrollback -v`
Expected: FAIL

- [ ] **Step 3: Implement search function**

```go
// search.go
package agent

import (
    "bufio"
    "encoding/json"
    "os"
    "path/filepath"
    "regexp"
    "strings"

    pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// ANSI escape code regex for stripping terminal formatting
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
    return ansiRegex.ReplaceAllString(s, "")
}

type castEntry struct {
    Timestamp float64
    EventType string
    Data      string
}

func parseCastFile(path string) ([]castEntry, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    var entries []castEntry
    scanner := bufio.NewScanner(f)
    scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer
    for scanner.Scan() {
        line := scanner.Text()
        // Skip header (JSON object, not array)
        if strings.HasPrefix(line, "{") {
            continue
        }
        var raw []json.RawMessage
        if err := json.Unmarshal([]byte(line), &raw); err != nil || len(raw) < 3 {
            continue
        }
        var ts float64
        var eventType, data string
        json.Unmarshal(raw[0], &ts)
        json.Unmarshal(raw[1], &eventType)
        json.Unmarshal(raw[2], &data)
        if eventType == "o" {
            entries = append(entries, castEntry{Timestamp: ts, Data: stripANSI(data)})
        }
    }
    return entries, scanner.Err()
}

func searchScrollbackFiles(dataDir, query string, sessionIDs []string, maxResults int32) ([]*pb.ScrollbackMatch, error) {
    pattern := filepath.Join(dataDir, "*.cast")
    files, err := filepath.Glob(pattern)
    if err != nil {
        return nil, err
    }

    sessionFilter := make(map[string]bool)
    for _, id := range sessionIDs {
        sessionFilter[id] = true
    }

    var matches []*pb.ScrollbackMatch
    queryLower := strings.ToLower(query)

    for _, file := range files {
        sessionID := strings.TrimSuffix(filepath.Base(file), ".cast")
        if len(sessionFilter) > 0 && !sessionFilter[sessionID] {
            continue
        }

        entries, err := parseCastFile(file)
        if err != nil {
            continue
        }

        // Concatenate output into lines for searching
        var fullText strings.Builder
        for _, e := range entries {
            fullText.WriteString(e.Data)
        }
        lines := strings.Split(fullText.String(), "\n")

        for i, line := range lines {
            if strings.Contains(strings.ToLower(line), queryLower) {
                match := &pb.ScrollbackMatch{
                    SessionId: sessionID,
                    Line:      strings.TrimSpace(line),
                    TimestampMs: int64(entries[min(i, len(entries)-1)].Timestamp * 1000),
                }
                // Context: 2 lines before/after
                if i > 0 {
                    match.ContextBefore = strings.TrimSpace(lines[i-1])
                }
                if i < len(lines)-1 {
                    match.ContextAfter = strings.TrimSpace(lines[i+1])
                }
                matches = append(matches, match)
                if int32(len(matches)) >= maxResults {
                    return matches, nil
                }
            }
        }
    }
    return matches, nil
}
```

- [ ] **Step 4: Handle SearchScrollbackCmd in session_manager**

In `session_manager.go`, add a case in the command handler switch for `SearchScrollbackCmd`:

```go
case *pb.ServerCommand_SearchScrollback:
    cmd := c.SearchScrollback
    matches, err := searchScrollbackFiles(sm.dataDir, cmd.Query, cmd.SessionIds, cmd.MaxResults)
    if err != nil {
        sm.logger.Error("scrollback search failed", "error", err)
        matches = nil
    }
    // Send result event back through the stream
    sm.sendEvent(&pb.AgentEvent{
        Event: &pb.AgentEvent_SearchScrollbackResult{
            SearchScrollbackResult: &pb.SearchScrollbackResultEvent{
                RequestId: cmd.RequestId,
                Matches:   matches,
            },
        },
    })
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -race ./internal/agent/ -run TestSearchScrollback -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/agent/search.go internal/agent/search_test.go internal/agent/session_manager.go
git commit -m "feat: agent-side scrollback search with ANSI stripping and cast file parsing"
```

---

### Task 14: Server-Side Search Aggregation

**Files:**
- Create: `internal/server/handler/search.go`
- Create: `internal/server/handler/search_test.go`
- Modify: `internal/server/api/router.go` (add route)
- Modify: `internal/server/connmgr/connmgr.go` (add search fan-out method)

- [ ] **Step 1: Write failing test for search handler**

Test that GET `/api/v1/search/sessions?q=error&limit=10` fans out to connected agents and returns aggregated results.

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Add SearchScrollback method to connection manager**

In `connmgr.go`, add a method that sends `SearchScrollbackCmd` to all connected agents and collects responses with a 10-second timeout:

```go
func (cm *ConnectionManager) SearchScrollback(ctx context.Context, query string, sessionIDs []string, maxResults int32) ([]*pb.ScrollbackMatch, error) {
    requestID := uuid.NewString()
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    // Send search command to all connected agents
    agents := cm.ListConnected()
    resultCh := make(chan []*pb.ScrollbackMatch, len(agents))

    for _, agent := range agents {
        go func(a *AgentConnection) {
            a.SendCommand(&pb.ServerCommand{
                Command: &pb.ServerCommand_SearchScrollback{
                    SearchScrollback: &pb.SearchScrollbackCmd{
                        RequestId:  requestID,
                        Query:      query,
                        MaxResults: maxResults,
                        SessionIds: sessionIDs,
                    },
                },
            })
            // Wait for response event correlated by requestID
            matches := a.WaitForSearchResult(ctx, requestID)
            resultCh <- matches
        }(agent)
    }

    // Collect results with timeout
    var all []*pb.ScrollbackMatch
    for range agents {
        select {
        case matches := <-resultCh:
            all = append(all, matches...)
        case <-ctx.Done():
            break
        }
    }

    // Sort by timestamp descending
    sort.Slice(all, func(i, j int) bool {
        return all[i].TimestampMs > all[j].TimestampMs
    })

    // Apply limit
    if int32(len(all)) > maxResults {
        all = all[:maxResults]
    }
    return all, nil
}
```

**Important implementation detail: `WaitForSearchResult`** requires a request/response correlation mechanism on the `AgentConnection`:

1. Add a `pendingSearches sync.Map` (keyed by `request_id` → `chan []*pb.ScrollbackMatch`) to `AgentConnection`.
2. `WaitForSearchResult(ctx, requestID)`: creates a channel, stores it in `pendingSearches`, blocks on select (channel or ctx.Done), cleans up on return.
3. In the existing event handler loop (where `AgentEvent` messages are received from the stream), add a case for `SearchScrollbackResultEvent`: look up the channel in `pendingSearches` by `request_id`, send matches, close channel.

This is ~30 lines of concurrent code. Write tests for the correlation (happy path + timeout).

- [ ] **Step 4: Implement search handler**

```go
// search.go
package handler

type SearchHandler struct {
    connMgr *connmgr.ConnectionManager
    store   *store.Store
}

func (h *SearchHandler) SearchSessions(w http.ResponseWriter, r *http.Request) {
    query := r.URL.Query().Get("q")
    if query == "" {
        httputil.Error(w, http.StatusBadRequest, "q parameter required")
        return
    }
    limitStr := r.URL.Query().Get("limit")
    limit := int32(50)
    if limitStr != "" {
        if l, err := strconv.ParseInt(limitStr, 10, 32); err == nil && l > 0 && l <= 200 {
            limit = int32(l)
        }
    }

    matches, err := h.connMgr.SearchScrollback(r.Context(), query, nil, limit)
    if err != nil {
        httputil.Error(w, http.StatusInternalServerError, "search failed")
        return
    }

    // Enrich with session metadata
    type SearchResult struct {
        SessionID     string `json:"session_id"`
        MachineID     string `json:"machine_id"`
        Line          string `json:"line"`
        ContextBefore string `json:"context_before"`
        ContextAfter  string `json:"context_after"`
        TimestampMs   int64  `json:"timestamp_ms"`
        SessionStatus string `json:"session_status,omitempty"`
    }
    results := make([]SearchResult, 0, len(matches))
    for _, m := range matches {
        sr := SearchResult{
            SessionID:     m.SessionId,
            Line:          m.Line,
            ContextBefore: m.ContextBefore,
            ContextAfter:  m.ContextAfter,
            TimestampMs:   m.TimestampMs,
        }
        // Enrich with session metadata if available
        if sess, err := h.store.GetSession(r.Context(), m.SessionId); err == nil {
            sr.MachineID = sess.MachineID
            sr.SessionStatus = sess.Status
        }
        results = append(results, sr)
    }
    httputil.JSON(w, http.StatusOK, results)
}
```

- [ ] **Step 5: Register route**

In router.go: `r.Get("/api/v1/search/sessions", searchHandler.SearchSessions)`

- [ ] **Step 6: Run tests**

Run: `go test -race ./internal/server/handler/ -run TestSearchHandler -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/server/handler/search.go internal/server/handler/search_test.go internal/server/connmgr/connmgr.go internal/server/api/router.go cmd/server/main.go
git commit -m "feat: session log search endpoint with agent fan-out and result aggregation"
```

---

## Chunk 5: Frontend — Step Editor, Session Modal, Types

### Task 15: Update Frontend Types

**Files:**
- Modify: `web/src/types/job.ts:15-27` (Step interface)
- Modify: `web/src/types/job.ts` (CreateStepParams, UpdateStepParams)

- [ ] **Step 1: Add new fields to Step interface**

```typescript
export interface Step {
  step_id: string;
  job_id: string;
  name: string;
  prompt: string;
  machine_id: string;
  working_dir: string;
  command: string;
  args: string;
  timeout_seconds?: number;
  sort_order?: number;
  on_failure?: string;
  skip_permissions?: number | null;  // null = inherit, 1 = on, 0 = off
  model?: string;                    // '' | 'opus' | 'sonnet' | 'haiku'
  delay_seconds?: number;
}
```

- [ ] **Step 2: Update CreateStepParams and UpdateStepParams**

Add `skip_permissions`, `model`, `delay_seconds` to both param interfaces.

- [ ] **Step 3: Run typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: PASS (or note errors to fix in subsequent tasks)

- [ ] **Step 4: Commit**

```bash
git add web/src/types/job.ts
git commit -m "feat: add skip_permissions, model, delay_seconds to frontend Step type"
```

---

### Task 16: Update StepEditor Component

**Files:**
- Modify: `web/src/components/jobs/StepEditor.tsx:16-26` (getFormParams)
- Modify: `web/src/components/jobs/StepEditor.tsx:268-381` (form fields)

- [ ] **Step 1: Add model dropdown to form**

After the machine_id select (around line 327), add:

```tsx
<div>
  <label className="block text-sm font-medium text-zinc-300 mb-1">Model</label>
  <select
    name="model"
    defaultValue={step?.model ?? ''}
    className="w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm text-zinc-200"
  >
    <option value="">Default</option>
    <option value="opus">Opus</option>
    <option value="sonnet">Sonnet</option>
    <option value="haiku">Haiku</option>
  </select>
</div>
```

- [ ] **Step 2: Add skip_permissions toggle**

```tsx
<div>
  <label className="block text-sm font-medium text-zinc-300 mb-1">Skip Permission Prompts</label>
  <select
    name="skip_permissions"
    defaultValue={step?.skip_permissions === null || step?.skip_permissions === undefined ? '' : String(step.skip_permissions)}
    className="w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm text-zinc-200"
  >
    <option value="">Default (from settings)</option>
    <option value="1">On</option>
    <option value="0">Off</option>
  </select>
</div>
```

- [ ] **Step 3: Add delay_seconds input**

```tsx
<div>
  <label className="block text-sm font-medium text-zinc-300 mb-1">Delay Before Start (seconds)</label>
  <input
    type="number"
    name="delay_seconds"
    min={0}
    max={86400}
    defaultValue={step?.delay_seconds ?? 0}
    className="w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm text-zinc-200"
  />
  <p className="text-xs text-zinc-500 mt-1">Wait before executing this step (0 = immediate)</p>
</div>
```

- [ ] **Step 4: Update getFormParams to extract new fields**

```typescript
function getFormParams(form: FormData): UpdateStepParams {
  const skipPermsVal = form.get('skip_permissions') as string;
  return {
    name: form.get('name') as string,
    prompt: form.get('prompt') as string,
    machine_id: form.get('machine_id') as string,
    working_dir: form.get('working_dir') as string,
    command: (form.get('command') as string) || 'claude',
    args: form.get('args') as string,
    model: form.get('model') as string,
    skip_permissions: skipPermsVal === '' ? null : Number(skipPermsVal),
    delay_seconds: Number(form.get('delay_seconds')) || 0,
  };
}
```

- [ ] **Step 5: Update isDirty to include new fields**

Add model, skip_permissions, delay_seconds to the dirty check function.

- [ ] **Step 6: Run typecheck and tests**

Run: `cd web && npx tsc --noEmit && npx vitest run`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add web/src/components/jobs/StepEditor.tsx
git commit -m "feat: add model dropdown, skip_permissions toggle, delay input to StepEditor"
```

---

### Task 17: Update NewSessionModal

**Files:**
- Modify: `web/src/components/sessions/NewSessionModal.tsx:83-90` (create request)
- Modify: `web/src/components/sessions/NewSessionModal.tsx:108-183` (form fields)

- [ ] **Step 1: Add model dropdown and skip_permissions toggle to form**

After the Command field (around line 170), add model and skip_permissions fields using the same pattern as StepEditor.

- [ ] **Step 2: Update create session request to include new fields**

```typescript
const req = {
  machine_id: machineId,
  terminal_size: { cols, rows },
  command: command || undefined,
  working_dir: workingDir || undefined,
  template_id: selectedTemplate?.template_id,
  variables,
  model: model || undefined,
  skip_permissions: skipPermissions,
};
```

- [ ] **Step 3: Update session API types**

In `web/src/types/session.ts`, add to the create session request interface:

```typescript
model?: string;           // '' | 'opus' | 'sonnet' | 'haiku'
skip_permissions?: boolean;
```

In `web/src/api/sessions.ts`, ensure the create session function passes these fields through in the request body.

- [ ] **Step 4: Run typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sessions/NewSessionModal.tsx web/src/types/session.ts web/src/api/sessions.ts
git commit -m "feat: add model dropdown and skip_permissions toggle to NewSessionModal"
```

---

### Task 18: Update DAGCanvas for Delay Badge

**Files:**
- Modify: `web/src/components/dag/DAGCanvas.tsx:85-99` (node data)
- Modify: `web/src/components/dag/StepNode.tsx` (or wherever custom nodes are rendered)

- [ ] **Step 1: Pass delay_seconds to node data**

In DAGCanvas, when building nodes, include `delaySeconds` in the data:

```typescript
data: {
  label: step.name,
  status: runStepStatus,
  machineId: step.machine_id,
  selected: step.step_id === selectedStepId,
  delaySeconds: step.delay_seconds ?? 0,
}
```

- [ ] **Step 2: Render delay badge in node**

In the custom node component, if `delaySeconds > 0`, show a small badge:

```tsx
{data.delaySeconds > 0 && (
  <div className="absolute -top-2 -right-2 bg-amber-600 text-white text-[10px] px-1.5 py-0.5 rounded-full flex items-center gap-0.5">
    <Timer className="w-2.5 h-2.5" />
    {data.delaySeconds}s
  </div>
)}
```

- [ ] **Step 3: Run typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/components/dag/
git commit -m "feat: show delay badge on DAG step nodes"
```

---

## Chunk 6: Frontend — Settings Page

### Task 19: Create Preferences API and Hook

**Files:**
- Create: `web/src/api/preferences.ts`
- Create: `web/src/hooks/usePreferences.ts`
- Create: `web/src/types/preferences.ts`

- [ ] **Step 1: Define preferences types**

```typescript
// types/preferences.ts
export interface MachineOverride {
  working_dir: string;
  model: string;
  env_vars: Record<string, string>;
  max_concurrent_sessions: number;
}

export interface NotificationPrefs {
  events: string[];
}

export interface UIPrefs {
  theme: 'light' | 'dark' | 'system';
  terminal_font_size: number;
  auto_attach_session: boolean;
  command_center_cards: string[];
}

export interface UserPreferences {
  skip_permissions?: boolean;
  default_session_timeout?: number;
  default_step_timeout?: number;
  default_step_delay?: number;
  default_env_vars?: Record<string, string>;
  notifications?: NotificationPrefs;
  ui?: UIPrefs;
  machine_overrides?: Record<string, MachineOverride>;
}
```

- [ ] **Step 2: Create API functions**

```typescript
// api/preferences.ts
import { request } from './client';
import type { UserPreferences } from '../types/preferences';

export const preferencesApi = {
  get: () => request<UserPreferences>('/users/me/preferences'),
  put: (prefs: UserPreferences) => request<UserPreferences>('/users/me/preferences', { method: 'PUT', body: prefs }),
  patch: (partial: Partial<UserPreferences>) => request<UserPreferences>('/users/me/preferences', { method: 'PATCH', body: partial }),
};
```

- [ ] **Step 3: Create TanStack Query hook**

```typescript
// hooks/usePreferences.ts
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { preferencesApi } from '../api/preferences';
import type { UserPreferences } from '../types/preferences';

export function usePreferences() {
  return useQuery({
    queryKey: ['preferences'],
    queryFn: preferencesApi.get,
  });
}

export function useUpdatePreferences() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (prefs: UserPreferences) => preferencesApi.put(prefs),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['preferences'] }),
  });
}

export function usePatchPreferences() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (partial: Partial<UserPreferences>) => preferencesApi.patch(partial),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['preferences'] }),
  });
}
```

- [ ] **Step 4: Run typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/types/preferences.ts web/src/api/preferences.ts web/src/hooks/usePreferences.ts
git commit -m "feat: add preferences types, API client, and TanStack Query hooks"
```

---

### Task 20: Create Settings Page

**Files:**
- Create: `web/src/views/SettingsPage.tsx`
- Modify: `web/src/App.tsx:78-99` (add route)

- [ ] **Step 1: Create the SettingsPage component**

Create `web/src/views/SettingsPage.tsx` with a tabbed layout. Use the same styling patterns as existing pages. Tabs: Session Defaults, Job Defaults, Notifications, UI Preferences, Machines.

Each tab renders a form that reads from `usePreferences()` and saves via `useUpdatePreferences()`.

Key implementation details:
- Session Defaults tab: skip_permissions toggle, default_session_timeout number input, default_env_vars key-value editor
- Job Defaults tab: default_step_timeout, default_step_delay
- Notifications tab: checkboxes for each event type (use KNOWN_EVENT_TYPES)
- UI Preferences tab: theme select (light/dark/system), terminal_font_size number, auto_attach_session checkbox, command_center_cards multi-select
- Machines tab: list machines from `useMachines()`, each expandable with working_dir, model dropdown, env_vars, max_concurrent_sessions

The component should be ~300-400 lines. Use separate sub-components per tab if needed.

- [ ] **Step 2: Add route in App.tsx**

Add after the existing routes:

```tsx
<Route path="/settings" element={<SettingsPage />} />
```

- [ ] **Step 3: Add navigation link**

Add a Settings link in the sidebar/nav component (likely in `web/src/components/layout/Sidebar.tsx` or similar). Use the `Settings` icon from lucide-react.

- [ ] **Step 4: Run typecheck and tests**

Run: `cd web && npx tsc --noEmit && npx vitest run`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/views/SettingsPage.tsx web/src/App.tsx web/src/components/layout/
git commit -m "feat: add Settings page with tabbed preferences UI"
```

---

## Chunk 7: Frontend — Triggers + Search

### Task 21: Improve TriggerBuilder UX

**Files:**
- Modify: `web/src/components/jobs/TriggerBuilder.tsx`

- [ ] **Step 1: Add job chaining helper text**

After the event type selector, add an info section:

```tsx
{resolvedEventType === 'run.completed' && (
  <div className="text-xs text-zinc-400 bg-zinc-800/50 border border-zinc-700 rounded p-2 mt-1">
    <p className="font-medium text-zinc-300 mb-1">Job Chaining</p>
    <p>To trigger this job when a specific job completes, add a filter with the source job's ID.</p>
  </div>
)}
```

- [ ] **Step 2: Add job picker that auto-populates filter**

Add a "Select source job" dropdown that appears when `run.completed` or `run.failed` is selected. When a job is picked, auto-fill the filter with `{"job_id": "<selected_job_id>"}`.

```tsx
const { data: jobs } = useJobs();

{(resolvedEventType === 'run.completed' || resolvedEventType === 'run.failed') && (
  <div>
    <label className="block text-xs font-medium text-zinc-400 mb-1">Source Job (optional shortcut)</label>
    <select
      className="w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm"
      onChange={(e) => {
        if (e.target.value) {
          setForm(prev => ({ ...prev, filter: JSON.stringify({ job_id: e.target.value }) }));
        }
      }}
    >
      <option value="">— Select a job —</option>
      {jobs?.map(j => <option key={j.job_id} value={j.job_id}>{j.name}</option>)}
    </select>
  </div>
)}
```

- [ ] **Step 3: Make TriggerPanel display readable trigger descriptions**

In TriggerPanel's TriggerRow, parse the filter and event type to show a human-readable description:

```tsx
function describeTrigger(trigger: JobTrigger, jobsMap: Record<string, string>): string {
  let desc = `Fires on ${trigger.event_type}`;
  if (trigger.filter) {
    try {
      const filter = JSON.parse(trigger.filter);
      if (filter.job_id && jobsMap[filter.job_id]) {
        desc = `Fires when "${jobsMap[filter.job_id]}" ${trigger.event_type.split('.')[1]}s`;
      }
    } catch { /* use default */ }
  }
  return desc;
}
```

- [ ] **Step 4: Run typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/components/jobs/TriggerBuilder.tsx web/src/components/jobs/TriggerPanel.tsx
git commit -m "feat: improve trigger UX with job chaining helper, job picker, and readable descriptions"
```

---

### Task 22: Create Search Page

**Files:**
- Create: `web/src/api/search.ts`
- Create: `web/src/hooks/useSearch.ts`
- Create: `web/src/views/SearchPage.tsx`
- Modify: `web/src/App.tsx` (add route)

- [ ] **Step 1: Create search API and hook**

```typescript
// api/search.ts
export interface SearchResult {
  session_id: string;
  machine_id: string;
  line: string;
  context_before: string;
  context_after: string;
  timestamp_ms: number;
  session_status?: string;
}

export const searchApi = {
  sessions: (q: string, limit = 50) =>
    request<SearchResult[]>(`/search/sessions?q=${encodeURIComponent(q)}&limit=${limit}`),
};
```

```typescript
// hooks/useSearch.ts
export function useSessionSearch(query: string, limit = 50) {
  return useQuery({
    queryKey: ['search', 'sessions', query, limit],
    queryFn: () => searchApi.sessions(query, limit),
    enabled: query.length >= 2,
  });
}
```

- [ ] **Step 2: Create SearchPage component**

Build `web/src/views/SearchPage.tsx`:
- Search bar with debounced input (300ms)
- Results list with: session ID (clickable → navigates to terminal view), machine name, matched line with query highlighted, context before/after in muted text, relative timestamp
- Loading state, empty state, error state
- Filter sidebar: machine dropdown, date range (optional for v1 — can skip)

- [ ] **Step 3: Add route and nav link**

In `App.tsx`: `<Route path="/search" element={<SearchPage />} />`
Add Search icon link in sidebar.

- [ ] **Step 4: Run typecheck and tests**

Run: `cd web && npx tsc --noEmit && npx vitest run`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/api/search.ts web/src/hooks/useSearch.ts web/src/views/SearchPage.tsx web/src/App.tsx web/src/components/layout/
git commit -m "feat: add session log search page with debounced query and result highlighting"
```

---

## Chunk 8: Backend Session Handler + Integration Tests

### Task 23: Update Session Handler for Model and Skip Permissions

**Files:**
- Modify: `internal/server/handler/sessions.go` (create session request)
- Modify: `internal/server/session/` (session creation flow)

- [ ] **Step 1: Add model and skip_permissions to create session request**

In the session handler, update the create session request struct to accept `model` and `skip_permissions` fields. When creating a session directly (not via job), resolve preferences the same way as the executor:

```go
type createSessionRequest struct {
    MachineID       string            `json:"machine_id"`
    TerminalSize    *terminalSize     `json:"terminal_size"`
    Command         string            `json:"command"`
    WorkingDir      string            `json:"working_dir"`
    TemplateID      string            `json:"template_id"`
    Variables       map[string]string `json:"variables"`
    Model           string            `json:"model"`
    SkipPermissions *bool             `json:"skip_permissions"`
}
```

- [ ] **Step 2: Resolve preferences before creating session**

Load user preferences and apply the same resolution chain:

```go
// In CreateSession handler, after parsing request:
userPrefs, _ := h.store.GetUserPreferences(ctx, claims.UserID)
parsed := parsePreferences(userPrefs.Preferences)

// Resolve model
model := req.Model
if model == "" {
    if mo, ok := parsed.MachineOverrides[req.MachineID]; ok {
        model = mo.Model
    }
}

// Resolve skip_permissions
skipPerms := true // default
if req.SkipPermissions != nil {
    skipPerms = *req.SkipPermissions
} else if parsed.SkipPermissions != nil {
    skipPerms = *parsed.SkipPermissions
}

// Inject into args
args := baseArgs
if skipPerms {
    args = append([]string{"--dangerously-skip-permissions"}, args...)
}
if model != "" {
    args = append(args, "--model", model)
}
```

- [ ] **Step 3: Run tests**

Run: `go test -race ./internal/server/handler/ -run TestSessionHandler -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/server/handler/sessions.go
git commit -m "feat: resolve model and skip_permissions from user prefs when creating sessions"
```

---

### Task 24: Full Backend Integration Test

**Files:**
- Create or modify: `internal/server/handler/integration_test.go` (or appropriate test file)

- [ ] **Step 1: Write end-to-end preference resolution test**

Test the full flow: set user preferences → create job with step (model="", skip_permissions=nil) → trigger run → verify the executor receives args with `--dangerously-skip-permissions` and `--model sonnet` (from preferences).

- [ ] **Step 2: Write trigger filter integration test**

Test: create job A, create job B with trigger on `run.completed` + filter `{"job_id": "<A_id>"}` → complete a run of job A → verify job B was triggered. Also verify job C's run completing does NOT trigger job B.

- [ ] **Step 3: Run all backend tests**

Run: `go test -race ./...`
Expected: ALL PASS

- [ ] **Step 4: Run go vet**

Run: `go vet ./...`
Expected: Clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "test: integration tests for preference resolution and trigger filter evaluation"
```

---

### Task 25: Full Frontend Build Verification

**Files:** None (verification only)

- [ ] **Step 1: Run typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: PASS with zero errors

- [ ] **Step 2: Run linter**

Run: `cd web && npm run lint`
Expected: PASS

- [ ] **Step 3: Run all frontend tests**

Run: `cd web && npx vitest run`
Expected: ALL PASS

- [ ] **Step 4: Run production build**

Run: `cd web && npm run build`
Expected: Build succeeds, output in `internal/server/frontend/dist/`

- [ ] **Step 5: Commit any remaining fixes**

```bash
git add -A
git commit -m "chore: fix remaining lint/type errors from features batch"
```
