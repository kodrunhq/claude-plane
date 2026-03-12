---
phase: 02-agent-core
plan: 01
subsystem: grpc
tags: [grpc, mtls, reconnect, backoff, bidirectional-streaming]

requires:
  - phase: 01-foundation
    provides: "Generated gRPC stubs, mTLS tlsutil, agent config"
provides:
  - "Server gRPC listener with mTLS auth interceptors and connection manager"
  - "Agent gRPC client with reconnect loop, registration, and bidi stream"
  - "Exponential backoff with jitter for agent reconnection"
  - "SessionProvider interface for session manager decoupling"
affects: [02-agent-core, 03-session-lifecycle, 04-terminal-streaming]

tech-stack:
  added: [google.golang.org/grpc/keepalive, google.golang.org/grpc/credentials, google.golang.org/grpc/peer]
  patterns: [mTLS-auth-interceptor, context-machine-id-extraction, channel-based-sender-goroutine, exponential-backoff-with-jitter]

key-files:
  created:
    - internal/server/grpc/auth.go
    - internal/server/grpc/server.go
    - internal/server/grpc/connection_mgr.go
    - internal/server/grpc/auth_test.go
    - internal/agent/client.go
    - internal/agent/client_test.go
    - internal/agent/backoff.go
    - internal/agent/backoff_test.go
  modified: []

key-decisions:
  - "Channel-based sender goroutine pattern prevents concurrent stream.Send calls"
  - "Machine-id extracted from cert CN with agent- prefix validation in interceptors"
  - "SessionProvider interface decouples agent client from session manager for Plan 02"

patterns-established:
  - "mTLS interceptor pattern: extractMachineID -> context.WithValue -> handler"
  - "wrappedServerStream for enriching stream context in streaming interceptors"
  - "Sender goroutine with channel for thread-safe bidi stream Send"
  - "Reconnect loop with exponential backoff (1s-60s) and 20% jitter"

requirements-completed: [AGNT-02, AGNT-03]

duration: 7min
completed: 2026-03-12
---

# Phase 2 Plan 1: Agent gRPC Connection Lifecycle Summary

**Server mTLS gRPC listener with machine-id auth interceptors and agent client with reconnect state machine, exponential backoff, and bidirectional streaming**

## Performance

- **Duration:** 7 min
- **Started:** 2026-03-12T08:39:55Z
- **Completed:** 2026-03-12T08:47:00Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Server gRPC listener with mTLS accepting agent connections, extracting machine-id from cert CN, rejecting invalid certs
- Connection manager tracking connected agents with thread-safe map (Add/Remove/Get/List)
- Agent gRPC client with mTLS dial, Register RPC, and persistent CommandStream bidi stream
- Exponential backoff with 20% jitter for reconnection (1s min, 60s max, doubles each attempt)
- Full integration tests: mTLS auth, CA mismatch rejection, reconnect after server restart
- All tests pass with race detector enabled

## Task Commits

Each task was committed atomically:

1. **Task 1: Server gRPC listener with mTLS and machine-id auth interceptors** - `5a1ca44` (feat)
2. **Task 2: Agent gRPC client with mTLS dial, register, bidi stream, and reconnect loop** - `46e4936` (feat)

## Files Created/Modified
- `internal/server/grpc/auth.go` - mTLS interceptors for machine-id extraction and validation
- `internal/server/grpc/server.go` - GRPCServer with mTLS, keepalive, AgentService implementation
- `internal/server/grpc/connection_mgr.go` - Thread-safe connected agent tracking
- `internal/server/grpc/auth_test.go` - Integration tests: valid cert, no peer, invalid CN, concurrent access
- `internal/agent/client.go` - AgentClient with reconnect loop, register, bidi stream, sender goroutine
- `internal/agent/client_test.go` - Integration tests: connect+register, mTLS rejection, reconnect on drop
- `internal/agent/backoff.go` - Exponential backoff with jitter
- `internal/agent/backoff_test.go` - Backoff increasing, cap, reset tests

## Decisions Made
- Channel-based sender goroutine pattern prevents concurrent stream.Send calls (per research Pitfall 1)
- Machine-id extracted from cert CN with "agent-" prefix validation in both unary and stream interceptors
- SessionProvider interface decouples agent client from session manager (to be wired in Plan 02)
- Keepalive: 30s time, 10s timeout, 15s min enforcement, permit without stream on both sides

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Server gRPC listener and agent client ready for session management wiring (Plan 02)
- SessionProvider interface defined; session manager will implement GetStates, HandleCommand, StartRelay, StopRelay
- CommandStream recv loop dispatches to SessionProvider.HandleCommand -- server-side command dispatch placeholder ready

---
*Phase: 02-agent-core*
*Completed: 2026-03-12*
