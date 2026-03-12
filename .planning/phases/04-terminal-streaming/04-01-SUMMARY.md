---
phase: 04-terminal-streaming
plan: 01
subsystem: agent
tags: [pty, scrollback, asciicast, session-lifecycle, attach-detach]

# Dependency graph
requires:
  - phase: 02-agent-core
    provides: "Session struct with PTY lifecycle, SessionManager with relay"
provides:
  - "ScrollbackWriter for asciicast v2 JSONL persistence"
  - "ReadScrollbackChunks for chunked scrollback replay"
  - "Session with scrollback integration (tees PTY output to file)"
  - "SessionManager with attach/detach/scrollback replay command handling"
affects: [04-terminal-streaming, 05-frontend-terminal]

# Tech tracking
tech-stack:
  added: []
  patterns: ["asciicast v2 JSONL scrollback format", "attach/detach lifecycle without PTY termination"]

key-files:
  created:
    - internal/agent/scrollback.go
    - internal/agent/scrollback_test.go
  modified:
    - internal/agent/session.go
    - internal/agent/session_manager.go
    - internal/agent/session_test.go
    - internal/agent/session_manager_test.go

key-decisions:
  - "Used json.Marshal for scrollback data escaping to handle binary, control chars, quotes safely"
  - "32KB chunk size for scrollback replay balances memory usage and network efficiency"
  - "New sessions auto-attach for backward compatibility with existing relay behavior"

patterns-established:
  - "Scrollback tee pattern: readLoop writes to both outputCh and ScrollbackWriter"
  - "Attach/detach state: attached map controls live relay without affecting PTY lifecycle"

requirements-completed: [SESS-03, SESS-06, TERM-03]

# Metrics
duration: 5min
completed: 2026-03-12
---

# Phase 4 Plan 1: Scrollback & Attach/Detach Summary

**Asciicast v2 scrollback persistence with session attach/detach lifecycle -- every PTY session writes timestamped JSONL output, scrollback replays in chunks on attach, detach leaves PTY running**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-12T10:20:53Z
- **Completed:** 2026-03-12T10:26:00Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- ScrollbackWriter creates valid asciicast v2 JSONL files with properly escaped data
- All PTY output is teed to scrollback files via readLoop integration
- Scrollback replay streams chunks with byte offsets and final markers via RequestScrollbackCmd
- Detach stops live relay without killing PTY (SESS-03)
- Sessions survive disconnection with scrollback persistence (SESS-06)

## Task Commits

Each task was committed atomically:

1. **Task 1: Scrollback writer and reader** - `8dcd332` (feat)
2. **Task 2: Enhance session and session manager with scrollback integration and attach/detach** - `9d66e26` (feat)

_Both tasks followed TDD: tests written first, then implementation._

## Files Created/Modified
- `internal/agent/scrollback.go` - ScrollbackWriter and ReadScrollbackChunks for asciicast v2 JSONL
- `internal/agent/scrollback_test.go` - Tests for write, read, special chars, chunking, offsets
- `internal/agent/session.go` - Enhanced with scrollback writer integration and dataDir parameter
- `internal/agent/session_manager.go` - Added attach/detach/scrollback replay handlers and attached state tracking
- `internal/agent/session_test.go` - Added scrollback creation, content, and detach tests
- `internal/agent/session_manager_test.go` - Added scrollback replay and detach-keeps-PTY tests

## Decisions Made
- Used `json.Marshal(string(data))` for scrollback data escaping -- handles binary, control chars, quotes, backslashes safely without custom escaping
- 32KB chunk size for scrollback replay provides good balance of memory and network efficiency
- New sessions are auto-attached (attached map defaults to true) to preserve backward compatibility with existing relay tests
- Scrollback write errors are logged but do not stop the readLoop to prevent scrollback I/O issues from affecting session operation

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Scrollback foundation ready for server-side WebSocket relay (Plan 02)
- Attach/detach protocol complete for server-to-agent command forwarding
- ScrollbackChunkEvent proto messages exercised end-to-end in tests

---
*Phase: 04-terminal-streaming*
*Completed: 2026-03-12*
