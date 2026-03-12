---
phase: 05-frontend
verified: 2026-03-12T12:50:00Z
status: human_needed
score: 4/4 success criteria verified
re_verification: false
human_verification:
  - test: "Open http://localhost:3000 with dev server and Go server running"
    expected: "Dark-themed app shell renders with sidebar (Command Center, Sessions, Machines links), top bar with logo and hamburger toggle, and status bar at bottom"
    why_human: "Visual appearance and layout can only be confirmed in a running browser"
  - test: "Navigate to / (Command Center). Ensure sessions and machines exist via API first."
    expected: "Stats row shows active session count, online machine count, total session count. Active sessions grid shows session cards with StatusBadge (colored dot), truncated IDs, TimeAgo relative timestamps, Attach and Terminate buttons."
    why_human: "Cross-fleet session visibility requires live data from running backend"
  - test: "Navigate to /sessions. Use the status and machine filter dropdowns."
    expected: "Session list updates to show only sessions matching selected filters. Each filter change re-fetches data. Empty state appears when no sessions match."
    why_human: "Filter wiring to query params requires live interaction to verify end-to-end"
  - test: "Click 'New Session' button on any view. Select an online machine."
    expected: "Modal opens with machine dropdown populated with online machines only. Creating a session closes the modal, shows a toast, and navigates to /sessions/{id} terminal view."
    why_human: "Session creation flow requires a running backend and agent to confirm machine dropdown is populated and navigation works"
  - test: "Click 'Terminate' on an active session."
    expected: "ConfirmDialog appears with danger (red) Terminate button. Confirming calls the terminate API, shows a success toast, and removes/updates the session in the list."
    why_human: "Destructive action confirmation flow requires live UI interaction"
  - test: "Navigate to /machines."
    expected: "Machine cards show hostname, OS, arch, online/offline StatusBadge, last_seen_at TimeAgo, and a 'New Session' button that is disabled for offline machines."
    why_human: "Machine card rendering with correct disabled state requires live data"
  - test: "Click the hamburger menu in the top bar."
    expected: "Sidebar collapses to 64px (icons only) and expands back to 240px on subsequent click."
    why_human: "Sidebar toggle animation and width transition require visual confirmation"
  - test: "Create a session via the API directly, then watch the sessions list."
    expected: "The new session appears in the list within ~30 seconds (polling fallback) or immediately if WebSocket event is received."
    why_human: "Real-time update behavior requires both browser and backend running"
  - test: "Navigate directly to /sessions or /machines (type URL in address bar)."
    expected: "React Router client-side routing works. The correct view renders without a 404. The SPA handler on the server serves index.html for these paths."
    why_human: "SPA fallback behavior for direct URL navigation requires running Go server serving the embedded frontend"
---

# Phase 5: Frontend Verification Report

**Phase Goal:** Users can interact with the control plane through a responsive, dark-themed web UI that shows session status, machine health, and supports session lifecycle actions. The frontend is embedded in the server binary and served as a single-page application.
**Verified:** 2026-03-12T12:50:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (from ROADMAP.md Success Criteria)

| #  | Truth                                                                                                      | Status     | Evidence                                                                                                                                            |
|----|-----------------------------------------------------------------------------------------------------------|------------|-----------------------------------------------------------------------------------------------------------------------------------------------------|
| 1  | User can view a dashboard listing all active sessions across all connected machines with their status      | VERIFIED   | `CommandCenter.tsx` calls `useSessions()`, filters to active, renders `SessionList` with `StatusBadge` per session. `useEventStream()` wired for live updates. |
| 2  | User can see which machines are online/offline and navigate to create or attach sessions from machine list | VERIFIED   | `MachinesPage.tsx` calls `useMachines()`, renders `MachineCard` with `StatusBadge`. `NewSessionModal` opens with `preselectedMachineId`. `Sidebar.tsx` has `/machines` nav link. |
| 3  | Session lifecycle actions (create, attach, detach, terminate) are accessible through the UI               | VERIFIED   | `NewSessionModal.tsx` wires `useCreateSession` mutation. `SessionCard.tsx` has Attach/Terminate buttons. `ConfirmDialog` guards terminate. Navigation to `/sessions/:id` wires attach. |
| 4  | The frontend is embedded in the server binary and served as a single-page application                     | VERIFIED   | `internal/server/frontend/embed.go` has `//go:embed dist/*`, exports `FrontendFS` and `NewSPAHandler`. `vite.config.ts` `build.outDir` points to `../internal/server/frontend/dist`. `go vet` passes clean. |

