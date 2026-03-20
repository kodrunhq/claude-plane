# Idle Detection Redesign & Session Bug Fixes

**Date:** 2026-03-21
**Status:** Approved
**Scope:** Agent idle detector rewrite, DB schema fix, frontend fixes

## Problem Statement

The current idle detection system uses prompt marker detection (`❯\u00A0`) to determine when the Claude CLI is waiting for input. This approach is fundamentally broken because the Claude CLI renders the `❯` prompt as a persistent TUI element — it is always visible at the bottom of the terminal, even while Claude is actively working (thinking, running tools, generating output). The marker-based detector produces false positives, causing sessions to incorrectly report `waiting_for_input` during active work.

Additionally, two related bugs were discovered:
1. The `ended_at` DB column is never set for non-terminal status changes, causing the session reaper to miscalculate idle duration.
2. The loading screen on page refresh is nearly invisible (dark text on dark background), making the app appear broken during auth checks.

A third reported issue — status mismatch between session detail and the Command Center/Sessions list — is likely a downstream effect of the idle detector false positives rather than a cache invalidation bug. TanStack Query's `invalidateQueries({ queryKey: ['sessions'] })` already performs prefix matching, which covers all session query variants (`['sessions', filters]`, `['sessions', id]`). Once the idle detector is fixed and status transitions are correct, we expect this mismatch to resolve. If it persists, we will investigate further.

## Design

### 1. Silence-Based Idle Detector

**Replaces:** The current `IdleDetector` with Phase 0/1 state machine, marker detection, rolling buffer, `containsAnyMarker()`, `triggered` flag, `keepAlive` mode, and `ResetToPhase1()`.

**Key insight:** Even during long "Thinking..." phases with no visible text output, the CLI continuously sends bytes through the PTY — spinner animations, token counter updates, subagent status changes. When Claude is truly idle at the prompt, the PTY goes completely silent. Therefore, detecting silence is a reliable and format-independent idle signal.

#### State Machine

```
                  Feed() with len(data) >= minActivityBytes
    ┌──────────────────────────────────────────────────────┐
    │                                                      │
    v                                                      │
 [active]  ──────── silence timer fires ────────>  [idle]
 (running)      (no output for silenceTimeout)     (waiting_for_input)
```

#### Interface

```go
type IdleDetector struct {
    silenceTimeout   time.Duration  // idle threshold after first output
    minActivityBytes int            // minimum bytes to count as activity
    startupTimeout   time.Duration  // fallback if CLI never produces output
    timer            *time.Timer    // silence timer (reset on each Feed)
    startupTimer     *time.Timer    // fires if no output at all within startupTimeout
    onIdle           func()         // called on active → idle transition
    onActive         func()         // called on idle → active transition (may be nil)
    isIdle           bool           // current state
    outputSeen       bool           // true after first meaningful Feed()
    mu               sync.Mutex
}
```

**Start():**
- Start the `startupTimer` (fires `onIdle()` if no output arrives within `startupTimeout`)
- Set `isIdle = false`, `outputSeen = false`

**Feed(data []byte):**
- If `len(data) < minActivityBytes` → ignore (cursor noise filter)
- Set `outputSeen = true`; cancel `startupTimer` if running
- Reset silence timer to `silenceTimeout`
- If currently idle → set `isIdle = false`, call `onActive()` (only on idle→active transitions, not on initial Feed)

**Silence timer fires:**
- Set `isIdle = true`, call `onIdle()`

**Startup timer fires:**
- If `!outputSeen` → set `isIdle = true`, call `onIdle()` as fallback

**Stop():**
- Stop both `timer` and `startupTimer`
- Safe to call multiple times
- **Must be called by `removeSession()` before deleting from the detectors map** to prevent timer goroutine leaks

#### Parameters

| Parameter | Default | Configurable | Rationale |
|-----------|---------|-------------|-----------|
| `silenceTimeout` | 10s | Yes (agent TOML: `agent.idle_silence_timeout`) | Claude spinners update every ~200-500ms. 10s of silence is a strong idle signal. |
| `minActivityBytes` | 10 | Yes (option func) | Filters cursor repositioning (3-6 bytes). Real output (spinners, text, ANSI sequences for colors) exceeds this. |
| `startupTimeout` | 60s | Yes (option func) | Fallback if CLI hangs and never produces output. Increased from 10s (old default) since silence-based detection doesn't need fast startup. |

#### What Is Removed

