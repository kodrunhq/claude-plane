---
phase: 01-foundation
plan: 03
subsystem: database
tags: [sqlite, wal, argon2id, store, migrations, admin-seeding]

# Dependency graph
requires:
  - phase: 01-foundation-01
    provides: Go module scaffold with modernc.org/sqlite dependency
provides:
  - SQLite store layer with dual connection pools (writer/reader)
  - Full database schema (10 tables with indexes)
  - Admin account seeding with Argon2id password hashing
  - User CRUD operations (GetUserByEmail)
affects: [auth, sessions, jobs, api]

# Tech tracking
tech-stack:
  added: [modernc.org/sqlite, golang.org/x/crypto/argon2]
  patterns: [single-writer-sqlite, argon2id-password-hashing, dual-connection-pool]

key-files:
  created:
    - internal/server/store/db.go
    - internal/server/store/migrations.go
    - internal/server/store/users.go
    - internal/server/store/db_test.go
    - internal/server/store/users_test.go
  modified: []

key-decisions:
  - "Set pragmas explicitly after sql.Open instead of via DSN params -- modernc.org/sqlite does not support _pragma DSN syntax"
  - "Used BEGIN IMMEDIATE + raw SQL exec for migrations instead of transaction wrapper for simplicity"

patterns-established:
  - "Single-writer SQLite: writer pool MaxOpenConns=1, reader pool MaxOpenConns=4, explicit PRAGMA setup per pool"
  - "Argon2id hashing: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash> format with OWASP params"
  - "UUID generation via crypto/rand with RFC 4122 v4 formatting"

requirements-completed: [INFR-03, AUTH-04]

# Metrics
duration: 3min
completed: 2026-03-12
---

# Phase 1 Plan 3: SQLite Store Summary

**SQLite store with WAL mode, 10-table schema, and Argon2id admin seeding**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-12T07:21:42Z
- **Completed:** 2026-03-12T07:25:11Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- SQLite store with dual connection pools (single-writer, multi-reader) and WAL mode
- Complete schema with all 10 tables and indexes created idempotently
- Admin seeding with Argon2id password hashing and graceful duplicate handling
- 7 tests covering store initialization, migrations, and user operations

## Task Commits

Each task was committed atomically:

1. **Task 1: SQLite store with WAL mode and full schema** - `ebc627b` (test RED), `7a6d7c5` (feat GREEN)
2. **Task 2: Admin seeding with Argon2id password hashing** - `7f9ed67` (test RED), `f770713` (feat GREEN)

_TDD tasks have separate test and implementation commits._

## Files Created/Modified
- `internal/server/store/db.go` - Store struct with dual connection pools, pragma setup, NewStore/Close
- `internal/server/store/migrations.go` - Full schema SQL with all 10 tables and indexes
- `internal/server/store/users.go` - User model, HashPassword, VerifyPassword, SeedAdmin, GetUserByEmail
- `internal/server/store/db_test.go` - Tests for WAL mode, foreign keys, migrations idempotency, single-writer
- `internal/server/store/users_test.go` - Tests for password hashing, admin seeding, duplicate handling

## Decisions Made
- Set SQLite pragmas explicitly after sql.Open rather than via DSN parameters, because modernc.org/sqlite does not support the `_pragma` prefix DSN syntax that mattn/go-sqlite3 uses
- Used BEGIN IMMEDIATE with inline SQL for migrations rather than Go transaction wrapper for simplicity

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed PRAGMA application via DSN parameters**
- **Found during:** Task 1 (SQLite store initialization)
- **Issue:** modernc.org/sqlite does not support `_journal_mode=WAL` and `_foreign_keys=ON` as DSN parameters. WAL mode showed as "delete" and foreign keys as 0.
- **Fix:** Changed to explicit PRAGMA statements after sql.Open. WAL mode set on writer (database-level), foreign_keys/busy_timeout/synchronous set on both pools.
- **Files modified:** internal/server/store/db.go
- **Verification:** TestNewStore passes with WAL mode and foreign_keys=1
- **Committed in:** 7a6d7c5 (Task 1 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Essential fix for correctness. The research doc's example used DSN params which don't work with modernc.org/sqlite. No scope creep.

## Issues Encountered
- Pre-existing test failure in `internal/agent/config` (LoadAgentConfig undefined) -- out of scope for this plan, not related to store changes.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Store layer ready for use by server config (Plan 02) and all future phases
- Schema includes all tables needed through Phase 6
- Admin seeding ready for CLI integration (seed-admin command)

## Self-Check: PASSED

All 5 created files verified. All 4 task commits verified.

---
*Phase: 01-foundation*
*Completed: 2026-03-12*
