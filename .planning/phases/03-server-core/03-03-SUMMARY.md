---
phase: 03-server-core
plan: 03
subsystem: api
tags: [chi-router, rest-api, jwt-middleware, http-handlers, integration-tests]

requires:
  - phase: 03-server-core
    provides: "JWT auth service (Plan 01) and connection manager (Plan 02)"
  - phase: 01-foundation
    provides: "SQLite store with user model, password hashing"
provides:
  - "Chi router with public and protected route groups"
  - "JWT auth middleware extracting Bearer tokens into request context"
  - "Register, Login, Logout HTTP handlers with full token lifecycle"
  - "Machine list/get handlers with live status overlay from connection manager"
  - "JSON response helpers for consistent error formatting"
  - "CreateUser and IsUniqueViolation store helpers"
affects: [04-terminal-streaming, 05-frontend]

tech-stack:
  added: [github.com/go-chi/chi/v5]
  patterns: [httptest integration tests with full router, context-based claims injection]

key-files:
  created:
    - internal/server/api/router.go
    - internal/server/api/middleware.go
    - internal/server/api/response.go
    - internal/server/api/auth_handler.go
    - internal/server/api/auth_handler_test.go
    - internal/server/api/machine_handler.go
    - internal/server/api/machine_handler_test.go
  modified:
    - internal/server/store/users.go

key-decisions:
  - "Handlers struct holds store, authSvc, connMgr -- single dependency point for all handlers"
  - "Public/protected route split via chi.Group with JWTAuthMiddleware applied to protected group only"
  - "Generic 'invalid credentials' message for both wrong-password and user-not-found to prevent enumeration"

patterns-established:
  - "Integration tests use httptest.NewServer with full chi router and real SQLite for end-to-end coverage"
  - "Context key type is unexported contextKey string to prevent collisions"

requirements-completed: [AUTH-01, AUTH-02, AUTH-03, AGNT-04]

duration: 4min
completed: 2026-03-12
---

# Phase 03 Plan 03: REST API Layer Summary

**Chi router with JWT middleware, auth handlers (register/login/logout with token lifecycle), and machine list handlers with live status overlay**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-12T09:49:24Z
- **Completed:** 2026-03-12T09:53:40Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Chi router with global middleware (RequestID, RealIP, Logger, Recoverer) and public/protected route groups
- Full auth lifecycle: register (201/400/409), login (200/401), logout (200) with token revocation
- Machine list/get handlers overlaying live connection status from connection manager
- 10 integration tests covering auth lifecycle, token revocation verification, and protected endpoint enforcement

## Task Commits

Each task was committed atomically:

1. **Task 1: Router, middleware, and response helpers** - `5f1a0f3` (feat)
2. **Task 2 RED: Failing integration tests** - `02b2e4b` (test, TDD)
3. **Task 2 GREEN: Auth and machine handler implementation** - `09d0077` (feat, TDD)

## Files Created/Modified
- `internal/server/api/router.go` - Chi router with NewHandlers, NewRouter, public/protected groups
- `internal/server/api/middleware.go` - JWTAuthMiddleware, GetClaims, UserClaimsKey
- `internal/server/api/response.go` - writeJSON, writeError helpers
- `internal/server/api/auth_handler.go` - Register, Login, Logout handlers
- `internal/server/api/auth_handler_test.go` - 8 integration tests for auth endpoints
- `internal/server/api/machine_handler.go` - ListMachines, GetMachine handlers with live overlay
- `internal/server/api/machine_handler_test.go` - 2 integration tests for machine endpoints
- `internal/server/store/users.go` - Added CreateUser and IsUniqueViolation

## Decisions Made
- Handlers struct centralizes all dependencies (store, authSvc, connMgr) as single injection point
- Used chi.Group for protected routes with JWTAuthMiddleware applied only to that group
- Login returns same "invalid credentials" for both not-found and wrong-password to prevent user enumeration
- Machine handler overlays live "connected" status from connection manager onto DB records

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added CreateUser and IsUniqueViolation to store**
- **Found during:** Task 2
- **Issue:** Plan called for CreateUser but it did not exist in store/users.go
- **Fix:** Added CreateUser method and IsUniqueViolation helper
- **Files modified:** internal/server/store/users.go
- **Verification:** All tests pass including duplicate email detection
- **Committed in:** 09d0077 (Task 2 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** Required for handler functionality. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- REST API layer complete, ready for WebSocket terminal streaming in Phase 4
- All Phase 3 requirements fulfilled (AUTH-01, AUTH-02, AUTH-03, AGNT-04)
- Handlers struct ready to accept additional dependencies (e.g., WebSocket upgrader)

---
*Phase: 03-server-core*
*Completed: 2026-03-12*
