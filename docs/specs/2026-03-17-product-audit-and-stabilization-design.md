# Product Audit & Stabilization — Design Spec

**Date:** 2026-03-17
**Scope:** Full product audit of claude-plane — 7 phases, ~45 items
**Goal:** Ship a production-ready app where every feature works end-to-end, nothing silently fails, and changes are protected by real tests.

---

## Table of Contents

1. [Phase 1: Critical Fixes](#phase-1-critical-fixes)
2. [Phase 2: Test Infrastructure](#phase-2-test-infrastructure)
3. [Phase 3: Broken UI & Missing CRUD](#phase-3-broken-ui--missing-crud)
4. [Phase 4: Event System Enrichment](#phase-4-event-system-enrichment)
5. [Phase 5: UX Polish & Quick Wins](#phase-5-ux-polish--quick-wins)
6. [Phase 6: New Features](#phase-6-new-features)
7. [Phase 7: Low-Priority Polish](#phase-7-low-priority-polish)

---

## Phase 1: Critical Fixes

Data integrity and silent failure issues. If left unfixed, the app is losing data and lying to users.

### 1.1 — Persist session metadata in SQLite

**Problem:** `model`, `skip_permissions`, `env_vars` are sent in the CreateSession request, passed to the agent, then lost forever. The `sessions` table has columns for `args` and `initial_prompt` (added in existing migrations) but the `Session` struct in `store/sessions.go` does not have fields for `model`, `skip_permissions`, or `env_vars`, and `CreateSession` does not persist them. Note: `skip_permissions` and `model` exist on the `steps` table (for job tasks) but not on `sessions`.

**Fix:**
- Add migration: `ALTER TABLE sessions ADD COLUMN` for `model TEXT`, `skip_permissions TEXT`, `env_vars TEXT` (JSON). Do NOT add `args` or `initial_prompt` — they already exist in the schema.
- Add `Model`, `SkipPermissions`, `EnvVars` fields to the `Session` struct in `store/sessions.go`
- Update `CreateSession` in store to persist these fields alongside the existing `args` and `initial_prompt` columns
- Update `GetSession` / `ListSessions` to scan and return them
- Frontend session detail view shows metadata in a header above the terminal
- Frontend SessionCard shows model badge if set

**Files:** `internal/server/store/migrations.go`, `internal/server/store/sessions.go`, `internal/server/handler/sessions.go`, `web/src/types/session.ts`, `web/src/components/sessions/SessionCard.tsx`

### 1.2 — Fix frontend/backend event type mismatches

**Problem:** Frontend `useEventStream.ts` listens for event types that don't exist in the backend:
- `machine.status` → should be `machine.connected` / `machine.disconnected`
- `machine.health` → doesn't exist
- `run.step.status` → should be `run.step.completed` / `run.step.failed`
- Payload field names differ: frontend expects camelCase, backend sends snake_case

**Fix:**
- Create `web/src/constants/eventTypes.ts` that mirrors Go `event.go` constants exactly. Item 2.1 will later add the CI generator and sync test — this item creates the file and fixes the immediate mismatches.
- Update `useEventStream.ts` to use the registry and handle correct event types
- Standardize on snake_case for all payload fields (backend convention). Frontend reads `msg.payload.run_id`, `msg.payload.step_id`, etc. — not camelCase.

**Payload field mapping (backend → frontend):**
| Backend sends | Frontend reads |
|---------------|---------------|
| `run_id` | `msg.payload.run_id` |
| `step_id` | `msg.payload.step_id` |
| `run_step_id` | `msg.payload.run_step_id` |
| `session_id` | `msg.payload.session_id` |
| `machine_id` | `msg.payload.machine_id` |
| `status` | `msg.payload.status` |

**Files:** `web/src/hooks/useEventStream.ts`, `web/src/constants/eventTypes.ts` (new), `internal/server/event/event.go`

### 1.3 — Fix machine display names

**Problem:** `display_name` is NULL on machines because there's no way to set it. Machines register with a `machine_id` only.

**Fix:**
- Add `PUT /api/v1/machines/{machineID}` endpoint — accepts `display_name` and `max_sessions`
- Add store method `UpdateMachine(machineID, displayName, maxSessions)`
- Add "Rename" action to MachineCard (inline edit or small modal)
- Agent registration: if agent config has a `display_name` field, send it during `Register()` RPC and persist on first connection

**Files:** `internal/server/handler/machines.go` **(new file)**, `internal/server/api/router.go` (register new routes), `internal/server/store/machines.go`, `web/src/components/machines/MachineCard.tsx`, `internal/agent/config/config.go`

### 1.4 — Fix Command Center active sessions race condition

**Problem:** Sessions appear empty because the first render happens before the API response arrives. No `placeholderData` means `sessions` is `undefined` on first render, which `useMemo` turns into an empty filtered array.

**Fix:**
- Add `placeholderData: []` to the `useSessions()` query options
- Loading skeleton shows while fetching, data appears when ready
- No more "flash of empty then populated"

**Files:** `web/src/hooks/useSessions.ts`

### 1.5 — Auto-generate encryption key for credentials vault

**Problem:** Credentials page crashes with "encryption key not set" because it requires manual config. Most users forget.

**Fix:**
- On server startup, if no `encryption_key` in config: derive key file path from the configured data directory in server TOML (e.g., `{data_dir}/encryption.key`)
- Create the data directory if it doesn't exist (`os.MkdirAll`)
- If key file doesn't exist, generate 32 random bytes via `crypto/rand`, write to key file with `0600` permissions, log a notice: "Generated encryption key at {path}"
- Use this key for the credentials vault transparently
- Credentials page works out of the box, zero config

**Files:** `internal/server/config/config.go`, `cmd/server/serve.go` (startup logic)

### 1.6 — Add 404 page and React error boundary

**Problem:** Invalid URLs render blank pages. Component crashes propagate to white screen.

**Fix:**
- Add catch-all route in React Router rendering `NotFoundPage` component
- Add `ErrorBoundary` wrapper around the app's route outlet
- Invalid entity IDs (session, job, etc.) that return 404 from API show inline error states

**Files:** `web/src/App.tsx`, `web/src/views/NotFoundPage.tsx` (new), `web/src/components/shared/ErrorBoundary.tsx` (new)

---

## Phase 2: Test Infrastructure

Small upfront investment so every subsequent phase ships with real tests that catch integration issues.

### 2.1 — Shared event type registry with CI validation

**Problem:** Frontend and backend can drift on event type strings with no build-time detection.

**Relationship to 1.2:** Item 1.2 creates `web/src/constants/eventTypes.ts` with hardcoded constants matching the current backend. This item (2.1) adds the automated sync mechanism so the file can never drift again.

**Fix:**
- Go: `cmd/generate-event-types/main.go` reads all `Type*` constants from `event.go` via AST parsing, writes `internal/server/event/event_types.json` with event type strings and their payload field names
- Add `//go:generate go run ./cmd/generate-event-types` directive to `event.go`
- Frontend: vitest test `web/src/__tests__/eventTypeSync.test.ts` reads the generated JSON and asserts every backend event type has a matching constant in `eventTypes.ts`, and vice versa
- CI: `go generate ./...` runs the generator, then `vitest run` catches drift
- Adding a new event type in Go without updating frontend `eventTypes.ts` **fails CI**

**Files:** `cmd/generate-event-types/main.go` (new), `internal/server/event/event_types.json` (generated), `web/src/constants/eventTypes.ts` (created in 1.2), `web/src/__tests__/eventTypeSync.test.ts` (new)

### 2.2 — Go test factories

**Problem:** Handler tests manually construct entities — brittle, verbose, break when fields are added.

**Fix:**
- Create `internal/server/testutil/factory.go` with builder functions: `NewTestJob()`, `NewTestSession()`, `NewTestMachine()`, `NewTestRun()`, `NewTestTemplate()`, `NewTestUser()`
- Functional options for overrides: `NewTestJob(WithName("my-job"), WithTimeout(60))`
- `MustCreate(t, store)` variant that persists to test DB and returns the created entity
- Refactor 3-4 existing handler tests to use factories as proof of pattern

**Files:** `internal/server/testutil/factory.go` (new), selected handler test files

### 2.3 — Integration test harness for Go

**Problem:** Unit tests mock the store. Integration tests that hit a real SQLite DB don't exist. Mismatches between handler ↔ store ↔ DB go undetected.

**Fix:**
- Create `internal/server/testutil/testserver.go` — spins up real server (HTTP + in-memory SQLite) with all middleware, handlers, store, event bus
- Pattern: `srv := testutil.NewTestServer(t)` → `resp := srv.Client().POST("/api/v1/jobs", body)` → assert response
- Integration test tags (`//go:build integration`) so they run separately but CI runs both
- 5-6 seed integration tests:
  - Create job → add task → trigger run → verify run created with correct status
  - Create session → verify event published with correct payload
  - Create webhook → publish event → verify delivery record created
  - Create template → clone → verify clone has different ID but same content
  - Create user → login → verify JWT cookie set

**Files:** `internal/server/testutil/testserver.go` (new), `internal/server/integration_test.go` (new)

### 2.4 — Frontend test utilities

**Problem:** Frontend has 8 test files, ~512 lines. No shared test patterns or utilities.

**Fix:**
- `web/src/test/setup.ts` — vitest setup with MSW handlers for all API endpoints
- `web/src/test/factories.ts` — TypeScript entity builders: `buildSession()`, `buildJob()`, `buildMachine()`
- `web/src/test/render.tsx` — `renderWithProviders()` wrapping components in Query, Auth, Router providers
- 4 seed tests:
  - `SessionsPage.test.tsx` — renders with mock sessions, filters work, empty state shows
  - `JobsPage.test.tsx` — renders with mock jobs, search filters, Run button calls API
  - `CommandCenter.test.tsx` — renders dashboard cards with mock data
  - `useEventStream.test.ts` — mock WebSocket, verify query invalidation on correct event types

**Files:** `web/src/test/setup.ts` (new), `web/src/test/factories.ts` (new), `web/src/test/render.tsx` (new), 4 new test files

### 2.5 — API contract tests

**Problem:** Backend can change response shapes and frontend silently breaks.

**Fix:**
- Go integration test: for each endpoint, assert JSON response shape matches documented schema (snapshot-style)
- Frontend MSW handlers: assert mock response shapes match TypeScript types
- If a Go handler adds/removes a field, contract test catches it

**Files:** Integration tests in `internal/server/`, MSW handler definitions in `web/src/test/`

---

## Phase 3: Broken UI & Missing CRUD

Everything that exists but doesn't work, plus missing operations users expect.

### Jobs Domain

#### 3.1 — Job delete button
- Add Trash2 icon button to JobsPage table rows (next to Run button)
- Add delete button to JobEditor header
- ConfirmDialog with `variant="danger"` showing job name
- Calls existing `DELETE /api/v1/jobs/{jobID}` endpoint
- Invalidate `['jobs']` query on success

**Files:** `web/src/views/JobsPage.tsx`, `web/src/views/JobEditor.tsx`

#### 3.2 — Job duplicate
- Add `POST /api/v1/jobs/{jobID}/clone` backend endpoint
  - **Request body (optional):** `{ "name": "override name" }` — if omitted, defaults to `"{original} (copy)"`
  - **Response:** full cloned `Job` object including all steps and dependencies (same shape as `GET /api/v1/jobs/{jobID}`)
  - Deep clones: job metadata + all steps + all step dependencies. New UUIDs for job and all steps. Dependency edges remapped to new step IDs.
- Add CopyPlus icon button to JobsPage table rows
- Navigate to cloned job's editor on success

**Files:** `internal/server/handler/jobs.go`, `internal/server/store/jobs.go`, `web/src/views/JobsPage.tsx`, `web/src/api/jobs.ts`

#### 3.3 — Fix job parameter Add button
- Live debug ParameterEditor — code looks correct (`type="button"`, `handleAdd()` wired)
- Likely CSS overflow, z-index, or parent form event propagation issue
- Fix rendering or add `e.stopPropagation()` as needed

**Files:** `web/src/components/jobs/ParameterEditor.tsx`

#### 3.4 — DAG edge removal UI
- Add right-click context menu on edges with "Remove dependency" option
- Or: click edge to select, Delete/Backspace key removes it
- Calls existing `DELETE /api/v1/jobs/{jobID}/steps/{stepID}/deps/{depID}`

**Files:** `web/src/components/dag/TaskEdge.tsx`, `web/src/components/dag/DAGCanvas.tsx`

### Sessions Domain

#### 3.5 — Session detail metadata header
- Compact header bar above terminal: command, machine name, model, working directory, created time, duration
- "Back to sessions" button, session ID with copy-to-clipboard
- "Terminate" button (currently only in list view)

**Files:** `web/src/components/terminal/TerminalView.tsx` (or new `SessionHeader.tsx`), session detail route component

#### 3.6 — Fix multiview workspace creation
- Live debug SessionPicker click handlers and data loading
- May self-resolve after 1.3 (machine rename fix) improves display
- Verify: running sessions appear, machine filter populates, clicking a session creates workspace

**Files:** `web/src/components/multiview/SessionPicker.tsx`, `web/src/components/multiview/MultiviewPage.tsx`

### Templates Domain

#### 3.7 — Wire LaunchTemplateModal into Templates page
- Import existing dead-code LaunchTemplateModal
- Add Play icon "Launch" button to each template row
- On launch, navigate to new session

**Files:** `web/src/views/TemplatesPage.tsx`, `web/src/components/templates/LaunchTemplateModal.tsx`

#### 3.8 — Remove terminal rows/cols and tags from template form
- Remove terminal dimensions fields and tags field from TemplateForm UI
- Remove tag filter from TemplatesPage
- Keep DB columns for backward compatibility, stop exposing in UI

**Files:** `web/src/components/templates/TemplateForm.tsx`, `web/src/views/TemplatesPage.tsx`

### Credentials Domain

#### 3.9 — Credentials graceful degradation
- Phase 1 auto-generates encryption key (1.5), so this should never trigger in normal operation. This is a defense-in-depth fallback.
- **Backend change:** Modify the credential handler to check if encryption key is configured. If not, store credential values as plaintext (no encryption/decryption calls). Add a `GET /api/v1/credentials/status` endpoint that returns `{ "encryption_enabled": bool }`.
- **Frontend change:** CredentialsPage calls the status endpoint on mount. If encryption is disabled, show warning banner: "Credentials are stored without encryption. This is unusual — check server logs." Allow all CRUD operations to work normally.
- Never crash the page regardless of encryption state.

**Files:** `internal/server/handler/credentials.go`, `internal/server/store/credentials.go`, `web/src/views/CredentialsPage.tsx`, `web/src/api/credentials.ts`

### Machines Domain

#### 3.10 — Machine delete/deregister
- Add `DELETE /api/v1/machines/{machineID}` endpoint — only works if disconnected (409 if connected)
- **Cascade strategy:** Use soft-delete. Add `deleted_at TIMESTAMP` column to machines table. `DELETE` sets `deleted_at` to now. `ListMachines` filters out soft-deleted machines. Historical sessions retain their `machine_id` FK reference (no orphaned data). This avoids breaking FK constraints from existing sessions, runs, etc.
- Add "Remove" button to MachineCard (visible only when disconnected)
- ConfirmDialog: "This will remove {machine} from the dashboard. Historical sessions and runs will retain their machine reference. The machine can re-register with a new provisioning token."

**Files:** `internal/server/handler/machines.go`, `internal/server/store/machines.go`, `internal/server/store/migrations.go`, `web/src/components/machines/MachineCard.tsx`

### Auth Domain

#### 3.11 — User password change
- Add `POST /api/v1/users/me/password` — uses POST (not PUT) to avoid Chi router conflict with `{userID}` param matching "me". Requires `{ "current_password": "...", "new_password": "..." }`, min 8 chars for new password.
- Add `POST /api/v1/users/{userID}/reset-password` — admin can reset without current password. Requires `{ "new_password": "..." }`.
- **Route registration:** Register `/api/v1/users/me/password` BEFORE the `/{userID}` routes in `router.go` to prevent Chi from matching `me` as a userID.
- Add "Change Password" section to Settings → new "Account" tab. Note: SettingsPage already has a tab system (Session Defaults, Job Defaults, UI Preferences, Machines, Notifications, Data Retention). "Account" becomes a new tab.
- Admin Users page: "Reset Password" action in user row dropdown

**Files:** `internal/server/handler/users.go`, `internal/server/auth/auth.go`, `internal/server/api/router.go`, `web/src/views/SettingsPage.tsx`, `web/src/views/AdminPage.tsx`

#### 3.12 — User profile self-edit
- Add `PUT /api/v1/users/me` endpoint — update own `display_name`
- Settings "Account" tab: display name (editable), email (read-only), role (read-only), password change form

**Files:** `internal/server/handler/users.go`, `web/src/views/SettingsPage.tsx`

### Webhooks Domain

#### 3.13 — Webhook secret reveal on creation
- After creating webhook, show one-time modal: "Save your webhook secret — it won't be shown again" with copyable field
- Same pattern as CreateKeyModal for API keys

**Files:** `web/src/components/webhooks/WebhookForm.tsx`

### Triggers Domain

#### 3.14 — Trigger edit capability
- Add `PUT /api/v1/triggers/{triggerID}` endpoint
- Add store method `UpdateJobTrigger(triggerID, eventType, filter)` — deliberately excludes `enabled` field; use the toggle endpoint (3.15) for that. PUT updates configuration, toggle controls activation.
- Add "Edit" button to trigger cards, opens TriggerBuilder pre-filled with current values

**Files:** `internal/server/handler/triggers.go`, `internal/server/store/triggers.go`, `web/src/components/jobs/TriggerPanel.tsx`, `web/src/components/jobs/TriggerBuilder.tsx`

#### 3.15 — Trigger enable/disable toggle
- Add `POST /api/v1/triggers/{triggerID}/toggle` endpoint — flips `enabled` field. This is a convenience endpoint separate from PUT (3.14) because toggling is a common quick action that shouldn't require sending the full trigger config.
- Add Pause/Play icon button on trigger cards
- Disabled triggers show muted styling (50% opacity, strikethrough event type)

**Files:** `internal/server/handler/triggers.go`, `internal/server/store/triggers.go`, `web/src/components/jobs/TriggerPanel.tsx`

---

## Phase 4: Event System Enrichment

Make events actually useful for debugging, notifications, and automation.

### 4.1 — Add names to all event payloads

Update event builder functions in `internal/server/event/builders.go`:
- `NewTemplateEvent` → add `template_name`
- `NewRunEvent` → add `job_name`
- `NewSessionEvent` → add `machine_name`, `command`
- `NewMachineEvent` → add `display_name`
- `NewRunStepEvent` → add `step_name`, `job_name`

Each handler already has the entity — passing the name is trivial.

**Files:** `internal/server/event/builders.go`, all handler files that publish events

### 4.2 — Add missing event types

New constants in `event.go`:
- `job.created`, `job.updated`, `job.deleted`
- `user.created`, `user.deleted`
- `schedule.created`, `schedule.paused`, `schedule.resumed`, `schedule.deleted`
- `credential.created`, `credential.deleted`
- `webhook.created`, `webhook.deleted`

Each:
- Gets a constant + builder function
- Added to shared event type registry
- Added to frontend `eventTypes.ts`
- Added to webhook event selector checkboxes
- Added to notification subscription matrix

**Files:** `internal/server/event/event.go`, `internal/server/event/builders.go`, all relevant handlers, `web/src/constants/eventTypes.ts`

### 4.3 — Fix Telegram formatter

Current formatter only handles session, run, machine events. Silently drops everything else.

- Add format cases for all new event types
- Use names from enriched payloads: "Job **deploy-prod** created" not "Job abc123 created"

**Files:** `internal/bridge/connector/telegram/formatter.go`

### 4.4 — Add payload view to webhook delivery history

- Add migration: `ALTER TABLE webhook_deliveries ADD COLUMN payload TEXT` — stores the serialized JSON that was POSTed
- Update delivery creation in `webhook_delivery.go` to persist the payload at delivery time
- Frontend: expandable delivery row shows full payload with syntax highlighting + "Copy payload" button

**Files:** `internal/server/event/webhook_delivery.go`, `internal/server/store/webhooks.go`, `web/src/components/webhooks/DeliveryHistory.tsx`

### 4.5 — Frontend event stream handles all new types

Update `useEventStream.ts` to invalidate correct queries for all new event types:
- `job.*` → `['jobs']`
- `template.*` → `['templates']`
- `schedule.*` → `['schedules']`
- `credential.*` → `['credentials']`
- `webhook.*` → `['webhooks']`
- `user.*` → `['users']`

Entire app becomes truly reactive — any change is immediately reflected in any open page.

**Files:** `web/src/hooks/useEventStream.ts`

---

## Phase 5: UX Polish & Quick Wins

Small changes that make the app feel finished.

### Shared Components

#### 5.1 — CopyableId component
- Truncated ID (8 chars, monospace) + copy icon button
- Click copies full ID, shows "Copied!" tooltip
- Full ID on hover
- Replace all inline `id.slice(0, 8)` across all pages

**Files:** `web/src/components/shared/CopyableId.tsx` (new), all pages displaying IDs

#### 5.2 — Breadcrumb component
- Path: `Jobs > deploy-prod > Run abc123`
- Add to: RunDetail, JobEditor, WebhookDeliveriesPage, session detail
- Derives from React Router location + fetched entity names

**Files:** `web/src/components/shared/Breadcrumb.tsx` (new), relevant view files

#### 5.3 — RefreshButton component
- RefreshCw icon, spins while loading
- Extract from EventsPage pattern, reuse on all data pages

**Files:** `web/src/components/shared/RefreshButton.tsx` (new), all view files

### Page-Level Fixes

#### 5.4 — Consistent empty states
- JobsPage and RunsPage: replace plain text with EmptyState component (icon + title + description + CTA)

**Files:** `web/src/views/JobsPage.tsx`, `web/src/views/RunsPage.tsx`

#### 5.5 — Consistent skeleton loading
- Standardize all backgrounds to `bg-bg-secondary`
- Extract `SkeletonCard` and `SkeletonRow` shared components

**Files:** `web/src/components/shared/SkeletonCard.tsx` (new), `web/src/components/shared/SkeletonRow.tsx` (new), all view files

#### 5.6 — Consistent table row hover
- Standardize to `hover:bg-bg-tertiary/50` across all tables

**Files:** All table components

#### 5.7 — Form validation feedback
- Red border on invalid fields, inline error text, asterisk on required labels
- Apply to: NewSessionModal, CreateCredentialModal, CreateUserModal, EditUserModal, WebhookForm, TemplateForm, JobMetaForm, ScheduleForm, TriggerBuilder

**Files:** All form components

#### 5.8 — Fix "Step" vs "Task" terminology
- Ensure no user-facing frontend string says "step"
- Backend/API paths stay as-is (no breaking change)

**Files:** All frontend components with user-facing strings

### Layout Fixes

#### 5.9 — Sidebar tooltips when collapsed
- Tooltip on hover showing page name in icon-only mode

**Files:** `web/src/components/layout/Sidebar.tsx`

#### 5.10 — User avatar shows initials
- Replace static "CP" with first letter(s) of user's display name
- Fallback to email first letter

**Files:** `web/src/components/layout/TopBar.tsx`

#### 5.11 — Back button on detail views
- "← Back" link at top of session detail, RunDetail, WebhookDeliveriesPage
- Complements breadcrumbs

**Files:** Relevant view files

### Data Display Fixes

#### 5.12 — Dates: relative + absolute on hover
- Primary: relative ("5 min ago")
- Tooltip: absolute in user locale ("Mar 17, 2026, 10:30:45 AM")

**Files:** `web/src/components/shared/TimeAgo.tsx` or equivalent, `web/src/lib/formatTimeAgo.ts`

#### 5.13 — Status badges accessibility
- Verify all status badges use both color AND icon (not just color)
- completed=CheckCircle, failed=XCircle, running=Loader2, pending=Clock, cancelled=Ban, terminated=Square

**Files:** `web/src/components/shared/StatusBadge.tsx`

---

## Phase 6: New Features

### 6.1 — SMTP notification service (built into server)

**Backend:**
- New package `internal/server/notify/` with:
  - `Notifier` interface: `Send(ctx, channel, event) error`
  - `SMTPNotifier` implementation using Go standard library `net/smtp` (no external dependency needed for basic SMTP; if TLS/STARTTLS is needed, use `go-mail/mail/v2`)
  - `Dispatcher` — event bus subscriber registered at startup (same pattern as `webhook_delivery.go`'s `StartDeliverer`). Subscribes to `"*"` on the event bus, checks notification_subscriptions table, fans out to matching channels.
- New tables:
  - `notification_channels`: `channel_id TEXT PK`, `channel_type TEXT` (email/telegram), `config TEXT` (JSON: host, port, from, username, encrypted_password, tls), `enabled INTEGER`, `created_by TEXT`, `created_at TIMESTAMP`
  - `notification_subscriptions`: `user_id TEXT`, `channel_id TEXT`, `event_type TEXT`, `PRIMARY KEY (user_id, channel_id, event_type)`
- Email HTML template: stored as embedded Go template in `internal/server/notify/templates/email.html`. Contains: event type header, payload summary table, timestamp, "View in App" link (constructed from server base URL config).
- Rate limiting: in-memory `sync.Map` keyed by `{channel_id}:{event_type}`, stores last-sent timestamp. Max 1 notification per channel per event type per 60 seconds. Resets on server restart (acceptable for single-instance).
- Test endpoint: sends a synthetic `notification.test` event with payload `{"message": "Test notification from claude-plane"}` through the selected channel.

**API:**
- `GET/POST /api/v1/notification-channels` — CRUD
- `PUT/DELETE /api/v1/notification-channels/{id}`
- `POST /api/v1/notification-channels/{id}/test` — send test notification
- `GET/PUT /api/v1/notifications/subscriptions` — get/set subscription matrix

**Frontend — Settings → Notifications tab redesign:**
- Channels section: "Add Channel" → Email or Telegram config modal with "Test" button
- Subscription matrix: rows = event types grouped by category, columns = configured channels, checkboxes at intersections
- "Select all" per row and per column

**Files:** `internal/server/notify/` (new package), `internal/server/store/notifications.go` (new), `internal/server/handler/notifications.go` (new), `web/src/views/SettingsPage.tsx`, `web/src/components/settings/NotificationsTab.tsx`

### 6.2 — Telegram as first-class notification channel

- Server sends directly to Telegram Bot API (no bridge required for notifications)
- `TelegramNotifier` implementation in `internal/server/notify/`
- Users configure Telegram in same Notifications tab as email
- Same subscription matrix applies
- Bridge still handles inbound Telegram → job triggers (different flow)

**Files:** `internal/server/notify/telegram.go` (new)

### 6.3 — In-app documentation pages

**Frontend:**
- New route `/docs` with sub-routes: `/docs/telegram-setup`, `/docs/github-setup`, `/docs/smtp-setup`, `/docs/getting-started`
- Sidebar: "Docs" link in new "Help" section
- Markdown rendered with TOC sidebar, code blocks with copy buttons
- Content in `docs/guides/*.md` in repo, bundled via Vite raw import. **Size budget:** keep total guide markdown under 100KB to avoid bloating the JS bundle. If guides grow beyond this, switch to lazy-loading via dynamic `import()` or fetch from a static asset.

**Guides:**
- Telegram Setup: BotFather → token → group/topic → configure → test
- GitHub Setup: App/PAT → bridge config → webhook triggers → event mapping
- SMTP Setup: Credentials → configure in Settings → test → subscribe
- Getting Started: Overview linking all three + agent provisioning

**Contextual help links:**
- Notifications tab: "How to set up email" / "How to set up Telegram"
- Provisioning page: "How to install an agent"
- Webhooks page: "How webhook signing works"

**Files:** `docs/guides/` (new directory), `web/src/views/DocsPage.tsx` (new), `web/src/components/docs/` (new), `web/src/App.tsx` (routes)

### 6.4 — Top-level Triggers & Schedules management pages

- `/triggers` — all triggers across all jobs. Columns: event type, filter, job name, enabled, created date. Actions: toggle, delete
- `/schedules` — all schedules across all jobs. Columns: cron (+ English), timezone, job name, next run, status. Actions: pause/resume, delete
- Sidebar: add to "Automation" section
- Backend: add new store methods and handler endpoints:
  - `ListAllTriggers(ctx) ([]JobTriggerWithJobName, error)` — joins `job_triggers` with `jobs` to include `job_name`. SQL: `SELECT t.*, j.name as job_name FROM job_triggers t JOIN jobs j ON t.job_id = j.job_id ORDER BY t.created_at DESC`
  - `ListAllSchedules(ctx) ([]ScheduleWithJobName, error)` — joins `cron_schedules` with `jobs`. SQL: `SELECT s.*, j.name as job_name FROM cron_schedules s JOIN jobs j ON s.job_id = j.job_id ORDER BY s.created_at DESC`
  - `GET /api/v1/triggers` — calls `ListAllTriggers`, returns array with job_name included
  - `GET /api/v1/schedules` — calls `ListAllSchedules`, returns array with job_name included
  - Note: existing per-job endpoints (`GET /api/v1/jobs/{jobID}/triggers`) remain unchanged

**Files:** `web/src/views/TriggersPage.tsx` (new), `web/src/views/SchedulesPage.tsx` (new), `internal/server/handler/triggers.go`, `internal/server/handler/schedules.go`, `internal/server/store/triggers.go`, `internal/server/store/schedules.go`, sidebar navigation

---

## Phase 7: Low-Priority Polish

### 7.1 — Pagination on all list pages
- Shared `Pagination` component: prev/next, page size selector (25/50/100), "Showing X-Y of Z"
- Standardize backend `limit`/`offset` support
- Add to: JobsPage, RunsPage, SessionsPage, TemplatesPage, EventsPage, AdminPage, WebhooksPage, CredentialsPage

### 7.2 — Sortable table columns
- Shared `SortableHeader` component with chevron indicators
- Client-side for small datasets, server-side for paginated
- Per-page sortable columns defined

### 7.3 — Global search (Cmd+K palette)
- Command palette: searches jobs, sessions, machines, templates, runs by name/ID
- Quick actions: "New Session", "New Job", "New Template"
- Client-side search across cached query data

### 7.4 — Notification badge in header
- Bell icon in TopBar with unread event count badge
- Click navigates to Events page, clears count
- Count in localStorage

### 7.5 — Webhook test delivery
- "Send Test" button on webhook edit
- `POST /api/v1/webhooks/{webhookID}/test` publishes synthetic `webhook.test` event
- Appears in delivery history

### 7.6 — Schedule "Run Now" button
- Play icon on schedule cards
- `POST /api/v1/schedules/{scheduleID}/trigger` fires linked job immediately
- `trigger_type: "manual_schedule"` for audit trail

### 7.7 — Run logs export
- "Download Logs" button on RunDetail per task
- `GET /api/v1/sessions/{sessionID}/scrollback` returns raw content
- Download as `.txt`, also "Copy to clipboard" option

### 7.8 — Job validation before run
- 0 tasks → error toast: "Add at least one task before running"
- Claude task with no prompt → warning (not block)
- Client-side validation in RunNowModal

### 7.9 — Theme transition smoothing
- `transition-colors duration-200` on root element when switching themes

### 7.10 — Mobile DAG improvements
- Increase node tap targets: 120x44 → 160x56
- Pinch-to-zoom on DAG canvas
- RunDetail DAG height: 160px → 220px on mobile

### 7.11 — Session search
- Search input on SessionsPage filtering by session ID, command, or working directory
- Client-side filter

### 7.12 — Cron expression presets
- Preset buttons below cron input: "Every hour", "Daily at 9am", "Every Monday", "Every 5 minutes"
- Click fills the expression field

---

## Cross-Cutting Concerns

### Testing Strategy

**Phase ordering note:** Phase 1 tests use ad-hoc test patterns (direct struct construction, simple HTTP calls). Phase 2 builds the reusable infrastructure (factories, test server, MSW). Starting Phase 3, all tests use Phase 2 infrastructure. Phase 2 also retroactively improves Phase 1 tests by refactoring them to use the new factories/test server — this is part of the "refactor 3-4 existing tests" work in 2.2.

Every phase ships with tests:
- **Phase 1:** Ad-hoc tests for each fix (event type sync test, session metadata persistence test, encryption key generation test)
- **Phase 2:** Reusable infrastructure (factories, test server, MSW) + retroactive improvement of Phase 1 tests
- **Phase 3+:** Every new endpoint gets handler test + integration test. Every new component gets render test. Every bug fix gets a regression test proving the fix works.

Target: 80%+ backend coverage, meaningful frontend coverage for all pages and critical hooks.

### Migration Safety

All SQLite schema changes:
- Use `ALTER TABLE ADD COLUMN` (safe, no data loss)
- New tables created with `CREATE TABLE IF NOT EXISTS`
- No destructive migrations
- Backward compatible: old data still works

### API Backward Compatibility

- No existing endpoints change their response shape (only additions)
- New fields in responses are additive
- New endpoints don't conflict with existing routes
- Frontend gracefully handles missing optional fields (for users who haven't upgraded server yet)

### PR Strategy

One PR per phase. Each PR:
- Includes all items in that phase
- Includes tests for all changes
- Passes full CI (Go vet, Go test, frontend typecheck, lint, vitest, build)
- Reviewed before merge
