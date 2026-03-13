# Implementation Plan: Step Executor and Run Sessions

**Date:** March 13, 2026  
**Status:** Planning  
**Scope:** Wire up real step execution with session linkage and live/replay terminal viewing in the UI

---

## Executive Summary

This plan describes how to replace the `noopExecutor` stub with a real `StepExecutor` that:

1. Creates actual PTY-based Claude CLI sessions on agents
2. Links those sessions to run steps in the database
3. Enables live terminal viewing while runs execute
4. Enables post-run replay of step execution
5. Handles all edge cases (agent disconnect, timeout, concurrent steps)

The result is an end-to-end experience where users can watch runs execute in real-time OR review what happened after the fact via scrollback replay.

---

## Architecture Overview

### High-Level Data Flow

```
RunDetail Page (UI)
    |
    |-- Click "watch step 1" -->
    |
WebSocket (client)
    |
    |-- /ws/sessions/{sessionID}
    |
Session Registry (server)
    |
    |-- Publish to subscribers
    |
gRPC Event Stream (agent <- server)
    |
    |-- AgentEvent (SessionOutput, SessionStatus, SessionExit)
    |
Agent (PTY session)
    |
    |-- Claude CLI process in PTY
    |
```

### Components Involved

1. **Step Executor** (NEW): Implements `StepExecutor` interface
   - Creates sessions on agents via `CreateSessionCmd`
   - Monitors for session completion via agent events
   - Calls `onComplete(stepID, exitCode)` when done

2. **Session Handler**: Already wires sessions; now used by executor
   - `POST /api/v1/sessions` creates a session
   - Session is persisted to DB with `machine_id`, `command`, etc.
   - Returns `session_id` to caller

3. **Connection Manager**: Tracks connected agents
   - Provides `GetAgent(machineID)` to find agents
   - Agent has `SendCommand(cmd)` function to send gRPC messages

4. **gRPC Server**: Already handles `CommandStream()`
   - Receives `AgentEvent` from agents (output, status, exit)
   - Routes events to `Registry` for WebSocket forwarding
   - Already handles `SessionStatusEvent` and `ScrollbackChunkEvent`

5. **Session Registry**: Already routes terminal output
   - `Publish(sessionID, data)` fans out to WebSocket subscribers
   - Used by both live terminal connections and run detail page

6. **Session Manager** (Agent side): Already creates/manages PTY sessions
   - `handleCreate()` spawns the process in a PTY
   - Reads output and sends `SessionOutputEvent` via relay
   - When process exits, sends `SessionStatusEvent`

7. **RunDetail Page** (UI): Already shows run steps
   - Needs to embed terminal viewer for selected step's session
   - Needs to handle live vs. replay mode

---

## Data Model Changes

### Database

**run_steps** table already has:
- `session_id` (string, optional) - links to sessions table
- `exit_code` (nullable int)
- `started_at` / `ended_at` (timestamps)

No schema changes needed; just populate `session_id` when executor creates a session.

---

## Detailed Component Specifications

### 1. SessionStepExecutor (NEW)

**File:** `internal/server/executor/session_executor.go`

**Responsibility:** Orchestrate step execution by creating sessions and waiting for completion.

