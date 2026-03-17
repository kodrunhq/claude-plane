# Phase 3: Broken UI & Missing CRUD — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make everything that exists actually work, plus add missing CRUD operations users expect. 15 items across 7 domains.

**Architecture:** Mix of frontend-only fixes (wiring existing components, adding buttons) and full-stack features (new endpoints + UI). Each task is independent and gets its own commit.

**Tech Stack:** Go 1.25, SQLite, React 19, TypeScript, TanStack Query, xterm.js, @xyflow/react

**Design spec:** `docs/specs/2026-03-17-product-audit-and-stabilization-design.md` — Phase 3 section

---

## File Map

| Task | Domain | Creates | Modifies |
|------|--------|---------|----------|
| 1 | Jobs | — | `web/src/views/JobsPage.tsx`, `web/src/views/JobEditor.tsx`, `web/src/api/jobs.ts`, `web/src/hooks/useJobs.ts` |
| 2 | Jobs | — | `internal/server/handler/jobs.go`, `internal/server/store/jobs.go`, `web/src/views/JobsPage.tsx`, `web/src/api/jobs.ts`, `web/src/hooks/useJobs.ts` |
| 3 | Jobs | — | `web/src/components/jobs/ParameterEditor.tsx` (debug) |
| 4 | DAG | — | `web/src/components/dag/DAGCanvas.tsx`, `web/src/components/dag/TaskEdge.tsx` |
| 5 | Sessions | `web/src/components/terminal/SessionHeader.tsx` | `web/src/App.tsx` (TerminalRoute), `web/src/hooks/useSessions.ts` |
| 6 | Multiview | — | `web/src/components/multiview/SessionPicker.tsx`, `web/src/components/multiview/MultiviewPage.tsx` |
| 7 | Templates | — | `web/src/views/TemplatesPage.tsx` |
| 8 | Templates | — | `web/src/components/templates/TemplateForm.tsx`, `web/src/views/TemplatesPage.tsx` |
| 9 | Credentials | — | `web/src/views/CredentialsPage.tsx`, `web/src/api/credentials.ts` |
| 10 | Machines | — | `internal/server/store/machines.go`, `internal/server/store/migrations.go`, `internal/server/api/machine_handler.go`, `internal/server/api/router.go`, `web/src/components/machines/MachineCard.tsx` |
| 11 | Auth | — | `internal/server/handler/users.go`, `internal/server/store/users.go`, `internal/server/auth/auth.go`, `internal/server/api/router.go`, `web/src/views/SettingsPage.tsx` |
| 12 | Auth | — | `internal/server/handler/users.go`, `internal/server/api/router.go`, `web/src/views/SettingsPage.tsx` |
| 13 | Webhooks | — | `web/src/components/webhooks/WebhookForm.tsx` or parent |
| 14 | Triggers | — | `internal/server/handler/triggers.go`, `internal/server/store/triggers.go`, `web/src/components/jobs/TriggerPanel.tsx`, `web/src/components/jobs/TriggerBuilder.tsx` |
| 15 | Triggers | — | `internal/server/handler/triggers.go`, `internal/server/store/triggers.go`, `web/src/components/jobs/TriggerPanel.tsx` |

---

## Task 1: Job delete button (item 3.1) — Frontend only

The backend `DELETE /api/v1/jobs/{jobID}` handler already exists and works (handler/jobs.go:380-396). Just need to add the UI.

**Files:**
- Modify: `web/src/views/JobsPage.tsx:212-221` (add delete button to table row actions)
- Modify: `web/src/views/JobEditor.tsx` (add delete button to editor header)
- Modify: `web/src/api/jobs.ts` (add delete function if missing)
- Modify: `web/src/hooks/useJobs.ts` (add useDeleteJob mutation if missing)

### Steps

- [ ] **Step 1:** Read `web/src/api/jobs.ts` and `web/src/hooks/useJobs.ts` to check if delete API function and mutation hook exist. If not, add them:
  - API: `delete: (jobId: string) => request(\`/jobs/${jobId}\`, { method: 'DELETE' })`
  - Hook: `useDeleteJob()` mutation that invalidates `['jobs']` on success