**Score:** 4/4 truths verified

### Required Artifacts

| Artifact                                               | Expected                                              | Status     | Details                                                                                                      |
|-------------------------------------------------------|-------------------------------------------------------|------------|--------------------------------------------------------------------------------------------------------------|
| `web/src/__tests__/setup.ts`                          | Test setup with jsdom, jest-dom matchers               | VERIFIED   | Imports `@testing-library/jest-dom`. `vite.config.ts` references it via `setupFiles`.                       |
| `web/src/__tests__/views/CommandCenter.test.tsx`      | Stub test for CommandCenter dashboard                  | VERIFIED   | Exists, passes in vitest run.                                                                                |
| `web/src/__tests__/views/SessionsPage.test.tsx`       | Stub test for SessionsPage                             | VERIFIED   | Exists, passes in vitest run.                                                                                |
| `web/src/__tests__/views/MachinesPage.test.tsx`       | Stub test for MachinesPage                             | VERIFIED   | Exists, passes in vitest run.                                                                                |
| `web/src/__tests__/components/sessions/NewSessionModal.test.tsx` | Stub test for NewSessionModal               | VERIFIED   | Exists, passes in vitest run.                                                                                |
| `web/src/App.tsx`                                     | Router + QueryClientProvider wrapping routes           | VERIFIED   | `BrowserRouter`, `QueryClientProvider`, `AppShell`, all four routes rendered with real view components.     |
| `web/src/styles/globals.css`                          | Tailwind v4 CSS-first theme with dark palette          | VERIFIED   | `@import "tailwindcss"`, `@theme` block with full dark palette and font vars. Body styled.                  |
| `web/src/main.tsx`                                    | React entry point rendering App into #root             | VERIFIED   | Imports `globals.css`, renders `<App />` into `#root` with `StrictMode`.                                    |
| `web/src/api/client.ts`                               | Base fetch wrapper with auth header injection          | VERIFIED   | `ApiError` class, `request<T>()` reads `localStorage.getItem('token')`, sets `Authorization: Bearer`.       |
| `web/src/api/machines.ts`                             | Machines REST API client                               | VERIFIED   | `machinesApi.list()` and `machinesApi.get()` using shared `request()`.                                       |
| `web/src/lib/types.ts`                                | Shared TypeScript types (Machine, EventMessage)        | VERIFIED   | `Machine`, `EventType`, `EventMessage` exported. Session types re-exported.                                  |
| `web/src/stores/ui.ts`                                | Zustand store for UI state                             | VERIFIED   | `useUIStore` with `sidebarCollapsed` + `toggleSidebar`.                                                      |
| `web/src/components/layout/AppShell.tsx`              | Application layout shell                               | VERIFIED   | Full flex layout: `TopBar`, `Sidebar`, `main > {children}`, `StatusBar`.                                     |
| `web/src/views/CommandCenter.tsx`                     | Dashboard view                                         | VERIFIED   | Stats row, session list, machine grid, NewSessionModal, ConfirmDialog, useEventStream all wired.             |
| `web/src/views/SessionsPage.tsx`                      | Sessions list view with filters                        | VERIFIED   | Status + machine filter selects, useSessions with filters, SessionList, ConfirmDialog for terminate.         |
| `web/src/views/MachinesPage.tsx`                      | Machines view                                          | VERIFIED   | useMachines, MachineCard grid, online/offline counts, NewSessionModal with preselected machine.              |
| `web/src/hooks/useEventStream.ts`                     | WebSocket event stream hook                            | VERIFIED   | Native WebSocket with exponential backoff reconnection. Invalidates `['sessions']` and `['machines']` caches. |
| `web/src/hooks/useSessions.ts`                        | TanStack Query hooks for sessions                      | VERIFIED   | `useSessions`, `useSession`, `useCreateSession`, `useTerminateSession` all implemented with cache invalidation. |
| `web/src/hooks/useMachines.ts`                        | TanStack Query hooks for machines                      | VERIFIED   | `useMachines`, `useMachine` implemented with 30s polling fallback.                                           |
| `web/src/components/sessions/NewSessionModal.tsx`     | Session creation modal                                 | VERIFIED   | `createPortal`, `useCreateSession`, `useMachines`, form with machine/dir/command, navigate on success.       |
| `internal/server/frontend/embed.go`                   | Go embed SPA handler                                   | VERIFIED   | `//go:embed dist/*`, `FrontendFS`, `NewSPAHandler` with `fs.Stat` check + index.html fallback. `go vet` clean. |
| `internal/server/frontend/dist/.gitkeep`              | Placeholder for go:embed compile-time requirement      | VERIFIED   | Dist directory present with `index.html`, `assets/`, `vite.svg` from prior build.                           |