```go
package executor

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "github.com/kodrunhq/claude-plane/internal/server/connmgr"
    "github.com/kodrunhq/claude-plane/internal/server/session"
    "github.com/kodrunhq/claude-plane/internal/server/store"
    pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// SessionStepExecutor creates real PTY-backed sessions on agents
// and monitors for completion.
type SessionStepExecutor struct {
    connMgr  *connmgr.ConnectionManager
    sessHandler *session.SessionHandler
    store    store.Store // For session lookup and update
    registry *session.Registry
    logger   *slog.Logger

    // sessionToStep maps session_id -> (stepID, onComplete callback)
    // Used to route exit events back to the executor
    mu           sync.RWMutex
    sessionToStep map[string]*stepTracking
}

type stepTracking struct {
    runID      string
    stepID     string
    onComplete func(stepID string, exitCode int)
    cancel     context.CancelFunc
}

func NewSessionStepExecutor(
    connMgr *connmgr.ConnectionManager,
    sessHandler *session.SessionHandler,
    st store.Store,
    registry *session.Registry,
    logger *slog.Logger,
) *SessionStepExecutor {
    return &SessionStepExecutor{
        connMgr:       connMgr,
        sessHandler:   sessHandler,
        store:         st,
        registry:      registry,
        logger:        logger,
        sessionToStep: make(map[string]*stepTracking),
    }
}

// ExecuteStep implements the StepExecutor interface.
// It creates a session, monitors for exit, and calls onComplete.
func (e *SessionStepExecutor) ExecuteStep(
    ctx context.Context,
    runStep store.RunStep,
    onComplete func(stepID string, exitCode int),
) {
    // Validate that agent is connected
    agent := e.connMgr.GetAgent(runStep.MachineIDSnapshot)
    if agent == nil {
        e.logger.Error("agent not connected", "machine_id", runStep.MachineIDSnapshot, "step_id", runStep.StepID)
        onComplete(runStep.StepID, 1)
        return
    }

    // Create a session ID and store tracking info
    sessionID := uuid.New().String()

    stepCtx, cancel := context.WithCancel(ctx)
    tracking := &stepTracking{
        runID:      runStep.RunID,
        stepID:     runStep.StepID,
        onComplete: onComplete,
        cancel:     cancel,
    }

    e.mu.Lock()
    e.sessionToStep[sessionID] = tracking
    e.mu.Unlock()

    // Create the session on the agent
    sessCmd := &pb.ServerCommand{
        Command: &pb.ServerCommand_CreateSession{
            CreateSession: &pb.CreateSessionCmd{
                SessionId:  sessionID,
                Command:    runStep.CommandSnapshot,
                Args:       parseArgs(runStep.ArgsSnapshot),
                WorkingDir: runStep.WorkingDirSnapshot,
                TerminalSize: &pb.TerminalSize{
                    Rows: 24,
                    Cols: 80,
                },
                // TODO: Send initial prompt if runStep.PromptSnapshot is set
                // This requires a second message after CLI is ready
            },
        },
    }

    if err := agent.SendCommand(sessCmd); err != nil {
        e.logger.Error("failed to send create session command",
            "step_id", runStep.StepID, "error", err)
        e.cleanup(sessionID)
        onComplete(runStep.StepID, 1)
        return
    }

    // Update run_step with session_id and mark as running
    if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusRunning, sessionID, 0); err != nil {
        e.logger.Warn("failed to update run_step with session_id",
            "step_id", runStep.StepID, "error", err)
    }

    // Start a monitor goroutine that waits for session exit
    go e.monitorSessionExit(stepCtx, sessionID, runStep)
}

// monitorSessionExit waits for the session to exit by polling its status.
// In a future version, we could subscribe to SessionExitEvent instead of polling.
func (e *SessionStepExecutor) monitorSessionExit(
    ctx context.Context,
    sessionID string,
    runStep store.RunStep,
) {
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    // Timeout: use step's TimeoutSeconds or default to 1 hour
    timeout := 1 * time.Hour
    // TODO: Get timeout from runStep if available

    timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    for {
        select {
        case <-ctx.Done():
            // Parent context cancelled (run aborted)
            e.logger.Debug("step execution context cancelled", "step_id", runStep.StepID)
            e.cleanup(sessionID)
            return

        case <-timeoutCtx.Done():
            // Timeout occurred
            e.logger.Warn("step execution timeout",
                "step_id", runStep.StepID, "session_id", sessionID)
            e.cleanup(sessionID)
            // Kill the session
            e.sendKillCommand(sessionID, runStep.MachineIDSnapshot)
            // Call onComplete with exit code 124 (timeout)
            e.completeStep(sessionID, 124)
            return

        case <-ticker.C:
            // Poll for session status
            sess, err := e.store.GetSession(sessionID)
            if err != nil {
                e.logger.Debug("session not found during polling",
                    "session_id", sessionID, "error", err)
                continue
            }

            // Check if session has exited
            if sess.Status == store.StatusCompleted || sess.Status == store.StatusFailed {
                exitCode := 0
                if sess.ExitCode != nil {
                    exitCode = *sess.ExitCode
                }
                e.logger.Info("session completed",
                    "session_id", sessionID, "step_id", runStep.StepID,
                    "status", sess.Status, "exit_code", exitCode)
                e.cleanup(sessionID)
                e.completeStep(sessionID, exitCode)
                return
            }
        }
    }
}

// completeStep finds the tracked step and calls its callback.
func (e *SessionStepExecutor) completeStep(sessionID string, exitCode int) {
    e.mu.Lock()
    tracking, exists := e.sessionToStep[sessionID]
    e.mu.Unlock()

    if !exists {
        e.logger.Warn("session not tracked", "session_id", sessionID)
        return
    }

    if tracking.onComplete != nil {
        tracking.onComplete(tracking.stepID, exitCode)
    }
}

// cleanup removes the session from tracking and cancels its context.
func (e *SessionStepExecutor) cleanup(sessionID string) {
    e.mu.Lock()
    tracking, exists := e.sessionToStep[sessionID]
    delete(e.sessionToStep, sessionID)
    e.mu.Unlock()

    if exists && tracking.cancel != nil {
        tracking.cancel()
    }
}

// sendKillCommand sends a KillSessionCmd to the agent for the given session.
func (e *SessionStepExecutor) sendKillCommand(sessionID, machineID string) {
    agent := e.connMgr.GetAgent(machineID)
    if agent == nil {
        e.logger.Warn("agent not connected for kill command",
            "machine_id", machineID)
        return
    }

    cmd := &pb.ServerCommand{
        Command: &pb.ServerCommand_KillSession{
            KillSession: &pb.KillSessionCmd{
                SessionId: sessionID,
                Signal:    "SIGTERM",
            },
        },
    }

    if err := agent.SendCommand(cmd); err != nil {
        e.logger.Error("failed to send kill command",
            "session_id", sessionID, "error", err)
    }
}

func parseArgs(argsJSON string) []string {
    // Parse JSON array string to []string
    var args []string
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        // Return empty args on parse error
        return []string{}
    }
    return args
}
```