- [ ] **Step 2:** Read `web/src/views/JobsPage.tsx` to understand the table row action pattern. Add a Trash2 icon button next to the existing Run button (line ~212). Wire it to open a ConfirmDialog. Use the same ConfirmDialog pattern as TemplatesPage (which already has delete).

- [ ] **Step 3:** Read `web/src/views/JobEditor.tsx` to find the header section. Add a delete button (Trash2 icon) that opens ConfirmDialog and navigates back to `/jobs` on success.

- [ ] **Step 4:** Write test in `web/src/__tests__/views/JobsPage.test.tsx` — verify delete button exists in table rows.

- [ ] **Step 5:** Run tests: `cd web && npx tsc -b && npx vitest run`

- [ ] **Step 6:** Commit: `git commit -m "feat: add job delete button to jobs page and editor"`

---

## Task 2: Job duplicate (item 3.2) — Full stack

No clone endpoint exists. Need backend + frontend.

**Files:**
- Modify: `internal/server/handler/jobs.go` (add CloneJob handler)
- Modify: `internal/server/store/jobs.go` (add CloneJob store method)
- Modify: `web/src/views/JobsPage.tsx` (add duplicate button)
- Modify: `web/src/api/jobs.ts` (add clone function)
- Modify: `web/src/hooks/useJobs.ts` (add useCloneJob mutation)
- Test: `internal/server/handler/jobs_test.go`, `internal/server/store/jobs_test.go`

### Steps

- [ ] **Step 1:** Read `internal/server/store/jobs.go` to understand Job, Step, and Dependency structs. Implement `CloneJob(ctx, jobID, newName) (*Job, error)`:
  - Begin transaction
  - Read original job + all steps + all dependencies
  - Create new job with new UUID, name = `"{original} (copy)"` (or newName if provided)
  - Create new steps with new UUIDs, same config
  - Create dependencies remapped to new step IDs
  - Commit transaction

- [ ] **Step 2:** Write store test for CloneJob — create job with 2 steps and 1 dependency, clone it, verify all pieces exist with new IDs.

- [ ] **Step 3:** Add handler `POST /api/v1/jobs/{jobID}/clone` in `handler/jobs.go`. Accepts optional `{ "name": "override" }` body. Returns the full cloned job. Register route in the jobs route group.

- [ ] **Step 4:** Write handler test for clone endpoint.

- [ ] **Step 5:** Add frontend API function and mutation hook for clone.

- [ ] **Step 6:** Add CopyPlus icon button to JobsPage table rows. On success, navigate to `/jobs/{clonedJobId}`.

- [ ] **Step 7:** Run all tests: `go test -race ./... && cd web && npx tsc -b && npx vitest run`

- [ ] **Step 8:** Commit: `git commit -m "feat: add job clone endpoint and duplicate button"`

---

## Task 3: Debug job parameter Add button (item 3.3)

Code in ParameterEditor.tsx looks correct (handleAdd at lines 91-97, button at lines 124-131). Likely a CSS overflow or parent form event issue.

**Files:**
- Debug: `web/src/components/jobs/ParameterEditor.tsx`
- Debug: `web/src/views/JobEditor.tsx` (parent component)

### Steps

- [ ] **Step 1:** Read both files thoroughly. Check: Is the ParameterEditor rendered inside a `<form>`? Does the parent pass an `onChange` callback? Is the button visible (check for overflow-hidden on parent containers)?

- [ ] **Step 2:** If it's a form submission issue, ensure the button has `type="button"`. If it's a missing onChange, wire it. If it's a CSS issue, fix overflow.

- [ ] **Step 3:** Write a simple test that renders ParameterEditor with an onChange spy, clicks the Add button, and verifies onChange was called.

- [ ] **Step 4:** Run tests and commit: `git commit -m "fix: job parameter Add button now works"`

---

## Task 4: DAG edge removal (item 3.4)

Edges in the DAG canvas have no delete functionality. Need to add click-to-select + delete key, or a context menu.

**Files:**
- Modify: `web/src/components/dag/DAGCanvas.tsx`
- Modify: `web/src/components/dag/TaskEdge.tsx`

### Steps

