# Frontend UX Improvements — Design Spec

**Date:** 2026-03-15
**Status:** Approved

## Overview

Seven improvements spanning Command Center layout, session creation UX, naming consistency, new task types, connector restructuring, and shell task defaults.

**Implementation order:** Section 3 (step→task rename) must be done first, as Sections 4 and 5 reference post-rename names. Sections 1, 2, 6 are independent of each other.

---

## 1. Command Center Layout Reorder

**Goal:** Reorder sections for operational priority — infrastructure status first, then work items, templates last.

**New section order (top to bottom):**

1. **Dashboard Totals** — 5 stat cards in a single row:
   1. Active Sessions (cyan)
   2. Online Machines (green)
   3. Total Jobs (blue, links to `/jobs`)
   4. Recent Runs (amber, links to `/runs`)
   5. Completion Rate (green)

   **Removed from current layout:** "Total Sessions" and "Jobs Run" cards.
2. **Machines & Active Sessions** — side-by-side two-column grid
3. **Recent Jobs** — list of recently created jobs (up to 5)
4. **Recent Runs** — list of recent runs (up to 5)
5. **Templates** — grid of templates (up to 6), conditional on templates existing

**Files affected:** `web/src/views/CommandCenter.tsx`

---

## 2. New Session Type Selector

**Goal:** Allow users to create plain terminal sessions (bash) in addition to Claude CLI sessions.

**UX design:**
- **Segmented control** (Claude | Terminal) as the first field in `NewSessionModal`
- Defaults to "Claude"

**Claude mode** (existing behavior):
- Template, Machine, Working Directory, Command (defaults to `claude`), Model, Skip Permissions, Template Variables

**Terminal mode:**
- Machine (required)
- Working Directory (optional)
- All other fields hidden
- Command auto-set to `bash` (not shown to user)

**Backend impact:** None — sessions already support arbitrary commands. The frontend just sends `command: "bash"` instead of `command: "claude"`.

**Visual differentiation:** Terminal sessions should display a terminal icon and "Terminal" badge in session lists (Command Center, Sessions page, Workbench) to distinguish them from Claude sessions. Use the `Terminal` lucide icon for terminal sessions vs the existing icon for Claude sessions.

**Files affected:** `web/src/components/sessions/NewSessionModal.tsx`, session list components (for visual differentiation)

---

## 3. Frontend "Step" → "Task" Rename

**Goal:** Consistent terminology — the UI already uses "Tasks" as the tab label in JobEditor, but everything else says "step".

**Scope:** Frontend-only. Backend API paths (`/steps`) and Go types remain unchanged.

**Rename mapping:**

| Current | New |
|---------|-----|
| `Step` (type) | `Task` |
| `RunStep` (type) | `RunTask` |
| `StepDependency` (type) | `TaskDependency` |
| `CreateStepParams` | `CreateTaskParams` |
| `UpdateStepParams` | `UpdateTaskParams` |
| `StepEditor.tsx` (file) | `TaskEditor.tsx` |
| `StepNode.tsx` (file) | `TaskNode.tsx` |
| `StepEdge.tsx` (file) | `TaskEdge.tsx` |
| `addStep()` | `addTask()` |
| `updateStep()` | `updateTask()` |
| `deleteStep()` | `deleteTask()` |
| `useAddStep()` | `useAddTask()` |
| `useUpdateStep()` | `useUpdateTask()` |
| `useDeleteStep()` | `useDeleteTask()` |
| `useRetryStep()` | `useRetryTask()` |
| `selectedStepId` | `selectedTaskId` |
| `selectStep()` | `selectTask()` |
| `stepStatuses` | `taskStatuses` |
| `setStepStatuses()` | `setTaskStatuses()` |
| `updateStepStatus()` | `updateTaskStatus()` |
| Form IDs `step-*` | `task-*` |
| UI strings: "Step Configuration", "Save Step", etc. | "Task Configuration", "Save Task", etc. |
| Toast messages containing "step" | Replaced with "task" |

