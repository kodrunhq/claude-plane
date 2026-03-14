# Phase 1: Session Templates — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add reusable session templates — named execution contexts that capture command, args, working directory, env vars, initial prompt, and terminal size — with full CRUD API, template-aware session creation, and complete frontend UI.

**Architecture:** New `session_templates` table + store + handler following existing patterns. Session creation handler gains template merge logic with `${VAR_NAME}` variable substitution. Frontend adds template cards to Command Center, template editor page, and "From Template" dropdown in session creation modal.

**Tech Stack:** Go (store, handler, Chi routes), SQLite migration, React 19, TypeScript, TanStack Query, Tailwind CSS

**Design Spec:** `docs/superpowers/specs/2026-03-14-v2-templates-injection-bridge-design.md` — Sections 2, 3

---

## File Map

### Backend — Create
| File | Responsibility |
|------|---------------|
| `internal/server/store/templates.go` | Template CRUD store methods + `TemplateStoreIface` interface |
| `internal/server/store/templates_test.go` | Store-level tests |
| `internal/server/handler/templates.go` | Template REST handler |
| `internal/server/handler/templates_test.go` | Handler-level tests |

### Backend — Modify
| File | Change |
|------|--------|
| `internal/server/store/migrations.go` | Add migration v5: `session_templates` table + `sessions.template_id` column |
| `internal/server/session/handler.go` | Add template merge logic to `CreateSession`, wire `EnvVars`/`InitialPrompt` into `CreateSessionCmd` proto |
| `internal/server/session/handler_test.go` | Tests for template-aware session creation (create if needed) |
| `internal/server/api/router.go` | Register template routes, add `templateHandler` to `NewRouter` |
| `internal/server/event/event.go` | Add `TypeTemplateCreated`, `TypeTemplateUpdated`, `TypeTemplateDeleted` constants |
| `internal/server/event/builders.go` | Add `NewTemplateEvent` builder |

### Frontend — Create
| File | Responsibility |
|------|---------------|
| `web/src/types/template.ts` | `SessionTemplate` TypeScript interface |
| `web/src/api/templates.ts` | API client functions |
| `web/src/hooks/useTemplates.ts` | TanStack Query hooks |
| `web/src/views/TemplatesPage.tsx` | Template list view |
| `web/src/views/TemplateEditor.tsx` | Template create/edit view |
| `web/src/components/templates/TemplateCard.tsx` | Card component for Command Center grid |
| `web/src/components/templates/TemplateForm.tsx` | Form component for editor |
| `web/src/components/templates/LaunchTemplateModal.tsx` | Launch modal with variable inputs + machine selector |
| `web/src/components/templates/TemplatePicker.tsx` | "From Template" dropdown for session creation modal |

### Frontend — Modify
| File | Change |
|------|--------|
| `web/src/App.tsx` | Add `/templates`, `/templates/new`, `/templates/:id/edit` routes |
| `web/src/components/layout/Sidebar.tsx` | Add "Templates" nav item |
| `web/src/views/CommandCenter.tsx` | Add template card grid section |
| `web/src/components/sessions/CreateSessionModal.tsx` | Add "From Template" dropdown (create if needed) |

---

## Chunk 1: Backend Store + Migration

### Task 1: Database Migration

**Files:**
- Modify: `internal/server/store/migrations.go`

- [ ] **Step 1: Write the migration SQL**

Add migration v5 to the `migrations` slice. Creates `session_templates` table and adds `template_id` column to `sessions`.

```go
{
    Version: 5,
    SQL: `
CREATE TABLE session_templates (
    template_id     TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(user_id),
    name            TEXT NOT NULL,
    description     TEXT,
    command         TEXT,
    args            TEXT,
    working_dir     TEXT,
    env_vars        TEXT,
    initial_prompt  TEXT,
    terminal_rows   INTEGER NOT NULL DEFAULT 24,
    terminal_cols   INTEGER NOT NULL DEFAULT 80,
    tags            TEXT,
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    deleted_at      DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);
CREATE INDEX idx_templates_user ON session_templates(user_id, deleted_at);

ALTER TABLE sessions ADD COLUMN template_id TEXT REFERENCES session_templates(template_id);
    `,
},
```