### Key Link Verification

| From                                     | To                                        | Via                                                  | Status  | Details                                                          |
|------------------------------------------|-------------------------------------------|------------------------------------------------------|---------|------------------------------------------------------------------|
| `web/vite.config.ts`                     | `web/src/__tests__/setup.ts`              | `setupFiles: ['./src/__tests__/setup.ts']`           | WIRED   | Line 29 of vite.config.ts                                        |
| `web/src/main.tsx`                       | `web/src/App.tsx`                         | `import App from './App.tsx'`                        | WIRED   | main.tsx line 4                                                  |
| `web/src/App.tsx`                        | `react-router`                            | `BrowserRouter`, `Routes`, `Route`                   | WIRED   | App.tsx line 1                                                   |
| `web/src/api/client.ts`                  | `localStorage`                            | `localStorage.getItem('token')`                      | WIRED   | client.ts line 14                                                |
| `web/src/api/machines.ts`                | `web/src/api/client.ts`                   | `import { request } from './client.ts'`              | WIRED   | machines.ts line 1                                               |
| `web/src/components/layout/AppShell.tsx` | `web/src/stores/ui.ts`                    | Sidebar uses `useUIStore` for collapsed state        | WIRED   | Sidebar.tsx line 3 — `useUIStore((s) => s.sidebarCollapsed)`     |
| `web/src/hooks/useSessions.ts`           | `web/src/api/sessions.ts`                 | `sessionsApi.list/get/create/kill`                   | WIRED   | useSessions.ts line 2, 8, 16, 24, 34                             |
| `web/src/hooks/useMachines.ts`           | `web/src/api/machines.ts`                 | `machinesApi.list/get`                               | WIRED   | useMachines.ts line 2, 7, 15                                     |
| `web/src/hooks/useEventStream.ts`        | TanStack Query cache                      | `queryClient.invalidateQueries`                      | WIRED   | useEventStream.ts lines 41, 45                                   |
| `web/src/views/SessionsPage.tsx`         | `/sessions/:sessionId`                    | `navigate('/sessions/${id}')` on attach              | WIRED   | SessionsPage.tsx line 35                                         |
| `web/src/components/sessions/NewSessionModal.tsx` | `web/src/hooks/useSessions.ts`  | `useCreateSession()` mutation                        | WIRED   | NewSessionModal.tsx lines 5, 16, 45                              |
| `internal/server/frontend/embed.go`      | `internal/server/frontend/dist/`          | `//go:embed dist/*`                                  | WIRED   | embed.go line 10; dist has index.html from prior build           |
| `web/vite.config.ts`                     | `internal/server/frontend/dist/`          | `build.outDir: '../internal/server/frontend/dist'`   | WIRED   | vite.config.ts line 10                                           |

