---
phase: 06-job-system
plan: 04
subsystem: api
tags: [chi, rest, jwt, sqlite, gap-closure]

requires:
  - phase: 06-job-system
    provides: JobStoreIface CRUD, handler package with RegisterJobRoutes/RegisterRunRoutes, orchestrator
provides:
  - All 15 job/run REST endpoints registered in production router under JWT auth
  - Working UpdateJob store method and handler (replaces 501 stub)
affects: [frontend-integration, server-startup]

tech-stack:
  added: []
  patterns: [flat route registration with JWT group on top-level router, optional handler params for gradual wiring]

key-files:
  created: []
  modified:
    - internal/server/store/jobs.go
    - internal/server/handler/jobs.go
    - internal/server/api/router.go
    - internal/server/api/auth_handler_test.go

key-decisions:
  - "Job/run routes registered as flat paths on top-level router in JWT-protected Group (not inside /api/v1 Route block) to avoid Chi mount conflicts"
  - "NewRouter accepts optional *handler.JobHandler and *handler.RunHandler params (nil-safe) for gradual production wiring"

patterns-established:
  - "Optional handler injection: NewRouter accepts nil handler pointers, skips registration when nil"

requirements-completed: [JOBS-01, JOBS-02, JOBS-03, JOBS-04]

duration: 5min
completed: 2026-03-12
---

# Phase 06 Plan 04: Gap Closure - Route Registration and UpdateJob Summary

**Job/run REST endpoints wired into production Chi router with JWT auth; UpdateJob store method replaces 501 stub**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-12T13:17:32Z
- **Completed:** 2026-03-12T13:22:55Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Implemented UpdateJob on JobStoreIface and Store with SQL UPDATE + re-read pattern
- Replaced 501 Not Implemented stub in handler with working UpdateJob that persists name/description changes
- Registered all 15 job/run endpoints in production router under JWT auth middleware
- Updated NewRouter signature with optional job/run handler params and fixed all call sites

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement UpdateJob in store and fix handler** - `ea458d2` (feat)
2. **Task 2: Register job and run routes in production router** - `7041382` (feat)

## Files Created/Modified
- `internal/server/store/jobs.go` - Added UpdateJob to JobStoreIface interface and implemented on *Store
- `internal/server/handler/jobs.go` - Replaced 501 stub with working UpdateJob handler
- `internal/server/api/router.go` - Added jobHandler/runHandler params, registered routes in JWT group
- `internal/server/api/auth_handler_test.go` - Updated NewRouter call with nil params

## Decisions Made
- Job/run routes registered as flat paths on top-level router in JWT-protected Group to avoid Chi mount conflicts with existing /api/v1 Route block
- NewRouter accepts optional handler pointers (nil-safe) so production wiring can be done incrementally

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All job system REST endpoints are now reachable in production
- Production server cmd/ wiring still needs to instantiate JobHandler/RunHandler and pass to NewRouter
- Frontend can now call UpdateJob without getting 501

---
*Phase: 06-job-system*
*Completed: 2026-03-12*
