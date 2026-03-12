---
phase: 05-frontend
plan: 02
subsystem: ui
tags: [react, zustand, tailwind, typescript, api-client, layout]

requires:
  - phase: 04-terminal-streaming
    provides: Vite React scaffold, session types, terminal components
provides:
  - Base API client with JWT auth header injection
  - Machines REST API client
  - Sessions API refactored to shared client
  - Machine and EventMessage shared types
  - Format utilities (timeAgo, duration, truncateId)
  - Zustand UI store for sidebar state
  - App shell layout (TopBar, Sidebar, StatusBar, AppShell)
  - Shared UI components (StatusBadge, TimeAgo, EmptyState, ConfirmDialog)
affects: [05-frontend]

tech-stack:
  added: [zustand, date-fns, react-router, "@tanstack/react-query", lucide-react, sonner, tailwindcss, "@tailwindcss/vite"]
  patterns: [api-client-wrapper, zustand-store, layout-shell, portal-dialog]

key-files:
  created:
    - web/src/api/client.ts
    - web/src/api/machines.ts
    - web/src/lib/types.ts
    - web/src/lib/format.ts
    - web/src/stores/ui.ts
    - web/src/components/layout/AppShell.tsx
    - web/src/components/layout/TopBar.tsx
    - web/src/components/layout/Sidebar.tsx
    - web/src/components/layout/StatusBar.tsx
    - web/src/components/shared/StatusBadge.tsx
    - web/src/components/shared/TimeAgo.tsx
    - web/src/components/shared/EmptyState.tsx
    - web/src/components/shared/ConfirmDialog.tsx
    - web/src/styles/globals.css
  modified:
    - web/src/api/sessions.ts
    - web/src/App.tsx
    - web/src/main.tsx
    - web/vite.config.ts
    - web/package.json

key-decisions:
  - "ApiError class with status code for typed error handling in API client"
  - "204 No Content handled as undefined return in base request helper"
  - "Sidebar width transitions via inline style for collapsed/expanded states"

patterns-established:
  - "API client pattern: request<T>(path, options) with auto auth headers"
  - "sessionsApi/machinesApi object pattern with named export aliases for backward compat"
  - "Zustand store pattern: create<Interface>((set) => ({...}))"
  - "Layout shell pattern: AppShell wraps TopBar + Sidebar + content + StatusBar"

requirements-completed: [SESS-04]

duration: 3min
completed: 2026-03-12
---

# Phase 05 Plan 02: API Client, Layout, and Shared Components Summary

**Base API client with JWT auth, machines API, app shell layout with sidebar/topbar/statusbar, and shared UI components (StatusBadge, TimeAgo, EmptyState, ConfirmDialog)**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-12T11:31:48Z
- **Completed:** 2026-03-12T11:35:00Z
- **Tasks:** 2
- **Files modified:** 18

## Accomplishments
- API client layer with JWT Bearer token injection from localStorage on all requests
- Machines API client calling /api/v1/machines endpoints
- Sessions API refactored from inline fetch to shared request() helper
- App shell layout with collapsible sidebar, top bar, and status bar
- Shared components ready for use by Plan 03 views

## Task Commits

Each task was committed atomically:

1. **Task 1: API client, types, utilities, and Zustand store** - `fc73546` (feat)
2. **Task 2: App shell layout and shared UI components** - `16112a8` (feat)

## Files Created/Modified
- `web/src/api/client.ts` - Base fetch wrapper with auth header injection and ApiError class
- `web/src/api/machines.ts` - Machines REST API client (list, get)
- `web/src/api/sessions.ts` - Refactored to use shared request() helper
- `web/src/lib/types.ts` - Machine, EventMessage types; re-exports session types
- `web/src/lib/format.ts` - formatTimeAgo, formatDuration, truncateId utilities
- `web/src/stores/ui.ts` - Zustand store for sidebar collapsed state
- `web/src/components/layout/AppShell.tsx` - Full viewport layout shell
- `web/src/components/layout/TopBar.tsx` - Header with sidebar toggle and logo
- `web/src/components/layout/Sidebar.tsx` - Nav links with active state and collapse
- `web/src/components/layout/StatusBar.tsx` - Connection and count indicators
- `web/src/components/shared/StatusBadge.tsx` - Status-to-color dot indicator
- `web/src/components/shared/TimeAgo.tsx` - Auto-refreshing relative timestamps
- `web/src/components/shared/EmptyState.tsx` - Centered empty state placeholder
- `web/src/components/shared/ConfirmDialog.tsx` - Portal modal with danger variant

## Decisions Made
- ApiError class carries HTTP status code for typed error handling
- 204 No Content returns undefined rather than attempting JSON parse
- Sidebar uses inline style for width transition (240px expanded, 64px collapsed)
- ConfirmDialog uses createPortal to render in document.body for proper z-index stacking

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Installed prerequisite dependencies and configured Tailwind/routing**
- **Found during:** Pre-task setup
- **Issue:** Plans 05-00 and 05-01 (dependencies, Tailwind v4, routing, test stubs) had not been executed
- **Fix:** Installed all missing dependencies (zustand, date-fns, react-router, @tanstack/react-query, lucide-react, sonner, tailwindcss), configured Vite with Tailwind v4 plugin, created theme CSS, set up React Router routes, created stub test files
- **Files modified:** web/package.json, web/vite.config.ts, web/src/styles/globals.css, web/src/styles/terminal.css, web/src/App.tsx, web/src/main.tsx, 4 test stub files
- **Verification:** TypeScript compiles, Vite builds, vitest passes
- **Committed in:** 8730cc8 (prerequisite setup commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Prerequisite setup was necessary to unblock this plan's tasks. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- API clients ready for Plan 03 views to consume
- App shell layout ready to wrap routed content
- Shared components exported and ready for use
- Plan 03 can import AppShell, StatusBadge, TimeAgo, EmptyState, ConfirmDialog directly

## Self-Check: PASSED

All 14 created files verified present. All 3 commits (8730cc8, 79dc93d, 16112a8) verified in git log.

---
*Phase: 05-frontend*
*Completed: 2026-03-12*
