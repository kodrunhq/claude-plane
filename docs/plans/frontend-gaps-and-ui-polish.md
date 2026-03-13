# Frontend Gaps and UI Polish Implementation Plan

## Part 1: Backend Features Missing from Frontend

### Task 1: User Management / Admin Panel (XL)

**Current State:** Backend has `SeedAdmin`, `CreateUser`, `GetUserByEmail` in `store/users.go`. Has `POST /api/v1/auth/register` but no user listing or management endpoints.

**Files to Create:**
- `web/src/api/users.ts` — API wrapper for user operations
- `web/src/hooks/useUsers.ts` — React Query hooks
- `web/src/views/AdminPage.tsx` — Main admin dashboard
- `web/src/components/admin/UsersList.tsx` — User table with sorting/filtering
- `web/src/components/admin/CreateUserModal.tsx` — Modal to create new users
- `web/src/components/admin/EditUserModal.tsx` — Modal to edit user roles/details
- `web/src/types/user.ts` — User TypeScript types

**Files to Modify:**
- `web/src/components/layout/Sidebar.tsx` — Add "Admin" nav item (admin-only)
- `web/src/App.tsx` — Add `/admin` route with role guard

**Backend Endpoints Needed (NEW):**
- `internal/server/handler/users.go`:
  - `GET /api/v1/users` — List all users (admin-only)
  - `POST /api/v1/users` — Create user (admin-only)
  - `PUT /api/v1/users/{userID}` — Update user role/display name (admin-only)
  - `DELETE /api/v1/users/{userID}` — Delete user (admin-only)
- `internal/server/store/users.go` — Add: `ListUsers()`, `GetUserByID()`, `UpdateUser()`, `DeleteUser()`

**Acceptance Criteria:**
- [ ] Admin can list all users with email, display name, role, created date
- [ ] Admin can create users with email, password, display name, role
- [ ] Admin can change user roles (admin <-> user)
- [ ] Admin can delete users (with confirmation dialog)
- [ ] Non-admin users cannot access `/admin`

**Dependencies:** None (first task)

---

### Task 2: Provisioning UI (L)

**Current State:** `POST /api/v1/provision/agent` (admin-only) creates provisioning token with one-time curl command. `GET /api/v1/provision/{token}/script` serves install script.

**Files to Create:**
- `web/src/api/provisioning.ts` — API wrapper: `createProvisioningToken(machineID, os, arch)`
- `web/src/hooks/useProvisioning.ts` — React Query hooks
- `web/src/views/ProvisioningPage.tsx` — Admin page for token management
- `web/src/components/provisioning/TokenGenerator.tsx` — Create token + copy curl command
- `web/src/components/provisioning/TokensList.tsx` — Table of tokens (active/expired/redeemed)

**Backend Endpoints Needed (NEW):**
- `GET /api/v1/provision/tokens` — List all tokens with status
- `DELETE /api/v1/provision/tokens/{tokenID}` — Revoke a token

**Files to Modify:**
- `web/src/components/layout/Sidebar.tsx` — Add "Provisioning" under Admin
- `web/src/App.tsx` — Add `/admin/provisioning` route

**Acceptance Criteria:**
- [ ] Admin can generate provisioning tokens with machine ID, OS, architecture
- [ ] Display curl command with copy-to-clipboard button
- [ ] List shows token status (active/expired/redeemed), expiry countdown
- [ ] Can revoke unused tokens

**Dependencies:** Task 1 (admin nav structure)

---

### Task 3: Webhook Management (M)

**Current State:** Full CRUD + delivery history exists in backend (6 endpoints). Zero frontend code.

**Files to Create:**
- `web/src/api/webhooks.ts` — API wrapper: `list()`, `get(id)`, `create()`, `update()`, `delete()`, `listDeliveries(id)`
- `web/src/hooks/useWebhooks.ts` — React Query hooks
- `web/src/views/WebhooksPage.tsx` — Main webhook management page
- `web/src/components/webhooks/WebhooksList.tsx` — Table of webhooks
- `web/src/components/webhooks/WebhookForm.tsx` — Create/edit form (name, URL, events multiselect, secret)
- `web/src/components/webhooks/DeliveryHistory.tsx` — Delivery logs (status, timestamp, response code)
- `web/src/types/webhook.ts` — TypeScript types

**Files to Modify:**
- `web/src/components/layout/Sidebar.tsx` — Add "Webhooks" nav item
- `web/src/App.tsx` — Add `/webhooks` route

**Acceptance Criteria:**
- [ ] Full CRUD for webhooks
- [ ] Toggle enabled/disabled status
- [ ] View delivery history per webhook with expandable request/response
- [ ] Event types multiselect

**Dependencies:** None

---

### Task 4: Trigger Management (M)

**Current State:** CRUD endpoints exist in backend (3 endpoints). Zero frontend code.

**Files to Create:**
- `web/src/api/triggers.ts` — API wrapper: `listByJob(jobID)`, `create(jobID, params)`, `delete(triggerID)`
- `web/src/hooks/useTriggers.ts` — React Query hooks
- `web/src/components/jobs/TriggerPanel.tsx` — Panel in job editor showing triggers
- `web/src/components/jobs/TriggerBuilder.tsx` — Create trigger modal
- `web/src/types/trigger.ts` — TypeScript types