**Key Points:**
- Constructor takes `connMgr`, `sessHandler`, `store`, `registry`, `logger`
- `ExecuteStep()` creates a session by sending `CreateSessionCmd` to agent
- Uses session status polling (TODO: switch to event-based exit detection)
- Tracks session -> step mapping to route exit events
- Updates `run_step.session_id` in DB so UI can find the session

---

### 2. Session Exit Event Handling (ENHANCEMENT)

**File:** `internal/server/grpc/server.go` (modify CommandStream)

Currently, `SessionStatusEvent` is logged but not routed to executor. We need to:

1. Route `SessionExitEvent` (proto already exists) to executor
2. Update `Session` model to store exit code and status

```go
// In agentService.CommandStream receive loop, add:
if se := res.event.GetSessionExit(); se != nil {
    s.logger.Info("session exit event",
        "session_id", se.GetSessionId(),
        "exit_code", se.GetExitCode(),
    )
    // Update session status in DB
    if err := s.store.UpdateSessionStatus(se.GetSessionId(), "completed"); err != nil {
        s.logger.Warn("failed to update session status on exit", "error", err)
    }
    // TODO: Route to executor if it's tracking this session
}
```

---

### 3. Initial Prompt Injection (FUTURE)

**Problem:** When `PromptSnapshot` is set, we need to send it to Claude CLI after it starts.