**Files affected (~18):**
- `web/src/types/job.ts`
- `web/src/api/jobs.ts`
- `web/src/hooks/useJobs.ts`
- `web/src/hooks/useRuns.ts`
- `web/src/hooks/useEventStream.ts`
- `web/src/stores/jobs.ts`
- `web/src/stores/runs.ts`
- `web/src/lib/types.ts`
- `web/src/components/jobs/StepEditor.tsx` → `TaskEditor.tsx`
- `web/src/components/dag/StepNode.tsx` → `TaskNode.tsx`
- `web/src/components/dag/StepEdge.tsx` → `TaskEdge.tsx`
- `web/src/components/dag/DAGCanvas.tsx`
- `web/src/components/runs/RunDAGView.tsx`
- `web/src/views/JobEditor.tsx`
- `web/src/views/RunDetail.tsx`
- `web/src/views/CommandCenter.tsx`
- `web/src/views/JobsPage.tsx`

**Note:** The event type string `'run.step.status'` in `useEventStream.ts` and `lib/types.ts` must NOT change — it matches the backend event name.

---

## 4. `run_job` Task Type

**Goal:** Enable job composition — a task in one job can trigger another job's execution.

**Behavior:** Fire-and-forget. The `run_job` task creates a new run of the target job with provided parameter overrides, then immediately marks itself as completed. The child run is independent.

### Frontend Changes

**TaskEditor (`TaskEditor.tsx`):**
- New task type option in the type selector: "Run Job"
- When `run_job` is selected, show:
  - **Job selector** — dropdown of all existing jobs (exclude current job to prevent self-referencing loops)
  - **Parameter fields** — dynamically rendered based on the selected job's defined `parameters`. Each parameter shows its name, type, and an input field. Values can use template expressions (e.g., `${{ params.branch }}`).
- Hide Claude-specific fields (prompt, model, skip permissions, session key) and shell-specific fields (command, args)
- Hide `machine_id` — `run_job` tasks don't run on a machine
- Keep common fields: name, delay, run_if, max_retries, retry_delay, on_failure

**Types (`types/job.ts`):**
- Add `target_job_id?: string` and `job_params?: Record<string, string>` to `Task` / `CreateTaskParams` / `UpdateTaskParams`

### Backend Changes

**Validation (`orchestrator/dag_runner.go`):**
- Accept `run_job` as valid `task_type`
- Validate: `target_job_id` must be non-empty and reference an existing job
- Validate: `target_job_id` must not equal the current job's ID (prevent direct self-loops)
- Validate: `machine_id` must be empty for `run_job` tasks (no machine needed)
- **Note:** Transitive cycle detection (A→B→A) is out of scope for v1. Document as known limitation.

**Step store (`store/jobs.go`):**
- Add `target_job_id` and `job_params` columns to `steps` table (migration)
- Add `target_job_id_snapshot` and `job_params_snapshot` to `run_steps` table

**Handler (`handler/jobs.go`):**
- Accept `target_job_id` and `job_params` in step create/update requests
- Default command to empty for `run_job` tasks (no CLI process)

**Executor (`executor/session_executor.go`):**
- New execution path for `run_job`: call the orchestrator's `StartRun` method directly (in-process, not via HTTP) with the target job ID and parameter overrides, then mark the step as completed immediately
- If the target job no longer exists at execution time, fail the step with an error message

**Migration:**
- New migration adding `target_job_id TEXT` and `job_params TEXT` (JSON) to `steps` table
- New migration adding `target_job_id_snapshot TEXT` and `job_params_snapshot TEXT` to `run_steps` table

---

## 5. Shell Task Command Fix

**Goal:** Shell tasks should not default to `claude`. Command field should be empty with a helpful placeholder.