**Files to Modify:**
- `web/src/views/JobEditor.tsx` — Add TriggerPanel below step editor

**Acceptance Criteria:**
- [ ] In job editor, show "Triggers" section with current triggers
- [ ] Create trigger: event type dropdown, filter expression, enabled toggle
- [ ] Delete trigger with confirmation

**Dependencies:** None

---

### Task 5: Event History / Audit Log (M)

**Current State:** `GET /api/v1/events` exists with filtering (type, since, limit, offset). Zero frontend code.

**Files to Create:**
- `web/src/api/events.ts` — API wrapper: `list(params)`
- `web/src/hooks/useEvents.ts` — React Query hook with pagination
- `web/src/views/EventsPage.tsx` — Event log viewer
- `web/src/components/events/EventsTable.tsx` — Table with expandable JSON payloads
- `web/src/components/events/EventFilters.tsx` — Filters: type pattern, date range, pagination
- `web/src/types/event.ts` — TypeScript types

**Files to Modify:**
- `web/src/components/layout/Sidebar.tsx` — Add "Events" nav item
- `web/src/App.tsx` — Add `/events` route

**Acceptance Criteria:**
- [ ] Table shows: event type, timestamp, source, payload preview
- [ ] Expandable rows for full JSON payload
- [ ] Filter by event type pattern and date range
- [ ] Pagination with limit selector

**Dependencies:** None

---

### Task 6: Credentials/Secrets Management (XL)

**Current State:** DB table exists (`credentials`) but zero store methods, zero handlers, zero API, zero UI.

**Backend Implementation Needed:**
- `internal/server/store/credentials.go` — Store methods: `CreateCredential`, `ListCredentialsByUser`, `GetCredential`, `UpdateCredential`, `DeleteCredential`
- `internal/server/handler/credentials.go` — HTTP handlers for CRUD
- Encryption: AES-256-GCM with nonce stored alongside encrypted value

**Frontend Files to Create:**
- `web/src/api/credentials.ts` — API wrapper
- `web/src/hooks/useCredentials.ts` — React Query hooks
- `web/src/views/CredentialsPage.tsx` — Credentials manager
- `web/src/components/credentials/CredentialsList.tsx` — Table (names only, never show values)
- `web/src/components/credentials/CreateCredentialModal.tsx` — Create form

**Acceptance Criteria:**
- [ ] User can create credential with name and secret value
- [ ] Values stored encrypted in DB (AES-256-GCM)
- [ ] List shows names but never plaintext values
- [ ] Delete with confirmation

**Dependencies:** Task 1 (user context for per-user credentials)

---

## Part 2: UI/UX Improvements

### Task 7: Dashboard Improvements (M)

**Modify:** `web/src/views/CommandCenter.tsx`

**Add:**
- Recent jobs list (last 5 with status)
- Recent runs (last 5 with status)
- Quick stats: total jobs, total runs, completion rate %
- Upcoming scheduled jobs (next 3 cron triggers)

---

### Task 8: Navigation Improvements (S)

**Modify:** `web/src/components/layout/Sidebar.tsx`

**Add sections:**
- Core: Command Center, Sessions, Machines, Jobs, Runs
- Automation: Webhooks, Schedules
- Monitoring: Events, Credentials
- Admin (role-gated): Users, Provisioning

---

### Task 9: Visual Polish (S)

**Modify:** `web/src/styles/globals.css` and all card/button components

- Subtle box shadows on cards, hover:shadow-lg transitions
- Gradient headers
- Consistent button styles (primary/danger/secondary)
- Alternate row colors in tables
- Smooth hover transitions

---

### Task 10: Empty States & Loading States (M)

**Create:**
- `web/src/components/shared/Skeleton.tsx` — Reusable skeleton with size variants
- `web/src/components/shared/SkeletonTable.tsx` — Skeleton matching table rows

**Modify:** All data-fetching views — replace manual skeletons with shared components

---

### Task 11: Status Indicators (M)

**Modify:** `web/src/components/shared/StatusBadge.tsx`
- Add icon support (checkmark, X, spinner)
- Pulse animation for "running" status
- Size variants: sm, md, lg

**Create:** `web/src/lib/statusColors.ts` — Centralized status->color mapping

---

## Dependency Order

**Phase 1 (Parallel):** Tasks 3, 4, 5 (wrap existing backend), Tasks 8, 9, 10, 11 (UI polish)
**Phase 2:** Task 1 (admin panel — needs new backend endpoints)
**Phase 3 (Parallel):** Tasks 2, 7 (depend on admin nav)
**Phase 4:** Task 6 (credentials — needs encryption layer)

## Estimated Effort

| Task | Complexity | Est. Days |
|------|-----------|----------|
| 1. User Management | XL | 4-5 |
| 2. Provisioning UI | L | 2 |
| 3. Webhooks | M | 2 |
| 4. Triggers | M | 2 |
| 5. Events | M | 2 |
| 6. Credentials | XL | 5-6 |
| 7. Dashboard | M | 2 |
| 8. Navigation | S | 1 |
| 9. Visual Polish | S | 2 |
| 10. Empty/Loading States | M | 2 |
| 11. Status Indicators | M | 2 |
| **Total** | — | **26-32 days** |