- [ ] **Step 1:** Read `DAGCanvas.tsx` and `TaskEdge.tsx`. Understand how @xyflow/react handles edge events. Check if `onEdgesDelete` or `onEdgeClick` callbacks are available.

- [ ] **Step 2:** Read the job editor to find the dependency deletion API call. It should be `DELETE /api/v1/jobs/{jobID}/steps/{stepID}/deps/{depID}` — verify it exists in the API layer.

- [ ] **Step 3:** Implement edge deletion. Recommended approach:
  - Add `edgesOptions={{ deletable: true }}` or `onEdgeClick` to the ReactFlow instance
  - On edge click, set it as selected (highlighted)
  - On Delete/Backspace key with selected edge, call the delete dependency API
  - Remove the edge from local state on success
  - Only enable in edit mode (not read-only run view)

- [ ] **Step 4:** Run tests and commit: `git commit -m "feat: add DAG edge deletion with click-to-select"`

---

## Task 5: Session detail metadata header (item 3.5)

The session detail view (`/sessions/:sessionId`) shows only a terminal — no metadata about what session is running.

**Files:**
- Create: `web/src/components/terminal/SessionHeader.tsx`
- Modify: `web/src/App.tsx:49-58` (TerminalRoute component)

### Steps

- [ ] **Step 1:** Read `App.tsx` TerminalRoute (lines 49-58) to understand current structure. Read `web/src/hooks/useSessions.ts` for the `useSession(id)` hook.

- [ ] **Step 2:** Create `SessionHeader.tsx`:
  - Compact bar above terminal: command, machine name, model badge (if set), working directory, created time, duration
  - "Back to sessions" button (ArrowLeft icon + text)
  - Session ID with copy-to-clipboard
  - "Terminate" button (only for running/created sessions)
  - Responsive: stack on mobile, inline on desktop

- [ ] **Step 3:** Update TerminalRoute in App.tsx to render SessionHeader above TerminalView. Use `useSession(sessionId)` to fetch session data.

- [ ] **Step 4:** Write test for SessionHeader component.

- [ ] **Step 5:** Run tests and commit: `git commit -m "feat: add session detail metadata header with back/terminate buttons"`

---

## Task 6: Fix multiview workspace creation (item 3.6)

SessionPicker may have data loading or click handler issues. May self-resolve after Phase 1 machine rename fix.

**Files:**
- Debug: `web/src/components/multiview/SessionPicker.tsx`
- Debug: `web/src/components/multiview/MultiviewPage.tsx`

### Steps

- [ ] **Step 1:** Read both files thoroughly. Trace the flow: New Workspace button → setPickerTarget('__create__') → SessionPicker opens → sessions loaded via useSessions({status:'running'}) → user clicks session → onSelect called → createScratchWorkspace.

- [ ] **Step 2:** Check if sessions are loading (is the hook firing? is the API returning data?). Check if machine names display correctly (after Phase 1 fix). Check if click handlers are wired to the session items.

- [ ] **Step 3:** Fix whatever is broken. Common issues: sessions filter not matching any running sessions, machine names showing as empty, click handlers not firing due to disabled state.

- [ ] **Step 4:** Add a test that renders SessionPicker with mock sessions and verifies clicking a session calls onSelect.

- [ ] **Step 5:** Run tests and commit: `git commit -m "fix: multiview workspace creation now works with session picker"`

---

## Task 7: Wire LaunchTemplateModal (item 3.7)

The LaunchTemplateModal component exists but is dead code — never imported in TemplatesPage.

**Files:**
- Modify: `web/src/views/TemplatesPage.tsx`

### Steps

- [ ] **Step 1:** Read `web/src/components/templates/LaunchTemplateModal.tsx` to understand its props (templateId, isOpen, onClose, onLaunch).

- [ ] **Step 2:** Read `web/src/views/TemplatesPage.tsx` to find where to add the launch action. Add a Play icon button to each template row (next to existing duplicate/delete buttons).

- [ ] **Step 3:** Add state for `launchTemplateId` and render LaunchTemplateModal conditionally. On launch success, navigate to the new session.

