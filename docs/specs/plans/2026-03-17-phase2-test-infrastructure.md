# Phase 2: Test Infrastructure — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build reusable test infrastructure so every subsequent phase ships with real tests that catch integration issues at build time.

**Architecture:** Go test factories + integration test harness built on top of existing `newTestStore(t)` and `setupTestAPI(t)`. Frontend gets MSW for API mocking, shared render utilities, and entity factories. A CI-enforced event type sync prevents frontend/backend drift.

**Tech Stack:** Go 1.25 (stdlib testing), vitest 4.0.18, @testing-library/react 16.3.2, MSW 2.x, jsdom

**Design spec:** `docs/specs/2026-03-17-product-audit-and-stabilization-design.md` — Phase 2 section

---

## File Map

| Task | Creates | Modifies |
|------|---------|----------|
| 2.1 Event type CI sync | `cmd/generate-event-types/main.go`, `web/src/__tests__/eventTypeSync.test.ts` | `internal/server/event/event.go` (go:generate directive) |
| 2.2 Go test factories | `internal/server/testutil/factory.go` | Selected existing test files (refactor to use factories) |
| 2.3 Go integration test harness | `internal/server/testutil/testserver.go`, `internal/server/integration_test.go` | — |
| 2.4 Frontend test utilities | `web/src/test/setup.ts`, `web/src/test/factories.ts`, `web/src/test/render.tsx`, `web/src/test/handlers.ts` | `web/package.json`, `web/vite.config.ts`, `web/src/__tests__/setup.ts` |
| 2.5 Frontend seed tests | `web/src/__tests__/views/JobsPage.test.tsx`, `web/src/__tests__/views/SessionsPage.test.tsx` (rewrite stubs), `web/src/__tests__/views/CommandCenter.test.tsx` (rewrite stub) | — |
| 2.6 API contract tests | `internal/server/api/contract_test.go`, `web/src/__tests__/contracts/apiShapes.test.ts` | — |

---

## Task 1: Event type CI sync generator (item 2.1)

Phase 1 created `web/src/constants/eventTypes.ts` manually. This task adds automated sync so they can never drift.

**Files:**
- Create: `cmd/generate-event-types/main.go`
- Modify: `internal/server/event/event.go:10` (add go:generate directive)
- Create: `web/src/__tests__/eventTypeSync.test.ts`

### Steps

- [ ] **Step 1: Create the Go generator**

Create `cmd/generate-event-types/main.go`. This program uses `go/ast` + `go/parser` to read all `const` declarations in `internal/server/event/event.go` whose names start with `Type` and whose values are string literals. It outputs a JSON file `internal/server/event/event_types.json`:

```go
package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type EventType struct {
	GoName string `json:"go_name"`
	Value  string `json:"value"`
}

func main() {
	// Find event.go relative to this binary's source location
	_, thisFile, _, _ := runtime.Caller(0)
	rootDir := filepath.Join(filepath.Dir(thisFile), "..", "..")
	eventFile := filepath.Join(rootDir, "internal", "server", "event", "event.go")
	outFile := filepath.Join(rootDir, "internal", "server", "event", "event_types.json")

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, eventFile, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("parse event.go: %v", err)
	}

	var types []EventType
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 || len(vs.Values) == 0 {
				continue
			}
			name := vs.Names[0].Name
			if !strings.HasPrefix(name, "Type") {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			// Strip quotes from string literal
			value := strings.Trim(lit.Value, `"`)
			types = append(types, EventType{GoName: name, Value: value})
		}
	}

	data, err := json.MarshalIndent(types, "", "  ")
	if err != nil {
		log.Fatalf("marshal: %v", err)
	}

	if err := os.WriteFile(outFile, data, 0644); err != nil {
		log.Fatalf("write: %v", err)
	}

	log.Printf("Generated %d event types → %s", len(types), outFile)
}
```

- [ ] **Step 2: Add go:generate directive**

At the top of `internal/server/event/event.go`, before the const block, add:

```go
//go:generate go run ../../cmd/generate-event-types/main.go
```

- [ ] **Step 3: Run the generator**

```bash
go generate ./internal/server/event/
```

Verify `internal/server/event/event_types.json` is created with 19 entries.

- [ ] **Step 4: Create the frontend sync test**

Create `web/src/__tests__/eventTypeSync.test.ts`:

```typescript
import { readFileSync } from 'fs';
import { resolve } from 'path';
import { ALL_EVENT_TYPES } from '../../constants/eventTypes';

