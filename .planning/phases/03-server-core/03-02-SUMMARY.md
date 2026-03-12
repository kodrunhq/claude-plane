---
phase: 03-server-core
plan: 02
subsystem: server
tags: [sqlite, grpc, connection-management, thread-safety]

requires:
  - phase: 01-foundation
    provides: "Store layer with SQLite writer/reader pools and migrations"
  - phase: 02-agent-core
    provides: "gRPC server with mTLS auth interceptors and AgentService"
provides:
  - "Machine CRUD operations on SQLite (UpsertMachine, UpdateMachineStatus, ListMachines, GetMachine)"
  - "DB-backed connection manager tracking live agent connections"
  - "gRPC AgentStream integration registering/disconnecting agents with connection manager"
affects: [03-server-core, 04-terminal-streaming, 05-frontend]

tech-stack:
  added: []
  patterns: [MachineStore-interface-for-testable-dependency, in-memory-map-with-DB-persistence]

key-files:
  created:
    - internal/server/store/machines.go
    - internal/server/store/machines_test.go
    - internal/server/connmgr/manager.go
    - internal/server/connmgr/manager_test.go
  modified:
    - internal/server/grpc/server.go
    - internal/server/grpc/auth_test.go
    - internal/agent/client_test.go

key-decisions:
  - "MachineStore interface in connmgr for testable store dependency via mock"
  - "Machine struct matches actual DB schema (no updated_at column)"
  - "Connection manager uses RWMutex with DB operations outside lock to prevent lock contention"
  - "agentConnMgr is nullable in NewGRPCServer to maintain backward compatibility in tests"

patterns-established:
  - "MachineStore interface: connmgr depends on interface not concrete store"
  - "DB operations outside lock: Lock for map mutation, unlock before I/O"

requirements-completed: [AGNT-04]

duration: 6min
completed: 2026-03-12
---

# Phase 03 Plan 02: Machine Store, Connection Manager & gRPC Wiring Summary

**Machine CRUD on SQLite, in-memory connection manager with DB-backed status, and gRPC AgentStream integration for agent online/offline tracking**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-12T09:41:18Z
- **Completed:** 2026-03-12T09:47:13Z
- **Tasks:** 3
- **Files modified:** 7

## Accomplishments
- Machine store with UpsertMachine, UpdateMachineStatus, ListMachines, GetMachine against SQLite
- Connection manager tracks live agents in-memory with DB status persistence on register/disconnect
- gRPC CommandStream registers agents with connmgr on stream start and disconnects on stream end
- Thread-safe under concurrent access (verified with race detector, 50 goroutines)

## Task Commits

Each task was committed atomically:

1. **Task 1: Machine store CRUD operations** - `3e7c3c7` (feat, TDD)
2. **Task 2: Connection manager with DB-backed status tracking** - `ea9c5f0` (feat, TDD)
3. **Task 3: Wire gRPC AgentService to connection manager** - `ddfab46` (feat)

## Files Created/Modified
- `internal/server/store/machines.go` - Machine struct, UpsertMachine, UpdateMachineStatus, ListMachines, GetMachine
- `internal/server/store/machines_test.go` - 5 tests covering CRUD and not-found
- `internal/server/connmgr/manager.go` - ConnectionManager, MachineStore interface, Register/Disconnect/GetAgent/ListAgents
- `internal/server/connmgr/manager_test.go` - 5 tests including concurrent access with race detector
- `internal/server/grpc/server.go` - Added connmgr.ConnectionManager dependency, wired into CommandStream
- `internal/server/grpc/auth_test.go` - Updated NewGRPCServer call signature
- `internal/agent/client_test.go` - Updated NewGRPCServer call signature

## Decisions Made
- MachineStore interface in connmgr for testable dependency injection via mock
- Machine struct matches actual DB schema (without updated_at since table lacks it)
- DB operations performed outside mutex lock to prevent lock contention on I/O
- agentConnMgr parameter is nullable in NewGRPCServer for backward compatibility in existing tests

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Updated NewGRPCServer callers for new signature**
- **Found during:** Task 3
- **Issue:** Adding connmgr parameter to NewGRPCServer broke 3 call sites in test files
- **Fix:** Updated all callers to pass nil for agentConnMgr
- **Files modified:** internal/agent/client_test.go, internal/server/grpc/auth_test.go
- **Verification:** go build ./... passes
- **Committed in:** ddfab46 (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Required fix for compilation. No scope creep.

## Issues Encountered
- Pre-existing orphaned tokens_test.go and tokens.go files in store package (untracked, not committed). They reference a revoked_tokens table not in migrations. Out of scope for this plan; did not fix.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Machine store and connection manager ready for REST API layer
- gRPC service properly tracks agent connections for online/offline display
- AgentInfo DTO ready for REST responses

---
*Phase: 03-server-core*
*Completed: 2026-03-12*
