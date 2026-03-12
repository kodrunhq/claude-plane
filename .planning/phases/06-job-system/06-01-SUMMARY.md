---
phase: 06-job-system
plan: 01
subsystem: database, orchestrator
tags: [sqlite, dag, kahn-algorithm, job-runner, goroutines]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: SQLite Store with writer/reader pools, migrations, uuid generation
provides:
  - JobStore interface with full CRUD for jobs, steps, dependencies, runs, run_steps
  - DAGRunner execution engine with Kahn's algorithm for dependency resolution
  - Orchestrator managing active runs with create, retry, cancel lifecycle
  - Snapshot-on-run pattern for immutable run step configuration
affects: [06-job-system]

# Tech tracking
tech-stack:
  added: []
  patterns: [DAGRunner with Kahn's in-degree tracking, snapshot-on-run, StepExecutor interface, JobStoreIface for DI]

key-files:
  created:
    - internal/server/store/jobs.go
    - internal/server/store/jobs_test.go
    - internal/server/orchestrator/dag_runner.go
    - internal/server/orchestrator/dag_runner_test.go
    - internal/server/orchestrator/orchestrator.go
    - internal/server/orchestrator/orchestrator_test.go
    - internal/server/orchestrator/helpers_test.go
  modified:
    - internal/server/store/migrations.go

key-decisions:
  - "JobStoreIface interface for orchestrator DI and testability"
  - "StepExecutor interface with onComplete callback for async step completion"
  - "DAGRunner uses context cancellation for run failure/cancellation propagation"
  - "RunStep.OnFailure populated from original Steps at runtime (not stored in DB snapshot)"
  - "Mock executor with channels for deterministic concurrent test control"

patterns-established:
  - "DAGRunner per-run pattern: one DAGRunner instance per active run, mutex-protected state"
  - "Snapshot-on-run: step config copied to run_steps at run creation for immutability"
  - "StepExecutor interface: orchestrator delegates step execution via interface for testability"

requirements-completed: [JOBS-01, JOBS-02, JOBS-04]

# Metrics
duration: 12min
completed: 2026-03-12
---

# Phase 6 Plan 1: Job System Data Layer and DAG Execution Summary

**JobStore CRUD with SQLite persistence, DAGRunner with Kahn's algorithm for parallel dependency-ordered execution, and Orchestrator managing run lifecycle including retry and cancellation**

## Performance

- **Duration:** 12 min
- **Started:** 2026-03-12T12:12:15Z
- **Completed:** 2026-03-12T12:24:37Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Full JobStore with CRUD for jobs, steps, step dependencies, runs, and run_steps with snapshot-on-run
- DAG cycle detection via Kahn's algorithm rejects circular dependencies
- DAGRunner executes steps in dependency order with parallel independent steps via goroutines
- Orchestrator manages active runs: CreateRun (with DAG validation + snapshots), RetryStep (cascade reset), CancelRun
- Race detector passes on all concurrent completion scenarios

## Task Commits

Each task was committed atomically:

1. **Task 1: Job store -- DB queries for jobs, steps, dependencies, runs, run_steps** - `83f52bd` (feat)
2. **Task 2: DAGRunner + Orchestrator -- DAG validation, execution engine, run lifecycle** - `69c23a9` (feat)

_Note: TDD tasks had RED/GREEN phases within single commits_

## Files Created/Modified
- `internal/server/store/jobs.go` - JobStoreIface interface, Job/Step/Run/RunStep types, all CRUD operations
- `internal/server/store/jobs_test.go` - Comprehensive tests for all JobStore methods
- `internal/server/store/migrations.go` - Added on_failure to steps, snapshot columns to run_steps
- `internal/server/orchestrator/dag_runner.go` - ValidateDAG, DAGRunner, StepExecutor interface
- `internal/server/orchestrator/dag_runner_test.go` - Linear, diamond, parallel, cycle, failure tests
- `internal/server/orchestrator/orchestrator.go` - Orchestrator with CreateRun, RetryStep, CancelRun
- `internal/server/orchestrator/orchestrator_test.go` - Integration tests with real SQLite store
- `internal/server/orchestrator/helpers_test.go` - Mock executor with channel-based step control

## Decisions Made
- Used JobStoreIface interface for orchestrator dependency injection instead of concrete Store type
- StepExecutor.ExecuteStep takes an onComplete callback rather than returning a channel, matching async session completion pattern
- DAGRunner uses context.WithCancel for propagating run failure/cancellation to in-flight steps
- RunStep.OnFailure is populated at runtime from original Step definitions (not persisted as a snapshot column) to keep schema simpler
- Mock executor uses buffered channels for deterministic concurrent test control without flakiness

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added on_failure column to steps table and snapshot columns to run_steps**
- **Found during:** Task 1 (Job store implementation)
- **Issue:** Migration schema lacked on_failure column on steps table and snapshot columns (prompt_snapshot, machine_id_snapshot, working_dir_snapshot, command_snapshot, args_snapshot) on run_steps table
- **Fix:** Updated migrations.go to add missing columns
- **Files modified:** internal/server/store/migrations.go
- **Verification:** All store tests pass, snapshot data correctly persisted and retrieved
- **Committed in:** 83f52bd (Task 1 commit)

**2. [Rule 3 - Blocking] Used NULL for empty user_id FK in job creation**
- **Found during:** Task 1 (Job store testing)
- **Issue:** FK constraint failed when inserting jobs with empty user_id string (references users table)
- **Fix:** Added nullIfEmpty helper to convert empty strings to SQL NULL for FK columns
- **Files modified:** internal/server/store/jobs.go
- **Verification:** All job CRUD tests pass without FK violations
- **Committed in:** 83f52bd (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (2 blocking)
**Impact on plan:** Both fixes necessary for correct database operations. No scope creep.

## Issues Encountered
None beyond the auto-fixed deviations.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Job data layer and execution engine complete
- Ready for REST API handlers (plan 06-02) to expose job CRUD and run management
- Ready for frontend job editor (plan 06-03) to build DAG canvas

## Self-Check: PASSED

All 7 created files verified present on disk. Both commit hashes (83f52bd, 69c23a9) verified in git log.

---
*Phase: 06-job-system*
*Completed: 2026-03-12*