interface BackendEventType {
  go_name: string;
  value: string;
}

describe('Event type sync', () => {
  const jsonPath = resolve(__dirname, '../../../../internal/server/event/event_types.json');
  let backendTypes: BackendEventType[];

  beforeAll(() => {
    const raw = readFileSync(jsonPath, 'utf-8');
    backendTypes = JSON.parse(raw);
  });

  it('frontend has all backend event types', () => {
    const frontendSet = new Set(ALL_EVENT_TYPES);
    const missing = backendTypes.filter((bt) => !frontendSet.has(bt.value));
    if (missing.length > 0) {
      throw new Error(
        `Frontend is missing event types from backend:\n${missing.map((m) => `  ${m.go_name} = "${m.value}"`).join('\n')}\n\nAdd them to web/src/constants/eventTypes.ts`,
      );
    }
  });

  it('frontend has no extra event types not in backend', () => {
    const backendSet = new Set(backendTypes.map((bt) => bt.value));
    const extra = ALL_EVENT_TYPES.filter((ft) => !backendSet.has(ft));
    if (extra.length > 0) {
      throw new Error(
        `Frontend has event types not in backend:\n${extra.map((e) => `  "${e}"`).join('\n')}\n\nRemove them from web/src/constants/eventTypes.ts or add them to internal/server/event/event.go`,
      );
    }
  });
});
```

- [ ] **Step 5: Run the sync test**

```bash
cd web && npx vitest run src/__tests__/eventTypeSync.test.ts
```

Expected: PASS (both directions match).

- [ ] **Step 6: Verify CI catches drift**

Temporarily add a fake event type to `eventTypes.ts`, run the test, verify it fails. Remove it.

- [ ] **Step 7: Commit**

```bash
git add cmd/generate-event-types/ internal/server/event/event.go internal/server/event/event_types.json web/src/__tests__/eventTypeSync.test.ts
git commit -m "feat: add event type CI sync between Go and TypeScript"
```

---

## Task 2: Go test factories (item 2.2)

Existing tests manually construct entities. Factories reduce boilerplate and prevent breakage when fields are added.

**Files:**
- Create: `internal/server/testutil/factory.go`
- Modify: 2-3 existing test files to use factories as proof of pattern

### Steps

- [ ] **Step 1: Create factory package**

Create `internal/server/testutil/factory.go`. Use functional options pattern matching stdlib testing style (no testify):

```go
package testutil