- [ ] **Step 4:** Run tests and commit: `git commit -m "feat: wire LaunchTemplateModal into templates page"`

---

## Task 8: Remove template noise fields (item 3.8)

Remove terminal rows/cols and tags from the template creation form UI.

**Files:**
- Modify: `web/src/components/templates/TemplateForm.tsx:25-27` (remove state), `~261-315` (remove fields)
- Modify: `web/src/views/TemplatesPage.tsx` (remove tag filter if present)

### Steps

- [ ] **Step 1:** Read TemplateForm.tsx fully. Find and remove: terminal rows/cols inputs, tags input, and their state declarations. Keep the backend API fields in the submission payload with default values (24/80) so existing templates still work.

- [ ] **Step 2:** Read TemplatesPage.tsx. If there's a tag filter dropdown, remove it.

- [ ] **Step 3:** Run tests and commit: `git commit -m "fix: remove terminal rows/cols and tags from template form"`

---

## Task 9: Credentials graceful degradation (item 3.9)

Phase 1 auto-generates the encryption key, so the 503 should rarely trigger. But as defense-in-depth, the backend should allow plaintext storage when no key is configured, and the frontend should show a clear warning.

**Files:**
- Modify: `internal/server/handler/credentials.go` (fall back to plaintext when no encryption key)
- Modify: `internal/server/store/credentials.go` (add plaintext mode)
- Modify: `web/src/views/CredentialsPage.tsx` (show warning banner when encryption disabled)
- Modify: `web/src/api/credentials.ts` (add status check function)

### Steps

- [ ] **Step 1:** Read `internal/server/handler/credentials.go` to understand the current 503 pattern. Read `internal/server/store/credentials.go` to understand how encryption is used.

- [ ] **Step 2:** Add `GET /api/v1/credentials/status` endpoint returning `{ "encryption_enabled": bool }`. This lets the frontend check encryption state.

- [ ] **Step 3:** Modify the credential handler to accept a nil/no-op encryptor. When no encryption key is configured, credentials should be stored/retrieved as plaintext (no encrypt/decrypt calls). Remove the 503 returns — allow all CRUD operations to work.

- [ ] **Step 4:** On the frontend, CredentialsPage calls the status endpoint on mount. If encryption is disabled, show a warning banner: "Credentials are stored without encryption. This is unusual — check server logs." All CRUD operations work normally regardless.

- [ ] **Step 5:** Write handler test verifying credentials work both with and without encryption.

- [ ] **Step 6:** Run tests and commit: `git commit -m "fix: credentials work without encryption key (plaintext fallback with warning)"`

---

## Task 10: Machine delete/deregister (item 3.10)

Soft-delete machines with `deleted_at` column.

**Files:**
- Modify: `internal/server/store/migrations.go` (add migration for deleted_at column)
- Modify: `internal/server/store/machines.go` (add SoftDeleteMachine, filter ListMachines)
- Modify: `internal/server/api/machine_handler.go` (add DeleteMachine handler)
- Modify: `internal/server/api/router.go` (register DELETE route)
- Modify: `web/src/components/machines/MachineCard.tsx` (add Remove button for disconnected machines)
- Modify: `web/src/api/machines.ts`, `web/src/hooks/useMachines.ts`
- Test: store and handler tests

### Steps

- [ ] **Step 1:** Add migration: `ALTER TABLE machines ADD COLUMN deleted_at TIMESTAMP`

- [ ] **Step 2:** Add `SoftDeleteMachine(machineID)` store method — sets `deleted_at = CURRENT_TIMESTAMP`. Returns 409 if machine status is 'connected'. Update `ListMachines` to filter `WHERE deleted_at IS NULL`.

- [ ] **Step 3:** Add `DeleteMachine` handler (admin-only). Returns 409 if connected, 204 on success. Register `r.Delete("/machines/{machineID}", h.DeleteMachine)`.

- [ ] **Step 4:** Add frontend API function, mutation hook, and Remove button on MachineCard (visible only when disconnected). ConfirmDialog before deletion.

- [ ] **Step 5:** Write store + handler tests.

- [ ] **Step 6:** Run all tests and commit: `git commit -m "feat: add machine soft-delete with admin-only endpoint"`