- [ ] **Step 2: Run tests to verify migration applies**

Run: `go test -race ./internal/server/store/ -run TestMigrations -v`
Expected: PASS (existing migration tests should cover sequential application)

- [ ] **Step 3: Commit**

```
git add internal/server/store/migrations.go
git commit -m "feat: add migration v5 for session_templates table"
```

---

### Task 2: Template Store — Struct, Interface, Create, Get

**Files:**
- Create: `internal/server/store/templates.go`
- Create: `internal/server/store/templates_test.go`

- [ ] **Step 1: Write failing tests for CreateTemplate and GetTemplate**

Test cases:
- Create a template with all fields, verify it returns with generated ID and timestamps
- Create two templates with same name + same user, expect unique constraint error
- Create two templates with same name + different users, expect success
- GetTemplate by ID, expect match
- GetTemplate with non-existent ID, expect `ErrNotFound`

Run: `go test -race ./internal/server/store/ -run TestTemplate -v`
Expected: FAIL (templates.go doesn't exist)

- [ ] **Step 2: Implement SessionTemplate struct, TemplateStoreIface, CreateTemplate, GetTemplate**

In `templates.go`:
- Define `SessionTemplate` struct with JSON tags (see spec section 3.2)
- Define `TemplateStoreIface` interface (see spec section 3.6)
- Define `ListTemplateOptions` struct with `Tag` and `Name` filter fields
- Implement `CreateTemplate`: generate UUID, marshal `Args`/`Tags` as JSON arrays, `EnvVars` as JSON object, INSERT, return hydrated struct
- Implement `GetTemplate`: SELECT with `deleted_at IS NULL` check, unmarshal JSON fields, return `ErrNotFound` if missing or soft-deleted
- Use `s.writer` for writes, `s.reader` for reads (existing pattern)
- Use `sql.NullString` for nullable fields (existing pattern)

- [ ] **Step 3: Run tests**

Run: `go test -race ./internal/server/store/ -run TestTemplate -v`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/server/store/templates.go internal/server/store/templates_test.go
git commit -m "feat: add template store with Create and Get methods"
```

---

### Task 3: Template Store — GetByName, List, Update, Delete, Clone

**Files:**
- Modify: `internal/server/store/templates.go`
- Modify: `internal/server/store/templates_test.go`

- [ ] **Step 1: Write failing tests**

Test cases:
- `GetTemplateByName`: look up by user_id + name, expect match; look up non-existent name, expect `ErrNotFound`; look up soft-deleted template by name, expect `ErrNotFound`
- `ListTemplates`: create 3 templates (2 for user A, 1 for user B), list for user A expects 2; filter by tag expects subset; filter by `?name=exact` expects 1
- `UpdateTemplate`: update all fields, verify `updated_at` changes; update soft-deleted template, expect `ErrNotFound`
- `DeleteTemplate`: soft delete sets `deleted_at`; deleted template excluded from List and Get
- `CloneTemplate`: creates copy with `-copy` suffix; clone when `-copy` exists creates `-copy-2`; clone of non-existent template returns `ErrNotFound`

- [ ] **Step 2: Implement remaining store methods**

- `GetTemplateByName`: SELECT WHERE `user_id = ? AND name = ? AND deleted_at IS NULL`
- `ListTemplates`: SELECT with optional `tag` filter (JSON `LIKE '%"tagvalue"%'`) and `name` exact filter. WHERE `user_id = ? AND deleted_at IS NULL`. Admin mode: if `userID` is empty, return all non-deleted.
- `UpdateTemplate`: UPDATE all fields. Check `deleted_at IS NULL` in WHERE. Return `ErrNotFound` if 0 rows affected.
- `DeleteTemplate`: UPDATE `deleted_at = CURRENT_TIMESTAMP` WHERE `template_id = ? AND deleted_at IS NULL`. Return `ErrNotFound` if 0 rows.
- `CloneTemplate`: get original, generate new name with `-copy` suffix (retry up to 10 with `-copy-2`, `-copy-3`...), insert copy with new UUID.

- [ ] **Step 3: Run tests**

Run: `go test -race ./internal/server/store/ -run TestTemplate -v`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/server/store/templates.go internal/server/store/templates_test.go
git commit -m "feat: add template store List, Update, Delete, Clone methods"
```

---

## Chunk 2: Backend Handler + Session Merge

### Task 4: Template Event Types

**Files:**
- Modify: `internal/server/event/event.go`
- Modify: `internal/server/event/builders.go`

- [ ] **Step 1: Add event type constants**

In `event.go`, add to the constants block:
```go
// Template lifecycle events.
TypeTemplateCreated = "template.created"
TypeTemplateUpdated = "template.updated"
TypeTemplateDeleted = "template.deleted"
```

- [ ] **Step 2: Add builder function**

In `builders.go`, add:
```go
// NewTemplateEvent constructs an event for template lifecycle changes.
func NewTemplateEvent(eventType, templateID, userID string) Event {
    return newEvent(eventType, "template", map[string]any{
        "template_id": templateID,
        "user_id":     userID,
    })
}
```

- [ ] **Step 3: Run existing event tests**

Run: `go test -race ./internal/server/event/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/server/event/event.go internal/server/event/builders.go
git commit -m "feat: add template lifecycle event types and builder"
```

---

### Task 5: Template Handler — CRUD Endpoints

**Files:**
- Create: `internal/server/handler/templates.go`
- Create: `internal/server/handler/templates_test.go`

- [ ] **Step 1: Write failing tests for CRUD endpoints**

Follow the pattern in `handler/jobs_test.go`. Create a mock `TemplateStoreIface`. Test:
- `POST /api/v1/templates` — valid request returns 201; missing name returns 400; duplicate name returns 409
- `GET /api/v1/templates` — returns user's templates; admin sees all; supports `?tag=` and `?name=` filters
- `GET /api/v1/templates/{templateID}` — returns template; non-existent returns 404; other user's template returns 404 (unless admin)
- `PUT /api/v1/templates/{templateID}` — updates and returns 200; non-owner returns 404
- `DELETE /api/v1/templates/{templateID}` — soft deletes, returns 204; non-owner returns 404
- `POST /api/v1/templates/{templateID}/clone` — returns 201 with cloned template

Run: `go test -race ./internal/server/handler/ -run TestTemplate -v`
Expected: FAIL

- [ ] **Step 2: Implement TemplateHandler**

In `templates.go`:
```go
type TemplateHandler struct {
    store     store.TemplateStoreIface
    getClaims ClaimsGetter
    publisher event.Publisher
}

func NewTemplateHandler(s store.TemplateStoreIface, getClaims ClaimsGetter) *TemplateHandler {
    return &TemplateHandler{store: s, getClaims: getClaims}
}

func (h *TemplateHandler) SetPublisher(p event.Publisher) {
    h.publisher = p
}

func RegisterTemplateRoutes(r chi.Router, h *TemplateHandler) {
    r.Post("/api/v1/templates", h.Create)
    r.Get("/api/v1/templates", h.List)
    r.Get("/api/v1/templates/{templateID}", h.Get)
    r.Put("/api/v1/templates/{templateID}", h.Update)
    r.Delete("/api/v1/templates/{templateID}", h.Delete)
    r.Post("/api/v1/templates/{templateID}/clone", h.Clone)
}
```

Implement each handler method following `handler/jobs.go` patterns:
- Extract claims via `h.getClaims(r)`
- Authorization: owner or admin
- JSON decode request body
- Validate required fields (`name` for create)
- Validate `initial_prompt` variable names match `[A-Z][A-Z0-9_]*` regex
- Call store methods
- Publish events
- Return JSON response

- [ ] **Step 3: Run tests**

Run: `go test -race ./internal/server/handler/ -run TestTemplate -v`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/server/handler/templates.go internal/server/handler/templates_test.go
git commit -m "feat: add template handler with CRUD endpoints"
```

---

### Task 6: Template-Aware Session Creation

**Files:**
- Modify: `internal/server/session/handler.go`
- Create or modify: `internal/server/session/handler_test.go`

- [ ] **Step 1: Write failing tests for template merge in CreateSession**

Test cases:
- Create session with `template_id` — template fields used as defaults
- Create session with `template_name` — looked up by name for user
- Create session with both `template_id` and explicit `command` — explicit overrides template
- Create session with template containing `${PR_URL}` in `initial_prompt` + `variables: {"PR_URL": "https://..."}` — substitution applied
- Create session with template that has `env_vars` — `EnvVars` populated in `CreateSessionCmd` proto
- Create session with template that has `initial_prompt` — `InitialPrompt` populated in proto
- Create session with invalid `template_id` — returns 404

- [ ] **Step 2: Modify createSessionRequest struct**

Add new fields to the request struct in `handler.go`:
```go
type createSessionRequest struct {
    MachineID    string            `json:"machine_id"`
    Command      string            `json:"command"`
    Args         []string          `json:"args"`
    WorkingDir   string            `json:"working_dir"`
    TerminalSize *terminalSize     `json:"terminal_size"`
    EnvVars      map[string]string `json:"env_vars"`
    InitialPrompt string           `json:"initial_prompt"`
    // Template fields
    TemplateID   string            `json:"template_id"`
    TemplateName string            `json:"template_name"`
    Variables    map[string]string `json:"variables"`
}
```

- [ ] **Step 3: Add template store dependency to SessionHandler**

Add `templateStore store.TemplateStoreIface` field to `SessionHandler`. Update `NewSessionHandler` constructor to accept it. This is needed to resolve templates by ID or name.

- [ ] **Step 4: Implement template merge logic in CreateSession**

After parsing the request body, before validation:
1. If `TemplateID` or `TemplateName` is set, resolve the template
2. For `TemplateName`, call `templateStore.GetTemplateByName(userID, name)`
3. Merge: for each field, use template value as default, request value as override (only if non-zero)
4. Variable substitution: for each key in `Variables`, replace `${KEY}` in `InitialPrompt` with value
5. Store `template_id` on the session record

- [ ] **Step 5: Wire EnvVars and InitialPrompt into CreateSessionCmd proto**

Update the `CreateSessionCmd` construction block (currently lines 135-147) to include:
```go
CreateSession: &pb.CreateSessionCmd{
    SessionId:     sessionID,
    Command:       req.Command,
    Args:          req.Args,
    WorkingDir:    req.WorkingDir,
    EnvVars:       req.EnvVars,        // NEW
    InitialPrompt: req.InitialPrompt,  // NEW
    TerminalSize: &pb.TerminalSize{
        Rows: req.TerminalSize.Rows,
        Cols: req.TerminalSize.Cols,
    },
},
```

- [ ] **Step 6: Run tests**

Run: `go test -race ./internal/server/session/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```
git add internal/server/session/handler.go internal/server/session/handler_test.go
git commit -m "feat: add template-aware session creation with merge and variable substitution"
```

---

### Task 7: Register Template Routes in Router

**Files:**
- Modify: `internal/server/api/router.go`

- [ ] **Step 1: Add templateHandler parameter to NewRouter**

Add `templateHandler *handler.TemplateHandler` to the `NewRouter` function signature. Register routes inside the JWT-protected group:

```go
if templateHandler != nil {
    handler.RegisterTemplateRoutes(r, templateHandler)
}
```

- [ ] **Step 2: Wire up in server main**

Wherever `NewRouter` is called (likely `cmd/server/main.go` or `cmd/server/serve.go`), instantiate `TemplateHandler` and pass it. Follow the existing pattern for `jobHandler`, `runHandler`, etc.

- [ ] **Step 3: Run full test suite**

Run: `go test -race ./...`
Expected: PASS (update any callers of `NewRouter` in tests)

- [ ] **Step 4: Commit**

```
git add internal/server/api/router.go cmd/server/
git commit -m "feat: register template routes in API router"
```

---

## Chunk 3: Frontend

### Task 8: Frontend Types + API Client

**Files:**
- Create: `web/src/types/template.ts`
- Create: `web/src/api/templates.ts`

- [ ] **Step 1: Create TypeScript types**

In `types/template.ts`:
```typescript
export interface SessionTemplate {
  template_id: string;
  user_id: string;
  name: string;
  description?: string;
  command?: string;
  args?: string[];
  working_dir?: string;
  env_vars?: Record<string, string>;
  initial_prompt?: string;
  terminal_rows: number;
  terminal_cols: number;
  tags?: string[];
  timeout_seconds: number;
  created_at: string;
  updated_at: string;
}

export interface CreateTemplateParams {
  name: string;
  description?: string;
  command?: string;
  args?: string[];
  working_dir?: string;
  env_vars?: Record<string, string>;
  initial_prompt?: string;
  terminal_rows?: number;
  terminal_cols?: number;
  tags?: string[];
  timeout_seconds?: number;
}
```

- [ ] **Step 2: Create API client**

In `api/templates.ts`, follow the pattern in `api/jobs.ts`:
```typescript
export const templatesApi = {
  list: (params?: { tag?: string; name?: string }) =>
    request<SessionTemplate[]>(`/templates${buildQuery(params)}`),
  get: (id: string) =>
    request<SessionTemplate>(`/templates/${encodeURIComponent(id)}`),
  getByName: (name: string) =>
    templatesApi.list({ name }).then(r => r[0] ?? null),
  create: (params: CreateTemplateParams) =>
    request<SessionTemplate>('/templates', { method: 'POST', body: JSON.stringify(params) }),
  update: (id: string, params: CreateTemplateParams) =>
    request<SessionTemplate>(`/templates/${encodeURIComponent(id)}`, {
      method: 'PUT', body: JSON.stringify(params),
    }),
  delete: (id: string) =>
    request<void>(`/templates/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  clone: (id: string) =>
    request<SessionTemplate>(`/templates/${encodeURIComponent(id)}/clone`, { method: 'POST' }),
};
```

- [ ] **Step 3: Commit**

```
git add web/src/types/template.ts web/src/api/templates.ts
git commit -m "feat: add template TypeScript types and API client"
```

---

### Task 9: TanStack Query Hooks

**Files:**
- Create: `web/src/hooks/useTemplates.ts`

- [ ] **Step 1: Create hooks**

Follow the pattern in `hooks/useJobs.ts`. Implement:
- `useTemplates(params?)` — query with key `['templates', params]`
- `useTemplate(id)` — query with key `['template', id]`
- `useCreateTemplate()` — mutation that invalidates `['templates']`
- `useUpdateTemplate()` — mutation that invalidates `['templates']` and `['template', id]`
- `useDeleteTemplate()` — mutation that invalidates `['templates']`
- `useCloneTemplate()` — mutation that invalidates `['templates']`

- [ ] **Step 2: Commit**

```
git add web/src/hooks/useTemplates.ts
git commit -m "feat: add TanStack Query hooks for templates"
```

---

### Task 10: Template Card + Command Center Integration

**Files:**
- Create: `web/src/components/templates/TemplateCard.tsx`
- Create: `web/src/components/templates/LaunchTemplateModal.tsx`
- Modify: `web/src/views/CommandCenter.tsx`

- [ ] **Step 1: Create TemplateCard component**

A card displaying: name, description (truncated), tags as badges, "Launch" button, "Edit" link, "Clone" action. Matches existing card styling in the Command Center.

- [ ] **Step 2: Create LaunchTemplateModal**

Modal that appears when "Launch" is clicked:
- Machine selector dropdown (from `useMachines()`)
- If template has `${VAR_NAME}` placeholders in `initial_prompt`, render input fields for each detected variable
- "Launch" button calls session creation API with `template_id`, selected `machine_id`, and `variables` map
- Parse variables from `initial_prompt` using regex: `/\$\{([A-Z][A-Z0-9_]*)\}/g`

- [ ] **Step 3: Add template card grid to CommandCenter**

In `CommandCenter.tsx`, add a section below the machine list:
- Use `useTemplates()` to fetch templates
- Render as a card grid using `TemplateCard`
- Show empty state if no templates

- [ ] **Step 4: Run frontend lint**

Run: `cd web && npm run lint`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add web/src/components/templates/ web/src/views/CommandCenter.tsx
git commit -m "feat: add template cards to Command Center with launch modal"
```