**Solution:**
1. Create session with `CreateSessionCmd` (no prompt)
2. Wait 200ms for CLI to be ready
3. Send `InputDataCmd` with the prompt + newline

This is deferred to Phase 2 but the protocol supports it via `InputDataCmd`.

---

### 4. Main.go Wiring (CHANGE)

**File:** `cmd/server/main.go` (lines 150)

Replace:
```go
orch := orchestrator.NewOrchestrator(ctx, s, noopExecutor{})
```

With:
```go
stepExecutor := executor.NewSessionStepExecutor(
    connMgr,
    sessionHandler,
    s,
    registry,
    slog.Default(),
)
orch := orchestrator.NewOrchestrator(ctx, s, stepExecutor)
```

---

### 5. RunDetail.tsx Terminal Embedding (UI CHANGE)

**File:** `web/src/views/RunDetail.tsx`

Current structure: Shows RunDAGView on left, needs terminal on right when step is selected.

```tsx
// Add to component state
const [sessionId, setSessionId] = useState<string | null>(null);

// Effect to extract session_id from selected run step
useEffect(() => {
    if (selectedRunStep?.session_id) {
        setSessionId(selectedRunStep.session_id);
    } else {
        setSessionId(null);
    }
}, [selectedRunStep]);

// In render, conditionally show terminal:
<div className="grid grid-cols-[1fr_1fr] gap-6 h-[600px]">
    <RunDAGView
        steps={steps}
        runSteps={mergedRunSteps}
        dependencies={dependencies}
        selectedStepId={selectedStepId}
        onStepSelect={handleStepSelect}
    />
    {sessionId ? (
        <div className="border border-border rounded-lg overflow-hidden">
            <TerminalView
                sessionId={sessionId}
                title={selectedStepName}
                readOnly={true}
            />
        </div>
    ) : (
        <div className="border border-border rounded-lg flex items-center justify-center text-text-secondary">
            Select a step to view its terminal
        </div>
    )}
</div>
```

**Key Behaviors:**
- When step is selected and has a `session_id`, show terminal
- Terminal connects to `/ws/sessions/{sessionId}` WebSocket
- If session is still running, shows live output
- If session is completed, shows scrollback replay (already supported by TerminalView)
- Read-only mode: user cannot send input

---

### 6. Session Recording & Replay (ALREADY WORKS)

The agent already:
1. Records PTY output to `.cast` files (asciicast v2 format)
2. Sends scrollback chunks via `ScrollbackChunkEvent` when client attaches
3. The frontend `TerminalView` already supports replay via xterm.js

**What we need:** Ensure sessions created by executor trigger scrollback recording.

**Solution:** Agent's `NewSession()` always creates `ScrollbackWriter`, so this is automatic. ✓

---

## Task Breakdown

### Phase 1: Core Step Execution

#### Task 1.1: Create SessionStepExecutor
- **File:** `internal/server/executor/session_executor.go` (NEW)
- **What:** Implement `StepExecutor` interface, create sessions, poll for completion
- **Acceptance Criteria:**
  - Compiles
  - ExecuteStep() sends CreateSessionCmd to agent
  - ExecuteStep() updates run_step.session_id in DB
  - Polls session status and calls onComplete() with exit code
  - Handles agent disconnection gracefully
  - Logs all major events
- **Dependencies:** None (uses existing connmgr, session handler, store)
- **Estimated:** 3 hours

#### Task 1.2: Wire Up SessionStepExecutor in Main
- **File:** `cmd/server/main.go` (modify line 150)
- **What:** Replace `noopExecutor` with `SessionStepExecutor`
- **Acceptance Criteria:**
  - Server starts without panic
  - RunCreateRun() uses real executor
  - Step execution tests pass
- **Dependencies:** Task 1.1
- **Estimated:** 30 minutes

