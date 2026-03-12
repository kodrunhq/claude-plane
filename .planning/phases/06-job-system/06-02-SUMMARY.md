---
phase: 06-job-system
plan: 02
subsystem: api, handlers
tags: [rest-api, chi-router, job-crud, run-management, dag-validation, tdd]

# Dependency graph
requires:
  - phase: 06-job-system
    provides: JobStoreIface CRUD, Orchestrator (CreateRun, RetryStep, CancelRun), ValidateDAG
  - phase: 03-server-core
    provides: Chi router patterns, writeJSON/writeError helpers, JWT middleware
provides:
  - JobHandler with CRUD for jobs, steps, and dependency edges (10 endpoints)
  - RunHandler with trigger, list, get, cancel, retry (5 endpoints)
  - DAG cycle validation on dependency add with rollback on failure
  - RegisterJobRoutes and RegisterRunRoutes for Chi router integration
affects: [06-job-system]

# Tech tracking
tech-stack:
  added: []
  patterns: [handler package for job system endpoints, flat route registration avoiding Chi mount conflicts]

key-files:
  created:
    - internal/server/handler/jobs.go
    - internal/server/handler/jobs_test.go
    - internal/server/handler/runs.go
    - internal/server/handler/runs_test.go
  modified: []

key-decisions:
  - "Created separate handler package instead of adding to api package for clean separation of job system concerns"
  - "Flat route registration (r.Post/Get paths) instead of r.Route groups to avoid Chi mount conflicts when combining job and run routes"
  - "DAG validation on AddDependency: add edge first, validate, rollback if cycle detected"
  - "RetryStep pre-validates step state (must be failed/skipped/cancelled) before calling orchestrator"

patterns-established:
  - "Handler package pattern: separate package per domain (api for auth/machines, handler for jobs/runs)"
  - "Pre-validation in handlers: check state before delegating to orchestrator for better error messages"

requirements-completed: [JOBS-01, JOBS-02, JOBS-03, JOBS-04]

# Metrics
duration: 8min
completed: 2026-03-12
---

# Phase 6 Plan 2: Job REST API Handlers Summary

**15 REST endpoints for job/step CRUD, dependency management with DAG validation, and run lifecycle (trigger, list, get, cancel, retry) with full TDD integration tests**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-12T12:29:26Z
- **Completed:** 2026-03-12T12:37:26Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Full job/step/dependency CRUD exposed as REST endpoints (10 handlers)
- Run management endpoints: trigger creates run with DAGRunner, cancel stops execution, retry validates state
- DAG cycle detection on dependency add returns 400 and rolls back the edge
- 17 integration tests covering happy paths and error cases (cycle rejection, not-found, retry non-failed)

## Task Commits

Each task was committed atomically:

1. **Task 1: Job and step REST handlers** - `37e5644` (feat)
2. **Task 2: Run REST handlers -- trigger, list, retry, cancel** - `1215d97` (feat)

_Note: TDD tasks had RED/GREEN phases within single commits_

## Files Created/Modified
- `internal/server/handler/jobs.go` - JobHandler with CRUD for jobs, steps, dependencies; DAG validation on add
- `internal/server/handler/jobs_test.go` - 11 integration tests for all job/step/dependency endpoints
- `internal/server/handler/runs.go` - RunHandler with trigger, list, get, cancel, retry
- `internal/server/handler/runs_test.go` - 6 integration tests for run lifecycle

## Decisions Made
- Created `internal/server/handler/` package rather than adding to existing `internal/server/api/` package. The api package depends on concrete *store.Store, auth.Service, and connmgr.ConnectionManager. Job handlers use the JobStoreIface interface and Orchestrator, making a separate package cleaner.
- Used flat route registration (direct r.Post/r.Get calls) instead of r.Route("/api/v1", ...) groups. Chi panics when two Route groups mount on the same path, and both job and run routes share the /api/v1 prefix.
- AddDependency handler adds the edge first, then validates the full DAG. If a cycle is detected, it rolls back (removes) the edge before returning 400.
- RetryStep handler pre-checks that the target step is in a retryable state (failed/skipped/cancelled) before delegating to the orchestrator. This provides a clear 400 error instead of relying on orchestrator internals.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Used handler package instead of plan-specified paths**
- **Found during:** Task 1 (initial setup)
- **Issue:** Plan specified `internal/server/handler/jobs.go` but project convention has handlers in `internal/server/api/`. However, api package has concrete dependencies unsuitable for job handlers.
- **Fix:** Created new `internal/server/handler/` package as plan specified, which turned out to be the correct approach for dependency separation.
- **Files modified:** internal/server/handler/jobs.go
- **Verification:** All tests pass with clean package boundaries.
- **Committed in:** 37e5644 (Task 1 commit)

**2. [Rule 1 - Bug] Fixed Chi route mount conflict**
- **Found during:** Task 2 (combining job and run routes)
- **Issue:** Chi panics when two r.Route() groups mount on the same path ("/api/v1"). Using Route groups for both job and run handlers caused runtime panic.
- **Fix:** Changed both RegisterJobRoutes and RegisterRunRoutes to use flat route registration instead of Route groups.
- **Files modified:** internal/server/handler/jobs.go, internal/server/handler/runs.go
- **Verification:** All 17 tests pass without panic.
- **Committed in:** 1215d97 (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug)
**Impact on plan:** Both fixes necessary for correct operation. No scope creep.

## Issues Encountered
None beyond the auto-fixed deviations.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All 15 REST endpoints implemented and tested
- Ready for router integration with main server (wire into NewRouter)
- Ready for frontend job editor (plan 06-03) to consume these endpoints

## Self-Check: PASSED

All 4 created files verified present on disk. Both commit hashes (37e5644, 1215d97) verified in git log.

---
*Phase: 06-job-system*
*Completed: 2026-03-12*