### Requirements Coverage

| Requirement | Source Plan | Description                                    | Status    | Evidence                                                                              |
|-------------|------------|------------------------------------------------|-----------|---------------------------------------------------------------------------------------|
| SESS-04     | 05-00, 05-01, 05-02, 05-03, 05-04 | User can list all active sessions across all machines | SATISFIED | `SessionsPage.tsx` calls `useSessions()` which queries `/api/v1/sessions` (all sessions), renders `SessionList`. Machine filter allows cross-machine filtering. |

No orphaned requirements. REQUIREMENTS.md traceability table assigns only SESS-04 to Phase 5.

### Anti-Patterns Found

No blockers or warnings detected. Scanned all `.ts`/`.tsx` files in `web/src/` (excluding `__tests__`):
- No TODO/FIXME/PLACEHOLDER comments in production code
- No empty implementations (`return null`, `return {}`, `return []`)
- No console.log-only handlers
- All views have substantive implementations with real data hooks, loading states, error states, and interactive controls

The only matches for "placeholder" were HTML `placeholder=` attributes on form inputs in `NewSessionModal.tsx` (correct usage, not stubs).

### Human Verification Required

The automated checks confirm all artifacts exist, are substantive (not stubs), and are correctly wired. The following require human verification:

**1. Dark-themed app shell visual appearance**
Test: Start `cd web && npm run dev` and open http://localhost:3000
Expected: Dark background (#0d1117), sidebar with Command Center/Sessions/Machines nav links, top bar with "claude-plane" logo and hamburger, status bar at bottom
Why human: Visual styling cannot be verified programmatically

**2. Dashboard cross-fleet session visibility (SESS-04 primary criterion)**
Test: With backend running and sessions active, open Command Center at /
Expected: Stats row shows live counts; active sessions list shows session cards with status badges, truncated IDs, timestamps; machine grid shows MachineCards
Why human: Requires live data from running backend

**3. Session filters on /sessions**
Test: Navigate to /sessions, use status and machine dropdowns
Expected: List updates to filtered results on each selection change
Why human: Filter → API call → render cycle requires live interaction

**4. Session creation modal end-to-end**
Test: Click "New Session", select machine, click "Create Session"
Expected: Session appears in list and browser navigates to terminal view
Why human: Requires running agent with registered machine

**5. Terminate confirmation dialog**
Test: Click "Terminate" on an active session
Expected: Red danger ConfirmDialog appears; confirming removes session; toast shows
Why human: Destructive action flow requires live UI interaction

**6. Sidebar collapse toggle**
Test: Click hamburger icon in top bar
Expected: Sidebar shrinks to 64px (icon-only); second click expands to 240px
Why human: CSS width transition and icon-only rendering require visual confirmation

**7. Real-time WebSocket updates**
Test: Create a session via API while watching /sessions in browser
Expected: Session appears within 30s (polling) or immediately (WebSocket event)
Why human: WebSocket event stream behavior requires running backend emitting events

**8. SPA fallback routing**
Test: Type http://localhost:{PORT}/sessions directly into address bar while server serves embedded frontend
Expected: Page loads correctly (not 404); React Router renders SessionsPage
Why human: Requires Go server serving the embedded assets via `NewSPAHandler`

### Gaps Summary

No gaps. All automated verifications passed. The phase is complete pending human confirmation of visual/UX aspects that cannot be checked programmatically.

---

_Verified: 2026-03-12T12:50:00Z_
_Verifier: Claude (gsd-verifier)_