---

### Task 11: Template Editor Page

**Files:**
- Create: `web/src/components/templates/TemplateForm.tsx`
- Create: `web/src/views/TemplateEditor.tsx`
- Create: `web/src/views/TemplatesPage.tsx`

- [ ] **Step 1: Create TemplateForm component**

Form with fields for all template properties:
- `name` — text input (required)
- `description` — text input
- `command` — text input (placeholder: "claude")
- `args` — list editor: add/remove string items
- `working_dir` — text input
- `env_vars` — key-value pair editor: add/remove rows with key + value inputs
- `initial_prompt` — textarea with visual highlighting of `${VAR_NAME}` patterns (use a wrapper that shows placeholders in a different color or with a badge)
- `terminal_rows`, `terminal_cols` — number inputs
- `tags` — tag input (type + enter to add, click x to remove)
- `timeout_seconds` — number input
- Save and Cancel buttons

- [ ] **Step 2: Create TemplateEditor view**

- `/templates/new` — renders `TemplateForm` in create mode
- `/templates/:id/edit` — loads template via `useTemplate(id)`, renders `TemplateForm` in edit mode
- On save: calls `useCreateTemplate()` or `useUpdateTemplate()`, navigates to `/templates`

- [ ] **Step 3: Create TemplatesPage view**

