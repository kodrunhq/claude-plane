---
phase: 06-job-system
plan: 05
subsystem: ui
tags: [typescript, react, frontend, type-alignment]

requires:
  - phase: 06-job-system
    provides: "Backend job/step/run REST API with JSON field names"
  - phase: 06-job-system
    provides: "Frontend job system views and components (06-03)"
provides:
  - "Frontend TypeScript interfaces aligned with backend Go JSON tags"
  - "Correct entity ID resolution across all job system views"
  - "Working DAG edge rendering using depends_on field"
  - "Correct API payload for addDependency"
affects: []

tech-stack:
  added: []
  patterns:
    - "Backend JSON tag names as single source of truth for frontend types"

key-files:
  created: []
  modified:
    - web/src/types/job.ts
    - web/src/api/jobs.ts
    - web/src/components/dag/DAGCanvas.tsx
    - web/src/components/jobs/StepEditor.tsx
    - web/src/views/JobEditor.tsx
    - web/src/views/JobsPage.tsx
    - web/src/views/RunDetail.tsx
    - web/src/stores/runs.ts

key-decisions:
  - "Synthetic edge IDs (dep.depends_on->dep.step_id) since backend StepDependency has no separate id field"
  - "Kept RunStep.error as optional field for future use despite backend not having it"

patterns-established:
  - "Frontend entity types mirror backend JSON tags exactly (job_id, step_id, run_id, run_step_id)"

requirements-completed: [JOBS-01, JOBS-02, JOBS-03, JOBS-04]

duration: 2min
completed: 2026-03-12
---

# Phase 6 Plan 5: Frontend Type Alignment Summary

**Aligned all frontend TypeScript types with backend JSON field names (job_id, step_id, run_id, run_step_id, depends_on, completed_at) fixing entity ID resolution across all job system views**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-12T13:17:20Z
- **Completed:** 2026-03-12T13:19:07Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- All TypeScript interfaces now match backend Go JSON tags exactly
- addDependency API call sends `{ depends_on: ... }` instead of `{ depends_on_step_id: ... }`
- DAG canvas builds edges using `dep.depends_on` as source with synthetic edge IDs
- All views use correct field names for navigation, selection, and display (job_id, step_id, run_id)
- Run elapsed time calculation uses `completed_at` matching backend field name

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix TypeScript types and API client** - `7a0427e` (feat)
2. **Task 2: Update all frontend consumers** - `a5f56f9` (fix)

## Files Created/Modified
- `web/src/types/job.ts` - Renamed all entity ID fields and timestamps to match backend JSON tags
- `web/src/api/jobs.ts` - Fixed addDependency payload field name
- `web/src/components/dag/DAGCanvas.tsx` - Fixed node IDs, edge sources, and edge ID generation
- `web/src/components/jobs/StepEditor.tsx` - Fixed step ID references in callbacks and React keys
- `web/src/views/JobEditor.tsx` - Fixed job/step/run ID references for navigation and selection
- `web/src/views/JobsPage.tsx` - Fixed job ID references for keys, navigation, and display
- `web/src/views/RunDetail.tsx` - Fixed step name lookup and elapsed time calculation
- `web/src/stores/runs.ts` - Fixed fallback RunStep creation to use run_step_id

## Decisions Made
- Used synthetic edge IDs (`${dep.depends_on}->${dep.step_id}`) since backend StepDependency has no separate id field
- Kept RunStep.error as optional field for future use despite backend not having it

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Frontend types fully aligned with backend -- all entity lookups, navigation, and DAG rendering will work against real server data
- No remaining type mismatches between frontend and backend

---
*Phase: 06-job-system*
*Completed: 2026-03-12*

## Self-Check: PASSED