- Phase 0 / Phase 1 state machine
- Prompt marker constants (`promptMarkerNBSP`, `promptMarkerSpace`)
- `markers [][]byte` field
- `containsAnyMarker()` method
- Rolling buffer (`buf []byte`)
- `maxMarkerLen()` method
- `triggered` flag
- `keepAlive` mode and `WithKeepAlive()` option
- `ResetToPhase1()` method

#### Config Migration

| Current Field | Action |
|---------------|--------|
| `agent.idle_prompt_marker` | **Remove.** No longer used. |
| `agent.idle_startup_timeout` | **Keep.** Maps to `startupTimeout`. Default changes from 10s to 60s. |
| *(new)* `agent.idle_silence_timeout` | **Add.** Maps to `silenceTimeout`. Default 10s. |

### 2. Session Manager Integration

The session manager still decides **what to do** on idle/active based on session type. The detector only signals **when**.

#### Callback Wiring by Session Type

**Standalone sessions** (no initial prompt, no keep-alive):
```go
onIdle = func() {
    if lastStatus[sessionID] == status.WaitingForInput { return }
    lastStatus[sessionID] = status.WaitingForInput
    sendEvent(SessionStatusEvent{Status: WaitingForInput})
}
onActive = func() {
    if lastStatus[sessionID] == status.Running { return }
    lastStatus[sessionID] = status.Running
    sendEvent(SessionStatusEvent{Status: Running})
}
```

**Job sessions** (has initial prompt, no keep-alive):

Note: With silence-based detection, the first `onIdle` fires ~10 seconds after CLI startup output settles (vs. near-instant with marker detection). This is acceptable because:
- The CLI still starts fast — `onIdle` fires as soon as the startup output goes silent
- In practice, the CLI takes several seconds to initialize anyway (loading config, connecting to API)
- The 10-second silence threshold is from the LAST output byte, not from process start. Once the CLI is done printing its startup banner, silence begins immediately.

If this proves too slow in practice, we can add a shorter `initialSilenceTimeout` (e.g., 3s) that applies only to the first idle transition.

```go
var promptSubmitted bool
onIdle = func() {
    if !promptSubmitted {
        promptSubmitted = true
        sess.WriteInput(prompt + "\r")
        return  // Don't report idle — we just submitted work
    }
    // Second idle = job complete
    sess.WriteInput("/exit\r")
}
onActive = nil  // No need to report running for job sessions
```

**Keep-alive sessions** (shared, multiple prompts):
```go
onIdle = func() {
    extractAndSendStepTaskValues(sessionID)
    sendEvent(StepIdleEvent{SessionId: sessionID})
}
onActive = nil  // Orchestrator manages status for job runs
```

#### Simplifications

**`handleInput()` reduces to writing input to the PTY.** The detector auto-transitions to active when the CLI produces output in response. No manual `ResetToPhase1()`, no status tracking in `handleInput`.

**Maps retained:** `standalone`, `lastStatus`, `detectors` — still needed for callback wiring, duplicate event guards, and cleanup.

#### Session Cleanup

**`removeSession()` must call `detector.Stop()` before deleting from the detectors map.** The new detector uses two resettable `time.Timer` instances (silence timer and startup timer). Without explicit `Stop()`, timer goroutines leak. The current implementation has a similar leak (the `time.AfterFunc` from `Start()` has no cancellation), which this fixes.

### 3. Database Schema Fix (`updated_at` Column)

**Problem:** `UpdateSessionStatus` only sets `ended_at` for terminal states. The reaper uses `ended_at` (via `UpdatedAt`) to compute idle duration, but for `waiting_for_input` sessions `ended_at` is NULL, falling back to `CreatedAt`. This makes sessions appear idle since creation time.

**Fix:** Add a proper `updated_at` column.

#### Migration

```sql
ALTER TABLE sessions ADD COLUMN updated_at DATETIME DEFAULT CURRENT_TIMESTAMP;
-- For non-terminal sessions, use current time to avoid the reaper
-- mass-terminating existing waiting_for_input sessions on first sweep.
-- For terminal sessions, use ended_at (accurate historical data).
UPDATE sessions SET updated_at = ended_at
    WHERE status IN ('completed', 'failed', 'terminated');
UPDATE sessions SET updated_at = CURRENT_TIMESTAMP
    WHERE status NOT IN ('completed', 'failed', 'terminated');
```

Note: `CreateSession` explicitly lists columns in its INSERT but omits `updated_at`. SQLite will use the `DEFAULT CURRENT_TIMESTAMP`, which is correct. No change needed to `CreateSession`.

