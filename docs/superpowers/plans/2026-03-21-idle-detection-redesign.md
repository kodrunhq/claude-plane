# Idle Detection Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace marker-based idle detection with silence-based detection, fix the `ended_at` schema bug, and improve the loading screen.

**Architecture:** The new `IdleDetector` uses a silence timer instead of prompt marker matching. If no PTY output exceeding a minimum byte threshold arrives within a configurable timeout, the session is considered idle. A startup timeout provides a fallback if the CLI never produces output. The DB gets a proper `updated_at` column so the reaper calculates idle duration correctly.

**Tech Stack:** Go 1.25, SQLite (modernc.org/sqlite), React 19, TypeScript, Zustand, TanStack Query, Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-03-21-idle-detection-redesign.md`

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Rewrite | `internal/agent/idle_detector.go` | Silence-based idle detection |
| Rewrite | `internal/agent/idle_detector_test.go` | New detector tests |
| Modify | `internal/agent/session_manager.go` | New callback wiring, simplified handleInput, Stop() in removeSession |
| Modify | `internal/agent/config/config.go` | Replace `IdlePromptMarker` with `IdleSilenceTimeout` |
| Modify | `cmd/agent/main.go:89-100` | Wire new config field to detector options |
| Modify | `internal/server/store/migrations.go` | Add migration 19 for `updated_at` column |
| Modify | `internal/server/store/sessions.go` | Add `EndedAt` field, update all queries for `updated_at` column |
| Modify | `internal/server/store/sessions_test.go` | Test `updated_at` behavior |
| Modify | `internal/server/reaper/reaper.go` | No logic changes (uses `UpdatedAt` which now maps correctly) |
| Modify | `internal/server/reaper/reaper_test.go` | Verify tests still pass |
| Modify | `web/src/App.tsx:85-91` | Visible loading screen with spinner and retry |

---

### Task 1: Rewrite IdleDetector — Tests First

**Files:**
- Rewrite: `internal/agent/idle_detector_test.go`

- [ ] **Step 1: Write the new test file**

Replace the entire test file. The new tests cover the silence-based detector:

```go
package agent

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdleDetector_SilenceFiresOnIdle(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil,
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	// Feed meaningful data then wait for silence.
	d.Feed(make([]byte, 20))
	time.Sleep(100 * time.Millisecond)

	if !idleCalled.Load() {
		t.Fatal("expected onIdle to fire after silence timeout")
	}
}

