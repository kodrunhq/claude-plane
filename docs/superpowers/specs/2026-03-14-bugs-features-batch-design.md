# Bugs & Features Batch — Design Spec

**Date:** 2026-03-14
**Scope:** 7 items — 1 bug fix, 5 features, 1 investigation (templates kept as-is)

---

## 1. Bug Fix: Double Step on Job Creation

### Problem

In `JobEditor.tsx`, clicking "Add Step" before naming the job shows a validation message. After naming, clicking "Add Step" creates 2 steps instead of 1. Root cause: `handleAddStep()` calls `ensureJobCreated()` which navigates to the new job URL mid-flight, causing stale `steps.length` and duplicate query invalidations.

### Solution

- Remove `navigate()` from inside `ensureJobCreated()`. Defer navigation to after step creation completes.
- Stop deriving step name from local `steps.length`. Let the backend assign default names: `Step {current_count + 1}` when no name is provided.
- Single navigation after both job creation and step creation succeed — use the `addStep` mutation's `onSuccess` callback for both `navigate()` and `invalidateQueries()`, following the standard TanStack Query pattern used elsewhere in the frontend.

### Files

- `web/src/views/JobEditor.tsx` — refactor `ensureJobCreated` + `handleAddStep` flow
- `internal/server/handler/jobs.go` — default step name from current count when name is empty

---

## 2. `--dangerously-skip-permissions` Default

### Approach

Executor-level injection. The executor reads the user's preference and injects the flag into CLI args before sending `CreateSessionCmd`. Steps and sessions can explicitly opt out.

### Data Model

- User preferences JSON: `skip_permissions: boolean` (default `true`)
- `steps` table: `ALTER TABLE steps ADD COLUMN skip_permissions INTEGER` (nullable — `NULL` = inherit from user preference, `1` = force on, `0` = force off)
- Session creation request: `skip_permissions *bool` field (same semantics)

### Resolution Priority

1. Step/session-level `skip_permissions` — if non-nil, use it
2. User preferences `skip_permissions` — fallback
3. Default `true` if no preference set

### Execution

In `SessionStepExecutor.ExecuteStep()`, after resolving the flag, prepend `--dangerously-skip-permissions` to args before building `CreateSessionCmd`.

### Frontend

- StepEditor: toggle "Skip permission prompts" (Default / On / Off)
- NewSessionModal: same toggle
- Settings page: global toggle under Session Defaults

---

## 3. Model Selection

### Valid Values

`opus`, `sonnet`, `haiku` — maps to the `--model` CLI flag.

### Data Model

- User preferences JSON: `machine_overrides.<machine_id>.model: string`
- `steps` table: `ALTER TABLE steps ADD COLUMN model TEXT DEFAULT ''`
- Session creation request: `model string` field

### Resolution Priority

1. Step/session-level `model` — if non-empty, use it
2. User's per-machine default model from preferences
3. Empty — don't pass `--model` flag, let CLI use its own default

### Execution

In `SessionStepExecutor.ExecuteStep()`, after resolving the model, append `--model <value>` to args if non-empty.

### Frontend

- StepEditor: model dropdown (Default / Opus / Sonnet / Haiku)
- NewSessionModal: same dropdown
- Settings page: per-machine model dropdown under machine-specific overrides

---

## 4. Step Delay

### Data Model

- `steps` table: `ALTER TABLE steps ADD COLUMN delay_seconds INTEGER DEFAULT 0`
- `Step` struct: `DelaySeconds int`
- REST API: `delay_seconds` field in add/update step requests

### Execution

In `DAGRunner`, when a step becomes ready (dependencies satisfied or root step), apply delay using `time.After` with context cancellation before calling `executor.ExecuteStep()`. Delays are per-step — parallel fan-out steps can have independent delays. Cancelling the run cancels pending delays.

**Validation:** `delay_seconds` must be in range `[0, 86400]` (max 24 hours). Server-side validation in the handler rejects values outside this range.