#### Task 1.3: Test Session Creation and Execution
- **File:** `internal/server/executor/executor_test.go` (NEW)
- **What:** Unit tests for SessionStepExecutor
  - Mock agent connection
  - Verify CreateSessionCmd is sent
  - Verify onComplete is called with correct exit code
  - Test timeout behavior
  - Test agent disconnect
- **Acceptance Criteria:**
  - 80%+ coverage of executor code
  - All edge cases covered
- **Dependencies:** Task 1.1
- **Estimated:** 4 hours

---

### Phase 2: UI Terminal Embedding

#### Task 2.1: Update RunDetail to Show Terminal
- **File:** `web/src/views/RunDetail.tsx` (modify)
- **What:** Add terminal viewer panel that shows when step is selected
- **Acceptance Criteria:**
  - Terminal appears on right side when step selected
  - Terminal shows live output while step is running
  - Terminal shows scrollback when step is completed
  - "Select a step" placeholder shown when nothing selected
  - Responsive to step selection changes
- **Dependencies:** Task 1.1 (needs real sessions to test with)
- **Estimated:** 2 hours

#### Task 2.2: Style Terminal Panel
- **File:** `web/src/views/RunDetail.tsx` (styling)
- **What:** Make terminal viewer panel look good, handle responsive layout
- **Acceptance Criteria:**
  - Works on desktop (1920px+)
  - Works on tablet (768px-1024px)
  - Terminal doesn't overflow container
  - Matches design system (colors, fonts, spacing)
- **Dependencies:** Task 2.1
- **Estimated:** 1 hour

---

### Phase 3: Exit Event Routing (OPTIONAL, FUTURE)

#### Task 3.1: Handle SessionExitEvent in gRPC Server
- **File:** `internal/server/grpc/server.go` (modify CommandStream)
- **What:** Route SessionExitEvent to executor (instead of polling)
- **Acceptance Criteria:**
  - Executor receives exit events from agent
  - onComplete() called immediately on exit (not on next poll)
  - Exit code propagates correctly
- **Dependencies:** Tasks 1.1, 1.2
- **Estimated:** 2 hours (deferred)

---

### Phase 4: Prompt Injection (OPTIONAL, FUTURE)

#### Task 4.1: Send Initial Prompt to CLI
- **File:** `internal/server/executor/session_executor.go` (enhance)
- **What:** After creating session, wait 200ms then send prompt via InputDataCmd
- **Acceptance Criteria:**
  - If runStep.PromptSnapshot is set, it's sent to CLI after startup
  - Prompt appears in session output
  - User sees prompt in terminal
  - Timing doesn't cause race conditions
- **Dependencies:** Tasks 1.1, 1.2
- **Estimated:** 2 hours (deferred)

---

### Phase 5: Integration & E2E Testing

#### Task 5.1: E2E Test: Create Job, Run It, Watch Terminal
- **File:** `tests/e2e/run_with_terminal_test.go` (NEW)
- **What:** Full scenario test
  - Create job with 2 steps
  - Create run
  - Monitor step 1 via WebSocket
  - Verify output appears in real-time
  - Wait for completion
  - Replay step 2's scrollback
- **Acceptance Criteria:**
  - Test passes with real Claude CLI
  - Terminal output captured correctly
  - Exit codes correct
- **Dependencies:** All of Phase 1, 2
- **Estimated:** 4 hours

#### Task 5.2: Documentation
- **File:** `docs/internal/product/step_execution.md` (NEW)
- **What:** Explain how step execution works end-to-end
  - Data flow diagram
  - Message sequence diagram
  - Code pointers
  - How to debug failures
- **Acceptance Criteria:**
  - New dev can understand the system in 30 min
  - All edge cases documented
- **Dependencies:** All of Phases 1-4
- **Estimated:** 2 hours

---

## Edge Cases & Handling

### 1. Agent Disconnects During Step Execution

**Scenario:** Agent is running a step, then disconnects from server.

