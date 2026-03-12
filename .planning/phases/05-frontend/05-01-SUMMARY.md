---
phase: 05-frontend
plan: 01
subsystem: ui
tags: [react, vite, tailwind-v4, react-router, tanstack-query, zustand, typescript]

requires:
  - phase: 05-frontend/05-00
    provides: Vite React+TS scaffold with xterm.js, session types, API client, terminal hook
provides:
  - Vite configured with Tailwind v4 plugin, build output dir, dev proxy
  - Tailwind v4 CSS-first dark theme with custom color palette and fonts
  - React Router v7 routing with placeholder views for all main routes
  - QueryClientProvider and Toaster wired at app root
  - TypeScript project configs committed to version control
affects: [05-frontend/05-02, 05-frontend/05-03]

tech-stack:
  added: [react-router, "@tanstack/react-query", zustand, tailwindcss, "@tailwindcss/vite", lucide-react, sonner, date-fns]
  patterns: [css-first-tailwind-theming, react-router-v7-single-package, query-client-at-root]

key-files:
  created: []
  modified:
    - web/vite.config.ts
    - web/index.html
    - web/src/styles/globals.css
    - web/src/styles/terminal.css
    - web/src/main.tsx
    - web/src/App.tsx

key-decisions:
  - "Tailwind v4 CSS-first config -- no tailwind.config.js, theme defined in globals.css @theme block"
  - "Build output to ../internal/server/frontend/dist for go:embed integration"
  - "Dev proxy routes /api to https://localhost:8443, /ws to wss://localhost:8443"

patterns-established:
  - "CSS-first theming: all theme tokens in globals.css @theme block, no JS config"
  - "Route structure: /, /sessions, /sessions/:sessionId, /machines"
  - "App root wraps QueryClientProvider > BrowserRouter > Routes with Toaster"

requirements-completed: [SESS-04]

duration: 3min
completed: 2026-03-12
---

# Phase 05 Plan 01: App Shell and Routing Summary

**Vite + Tailwind v4 dark theme with React Router v7 placeholder routes and TanStack Query at app root**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-12T11:31:47Z
- **Completed:** 2026-03-12T11:34:41Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Installed all frontend dependencies (react-router, @tanstack/react-query, zustand, tailwindcss, lucide-react, sonner, date-fns)
- Configured Vite with Tailwind v4 plugin, build output to internal/server/frontend/dist, and dev server proxy
- Dark theme applied via CSS-first Tailwind v4 @theme block with custom colors and fonts
- React Router v7 routes defined for /, /sessions, /sessions/:sessionId, /machines with placeholders
- All TypeScript compiles, Vite builds successfully, all 8 vitest tests pass

## Task Commits

Each task was committed atomically:

1. **Task 1: Install dependencies and configure Vite + Tailwind v4** - `7ff80ad` (chore)
2. **Task 2: Create Tailwind theme, entry point, and router with placeholders** - `efd5593` (feat)

## Files Created/Modified
- `web/vite.config.ts` - Tailwind v4 plugin, build output, dev proxy config
- `web/index.html` - Updated title to Claude Plane
- `web/src/styles/globals.css` - Tailwind v4 CSS-first theme (already from 05-00)
- `web/src/styles/terminal.css` - xterm.js container styles (already from 05-00)
- `web/src/main.tsx` - React entry point importing globals.css (already from 05-00)
- `web/src/App.tsx` - Router + QueryClient + placeholder routes (already from 05-00)
- `web/tsconfig.json` - Project references config (committed to VCS)
- `web/tsconfig.app.json` - App TypeScript config (committed to VCS)
- `.gitignore` - Added internal/server/frontend/dist/ exclusion

## Decisions Made
- Tailwind v4 CSS-first configuration: no tailwind.config.js file, all theme tokens defined in globals.css @theme block
- Build output directed to ../internal/server/frontend/dist for future go:embed integration
- Dev proxy configured with secure: false for self-signed cert compatibility
- Added internal/server/frontend/dist/ to root .gitignore since build output should not be tracked

## Deviations from Plan

None - plan executed exactly as written. Plan 05-00 had already scaffolded most of the files (globals.css, terminal.css, main.tsx, App.tsx) with content matching plan specifications. This plan ensured all configs were committed and Vite build configuration was finalized.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- App shell with routing complete, ready for Plan 02 (API layer / shared components)
- Dark theme tokens available for all subsequent UI components
- QueryClient configured at root for TanStack Query usage in Plan 02+

---
*Phase: 05-frontend*
*Completed: 2026-03-12*