---

## Task 11: User password change (item 3.11)

No password change capability exists.

**Files:**
- Modify: `internal/server/handler/users.go` (add ChangePassword, AdminResetPassword handlers)
- Modify: `internal/server/store/users.go` (add UpdatePassword method)
- Modify: `internal/server/api/router.go` (register routes — POST /users/me/password BEFORE {userID} routes)
- Modify: `web/src/views/SettingsPage.tsx` (add Account tab)
- Modify: `web/src/views/AdminPage.tsx` (add Reset Password action)
- Modify: `web/src/api/users.ts`, `web/src/hooks/useUsers.ts`
- Test: handler tests

### Steps

- [ ] **Step 1:** Add `UpdatePassword(userID, newPasswordHash)` store method in `users.go`. The hashing itself lives in `internal/server/auth/auth.go` — use the existing `HashPassword` function from that package in the handler.

- [ ] **Step 2:** Add two handler endpoints:
  - `POST /api/v1/users/me/password` — requires `{ "current_password", "new_password" }`. Verifies current password, hashes new, updates. Min 8 chars.
  - `POST /api/v1/users/{userID}/reset-password` — admin-only. Requires `{ "new_password" }`. No current password needed.
  - CRITICAL: Register `/users/me/password` BEFORE `/{userID}` routes in router.go to prevent Chi from matching "me" as a userID.

- [ ] **Step 3:** Add Account tab to SettingsPage. The tab system uses a `TABS` array constant and `TabId` union type — add `{ id: 'account', label: 'Account', icon: User }` to the array, add `'account'` to the TabId type, and add a new render branch in the tab content section. The tab should contain a password change form (current password, new password, confirm new password).

- [ ] **Step 4:** Add "Reset Password" action to AdminPage user rows.

- [ ] **Step 5:** Write handler tests for both endpoints (success, wrong current password, too short, admin-only enforcement).

- [ ] **Step 6:** Run all tests and commit: `git commit -m "feat: add user password change and admin reset"`

---

## Task 12: User profile self-edit (item 3.12)

Users can't update their own display name.

**Files:**
- Modify: `internal/server/handler/users.go` (add UpdateProfile handler)
- Modify: `internal/server/api/router.go` (register PUT /users/me)
- Modify: `web/src/views/SettingsPage.tsx` (Account tab — add display name edit)

### Steps

- [ ] **Step 1:** Add `PUT /api/v1/users/me` handler — accepts `{ "display_name" }`. Updates own user record. Register BEFORE `/{userID}` routes.

- [ ] **Step 2:** In the Account tab (from Task 11), add display name field (editable), email (read-only), role (read-only).

- [ ] **Step 3:** Write handler test. Run all tests.

- [ ] **Step 4:** Commit: `git commit -m "feat: add user profile self-edit (display name)"`

---

## Task 13: Webhook secret reveal on creation (item 3.13)

After creating a webhook, the secret is never shown to the user.

**Files:**
- Modify: `web/src/components/webhooks/WebhookForm.tsx` or parent (WebhooksPage.tsx)

### Steps

- [ ] **Step 1:** Read WebhookForm.tsx and its parent to understand the creation flow. Find where `onSubmit` is called and how the response is handled.

- [ ] **Step 2:** After successful webhook creation, if a secret was provided, show a one-time modal: "Save your webhook secret — it won't be shown again" with the secret in a copyable field. Pattern: same as CreateKeyModal for API keys (check `web/src/components/admin/CreateKeyModal.tsx` for reference).

- [ ] **Step 3:** Run tests and commit: `git commit -m "feat: show webhook secret one-time after creation"`

---

## Task 14: Trigger edit capability (item 3.14)

Triggers can only be created and deleted, not edited.

**Files:**
- Modify: `internal/server/handler/triggers.go` (add UpdateTrigger handler)
- Modify: `internal/server/store/triggers.go` (add UpdateTrigger store method)
- Modify: `internal/server/api/router.go` (register PUT route)
- Modify: `web/src/components/jobs/TriggerPanel.tsx` (add Edit button)
- Modify: `web/src/components/jobs/TriggerBuilder.tsx` (support edit mode with prefilled values)
- Test: handler/store tests