### Frontend

- StepEditor: numeric input "Delay before start (seconds)"
- DAGCanvas: delay badge on step nodes (e.g., timer icon + "30s")

---

## 5. Trigger Filter Evaluation + DAG Correctness

### Filter Evaluation

In `trigger_subscriber.go`, when a trigger has a non-empty `filter` JSON:

1. Parse filter as a flat `map[string]string`
2. Match against the event's payload: every key in the filter must exist in the payload with an equal string value
3. If any key is missing or mismatched, the trigger does not fire

Example: trigger on `run.completed` with filter `{"job_id": "job-abc-123"}` fires only when that specific job completes.

Design constraint: string equality only, no operators, no nesting. Simple and predictable.

### DAG Step Dependency Verification

The existing `dag_runner.go` tracks in-degree and launches steps when dependencies resolve. Audit and add tests for:

- Diamond dependencies (A -> B, A -> C, B+C -> D)
- Failure propagation through multi-level chains
- Skip cascading: if B is skipped due to A failing, D (which depends on B) must also be skipped
- `on_failure: fail_run` marks the entire run as failed immediately; `on_failure: continue` allows the run to proceed with independent branches, but dependents of the failed step are still skipped

### Frontend Improvements

- TriggerBuilder: helper text explaining job chaining with example
- Job picker shortcut that auto-populates filter with `{"job_id": "<selected>"}`
- Readable trigger display: "Fires when **Build API** completes" instead of raw JSON

### Files

- `internal/server/event/trigger_subscriber.go` — add filter matching logic
- `internal/server/event/trigger_subscriber_test.go` — filter evaluation tests
- `internal/server/orchestrator/dag_runner_test.go` — DAG correctness tests
- `web/src/components/jobs/TriggerBuilder.tsx` — UX improvements

---

## 6. Settings Page

### Backend

**New migration — `user_preferences` table:**

```sql
CREATE TABLE IF NOT EXISTS user_preferences (
    user_id TEXT PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    preferences TEXT NOT NULL DEFAULT '{}',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**Preferences JSON schema:**

```json
{
  "skip_permissions": true,
  "default_session_timeout": 3600,
  "default_step_timeout": 0,
  "default_step_delay": 0,
  "default_env_vars": {"KEY": "value"},
  "notifications": {
    "events": ["run.completed", "run.failed", "machine.disconnected"]
  },
  "ui": {
    "theme": "system",
    "terminal_font_size": 14,
    "auto_attach_session": true,
    "command_center_cards": ["sessions", "machines", "jobs", "runs"]
  },
  "machine_overrides": {
    "<machine_id>": {
      "working_dir": "/home/user/projects",
      "model": "sonnet",
      "env_vars": {"KEY": "val"},
      "max_concurrent_sessions": 3
    }
  }
}
```

**REST API:**

- `GET /api/v1/users/me/preferences` — returns full preferences blob
- `PUT /api/v1/users/me/preferences` — replaces entire blob (server-side validation)
- `PATCH /api/v1/users/me/preferences` — shallow merge at top-level keys only. If `machine_overrides` is provided, it replaces the entire `machine_overrides` object. Array fields like `notifications.events` are replaced, not appended.

**Server-side validation:** Validate model values are in `{opus, sonnet, haiku}`, timeouts are non-negative, machine IDs reference existing machines, event types must match the pattern `<entity>.<action>` (e.g., `run.completed`, `machine.disconnected`).

### Frontend — `/settings` Route

Tabbed layout:

| Tab | Contents |
|-----|----------|
| Session Defaults | Skip permissions toggle, default timeout, default env vars |
| Job Defaults | Default step timeout, default step delay |
| Notifications | Event type checkboxes |
| UI Preferences | Theme picker (light/dark/system), terminal font size, auto-attach toggle, command center card visibility |
| Machines | Per-machine overrides: working dir, model dropdown, env vars, max concurrent sessions |

### Preference Resolution at Execution Time

The executor loads the requesting user's preferences once per run and passes them to each step execution. Explicit step/session values always win. Preferences are cached for the run duration to avoid per-step DB queries.

---

## 7. Session Log Search

### Agent-Side — CommandStream Command/Event Pair

Search uses the existing `CommandStream` bidirectional stream (agents dial in, server never dials out — Architecture Principle #1). No new RPC endpoint.

**New proto messages added to existing oneofs:**

```protobuf
// Added to ServerCommand oneof
message SearchScrollbackCmd {
  string request_id = 1;
  string query = 2;
  int32 max_results = 3;
  repeated string session_ids = 4;  // optional filter, empty = all
}