**Changes in `TaskEditor.tsx`:**
- In `getFormParams()`, change the command default logic to only default to `'claude'` when `taskType === 'claude_session'`
- For `shell` tasks: no default, field is required (already enforced)
- For `run_job` tasks: command field hidden (not applicable)
- Update placeholder text for shell: `"e.g., ./deploy.sh, python script.py"`
- Remove "required" label for claude_session command (it's not required — backend defaults it)

**Files affected:** `web/src/components/jobs/StepEditor.tsx` (→ `TaskEditor.tsx` after rename)

---

## 6. GitHub Connector Restructure

**Goal:** Simplify connector creation (name + token only), move watches to a dedicated detail page, and add new trigger types.

### Creation Flow (simplified)

**`GithubForm.tsx` changes:**
- Create mode: only Name + GitHub Token fields
- Remove inline watch editor from creation form
- On successful creation, navigate to connector detail page

### Connector Detail Page (new)

**New route:** `/connectors/:connectorId` (added to `App.tsx` route config)
**New view:** `web/src/views/ConnectorDetailPage.tsx`
**New hook:** `useConnector(id)` query hook in `useBridge.ts` (uses existing `getConnector` API function)

**Sections:**
1. **Header** — Connector name, type badge, enabled/disabled toggle, edit (name/token) button, delete button
2. **Watches** — List of configured watches with add/edit/remove. Each watch expandable to show triggers and filters.
3. **Status** — Connection status, last poll time (if available from bridge status)

**Watch management:** Reuse existing `WatchEditor.tsx` and `TriggerConfig.tsx` components, but now they live on the detail page instead of inline in the creation form. Edit mode: click a watch to expand and edit in-place.

**Watch CRUD:** Watches are part of the connector's `config` JSON. Adding/editing/removing watches is done by updating the full connector via `PUT /api/v1/bridge/connectors/{id}` with the updated `config` containing the modified watches array. No separate watch endpoints needed.

### New Triggers

Add 4 new trigger types to `TriggerConfig.tsx` and backend:

| Trigger | Filters |
|---------|---------|
| `issue_comment` | Authors ignore |
| `pull_request_comment` | Authors ignore |
| `pull_request_review` | Review state (approved, changes_requested, commented), Authors ignore |
| `release_published` | Tag patterns (e.g., `v1.*`) |

**Backend changes (`internal/bridge/connector/github/`):**
- Add polling logic for each new event type in `poller.go`:
  - `issue_comment`: poll `GET /repos/{owner}/{repo}/issues/comments?since={cursor}` (sorted by updated_at)
  - `pull_request_comment`: poll `GET /repos/{owner}/{repo}/pulls/comments?since={cursor}`
  - `pull_request_review`: poll `GET /repos/{owner}/{repo}/pulls/{number}/reviews` for each open PR
  - `release_published`: poll `GET /repos/{owner}/{repo}/releases?per_page=10` (track latest release ID)
- Add filter matching in `filters.go`
- Add variable extraction in `variables.go` (comment body, author, review state, release tag, etc.)
- Update `TriggerConfig` struct in `github.go`
- State tracker must maintain separate cursors per event type per watch (e.g., `comment:{repo}`, `review:{repo}`, `release:{repo}`)

**Note:** Adding 4 new polling targets per watch increases GitHub API usage. The existing rate-limit-aware polling logic should be sufficient, but document the increased API consumption.

**Frontend changes:**
- Update `TriggerConfig.tsx` with new trigger sections
- Update `WatchEditor.tsx` with new trigger defaults
- Update `web/src/components/connectors/watchDefaults.ts`
- Add new route and view for connector detail

---

## 7. Credentials vs API Keys vs Provisioning

**Decision:** All three pages remain as-is. They serve distinct purposes:

- **Credentials** (`/credentials`) — Encrypted secrets for use in job tasks
- **API Keys** (`/api-keys`) — Long-lived auth tokens for external service integration
- **Provisioning** (`/provisioning`) — One-time agent machine onboarding with mTLS certificates

No changes needed.
