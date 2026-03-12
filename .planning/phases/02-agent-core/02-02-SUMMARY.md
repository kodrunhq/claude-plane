---
phase: 02-agent-core
plan: 02
subsystem: agent
tags: [pty, session, creack-pty, goroutine, channel]

requires:
  - phase: 02-agent-core/01
    provides: SessionProvider interface in client.go
  - phase: 01-foundation
    provides: proto types (ServerCommand, AgentEvent, SessionState)
provides:
  - Session type: PTY-backed process with read loop, input, resize, kill
  - SessionManager type: thread-safe registry implementing SessionProvider
  - Command dispatch: create, input, resize, kill routed to sessions
  - Output relay: session output forwarded to gRPC send channel
affects: [03-server-core, 04-terminal-streaming]

tech-stack:
  added: [github.com/creack/pty v1.1.24]
  patterns: [readDone channel for goroutine coordination, non-blocking channel send with drop]

key-files:
  created:
    - internal/agent/session.go
    - internal/agent/session_test.go
    - internal/agent/session_manager.go
    - internal/agent/session_manager_test.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "readLoop signals readDone, waitForExit closes outputCh after status is set -- guarantees consumers see final status"
  - "Non-blocking send to outputCh with drop on full -- prevents slow consumers from blocking PTY read"
  - "SessionManager sends SessionStatusChanged on create and on exit via relay goroutines"

patterns-established:
  - "readDone channel pattern: coordinate readLoop/waitForExit without closing channel from wrong goroutine"
  - "Non-blocking channel send with select/default for backpressure management"

requirements-completed: [AGNT-03]

duration: 8min
completed: 2026-03-12
---

# Phase 02 Plan 02: PTY Session Management Summary

**PTY session lifecycle with creack/pty, thread-safe session manager implementing SessionProvider, command dispatch, and output relay to gRPC channel**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-12T08:52:18Z
- **Completed:** 2026-03-12T08:59:52Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Session spawns processes in PTYs via pty.StartWithSize, reads output to buffered channel, handles input/resize/kill
- SessionManager implements SessionProvider interface with thread-safe session registry
- Command dispatch routes CreateSession, InputData, ResizeTerminal, KillSession to correct sessions
- Output relay forwards session output as AgentEvent to gRPC send channel
- All tests pass with -race detector (count=3 stability verified)

## Task Commits

Each task was committed atomically:

1. **Task 1: PTY session lifecycle** - `9de70d8` (test), `2c64a80` (feat)
2. **Task 2: Session manager** - `c62c370` (test), `5a6ddd0` (feat), `9c77445` (fix: race)

_TDD tasks have RED (test) and GREEN (feat) commits._

## Files Created/Modified
- `internal/agent/session.go` - PTY session: spawn, readLoop, writeInput, resize, kill, waitForExit
- `internal/agent/session_test.go` - Tests: spawn+read, write input, exit status, resize
- `internal/agent/session_manager.go` - Thread-safe registry implementing SessionProvider
- `internal/agent/session_manager_test.go` - Tests: create, input, relay, concurrent, get-states
- `go.mod` / `go.sum` - Added creack/pty dependency

## Decisions Made
- readLoop signals completion via readDone channel; waitForExit closes outputCh after setting status. This guarantees consumers see the final status when they detect channel close.
- Non-blocking send to outputCh drops data when channel is full (cap 256) to prevent slow consumers from blocking the PTY read loop.
- AttachSession/DetachSession are logged no-ops deferred to Phase 4.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed data race between readLoop and waitForExit closing outputCh**
- **Found during:** Task 2 (full test suite with -race)
- **Issue:** readLoop was sending to outputCh while waitForExit closed it, causing a data race detected by -race
- **Fix:** readLoop only signals readDone; waitForExit waits for readDone then closes outputCh
- **Files modified:** internal/agent/session.go
- **Verification:** go test ./... -count=3 -race passes consistently
- **Committed in:** 9c77445

**2. [Rule 1 - Bug] Fixed flaky TestSessionExitStatus -- status not set before outputCh close**
- **Found during:** Task 2 (full test suite)
- **Issue:** For fast-exiting processes, kernel closes PTY slave causing readLoop to exit and close outputCh before waitForExit sets status
- **Fix:** Same fix as #1 -- waitForExit owns outputCh close, ensuring status is always set first
- **Files modified:** internal/agent/session.go
- **Verification:** go test ./... -count=3 -race all green
- **Committed in:** 9c77445

---

**Total deviations:** 2 auto-fixed (2 bugs, same commit)
**Impact on plan:** Race condition fix was essential for correctness. No scope creep.

## Issues Encountered
- Proto ExitCode field is `*int32` (optional), not `int32` -- adjusted SessionManager.GetStates accordingly.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- SessionProvider interface fully implemented, ready for AgentClient integration
- Agent can spawn PTY sessions, relay output, and handle server commands
- AttachSession/DetachSession deferred to Phase 4 (Terminal Streaming)

---
*Phase: 02-agent-core*
*Completed: 2026-03-12*
