---
phase: 04-terminal-streaming
plan: 02
subsystem: api, session, websocket
tags: [websocket, grpc, sqlite, flow-control, terminal, chi-router]

requires:
  - phase: 03-server-core
    provides: "Chi router, JWT middleware, ConnectionManager, SQLite store"
provides:
  - "Session store (CRUD) in SQLite"
  - "In-memory session registry with flow control channels"
  - "REST session handlers (create, list, get, terminate)"
  - "WebSocket terminal bridge (binary I/O relay)"
  - "gRPC event routing (SessionOutput, ScrollbackChunk to registry)"
  - "ConnectedAgent.SendCommand for type-safe command dispatch"
affects: [05-frontend-core, 04-terminal-streaming]

tech-stack:
  added: [github.com/coder/websocket v1.8.14]
  patterns: [subscriber-channel flow control, single-writer WebSocket goroutine, query-param token auth for WS]

key-files:
  created:
    - internal/server/session/registry.go
    - internal/server/session/handler.go
    - internal/server/session/ws.go
    - internal/server/store/sessions.go
    - internal/server/session/registry_test.go
    - internal/server/session/handler_test.go
    - internal/server/session/ws_test.go
    - internal/server/store/sessions_test.go
  modified:
    - internal/server/api/router.go
    - internal/server/grpc/server.go
    - internal/server/connmgr/manager.go

key-decisions:
  - "Added SendCommand func field to ConnectedAgent to dispatch commands without proto imports in connmgr"
  - "Used coder/websocket library for WebSocket (modern, single-dependency, context-aware)"
  - "Query-param token auth for WebSocket since browsers cannot set Authorization header during upgrade"
  - "Buffered channel cap 256 with non-blocking drop for slow WebSocket consumers"
  - "WebSocket close sends DetachSessionCmd (not KillSessionCmd) to preserve sessions"
  - "Single writer goroutine per WebSocket connection for thread-safe writes"
  - "ClaimsGetter function type to decouple session handler from api package (avoid import cycle)"

patterns-established:
  - "Flow control: buffered channels with select-default drop for slow consumers"
  - "WebSocket auth: query param ?token= validated by auth service"
  - "Command dispatch: ConnectedAgent.SendCommand function field for type-safe gRPC command sending"

requirements-completed: [SESS-01, SESS-02, SESS-05, TERM-01, TERM-02, TERM-04]

duration: 10min
completed: 2026-03-12
---

# Phase 04 Plan 02: Server Session Management Summary

**Session store, in-memory registry with flow-control channels, REST lifecycle handlers, WebSocket terminal bridge, and gRPC event routing for browser-to-agent terminal streaming**

## Performance

- **Duration:** 10 min
- **Started:** 2026-03-12T10:21:30Z
- **Completed:** 2026-03-12T10:31:18Z
- **Tasks:** 2
- **Files modified:** 11

## Accomplishments
- Session CRUD operations persisted in SQLite with FK constraints to machines and users
- In-memory session registry routes terminal output to WebSocket subscribers with non-blocking flow control (cap 256 buffered channels, drop policy for slow consumers)
- REST API endpoints: POST/GET/DELETE /api/v1/sessions for complete session lifecycle
- WebSocket terminal bridge at /ws/terminal/:sessionID relays binary terminal data bidirectionally between browser and agent
- gRPC service routes SessionOutput and ScrollbackChunk events from agents to session registry
- All 19 tests pass with race detector

## Task Commits

Each task was committed atomically:

1. **Task 1: Session store and session registry (RED)** - `a6e74a1` (test)
2. **Task 1: Session store and session registry (GREEN)** - `46c0525` (feat)
3. **Task 2: REST handlers, WebSocket bridge, gRPC routing** - `7ce40e4` (feat)

## Files Created/Modified
- `internal/server/store/sessions.go` - Session CRUD operations on SQLite
- `internal/server/store/sessions_test.go` - Session store tests
- `internal/server/session/registry.go` - In-memory session-to-subscriber routing with flow control
- `internal/server/session/registry_test.go` - Registry tests (subscribe, publish, slow consumer, multi-sub)
- `internal/server/session/handler.go` - REST handlers for session create, list, get, terminate
- `internal/server/session/handler_test.go` - Handler tests with mock agent
- `internal/server/session/ws.go` - WebSocket terminal bridge between browser and agent
- `internal/server/session/ws_test.go` - WebSocket tests (binary relay, input, resize, close/detach, flow control)
- `internal/server/api/router.go` - Added session routes and WebSocket endpoint to chi router
- `internal/server/grpc/server.go` - Routes agent events to session registry, thread-safe SendCommand
- `internal/server/connmgr/manager.go` - Added SendCommand func field to ConnectedAgent

## Decisions Made
- Added `SendCommand func(cmd interface{}) error` to `ConnectedAgent` to enable command dispatch without importing proto in connmgr package
- Used `coder/websocket` library (modern, context-aware, single dependency)
- WebSocket authentication via query param `?token=` since browsers cannot set Authorization headers during WebSocket upgrade
- `ClaimsGetter` function type decouples session handler from api package, preventing import cycles
- Single writer goroutine per WebSocket connection prevents concurrent write panics
- WebSocket close triggers DetachSessionCmd (not KillSessionCmd) so sessions survive browser disconnection

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed FK constraint violations in session store tests**
- **Found during:** Task 1 (Session store tests)
- **Issue:** Tests passed user_id "user-1" which didn't exist in users table, causing FK violation
- **Fix:** Used empty user_id (stored as NULL) and ensured machine records exist before creating sessions
- **Files modified:** internal/server/store/sessions.go, internal/server/store/sessions_test.go
- **Committed in:** 46c0525

**2. [Rule 1 - Bug] Fixed time.Time scanning for nullable ended_at column**
- **Found during:** Task 1 (Session store tests)
- **Issue:** COALESCE(ended_at, started_at) returned string that couldn't scan into time.Time
- **Fix:** Used sql.NullTime for ended_at column, fell back to CreatedAt when NULL
- **Files modified:** internal/server/store/sessions.go
- **Committed in:** 46c0525

**3. [Rule 1 - Bug] Fixed ResizeTerminalCmd field structure**
- **Found during:** Task 2 (WebSocket handler)
- **Issue:** Proto uses nested TerminalSize struct, not direct Cols/Rows fields
- **Fix:** Used &pb.TerminalSize{Cols, Rows} inside ResizeTerminalCmd.Size
- **Files modified:** internal/server/session/ws.go
- **Committed in:** 7ce40e4

**4. [Rule 3 - Blocking] Added SendCommand to ConnectedAgent**
- **Found during:** Task 2 (Session handler needs to send commands to agent)
- **Issue:** ConnectedAgent had no way to dispatch commands; Stream was interface{}
- **Fix:** Added SendCommand func field, set by gRPC service with mutex-protected stream.Send
- **Files modified:** internal/server/connmgr/manager.go, internal/server/grpc/server.go
- **Committed in:** 7ce40e4

---

**Total deviations:** 4 auto-fixed (3 bugs, 1 blocking)
**Impact on plan:** All auto-fixes necessary for correctness. No scope creep.

## Issues Encountered
None beyond the auto-fixed deviations above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Server-side session management complete, ready for agent-side session handling (04-01)
- WebSocket bridge ready for frontend terminal integration (Phase 5)
- Flow control tested and operational for high-throughput terminal output

---
*Phase: 04-terminal-streaming*
*Completed: 2026-03-12*