// Added to AgentEvent oneof
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

The `request_id` field correlates the command with its response event.

Agent implementation:
- Parse `.cast` files (asciicast v2 JSONL), strip ANSI escape codes, extract plain text
- Substring search across extracted text
- Return matches with surrounding context and timestamps
- Respect `max_results` to bound response size

### Server-Side Aggregation

- New REST endpoint: `GET /api/v1/search/sessions?q=<query>&limit=50`
- Server sends `SearchScrollbackCmd` through `CommandStream` to all connected agents in parallel
- Correlates responses via `request_id`, applies per-agent timeout of 10 seconds
- Returns partial results from agents that responded in time
- Merge results, sort by timestamp descending, return up to `limit` results (no cursor/offset pagination in v1 — partial agent responses make cursor-based pagination impractical)
- Include session metadata (machine, status, started_at) alongside each match

### Frontend — `/search` Route

- Search bar with text input
- Results as a list: session name, machine, timestamp, matched line with highlighting
- Click result navigates to session terminal view with scrollback replay
- Optional filters: machine, date range, session status

### Limitations (v1)

- Only searches sessions on currently connected agents
- No indexing — O(n) over `.cast` files per agent
- Large scrollback files may be slow — `max_results` bounds work

---

## Migration Summary

All schema changes in a single migration (version 9):

```sql
-- Steps: new columns
ALTER TABLE steps ADD COLUMN skip_permissions INTEGER;
ALTER TABLE steps ADD COLUMN model TEXT DEFAULT '';
ALTER TABLE steps ADD COLUMN delay_seconds INTEGER DEFAULT 0;

-- Run steps: snapshot columns (preserve snapshot-at-run-creation invariant)
ALTER TABLE run_steps ADD COLUMN skip_permissions_snapshot INTEGER;
ALTER TABLE run_steps ADD COLUMN model_snapshot TEXT DEFAULT '';
ALTER TABLE run_steps ADD COLUMN delay_seconds_snapshot INTEGER DEFAULT 0;
ALTER TABLE run_steps ADD COLUMN on_failure_snapshot TEXT DEFAULT 'fail_run';
ALTER TABLE run_steps ADD COLUMN timeout_seconds_snapshot INTEGER DEFAULT 0;

-- User preferences
CREATE TABLE IF NOT EXISTS user_preferences (
    user_id TEXT PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    preferences TEXT NOT NULL DEFAULT '{}',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

The `RunStep` struct and snapshot-creation logic in the orchestrator must be updated to copy `skip_permissions`, `model`, and `delay_seconds` from steps into `run_steps` at run creation time. The executor reads from snapshot columns, never from the live `steps` table.

## Cross-Cutting: Preference Resolution

The executor becomes the single point where user preferences merge with step/session overrides:

```
Step/Session override (explicit)
        ↓ fallback
User preference (per-machine if applicable)
        ↓ fallback
System default (no flag / empty)
```

This applies to: `skip_permissions`, `model`, `timeout`, `delay`, `working_dir`. Note: `env_vars` resolution is session/preference-level only (not per-step) — the step data model does not include env vars. Per-machine env vars from preferences are merged into `CreateSessionCmd.env_vars` at execution time.