List view for `/templates`:
- Table or card list of user's templates
- Each row: name, description, tags, command, actions (Edit, Clone, Delete, Launch)
- "New Template" button in header → navigates to `/templates/new`

- [ ] **Step 4: Commit**

```
git add web/src/components/templates/TemplateForm.tsx web/src/views/TemplateEditor.tsx web/src/views/TemplatesPage.tsx
git commit -m "feat: add template editor and list views"
```

---

### Task 12: Session Creation Modal + Routes + Navigation

**Files:**
- Create: `web/src/components/templates/TemplatePicker.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/components/layout/Sidebar.tsx`
- Modify: session creation modal (create if needed)

- [ ] **Step 1: Create TemplatePicker component**

A dropdown/select component that:
- Fetches templates via `useTemplates()`
- On selection, emits the selected template's field values
- Used in the session creation modal to pre-fill fields

- [ ] **Step 2: Integrate into session creation flow**

If a `CreateSessionModal` or similar exists, add `TemplatePicker` at the top. On template selection, pre-fill command, args, working_dir, terminal size. If the modal is inline in `SessionsPage.tsx`, add it there.

- [ ] **Step 3: Add routes to App.tsx**

Add inside the protected route group:
```typescript
<Route path="/templates" element={<TemplatesPage />} />
<Route path="/templates/new" element={<TemplateEditor />} />
<Route path="/templates/:id/edit" element={<TemplateEditor />} />
```