func TestIdleDetector_ActivityResetsTimer(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil,
		WithSilenceTimeout(80*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	// Feed data at 30ms intervals — should keep resetting the 80ms timer.
	for i := 0; i < 5; i++ {
		d.Feed(make([]byte, 20))
		time.Sleep(30 * time.Millisecond)
	}

	if idleCalled.Load() {
		t.Fatal("onIdle should not fire while activity continues")
	}

	// Now stop feeding — idle should fire after 80ms.
	time.Sleep(120 * time.Millisecond)
	if !idleCalled.Load() {
		t.Fatal("expected onIdle after activity stopped")
	}
}

func TestIdleDetector_MinActivityBytesFilter(t *testing.T) {
	var idleCount atomic.Int32
	d := NewIdleDetector(
		func() { idleCount.Add(1) },
		nil,
		WithSilenceTimeout(50*time.Millisecond),
		WithMinActivityBytes(15),
	)
	d.Start()
	defer d.Stop()

	// Feed meaningful data first.
	d.Feed(make([]byte, 20))
	time.Sleep(10 * time.Millisecond)

	// Now feed small data — should NOT reset silence timer.
	d.Feed(make([]byte, 5))
	time.Sleep(80 * time.Millisecond)

	if idleCount.Load() != 1 {
		t.Fatalf("expected 1 idle firing (small data ignored), got %d", idleCount.Load())
	}
}

func TestIdleDetector_OnActiveFiresOnTransition(t *testing.T) {
	var idleCalled, activeCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		func() { activeCalled.Store(true) },
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	// Trigger idle.
	d.Feed(make([]byte, 20))
	time.Sleep(80 * time.Millisecond)
	if !idleCalled.Load() {
		t.Fatal("expected onIdle")
	}

	// Feed data again — should trigger onActive.
	d.Feed(make([]byte, 20))
	time.Sleep(10 * time.Millisecond)
	if !activeCalled.Load() {
		t.Fatal("expected onActive on idle→active transition")
	}
}

func TestIdleDetector_OnActiveDoesNotFireInitially(t *testing.T) {
	var activeCalled atomic.Bool
	d := NewIdleDetector(
		func() {},
		func() { activeCalled.Store(true) },
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	// Feed data — onActive should NOT fire on initial feed (not a transition).
	d.Feed(make([]byte, 20))
	time.Sleep(10 * time.Millisecond)
	if activeCalled.Load() {
		t.Fatal("onActive should not fire on initial feed, only on idle→active transition")
	}
}

func TestIdleDetector_StartupTimeoutFallback(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil,
		WithStartupTimeout(50*time.Millisecond),
		WithSilenceTimeout(10*time.Second), // long silence — won't fire
	)
	d.Start()
	defer d.Stop()

	// Don't feed any data. Startup timeout should fire onIdle.
	time.Sleep(100 * time.Millisecond)
	if !idleCalled.Load() {
		t.Fatal("expected onIdle from startup timeout fallback")
	}
}

func TestIdleDetector_StartupTimeoutCancelledByOutput(t *testing.T) {
	var idleCount atomic.Int32
	d := NewIdleDetector(
		func() { idleCount.Add(1) },
		nil,
		WithStartupTimeout(100*time.Millisecond),
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	// Feed data before startup timeout — should cancel it.
	d.Feed(make([]byte, 20))
	time.Sleep(150 * time.Millisecond)

	// Should get exactly 1 idle (from silence timeout), not 2 (silence + startup).
	if idleCount.Load() != 1 {
		t.Fatalf("expected 1 idle firing, got %d", idleCount.Load())
	}
}

func TestIdleDetector_StopCancelsTimers(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil,
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()

	// Feed data, then stop before timer fires.
	d.Feed(make([]byte, 20))
	d.Stop()
	time.Sleep(100 * time.Millisecond)

	if idleCalled.Load() {
		t.Fatal("onIdle should not fire after Stop()")
	}
}

func TestIdleDetector_StopIsIdempotent(t *testing.T) {
	d := NewIdleDetector(func() {}, nil, WithSilenceTimeout(50*time.Millisecond))
	d.Start()
	d.Stop()
	d.Stop() // should not panic
}

func TestIdleDetector_RepeatedIdleActiveCycles(t *testing.T) {
	var idleCount, activeCount atomic.Int32
	d := NewIdleDetector(
		func() { idleCount.Add(1) },
		func() { activeCount.Add(1) },
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	for i := 0; i < 3; i++ {
		d.Feed(make([]byte, 20))
		time.Sleep(80 * time.Millisecond) // idle fires
		d.Feed(make([]byte, 20))          // active fires
		time.Sleep(10 * time.Millisecond)
	}

	if idleCount.Load() != 3 {
		t.Fatalf("expected 3 idle firings, got %d", idleCount.Load())
	}
	if activeCount.Load() != 3 {
		t.Fatalf("expected 3 active firings, got %d", activeCount.Load())
	}
}

func TestIdleDetector_NilOnActive(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil, // onActive is nil — should not panic
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	d.Feed(make([]byte, 20))
	time.Sleep(80 * time.Millisecond)
	d.Feed(make([]byte, 20)) // should not panic even with nil onActive
}

func TestIdleDetector_ConcurrentFeeds(t *testing.T) {
	var idleCount atomic.Int32
	d := NewIdleDetector(
		func() { idleCount.Add(1) },
		nil,
		WithSilenceTimeout(100*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	// Concurrent feeds should not panic or cause data races.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Feed(make([]byte, 20))
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go test -race ./internal/agent/ -run TestIdleDetector -count=1
```

Expected: Compilation errors because `NewIdleDetector` signature changed, `WithSilenceTimeout` and `WithMinActivityBytes` don't exist yet.

- [ ] **Step 3: Commit test file**

```bash
git add internal/agent/idle_detector_test.go
git commit -m "test: add silence-based idle detector tests (red)"
```

---

### Task 2: Implement New IdleDetector

**Files:**
- Rewrite: `internal/agent/idle_detector.go`

- [ ] **Step 1: Write the new implementation**

Replace the entire file:

```go
package agent

import (
	"sync"
	"time"
)

const (
	// DefaultSilenceTimeout is how long the detector waits with no output
	// before considering the session idle.
	DefaultSilenceTimeout = 10 * time.Second

	// DefaultMinActivityBytes is the minimum data size that counts as
	// meaningful output. Smaller chunks (e.g., cursor escape sequences)
	// are ignored.
	DefaultMinActivityBytes = 10

	// DefaultStartupTimeout is how long to wait for the CLI to produce
	// any output before assuming it is idle.
	DefaultStartupTimeout = 60 * time.Second
)

// IdleDetector watches PTY output volume to determine when a CLI session
// is idle. It fires onIdle when no meaningful output (>= minActivityBytes)
// arrives within silenceTimeout, and fires onActive when output resumes.
type IdleDetector struct {
	silenceTimeout   time.Duration
	minActivityBytes int
	startupTimeout   time.Duration

	onIdle   func()
	onActive func() // may be nil

	mu           sync.Mutex
	timer        *time.Timer // silence timer — reset on each meaningful Feed
	startupTimer *time.Timer // fires if no output at all within startupTimeout
	isIdle       bool
	outputSeen   bool
	stopped      bool
}

// IdleDetectorOption configures optional IdleDetector settings.
type IdleDetectorOption func(*IdleDetector)

// WithSilenceTimeout overrides the default silence timeout.
func WithSilenceTimeout(d time.Duration) IdleDetectorOption {
	return func(det *IdleDetector) {
		if d > 0 {
			det.silenceTimeout = d
		}
	}
}

// WithMinActivityBytes overrides the minimum data size that counts as activity.
func WithMinActivityBytes(n int) IdleDetectorOption {
	return func(det *IdleDetector) {
		if n > 0 {
			det.minActivityBytes = n
		}
	}
}

// WithStartupTimeout overrides the default startup timeout.
func WithStartupTimeout(d time.Duration) IdleDetectorOption {
	return func(det *IdleDetector) {
		if d > 0 {
			det.startupTimeout = d
		}
	}
}

// NewIdleDetector creates a detector that watches for silence in PTY output.
// onIdle fires when silence exceeds the threshold (active → idle).
// onActive fires when output resumes after an idle period (idle → active). May be nil.
func NewIdleDetector(onIdle func(), onActive func(), opts ...IdleDetectorOption) *IdleDetector {
	d := &IdleDetector{
		silenceTimeout:   DefaultSilenceTimeout,
		minActivityBytes: DefaultMinActivityBytes,
		startupTimeout:   DefaultStartupTimeout,
		onIdle:           onIdle,
		onActive:         onActive,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Start begins the startup timeout timer. Call Stop() to clean up.
func (d *IdleDetector) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = false
	d.isIdle = false
	d.outputSeen = false

	d.startupTimer = time.AfterFunc(d.startupTimeout, func() {
		d.mu.Lock()
		if d.stopped || d.outputSeen {
			d.mu.Unlock()
			return
		}
		d.isIdle = true
		d.mu.Unlock()
		d.onIdle()
	})
}

// Stop cancels all timers. Safe to call multiple times.
func (d *IdleDetector) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
	}
	if d.startupTimer != nil {
		d.startupTimer.Stop()
	}
}

// Feed processes a chunk of PTY output.
func (d *IdleDetector) Feed(data []byte) {
	if len(data) < d.minActivityBytes {
		return
	}

	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}

	// Cancel startup timer on first meaningful output.
	if !d.outputSeen {
		d.outputSeen = true
		if d.startupTimer != nil {
			d.startupTimer.Stop()
		}
	}

	// Reset (or create) the silence timer.
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.silenceTimeout, func() {
		d.mu.Lock()
		if d.stopped {
			d.mu.Unlock()
			return
		}
		d.isIdle = true
		d.mu.Unlock()
		d.onIdle()
	})

	// Transition: idle → active.
	wasIdle := d.isIdle
	if wasIdle {
		d.isIdle = false
	}
	d.mu.Unlock()

	if wasIdle && d.onActive != nil {
		d.onActive()
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go test -race ./internal/agent/ -run TestIdleDetector -count=1 -v
```

Expected: All tests PASS.

- [ ] **Step 3: Run go vet**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go vet ./internal/agent/...
```

Expected: No issues.

- [ ] **Step 4: Commit**

```bash
git add internal/agent/idle_detector.go internal/agent/idle_detector_test.go
git commit -m "feat: replace marker-based idle detector with silence-based detection

The old detector watched for the ❯ prompt marker in PTY output, but
Claude CLI renders this character persistently (even during active work).
The new detector fires onIdle after configurable silence and onActive
when output resumes."
```

---

### Task 3: Update Agent Config

**Files:**
- Modify: `internal/agent/config/config.go:50-57`

- [ ] **Step 1: Replace `IdlePromptMarker` with `IdleSilenceTimeout`**

In the `AgentSettings` struct, replace line 55:

```go
// Before:
IdlePromptMarker   string `toml:"idle_prompt_marker"`

// After:
IdleSilenceTimeout string `toml:"idle_silence_timeout"`
```

Keep `IdleStartupTimeout` as-is (line 56).

- [ ] **Step 2: Update `cmd/agent/main.go:89-100`**

Replace the idle options wiring block:

```go
// Before (lines 90-100):
var idleOpts []agent.IdleDetectorOption
if cfg.Agent.IdlePromptMarker != "" {
    idleOpts = append(idleOpts, agent.WithPromptMarker([]byte(cfg.Agent.IdlePromptMarker)))
}
if cfg.Agent.IdleStartupTimeout != "" {
    if d, err := time.ParseDuration(cfg.Agent.IdleStartupTimeout); err == nil {
        idleOpts = append(idleOpts, agent.WithStartupTimeout(d))
    } else {
        slog.Warn("invalid idle_startup_timeout, using default", "value", cfg.Agent.IdleStartupTimeout, "error", err)
    }
}

// After:
var idleOpts []agent.IdleDetectorOption
if cfg.Agent.IdleSilenceTimeout != "" {
    if d, err := time.ParseDuration(cfg.Agent.IdleSilenceTimeout); err == nil {
        idleOpts = append(idleOpts, agent.WithSilenceTimeout(d))
    } else {
        slog.Warn("invalid idle_silence_timeout, using default", "value", cfg.Agent.IdleSilenceTimeout, "error", err)
    }
}
if cfg.Agent.IdleStartupTimeout != "" {
    if d, err := time.ParseDuration(cfg.Agent.IdleStartupTimeout); err == nil {
        idleOpts = append(idleOpts, agent.WithStartupTimeout(d))
    } else {
        slog.Warn("invalid idle_startup_timeout, using default", "value", cfg.Agent.IdleStartupTimeout, "error", err)
    }
}
```

- [ ] **Step 3: Verify build**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go build ./cmd/agent/...
```

Expected: Compiles successfully.

- [ ] **Step 4: Commit**

```bash
git add internal/agent/config/config.go cmd/agent/main.go
git commit -m "feat: replace idle_prompt_marker config with idle_silence_timeout"
```

---

### Task 4: Update Session Manager

**Files:**
- Modify: `internal/agent/session_manager.go`

- [ ] **Step 1: Update `NewIdleDetector` call signature in `handleCreate`**

In `handleCreate` (around lines 196-291), replace the entire idle detector setup block (including the old comment referencing prompt marker detection). The key changes:

1. `NewIdleDetector` now takes `(onIdle, onActive, opts...)` instead of `(onReady, onIdle, opts...)`.
2. Remove `WithKeepAlive` from opts (lines 280-281).
3. The `onReady` concept is gone — for job sessions, prompt submission happens in `onIdle` (first idle = submit prompt).
4. For standalone sessions, add an `onActive` callback.

Replace lines 200-291:

```go
// For Claude sessions, set up an IdleDetector that watches for silence
// in PTY output to detect when the CLI is idle (waiting for input).
// Shell tasks skip this entirely.
if taskType != "shell" {
    sessionID := cmd.GetSessionId()
    keepAlive := cmd.GetKeepAlive()
    prompt := cmd.GetInitialPrompt()

    isStandalone := !keepAlive && prompt == ""
    if isStandalone {
        sm.standaloneMu.Lock()
        sm.standalone[sessionID] = true
        sm.standaloneMu.Unlock()
    }

    var onIdle func()
    var onActive func()

    if isStandalone {
        onIdle = func() {
            sm.lastStatusMu.Lock()
            if sm.lastStatus[sessionID] == status.WaitingForInput {
                sm.lastStatusMu.Unlock()
                return
            }
            sm.lastStatus[sessionID] = status.WaitingForInput
            sm.lastStatusMu.Unlock()

            sm.logger.Info("silence detected, reporting waiting_for_input (standalone)", "session_id", sessionID)
            sm.sendEvent(&pb.AgentEvent{
                Event: &pb.AgentEvent_SessionStatus{
                    SessionStatus: &pb.SessionStatusEvent{
                        SessionId: sessionID,
                        Status:    status.WaitingForInput,
                    },
                },
            })
        }
        onActive = func() {
            sm.lastStatusMu.Lock()
            if sm.lastStatus[sessionID] == status.Running {
                sm.lastStatusMu.Unlock()
                return
            }
            sm.lastStatus[sessionID] = status.Running
            sm.lastStatusMu.Unlock()

            sm.logger.Info("output resumed, reporting running (standalone)", "session_id", sessionID)
            sm.sendEvent(&pb.AgentEvent{
                Event: &pb.AgentEvent_SessionStatus{
                    SessionStatus: &pb.SessionStatusEvent{
                        SessionId: sessionID,
                        Status:    status.Running,
                    },
                },
            })
        }
    } else if keepAlive {
        onIdle = func() {
            sm.logger.Info("silence detected, extracting task values and sending StepIdleEvent (keep-alive)",
                "session_id", sessionID,
            )
            sm.extractAndSendStepTaskValues(sessionID)
            sm.sendEvent(&pb.AgentEvent{
                Event: &pb.AgentEvent_StepIdle{
                    StepIdle: &pb.StepIdleEvent{
                        SessionId: sessionID,
                    },
                },
            })
        }
    } else {
        // Job session with initial prompt.
        var promptSubmitted bool
        onIdle = func() {
            if !promptSubmitted {
                promptSubmitted = true
                input := []byte(prompt + "\r")
                if err := sess.WriteInput(input); err != nil {
                    sm.logger.Error("failed to write initial prompt", "session_id", sessionID, "error", err)
                } else {
                    sm.logger.Info("initial prompt submitted (on first silence)", "session_id", sessionID, "prompt_len", len(prompt))
                }
                return
            }
            sm.logger.Info("silence detected, sending /exit", "session_id", sessionID)
            if err := sess.WriteInput([]byte("/exit\r")); err != nil {
                sm.logger.Error("failed to send /exit after idle",
                    "session_id", sessionID, "error", err)
            }
        }
    }

    opts := make([]IdleDetectorOption, len(sm.idleDetectorOpts))
    copy(opts, sm.idleDetectorOpts)

    detector := NewIdleDetector(onIdle, onActive, opts...)
    detector.Start()
    sess.SetOutputObserver(detector.Feed)

    sm.detectorMu.Lock()
    sm.detectors[sessionID] = detector
    sm.detectorMu.Unlock()
}
```

- [ ] **Step 2: Simplify `handleInput`**

Replace `handleInput` (lines 311-354). Remove the standalone status tracking and `ResetToPhase1` logic — the detector handles transitions automatically via `onActive`:

```go
func (sm *SessionManager) handleInput(cmd *pb.InputDataCmd) {
	sessionID := cmd.GetSessionId()
	sess := sm.getSession(sessionID)
	if sess == nil {
		sm.logger.Warn("input for unknown session", "session_id", sessionID)
		return
	}

	if err := sess.WriteInput(cmd.GetData()); err != nil {
		sm.logger.Error("write input failed", "session_id", sessionID, "error", err)
	}
}
```

- [ ] **Step 3: Add `Stop()` call in `removeSession`**

In `removeSession` (lines 566-594), add detector cleanup before deleting from map. Replace the detectors block (lines 587-589):

```go
// Before:
sm.detectorMu.Lock()
delete(sm.detectors, sessionID)
sm.detectorMu.Unlock()

// After:
sm.detectorMu.Lock()
if d, ok := sm.detectors[sessionID]; ok {
    d.Stop()
    delete(sm.detectors, sessionID)
}
sm.detectorMu.Unlock()
```

- [ ] **Step 4: Verify build and tests**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go build ./cmd/agent/... && go test -race ./internal/agent/ -count=1
```

Expected: Compiles and all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/session_manager.go
git commit -m "feat: integrate silence-based idle detector in session manager

Simplify handleInput (detector auto-resets on output), add Stop() call
in removeSession to prevent timer leaks, wire onActive callback for
standalone sessions."
```

---

### Task 5: Database Migration — `updated_at` Column

**Files:**
- Modify: `internal/server/store/migrations.go` (append migration 19)

- [ ] **Step 1: Add migration 19**

Append to the `migrations` slice (after the entry with `Version: 18`):

```go
{
    Version:     19,
    Description: "add updated_at column to sessions",
    SQL: `
ALTER TABLE sessions ADD COLUMN updated_at DATETIME DEFAULT CURRENT_TIMESTAMP;
UPDATE sessions SET updated_at = ended_at
    WHERE status IN ('completed', 'failed', 'terminated');
UPDATE sessions SET updated_at = CURRENT_TIMESTAMP
    WHERE status NOT IN ('completed', 'failed', 'terminated');
`,
},
```

- [ ] **Step 2: Verify build**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go build ./cmd/server/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/server/store/migrations.go
git commit -m "feat: add migration 19 — updated_at column for sessions

Sets updated_at to CURRENT_TIMESTAMP for non-terminal sessions to
prevent the reaper from mass-terminating existing waiting_for_input
sessions on first sweep after upgrade."
```

---

### Task 6: Store Layer — `updated_at` and `EndedAt` Fields

**Files:**
- Modify: `internal/server/store/sessions.go`
- Modify: `internal/server/store/sessions_test.go`

- [ ] **Step 1: Update `Session` struct**

Replace the struct fields (lines 23-27):

```go
// Before:
// CreatedAt corresponds to the database column `started_at`.
CreatedAt time.Time `json:"created_at"`
// UpdatedAt corresponds to the database column `ended_at` (or CreatedAt if not ended).
UpdatedAt time.Time `json:"updated_at"`

// After:
// CreatedAt corresponds to the database column `started_at`.
CreatedAt time.Time  `json:"created_at"`
// UpdatedAt corresponds to the database column `updated_at`.
UpdatedAt time.Time  `json:"updated_at"`
// EndedAt corresponds to the database column `ended_at` (NULL if session has not ended).
EndedAt   *time.Time `json:"ended_at,omitempty"`
```

- [ ] **Step 2: Update `GetSession`**

Update the query to select `updated_at` and scan both `updated_at` and `ended_at`. The query (around line 57) needs `updated_at` added to the SELECT list, and the scan needs to read both columns:

```go
var endedAt sql.NullTime
err := s.reader.QueryRow(`
    SELECT session_id, machine_id, user_id, COALESCE(template_id, ''),
           COALESCE(command, 'claude'), COALESCE(working_dir, ''),
           status, COALESCE(model, ''), COALESCE(skip_permissions, ''),
           COALESCE(env_vars, ''), COALESCE(args, ''), COALESCE(initial_prompt, ''),
           started_at, updated_at, ended_at
    FROM sessions WHERE session_id = ?`, id,
).Scan(&sess.SessionID, &sess.MachineID, &userID, &templateID,
    &sess.Command, &sess.WorkingDir, &sess.Status,
    &sess.Model, &sess.SkipPerms, &sess.EnvVars, &sess.Args, &sess.InitialPrompt,
    &sess.CreatedAt, &sess.UpdatedAt, &endedAt)
```

After the scan, map `endedAt`:

```go
if endedAt.Valid {
    t := endedAt.Time
    sess.EndedAt = &t
}
```

- [ ] **Step 3: Update `scanSessions` AND all SELECT queries (must be done together)**

Update `scanSessions` to scan 15 columns, AND simultaneously update ALL SELECT queries in `ListSessions`, `ListSessionsByMachine`, and `ListSessionsByStatus` to include `updated_at` between `started_at` and `ended_at`. These changes must happen atomically — `scanSessions` reads 15 columns, so all queries that use it must return 15 columns.

Update each SELECT query's column list to:
```sql
SELECT session_id, machine_id, user_id, COALESCE(template_id, ''),
       COALESCE(command, 'claude'), COALESCE(working_dir, ''),
       status, COALESCE(model, ''), COALESCE(skip_permissions, ''),
       COALESCE(env_vars, ''), COALESCE(args, ''), COALESCE(initial_prompt, ''),
       started_at, updated_at, ended_at
FROM sessions ...
```

Then update `scanSessions`:

```go
func scanSessions(rows *sql.Rows) ([]Session, error) {
	var sessions []Session
	for rows.Next() {
		var sess Session
		var userID, templateID sql.NullString
		var endedAt sql.NullTime
		if err := rows.Scan(&sess.SessionID, &sess.MachineID, &userID, &templateID,
			&sess.Command, &sess.WorkingDir, &sess.Status,
			&sess.Model, &sess.SkipPerms, &sess.EnvVars, &sess.Args, &sess.InitialPrompt,
			&sess.CreatedAt, &sess.UpdatedAt, &endedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		if userID.Valid {
			sess.UserID = userID.String
		}
		if templateID.Valid {
			sess.TemplateID = templateID.String
		}
		if endedAt.Valid {
			t := endedAt.Time
			sess.EndedAt = &t
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}
```

- [ ] **Step 4: Update `UpdateSessionStatus`**

Add `updated_at` to both query branches (lines 142-158):

```go
func (s *Store) UpdateSessionStatus(id, status string) error {
	var query string
	switch status {
	case StatusCompleted, StatusFailed, StatusTerminated:
		query = `UPDATE sessions SET status = ?, updated_at = CURRENT_TIMESTAMP, ended_at = CURRENT_TIMESTAMP WHERE session_id = ?`
	default:
		query = `UPDATE sessions SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE session_id = ?`
	}
	result, err := s.writer.Exec(query, status, id)
	if err != nil {
		return fmt.Errorf("update session status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}
	return nil
}
```

- [ ] **Step 5: Update `UpdateSessionStatusIfNotTerminal`**

```go
func (s *Store) UpdateSessionStatusIfNotTerminal(id, status string) error {
	query := `UPDATE sessions SET status = ?, updated_at = CURRENT_TIMESTAMP, ended_at = CURRENT_TIMESTAMP
		WHERE session_id = ? AND status NOT IN (?, ?, ?)`
	result, err := s.writer.Exec(query, status, id, StatusCompleted, StatusFailed, StatusTerminated)
	if err != nil {
		return fmt.Errorf("update session status if not terminal: %w", err)
	}
	_ = result
	return nil
}
```

Note: This function is only called with terminal statuses (completed/failed), so always setting `ended_at` is correct.

- [ ] **Step 6: Add test for `updated_at` behavior**

Add to `sessions_test.go`. Use the existing test factory helpers (`mustNewStore`, `mustCreateMachine`, `mustCreateSession`) from `testfactory_test.go` to satisfy FK constraints:

```go
func TestUpdateSessionStatus_SetsUpdatedAt(t *testing.T) {
	s := mustNewStore(t)
	machineID := mustCreateMachine(t, s)

	sess := &Session{
		SessionID: "sess-upd-001",
		MachineID: machineID,
		Command:   "claude",
		Status:    StatusRunning,
	}
	if err := s.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// Small sleep to ensure updated_at changes.
	time.Sleep(10 * time.Millisecond)

	if err := s.UpdateSessionStatus("sess-upd-001", StatusWaitingForInput); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSession("sess-upd-001")
	if err != nil {
		t.Fatal(err)
	}

	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("UpdatedAt (%v) should be after CreatedAt (%v)", got.UpdatedAt, got.CreatedAt)
	}
	if got.EndedAt != nil {
		t.Error("EndedAt should be nil for non-terminal status")
	}

	// Terminal status should set both.
	time.Sleep(10 * time.Millisecond)
	if err := s.UpdateSessionStatus("sess-upd-001", StatusCompleted); err != nil {
		t.Fatal(err)
	}

	got, err = s.GetSession("sess-upd-001")
	if err != nil {
		t.Fatal(err)
	}

	if got.EndedAt == nil {
		t.Error("EndedAt should be set for terminal status")
	}
}
```

- [ ] **Step 7: Run tests**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go test -race ./internal/server/store/ -count=1 -v
```

Expected: All tests pass.

- [ ] **Step 8: Run reaper tests to verify no breakage**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go test -race ./internal/server/reaper/ -count=1 -v
```

Expected: All tests pass. The reaper uses `sess.UpdatedAt` which now correctly maps to `updated_at`.

- [ ] **Step 9: Commit**

```bash
git add internal/server/store/sessions.go internal/server/store/sessions_test.go
git commit -m "feat: add updated_at column to Session struct and update all queries

UpdatedAt now maps to a real updated_at column (set on every status
change). EndedAt is a new nullable field for the ended_at column
(set only on terminal states). Fixes reaper idle duration calculation."
```

---

### Task 7: Loading Screen Fix

**Files:**
- Modify: `web/src/App.tsx:85-91`

- [ ] **Step 1: Update the loading state in `App()`**

Replace the loading block (lines 85-91) with a visible spinner and retry:

```tsx
if (loading) {
  return (
    <div className="h-screen flex flex-col items-center justify-center bg-bg-primary gap-4">
      <div className="flex items-center gap-3">
        <svg className="animate-spin h-5 w-5 text-text-primary" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
        <span className="text-text-primary text-base font-mono">Connecting...</span>
      </div>
    </div>
  )
}
```

Note: We skip the retry button for now — the loading state resolves quickly in normal conditions, and the spinner makes it obvious the app is working. If the auth check fails, it falls through to the login page. We can add the timeout/retry in a follow-up if the issue persists.

- [ ] **Step 2: Verify frontend builds**

```bash
cd /home/joseibanez/develop/projects/claude-plane/web && npx tsc --noEmit && npm run build
```

Expected: No type errors, build succeeds.

- [ ] **Step 3: Commit**

```bash
git add web/src/App.tsx
git commit -m "fix: make loading screen visible with spinner and larger text

The previous loading state used near-invisible text on the dark
background, making the app appear broken during auth checks."
```

---

### Task 8: Full Integration Verification

- [ ] **Step 1: Run all Go tests**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go test -race ./... 2>&1 | tail -30
```

Expected: All tests pass.

- [ ] **Step 2: Run go vet**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go vet ./...
```

Expected: No issues.

- [ ] **Step 3: Build all binaries**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go build -o /dev/null ./cmd/server && go build -o /dev/null ./cmd/agent && go build -o /dev/null ./cmd/bridge
```

Expected: All compile.

- [ ] **Step 4: Run frontend CI checks**

```bash
cd /home/joseibanez/develop/projects/claude-plane/web && npx tsc -b && npx eslint . && npx vitest run
```

Expected: Types, lint, and tests all pass.

- [ ] **Step 5: Regenerate event types (CI check)**

```bash
cd /home/joseibanez/develop/projects/claude-plane && go generate ./internal/server/event/...
```

Verify no changes to `event_types.json` (no event types were added/changed in this work).
