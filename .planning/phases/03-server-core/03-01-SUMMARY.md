---
phase: 03-server-core
plan: 01
subsystem: auth
tags: [jwt, hs256, blocklist, sqlite, token-revocation]

requires:
  - phase: 01-foundation
    provides: SQLite store with writer/reader pools, TOML config, user model
provides:
  - JWT issuance and validation service (HS256, algorithm enforcement)
  - Token revocation blocklist with SQLite persistence
  - Revoked token store layer (persist, load, cleanup)
  - Config auth section with JWTSecretFile and TokenTTL
affects: [03-server-core, 04-terminal-streaming]

tech-stack:
  added: [github.com/golang-jwt/jwt/v5]
  patterns: [TokenRevoker interface for blocklist decoupling, RWMutex-protected in-memory cache with DB backing]

key-files:
  created:
    - internal/server/auth/jwt.go
    - internal/server/auth/jwt_test.go
    - internal/server/auth/blocklist.go
    - internal/server/auth/blocklist_test.go
    - internal/server/store/tokens.go
    - internal/server/store/tokens_test.go
  modified:
    - internal/server/store/migrations.go
    - internal/server/config/config.go

key-decisions:
  - "TokenRevoker interface decouples JWT service from concrete Blocklist for testability"
  - "Blocklist uses RWMutex with in-memory map backed by SQLite for fast lookups with persistence"
  - "UUID v4 for JTI via google/uuid (already an indirect dep)"

patterns-established:
  - "TokenRevoker interface: services depend on interfaces, not concrete types"
  - "In-memory cache + DB persistence: fast reads, durable writes, periodic cleanup"

requirements-completed: [AUTH-02, AUTH-03]

duration: 4min
completed: 2026-03-12
---

# Phase 3 Plan 01: JWT Auth Service Summary

**HS256 JWT service with token issuance/validation, in-memory blocklist backed by SQLite revocation store, and config auth extensions**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-12T09:41:32Z
- **Completed:** 2026-03-12T09:45:48Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- JWT service issues HS256 tokens with UUID JTI, validates with algorithm enforcement (rejects alg:none and HS384 attacks), checks blocklist
- Token revocation persists to SQLite and survives blocklist reconstruction on restart
- Blocklist is thread-safe (RWMutex), with periodic cleanup of expired entries
- Config extended with JWTSecretFile and TokenTTL fields (backward-compatible)

## Task Commits

Each task was committed atomically:

1. **Task 1: Token store persistence and JWT auth service** - `b2087ba` (feat)
2. **Task 2: Blocklist with persistence and config auth section** - `8803cea` (feat)

## Files Created/Modified
- `internal/server/auth/jwt.go` - JWT Claims, Service (IssueToken, ValidateToken, RevokeToken)
- `internal/server/auth/jwt_test.go` - 7 tests: issue, validate, expired, wrong-alg, revoked, uniqueness, wrong-signature
- `internal/server/auth/blocklist.go` - In-memory blocklist with DB persistence and cleanup
- `internal/server/auth/blocklist_test.go` - 5 tests: load-from-DB, add, not-revoked, cleanup, concurrency
- `internal/server/store/tokens.go` - RevokedToken struct, RevokeToken, LoadActiveRevocations, CleanExpired
- `internal/server/store/tokens_test.go` - 2 tests: revoke-and-load, clean-expired
- `internal/server/store/migrations.go` - Added revoked_tokens table migration
- `internal/server/config/config.go` - Added JWTSecretFile, TokenTTL fields and ParseTokenTTL helper

## Decisions Made
- Used TokenRevoker interface to decouple JWT service from concrete Blocklist, enabling easy test doubles
- Blocklist.Add passes empty string for userID when persisting -- the JWT service context doesn't carry userID, and the NOT NULL constraint is satisfied
- Kept existing JWTSecret field in config for backward compatibility alongside new JWTSecretFile and TokenTTL

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- JWT service and blocklist ready for consumption by REST API middleware (Plan 03-03)
- TokenRevoker interface allows middleware to validate tokens without coupling to blocklist implementation

---
*Phase: 03-server-core*
*Completed: 2026-03-12*