- [ ] **Step 4: Add sidebar navigation**

In `Sidebar.tsx`, add "Templates" nav item below "Jobs" (or in the appropriate position). Use a layout/template icon from lucide-react.

- [ ] **Step 5: Run full frontend test + lint**

Run: `cd web && npx vitest run && npm run lint`
Expected: PASS

- [ ] **Step 6: Commit**

```
git add web/src/components/templates/TemplatePicker.tsx web/src/App.tsx web/src/components/layout/Sidebar.tsx
git commit -m "feat: add template routes, sidebar nav, and session creation integration"
```

---

## Chunk 4: Integration + Verification

### Task 13: End-to-End Verification

- [ ] **Step 1: Run full Go test suite**

Run: `go test -race ./...`
Expected: PASS

- [ ] **Step 2: Run full frontend suite**

Run: `cd web && npx vitest run && npm run lint`
Expected: PASS

- [ ] **Step 3: Manual smoke test**

1. Start server and agent
2. Create a template via API: `POST /api/v1/templates` with name, command, initial_prompt containing `${PR_URL}`
3. List templates: `GET /api/v1/templates`
4. Create session from template: `POST /api/v1/sessions` with `template_name` and `variables`
5. Verify session started with correct command and initial prompt
6. Open frontend, verify template cards appear on Command Center
7. Create/edit/clone/delete template via UI
8. Launch session from template card with variable inputs

- [ ] **Step 4: Commit any fixes**

```
git commit -m "fix: address integration issues from Phase 1 smoke test"
```