**Solution:**
- `connMgr.GetAgent()` returns nil
- Executor detects agent is no longer available
- Logs warning
- Calls `onComplete(stepID, 1)` to fail the step
- Run fails (or continues depending on on_failure policy)

**Code:**
```go
if agent == nil {
    e.logger.Error("agent disconnected during execution",
        "machine_id", runStep.MachineIDSnapshot,
        "step_id", runStep.StepID)
    e.completeStep(sessionID, 1)
    return
}
```

---

### 2. Session Not Found During Polling

**Scenario:** Executor polls for session status, but session was deleted.

**Solution:**
- Treat as "session completed with exit code 1"
- Log warning
- Call `onComplete(stepID, 1)`

**Code:**
```go
sess, err := e.store.GetSession(sessionID)
if err == store.ErrNotFound {
    e.logger.Warn("session disappeared during polling",
        "session_id", sessionID)
    e.completeStep(sessionID, 1)
    return
}
```

---

### 3. Step Timeout

**Scenario:** Step takes longer than timeout.

**Solution:**
- Use `context.WithTimeout()` on step's deadline
- When timeout expires, send `KillSessionCmd` to agent
- Call `onComplete(stepID, 124)` (standard timeout exit code)

**Code:**
```go
timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
defer cancel()

select {
case <-timeoutCtx.Done():
    e.sendKillCommand(sessionID, machineID)
    e.completeStep(sessionID, 124)
}
```

---

### 4. Concurrent Steps (Dependencies)

**Scenario:** Two steps run in parallel (no dependency between them).

**Solution:**
- Executor creates separate sessions for each step
- Sessions are independent on agent
- Each has own session_id, tracked separately
- Both can run simultaneously

**Note:** Agent has `max_sessions` limit; if exceeded, command fails. Then step fails.

---

### 5. Exit Code Capture

**Scenario:** Process exits; we need to know the exit code.

**Solution:**
- Agent's `session.waitForExit()` captures it
- Sends `SessionStatusEvent` with status (Completed/Failed)
- Sends `SessionExitEvent` with exit code (proto already has it)
- Executor polls session and reads exit code from DB

**Current:** Polling `Session.ExitCode` field  
**Future:** Subscribe to `SessionExitEvent` for faster notification

---

### 6. Browser Disconnects During Live Terminal

**Scenario:** User watching step closes browser mid-execution.

**Solution:**
- WebSocket closes
- `RunDetail.tsx` unmounts
- Step execution continues on server (not killed)
- User can reopen the page and reconnect to the session later
- Session continues running; output is recorded to scrollback

---

## Data Flow Diagrams

### Live Terminal Viewing

```
Browser                    Server                 Agent
  |                          |                       |
  |-- WebSocket connect ----->|                       |
  |  /ws/sessions/sess123    |                       |
  |                          |                       |
  |                          |-- AttachSessionCmd -->|
  |                          |                       |
  |                          |<-- Scrollback chunks--|
  |<-- Scrollback messages ---|                       |
  |                          |                       |
  |                          |<-- Live output events-|
  |<-- Terminal data ---------|                       |
  |  (SessionOutput)          |                       |
  |                          |                       |
```

### Step Execution

```
Orchestrator              SessionExecutor         ConnectionMgr      Agent
     |                          |                       |               |
     |-- ExecuteStep() -------->|                       |               |
     |                          |                       |               |
     |                          |-- GetAgent --------->|               |
     |                          |<-- Agent ---|        |               |
     |                          |             |        |               |
     |                          |-- SendCommand -------|-- CreateSession->|
     |                          |             |        |               |
     |                          |             |        |               | (PTY started)
     |                          |-- UpdateRunStepStatus with session_id
     |                          |             |        |               |
     |                          |-- Monitor   |        |               |
     |                          |   (poll)    |        |               |
     |                          |             |        |<-- SessionStatus
     |                          |             |        |   /Exit events
     |                          |             |        |
     |                          |-- onComplete ------>|
     |<-- OnStepCompleted ------|               |      |
     |  (stepID, exitCode)       |               |      |
```