#### Store Changes

- **`UpdateSessionStatus`**: Always sets `updated_at = CURRENT_TIMESTAMP` on every status change. `ended_at` continues to be set only for terminal states.
- **`UpdateSessionStatusIfNotTerminal`**: Also sets `updated_at = CURRENT_TIMESTAMP`. (This function is called from the gRPC server for agent exit events and must also update the timestamp.)
- **`Session` struct**: `UpdatedAt time.Time` maps to `updated_at` column. New `EndedAt *time.Time` (pointer, nullable) maps to `ended_at`.
- **`scanSessions` / `GetSession`**: Read both `updated_at` and `ended_at`. `UpdatedAt` is always set (no NULL fallback needed). `EndedAt` uses `sql.NullTime` → `*time.Time`.
- **All SELECT queries**: Add `updated_at` to the column list in `ListSessions`, `ListSessionsByMachine`, `ListSessionsByStatus`, `GetSession`.

#### Reaper Changes

- `sweep()` uses `sess.UpdatedAt` which now maps to the real `updated_at` column.
- No logic changes needed — the data is just correct now.

### 4. Loading Screen Fix

**Problem:** During async `checkSession()` on page load, the app renders near-invisible "Loading..." text (`text-text-secondary text-sm`) on a dark background (`bg-bg-primary` = `#0a0e14`). Appears as a black screen.

**Fix:**
- Replace with a visible animated spinner and larger `text-text-primary` text ("Connecting...")
- Add a 5-second timeout that shows "Taking longer than expected" with a retry button
- If auth check fails, show actionable error instead of silent redirect

**Scope:** Changes to `App.tsx` loading state only.

## Changes Summary

| # | Change | Size | Primary Files |
|---|--------|------|---------------|
| 1 | Silence-based IdleDetector | Medium | `idle_detector.go`, `idle_detector_test.go` |
| 2 | Session manager integration | Medium | `session_manager.go` |
| 3 | Agent config for silence timeout | Small | `internal/agent/config/config.go` |
| 4 | `updated_at` column migration | Small | `store/migrations.go`, `store/sessions.go` |
| 5 | Reaper alignment (no logic changes) | Tiny | `reaper/reaper.go`, `reaper/reaper_test.go` |
| 6 | Loading screen fix | Small | `web/src/App.tsx` |

## Out of Scope

- Modular logging system (deferred until lifecycle detection is solid)
- gRPC protocol changes (`SessionStatusEvent` message unchanged)
- Frontend status display changes (already handles `waiting_for_input`)
- Original session termination bug (could not reproduce; monitoring)
- Query invalidation refactor (current prefix matching is correct; status mismatch likely caused by idle detector false positives)

## Testing

- **IdleDetector unit tests**: Silence fires after timeout, activity resets timer, `minActivityBytes` filter, startup timeout fallback, `Stop()` cleanup, concurrent `Feed()` safety.
- **Session manager tests**: Standalone idle/active transitions, job session prompt submission on first idle, keep-alive step completion on idle.
- **Store tests**: `updated_at` set on every status change, `EndedAt` only on terminal, migration backfill correctness.
- **Reaper tests**: Verify existing tests pass with `updated_at` column (no logic changes).
- **Frontend tests**: Update `useEventStream` test assertions if any reference invalidation behavior. Loading screen rendering with timeout.

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Silence threshold too short (false idle during thinking pauses) | Claude CLI always produces PTY bytes during thinking (spinner animations). 10s threshold is well above the ~200-500ms spinner interval. Threshold is configurable in agent TOML. |
| Silence threshold too long for job session startup | In practice, CLI startup output settles quickly. If 10s proves too slow, add a shorter `initialSilenceTimeout` for the first idle transition. Monitor job startup latency after deployment. |
| `minActivityBytes` filters out meaningful small output | Default of 10 bytes is conservative. Real output (spinners, text, ANSI color sequences) consistently exceeds this. Adjustable via option func. |
| Migration triggers mass termination of existing waiting_for_input sessions | Migration sets `updated_at = CURRENT_TIMESTAMP` for non-terminal sessions, so the reaper sees them as freshly updated. |
| Timer goroutine leaks | `Stop()` explicitly cancels both timers. `removeSession()` must call `Stop()` before map deletion. |
| CLI produces periodic bytes when truly idle (cursor blink, etc.) | Cursor blinking is handled by the terminal emulator (xterm.js), not by the CLI. If discovered during testing, increase `minActivityBytes` threshold. |