import (
	"fmt"
	"testing"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// --- Job Factory ---

type JobOption func(*store.CreateJobParams)

func WithJobName(name string) JobOption {
	return func(p *store.CreateJobParams) { p.Name = name }
}

func WithJobDescription(desc string) JobOption {
	return func(p *store.CreateJobParams) { p.Description = desc }
}

func WithJobUserID(id string) JobOption {
	return func(p *store.CreateJobParams) { p.UserID = id }
}

func NewJobParams(opts ...JobOption) store.CreateJobParams {
	p := store.CreateJobParams{
		Name:        "test-job",
		Description: "test job description",
		UserID:      "test-user",
	}
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

func MustCreateJob(t *testing.T, s *store.Store, opts ...JobOption) *store.Job {
	t.Helper()
	params := NewJobParams(opts...)
	job, err := s.CreateJob(params)
	if err != nil {
		t.Fatalf("MustCreateJob: %v", err)
	}
	return job
}

// --- Machine Factory ---

var machineCounter int

func MustCreateMachine(t *testing.T, s *store.Store) string {
	t.Helper()
	machineCounter++
	id := fmt.Sprintf("test-machine-%d", machineCounter)
	if err := s.UpsertMachine(id, 10); err != nil {
		t.Fatalf("MustCreateMachine: %v", err)
	}
	return id
}

// --- Session Factory ---

type SessionOption func(*store.Session)

func WithSessionCommand(cmd string) SessionOption {
	return func(s *store.Session) { s.Command = cmd }
}

func WithSessionModel(model string) SessionOption {
	return func(s *store.Session) { s.Model = model }
}

func WithSessionMachine(id string) SessionOption {
	return func(s *store.Session) { s.MachineID = id }
}

func MustCreateSession(t *testing.T, s *store.Store, machineID string, opts ...SessionOption) *store.Session {
	t.Helper()
	sess := &store.Session{
		SessionID: fmt.Sprintf("sess-%d", machineCounter),
		MachineID: machineID,
		Command:   "claude",
		Status:    "created",
	}
	for _, opt := range opts {
		opt(sess)
	}
	if err := s.CreateSession(sess); err != nil {
		t.Fatalf("MustCreateSession: %v", err)
	}
	return sess
}

// --- User Factory ---

func MustCreateUser(t *testing.T, s *store.Store, email, role string) *store.User {
	t.Helper()
	user, err := s.CreateUser(email, "password123", email, role)
	if err != nil {
		t.Fatalf("MustCreateUser: %v", err)
	}
	return user
}

// --- Template Factory ---

type TemplateOption func(*store.CreateTemplateParams)

func WithTemplateName(name string) TemplateOption {
	return func(p *store.CreateTemplateParams) { p.Name = name }
}

func MustCreateTemplate(t *testing.T, s *store.Store, userID string, opts ...TemplateOption) *store.SessionTemplate {
	t.Helper()
	params := store.CreateTemplateParams{
		Name:    "test-template",
		UserID:  userID,
		Command: "claude",
	}
	for _, opt := range opts {
		opt(&params)
	}
	tmpl, err := s.CreateTemplate(params)
	if err != nil {
		t.Fatalf("MustCreateTemplate: %v", err)
	}
	return tmpl
}
```

Note: Read the actual store types (`CreateJobParams`, `CreateTemplateParams`, `Session`, `User`) before writing. Adjust field names to match the actual structs. The above is a template — the implementer MUST verify against the real code.

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/server/testutil/
```

- [ ] **Step 3: Refactor 2-3 existing tests to use factories**

Pick 2-3 tests that manually construct entities. Good candidates:
- `internal/server/store/sessions_test.go` — uses manual `UpsertMachine` + `Session{}` construction
- `internal/server/store/jobs_test.go` — has its own `testCreateJob` helper that can be replaced

Replace manual construction with factory calls. Verify tests still pass.

- [ ] **Step 4: Run tests**

```bash
go test -race ./internal/server/store/ -v
go test -race ./internal/server/testutil/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/server/testutil/ internal/server/store/sessions_test.go internal/server/store/jobs_test.go
git commit -m "feat: add Go test factories for jobs, sessions, machines, users, templates"
```

---

## Task 3: Go integration test harness (item 2.3)

Unit tests mock the store. We need tests that hit the real HTTP stack end-to-end.

**Files:**
- Create: `internal/server/testutil/testserver.go`
- Create: `internal/server/integration_test.go`

### Steps

- [ ] **Step 1: Create test server builder**

Create `internal/server/testutil/testserver.go`. Build on top of existing `setupTestAPI` pattern from `internal/server/api/auth_handler_test.go:17-42`, but make it reusable:

```go
package testutil

import (
	"net/http/httptest"
	"testing"

	// Import necessary packages — read setupTestAPI to see what's needed
)

type TestServer struct {
	Server *httptest.Server
	Store  *store.Store
	// Add other components as needed (auth, event bus, etc.)
}

// NewTestServer creates a fully wired test server with real SQLite, auth, event bus.
// Pattern: copies from setupTestAPI in api/auth_handler_test.go but made reusable.
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()
	// 1. Create store via newTestStore pattern
	// 2. Create auth service
	// 3. Create event bus
	// 4. Create connection manager
	// 5. Wire handlers
	// 6. Build router
	// 7. Start httptest.Server
	// Return TestServer with all components accessible
}

// Client returns an HTTP client configured for the test server.
func (ts *TestServer) Client() *http.Client {
	return ts.Server.Client()
}

// LoginAsAdmin creates an admin user, logs in, and returns cookies for auth.
func (ts *TestServer) LoginAsAdmin(t *testing.T) []*http.Cookie {
	t.Helper()
	// Seed admin user, POST /api/v1/auth/login, extract cookies
}

// AuthRequest makes an authenticated request with the given cookies.
func (ts *TestServer) AuthRequest(t *testing.T, method, path string, body io.Reader, cookies []*http.Cookie) *http.Response {
	t.Helper()
	// Build request, add cookies, execute, return response
}
```

IMPORTANT: Read `internal/server/api/auth_handler_test.go` thoroughly to understand the existing `setupTestAPI` pattern. The test server should replicate it but be exported and reusable.

- [ ] **Step 2: Write 5 seed integration tests**

Create `internal/server/integration_test.go` with build tag:

```go
//go:build integration

package server_test

import (
	"testing"
	"github.com/kodrunhq/claude-plane/internal/server/testutil"
)
```

Tests:
1. **Create job → add step → trigger run → verify status**: Full job lifecycle via HTTP
2. **Create template → clone → verify clone**: Template CRUD via HTTP
3. **Create user → login → verify JWT cookie**: Auth flow via HTTP
4. **Create webhook → verify delivery on event**: Webhook delivery via HTTP + event bus
5. **Session metadata round-trip**: Create session with model, GET it back, verify metadata

Each test uses `testutil.NewTestServer(t)` and makes real HTTP calls.

- [ ] **Step 3: Run integration tests**

```bash
go test -race -tags=integration ./internal/server/ -v -count=1
```

- [ ] **Step 4: Also run without tag to verify they're skipped**

```bash
go test -race ./internal/server/ -v
```

Integration tests should NOT run in normal `go test ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/server/testutil/testserver.go internal/server/integration_test.go
git commit -m "feat: add Go integration test harness with 5 seed tests"
```

---

## Task 4: Frontend test utilities (item 2.4)

Install MSW, create shared render function with providers, create entity factories.

**Files:**
- Create: `web/src/test/setup.ts`, `web/src/test/factories.ts`, `web/src/test/render.tsx`, `web/src/test/handlers.ts`
- Modify: `web/package.json` (add msw), `web/vite.config.ts` (update setup), `web/src/__tests__/setup.ts`

### Steps

- [ ] **Step 1: Install MSW**

```bash
cd web && npm install -D msw@^2
```

- [ ] **Step 2: Create MSW handlers**

Create `web/src/test/handlers.ts` with default mock handlers for core API endpoints:

```typescript
import { http, HttpResponse } from 'msw';

// Default mock data
export const mockSessions = [
  { session_id: 'sess-1', machine_id: 'machine-1', command: 'claude', status: 'running', created_at: '2026-03-17T00:00:00Z', updated_at: '2026-03-17T00:00:00Z' },
];

export const mockMachines = [
  { machine_id: 'machine-1', display_name: 'Worker 1', status: 'connected', max_sessions: 10, created_at: '2026-03-17T00:00:00Z' },
];

export const mockJobs = [
  { job_id: 'job-1', name: 'Deploy', description: 'Deploy to prod', status: 'completed', created_at: '2026-03-17T00:00:00Z' },
];

export const handlers = [
  http.get('/api/v1/sessions', () => HttpResponse.json(mockSessions)),
  http.get('/api/v1/machines', () => HttpResponse.json(mockMachines)),
  http.get('/api/v1/jobs', () => HttpResponse.json(mockJobs)),
  http.get('/api/v1/templates', () => HttpResponse.json([])),
  http.get('/api/v1/runs', () => HttpResponse.json([])),
  http.get('/api/v1/events', () => HttpResponse.json([])),
];
```

- [ ] **Step 3: Create shared test setup**

Create `web/src/test/setup.ts`:

```typescript
import '@testing-library/jest-dom';
import { setupServer } from 'msw/node';
import { handlers } from './handlers';

export const server = setupServer(...handlers);

beforeAll(() => server.listen({ onUnhandledRequest: 'warn' }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
```

Update `web/vite.config.ts` setupFiles to point to the new setup:

```typescript
test: {
  globals: true,
  environment: 'jsdom',
  setupFiles: ['./src/test/setup.ts'],
  include: ['src/**/*.test.{ts,tsx}'],
}
```

Remove or redirect `web/src/__tests__/setup.ts` → import from `web/src/test/setup.ts`.

- [ ] **Step 4: Create entity factories**

Create `web/src/test/factories.ts`:

```typescript
import type { Session } from '../types/session';
import type { Machine } from '../lib/types';

let counter = 0;
function id(prefix: string) { return `${prefix}-${++counter}`; }

export function buildSession(overrides?: Partial<Session>): Session {
  return {
    session_id: id('sess'),
    machine_id: 'machine-1',
    user_id: 'user-1',
    command: 'claude',
    working_dir: '/home/user',
    status: 'running',
    created_at: '2026-03-17T00:00:00Z',
    updated_at: '2026-03-17T00:00:00Z',
    ...overrides,
  };
}

export function buildMachine(overrides?: Partial<Machine>): Machine {
  return {
    machine_id: id('machine'),
    display_name: 'Test Machine',
    status: 'connected',
    max_sessions: 10,
    last_health: '',
    last_seen_at: '2026-03-17T00:00:00Z',
    cert_expires: '2027-03-17T00:00:00Z',
    created_at: '2026-03-17T00:00:00Z',
    ...overrides,
  };
}

// Add buildJob, buildRun, buildTemplate, buildEvent similarly
// Read the actual TypeScript types before implementing
```

- [ ] **Step 5: Create shared render function**

Create `web/src/test/render.tsx`:

```tsx
import { render, type RenderOptions } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import type { ReactElement } from 'react';

interface Options extends Omit<RenderOptions, 'wrapper'> {
  route?: string;
}

export function renderWithProviders(ui: ReactElement, options: Options = {}) {
  const { route = '/', ...renderOptions } = options;

  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  });

  function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[route]}>
          {children}
        </MemoryRouter>
      </QueryClientProvider>
    );
  }

  return {
    ...render(ui, { wrapper: Wrapper, ...renderOptions }),
    queryClient,
  };
}

export { screen, waitFor, within } from '@testing-library/react';
export { userEvent } from '@testing-library/user-event';
```

- [ ] **Step 6: Verify existing tests still pass**

```bash
cd web && npx vitest run
```

All 57+ existing tests must still pass after the setup change.

- [ ] **Step 7: Commit**

```bash
git add web/package.json web/package-lock.json web/vite.config.ts web/src/test/ web/src/__tests__/setup.ts
git commit -m "feat: add frontend test utilities (MSW, factories, renderWithProviders)"
```

---

## Task 5: Frontend seed tests — rewrite stubs (item 2.4 continued)

5 of 10 frontend test files are stubs (`expect(true).toBe(true)`). Replace them with real tests using the new utilities.

**Files:**
- Rewrite: `web/src/__tests__/views/SessionsPage.test.tsx`
- Rewrite: `web/src/__tests__/views/CommandCenter.test.tsx`
- Rewrite: `web/src/__tests__/views/MachinesPage.test.tsx`
- Rewrite: `web/src/__tests__/components/sessions/NewSessionModal.test.tsx`
- Create: `web/src/__tests__/views/JobsPage.test.tsx`

### Steps

- [ ] **Step 1: Rewrite SessionsPage test**

```typescript
import { renderWithProviders, screen, waitFor } from '../../test/render';
import { SessionsPage } from '../../views/SessionsPage';

describe('SessionsPage', () => {
  it('renders session cards from API', async () => {
    renderWithProviders(<SessionsPage />);
    await waitFor(() => {
      expect(screen.getByText(/sess-1/)).toBeInTheDocument();
    });
  });

  it('shows empty state when no sessions', async () => {
    // Override MSW handler to return []
    server.use(http.get('/api/v1/sessions', () => HttpResponse.json([])));
    renderWithProviders(<SessionsPage />);
    await waitFor(() => {
      expect(screen.getByText(/No sessions/i)).toBeInTheDocument();
    });
  });
});
```

Read the actual SessionsPage component before writing — adjust selectors to match real rendered text.

- [ ] **Step 2: Rewrite CommandCenter test**

Test that it renders active sessions count, machine count, and job count from mocked API data.

- [ ] **Step 3: Rewrite MachinesPage test**

Test that machine cards render with display names from mocked API.

- [ ] **Step 4: Rewrite NewSessionModal test**

Test that the modal renders form fields and machine dropdown.

- [ ] **Step 5: Create JobsPage test**

Test that jobs render in the table, search filters work.

- [ ] **Step 6: Run all tests**

```bash
cd web && npx vitest run
```

All tests must pass. No more stubs.

- [ ] **Step 7: Commit**

```bash
git add web/src/__tests__/
git commit -m "feat: replace frontend test stubs with real tests using MSW"
```

---

## Task 6: API contract tests (item 2.5)

Ensure backend JSON response shapes match what frontend expects.

**Files:**
- Create: `internal/server/api/contract_test.go`
- Create: `web/src/__tests__/contracts/apiShapes.test.ts`

### Steps

- [ ] **Step 1: Write Go contract test**

Create `internal/server/api/contract_test.go`. For each major endpoint, make a real HTTP request to the test server and verify the JSON response contains the expected fields:

```go
func TestContract_ListSessions(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()
	cookies := loginAsAdmin(t, srv)

	// Create a session first
	// ...

	resp := authGet(t, srv, "/api/v1/sessions", cookies)
	defer resp.Body.Close()

	var sessions []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sessions)

	if len(sessions) == 0 {
		t.Skip("no sessions to check contract")
	}

	s := sessions[0]
	requiredFields := []string{"session_id", "machine_id", "status", "command", "created_at"}
	for _, field := range requiredFields {
		if _, ok := s[field]; !ok {
			t.Errorf("session response missing required field: %s", field)
		}
	}

	// Sensitive fields must NOT be in list response
	sensitiveFields := []string{"env_vars", "args", "initial_prompt"}
	for _, field := range sensitiveFields {
		if val, ok := s[field]; ok && val != "" {
			t.Errorf("session LIST response should not include sensitive field: %s", field)
		}
	}
}
```

Add similar contract tests for: jobs, machines, runs, templates, users (admin).

- [ ] **Step 2: Write frontend shape test**

Create `web/src/__tests__/contracts/apiShapes.test.ts` that verifies MSW mock responses match TypeScript types. This catches when mocks drift from actual types:

```typescript
import { mockSessions, mockMachines, mockJobs } from '../../test/handlers';
import type { Session } from '../../types/session';
import type { Machine } from '../../lib/types';

describe('API response shapes', () => {
  it('mock sessions match Session type required fields', () => {
    const requiredKeys: (keyof Session)[] = ['session_id', 'machine_id', 'status', 'command', 'created_at'];
    for (const session of mockSessions) {
      for (const key of requiredKeys) {
        expect(session).toHaveProperty(key);
      }
    }
  });

  it('mock machines match Machine type required fields', () => {
    const requiredKeys: (keyof Machine)[] = ['machine_id', 'status', 'max_sessions', 'created_at'];
    for (const machine of mockMachines) {
      for (const key of requiredKeys) {
        expect(machine).toHaveProperty(key);
      }
    }
  });
});
```

- [ ] **Step 3: Run both**

```bash
go test -race ./internal/server/api/ -run TestContract -v
cd web && npx vitest run src/__tests__/contracts/
```

- [ ] **Step 4: Commit**

```bash
git add internal/server/api/contract_test.go web/src/__tests__/contracts/
git commit -m "feat: add API contract tests for frontend/backend shape validation"
```

---

## Final Verification

- [ ] **Run complete CI check**

```bash
# Backend
go vet ./...
go generate ./internal/server/event/
go test -race ./...

# Frontend
cd web && npx tsc --noEmit && npx vitest run
```

All must pass.

- [ ] **Create PR for Phase 2**

Branch name: `feat/phase2-test-infrastructure`
PR title: "feat: Phase 2 — Test infrastructure (factories, integration harness, MSW, event sync)"

---

## Review Corrections (from plan review — MUST apply during implementation)

### Task 1 corrections
1. **Event type count is 18, not 19.** Step 3 says "Verify 19 entries" — the actual count is 18 (5 run + 3 session + 2 machine + 3 trigger + 3 template + 2 step).
2. **Sync test import path is wrong.** The test at `web/src/__tests__/eventTypeSync.test.ts` imports from `'../../constants/eventTypes'` but since the file is one level deep inside `__tests__/`, the correct path is `'../constants/eventTypes'`.
3. **Generator path resolution.** `runtime.Caller(0)` may not work via `go generate`. Use `os.Getwd()` or accept root dir as a CLI flag instead. Test from both `go generate ./internal/server/event/` and `go run ./cmd/generate-event-types/`.

### Task 2 corrections (Go factories — critical)
4. **All store methods require `context.Context` as first arg.** `s.CreateJob(ctx, params)`, `s.CreateTemplate(ctx, tmpl)`, etc. All factory `MustCreate*` functions must pass `context.Background()`.
5. **`CreateTemplate` takes `*SessionTemplate`, NOT `CreateTemplateParams`.** There is no `CreateTemplateParams` type. The factory must construct a `*store.SessionTemplate` struct.
6. **`CreateUser` takes `*User` struct, NOT 4 string args.** The actual signature is `s.CreateUser(user *User) error`. The `MustCreateUser` factory must construct a `*store.User`.
7. **Use separate counters per factory.** The plan uses a shared `machineCounter` for both machines and sessions, producing confusing IDs. Use separate counters or use `t.Name()` + UUID for unique IDs.
8. **Add `MustCreateRun` factory.** The spec lists `NewTestRun()` as required but the plan omits it. Port the existing `testCreateRun` helper from `jobs_test.go`.

### Task 3 corrections
9. **`setupTestAPI` is unexported and returns only `*httptest.Server`.** The `testutil.NewTestServer` must replicate the internals (create store, auth, connmgr, handlers, router) — it cannot wrap `setupTestAPI`. Read `api/auth_handler_test.go:19+` for the full pattern.
10. **Helper names differ from plan.** Existing helpers are `registerUser` and `loginUser`, not `loginAsAdmin` or `authGet`. The test server should provide its own named helpers.

### Task 4 corrections (Frontend)
11. **`userEvent` is a default export.** The plan has `export { userEvent } from '@testing-library/user-event'` which fails. Correct: `import userEvent from '@testing-library/user-event'; export { userEvent };`
12. **MSW global server conflicts with `useEventStream.test.ts`.** The existing `useEventStream.test.ts` uses `vi.stubGlobal('WebSocket', MockWebSocket)` to mock WebSocket globally. A global MSW server intercepting HTTP requests is fine, but verify it doesn't interfere with the WebSocket mock. If it does, add the WebSocket mock to the global setup or exclude that test from MSW.
13. **Old setup file handling.** Delete `web/src/__tests__/setup.ts` and move its content (`import '@testing-library/jest-dom'`) into the new `web/src/test/setup.ts`. Do NOT leave both files — vitest only reads one setupFile.

### Task 6 corrections
14. **Contract test helpers.** The plan references `loginAsAdmin` and `authGet` which don't exist. Use the existing `registerUser`/`loginUser` pattern from `api/auth_handler_test.go`, or create equivalent helpers in the contract test file.