---

## Implementation Checklist

### Pre-Implementation
- [ ] Review this plan with team
- [ ] Identify any blocking dependencies
- [ ] Set up test environment with real agent

### Core Executor (Phase 1)
- [ ] Create `internal/server/executor/session_executor.go`
- [ ] Implement `StepExecutor` interface
- [ ] Implement session creation logic
- [ ] Implement exit polling logic
- [ ] Implement timeout handling
- [ ] Implement error cases
- [ ] Add logging at key points
- [ ] Write unit tests (80%+ coverage)
- [ ] Wire up in `cmd/server/main.go`
- [ ] Integration test with real agent

### UI Terminal (Phase 2)
- [ ] Update `RunDetail.tsx` to show terminal panel
- [ ] Style terminal container
- [ ] Handle responsive layout
- [ ] Test live terminal streaming
- [ ] Test scrollback replay

### Exit Event Routing (Phase 3)
- [ ] Add SessionExitEvent handling to gRPC server
- [ ] Route events to executor
- [ ] Replace polling with event subscription
- [ ] Performance test

### Prompt Injection (Phase 4)
- [ ] Implement delayed prompt sending in executor
- [ ] Test with actual Claude CLI
- [ ] Verify prompt appears in output

### E2E & Docs (Phase 5)
- [ ] Write E2E test scenario
- [ ] Write architecture documentation
- [ ] Update README with new capabilities
- [ ] Create debugging guide

---

## Key Code References

- **StepExecutor interface:** `internal/server/orchestrator/dag_runner.go:13`
- **Current noopExecutor:** `cmd/server/main.go:41`
- **Session handler:** `internal/server/session/handler.go:82`
- **Connection manager:** `internal/server/connmgr/manager.go:142`
- **gRPC command stream:** `internal/server/grpc/server.go:142`
- **Agent session manager:** `internal/agent/session_manager.go:76`
- **Agent session/PTY:** `internal/agent/session.go:36`
- **RunStep model:** `internal/server/store/jobs.go:130`
- **Proto messages:** `proto/claudeplane/v1/agent.proto:64,129`
- **Session registry:** `internal/server/session/registry.go:29`
- **Scrollback writer:** `internal/agent/scrollback.go:19`
- **RunDetail page:** `web/src/views/RunDetail.tsx`

---

## Success Criteria

1. **Real step execution:** Steps create actual CLI sessions and execute commands
2. **Session linkage:** `run_steps.session_id` is populated when step runs
3. **Live terminal:** User can watch step executing in real-time via WebSocket
4. **Replay capability:** After completion, user can replay step's scrollback
5. **Error handling:** All edge cases handled gracefully (agent disconnect, timeout, etc.)
6. **Tests:** 80%+ coverage of executor logic
7. **No regression:** Existing job/run/session functionality still works
8. **Documentation:** New behavior documented for developers

---

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Agent crash during step | Executor detects session exit, fails step gracefully |
| Memory leak from session tracking | Cleanup is always called, even on error |
| WebSocket timeout during long step | xterm.js handles reconnection; session continues |
| Concurrent step race conditions | DAGRunner serializes step launches via mutex |
| Exit code not captured | Fallback: assume exit code 1 if session not found |
| Slow network latency in output | Non-blocking publish to registry drops old data |

---

## Future Enhancements (Out of Scope)

1. **Event-based exit detection:** Replace polling with SessionExitEvent subscription
2. **Prompt injection:** Auto-send initial prompt after CLI startup
3. **Step output caching:** Cache session outputs for faster history fetch
4. **Parallel step execution:** Allow concurrent execution within resource limits
5. **Step retry with context:** Preserve prompt/output history on retry
6. **Audit logging:** Track who triggered what step when
7. **Cost tracking:** Per-step token usage metrics