### Steps

- [ ] **Step 1:** Read store/triggers.go to understand the trigger schema. Add `UpdateTrigger(ctx, triggerID, eventType, filter string)` — deliberately excludes `enabled` (that's the toggle endpoint in Task 15).

- [ ] **Step 2:** Add `PUT /api/v1/triggers/{triggerID}` handler. Register in router.

- [ ] **Step 3:** Add Edit button to trigger cards in TriggerPanel. Opens TriggerBuilder pre-filled with current values.

- [ ] **Step 4:** Modify TriggerBuilder to accept optional `editingTrigger` prop for prefilling.

- [ ] **Step 5:** Write tests. Commit: `git commit -m "feat: add trigger edit capability"`

---

## Task 15: Trigger enable/disable toggle (item 3.15)

The `enabled` field exists in DB but there's no way to toggle it.

**Files:**
- Modify: `internal/server/handler/triggers.go` (add ToggleTrigger handler)
- Modify: `internal/server/store/triggers.go` (add ToggleTrigger store method)
- Modify: `internal/server/api/router.go` (register POST route)
- Modify: `web/src/components/jobs/TriggerPanel.tsx` (add Pause/Play toggle)

### Steps

- [ ] **Step 1:** Add `ToggleTrigger(ctx, triggerID) error` store method — reads current `enabled`, flips it, updates.

- [ ] **Step 2:** Add `POST /api/v1/triggers/{triggerID}/toggle` handler. Returns updated trigger.

- [ ] **Step 3:** Add Pause/Play icon button on trigger cards. Disabled triggers show muted styling (50% opacity).

- [ ] **Step 4:** Write tests. Commit: `git commit -m "feat: add trigger enable/disable toggle"`

---

## Final Verification

- [ ] **Run complete CI check**

```bash
go vet ./...
go test -race ./...

cd web && npx tsc -b && npx vitest run && npx vite build
```

All must pass.

- [ ] **Create PR for Phase 3**

Branch name: `fix/phase3-broken-ui-missing-crud`
PR title: "fix: Phase 3 — Broken UI & missing CRUD (15 items across 7 domains)"

---

## Review Corrections Checklist

Apply these during implementation:

1. **Job delete button — backend already exists.** Do NOT create a new delete endpoint. The handler at `handler/jobs.go:380-396` and store method at `store/jobs.go:399-427` already work. Frontend only.

2. **Job clone — transaction safety.** The CloneJob store method MUST use a transaction. If step creation fails mid-clone, the partial job must be rolled back.

3. **ParameterEditor — may already work.** The code looks correct. Before writing a fix, verify the bug is reproducible. It might have been fixed by a parent component change in Phase 1. If it works, skip this task and note it as resolved.

4. **DAG edge deletion — read-only guard.** Edge deletion must ONLY be available in edit mode (JobEditor), NOT in run view (RunDetail's RunDAGView). Check the `isEditMode` or similar prop.

5. **Session header — don't block terminal rendering.** The header should render immediately with whatever data is available. Use `useSession()` for metadata but don't gate the terminal on it. Show "Loading..." for metadata fields while fetching.

6. **Multiview SessionPicker — may already work.** After Phase 1 fixed machine display names, the SessionPicker might work correctly now. Test first before changing code.

7. **Password change — Chi routing order.** `POST /api/v1/users/me/password` MUST be registered BEFORE `/{userID}` routes. Chi matches in registration order. If "me" hits the `{userID}` handler, it will try to parse "me" as a user ID and return 404.

8. **Credentials — Phase 1 already fixed the root cause.** Auto-generated encryption key means the 503 should never trigger. This task is defense-in-depth only. Don't over-engineer — a clear error message is sufficient.

9. **Trigger edit vs toggle separation.** PUT updates config (event type, filter). Toggle flips enabled. Keep them as separate endpoints — users will toggle frequently but edit rarely.

10. **Use Phase 2 test infrastructure.** All new Go tests should use `testutil.MustCreate*` factories. All new frontend tests should use `renderWithProviders` from `web/src/test/render.tsx` and MSW handler overrides.
