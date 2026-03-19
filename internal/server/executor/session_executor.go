// Package executor provides StepExecutor implementations for the orchestrator.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/orchestrator"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	"github.com/kodrunhq/claude-plane/internal/shared/cliutil"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

const (
	defaultCommand      = "claude"
	defaultTermRows     = 24
	defaultTermCols     = 80
	pollInterval        = 500 * time.Millisecond
	defaultTimeout      = 1 * time.Hour
	timeoutExitCode     = 124
	failureExitCode     = 1
	maxNotFoundRetries  = 5
)

// RunStarter creates a new run for a job. Used by run_job tasks to trigger
// child job runs in a fire-and-forget manner.
type RunStarter interface {
	StartRun(ctx context.Context, jobID string, params map[string]string) error
}

// RunStarterFunc adapts a function to the RunStarter interface.
type RunStarterFunc func(ctx context.Context, jobID string, params map[string]string) error

// StartRun calls the underlying function.
func (f RunStarterFunc) StartRun(ctx context.Context, jobID string, params map[string]string) error {
	return f(ctx, jobID, params)
}

// sessionStore is the subset of store.Store methods needed by SessionStepExecutor.
// Defined as an interface for testability.
type sessionStore interface {
	CreateSession(sess *store.Session) error
	GetSession(id string) (*store.Session, error)
	UpdateSessionStatus(id, status string) error
}

// runStepStore is the subset of store.Store methods needed for run step updates.
type runStepStore interface {
	UpdateRunStepStatus(ctx context.Context, runStepID, status, sessionID string, exitCode int) error
	UpdateRunStepErrorMessage(ctx context.Context, runStepID, message string) error
}

// storeIface combines both store interfaces needed by the executor.
type storeIface interface {
	sessionStore
	runStepStore
}

// stepTracking holds in-flight state for a step being executed.
type stepTracking struct {
	runID      string
	runStepID  string
	stepID     string
	onComplete func(stepID string, exitCode int)
	cancel     context.CancelFunc
}

// sessionKeyEntry is a struct key for the shared sessions map.
// Using a struct (not string concatenation) prevents collision when runID
// or sessionKey contain the separator character.
type sessionKeyEntry struct {
	runID      string
	sessionKey string
}

// sharedSessionEntry tracks a shared session's state.
type sharedSessionEntry struct {
	sessionID string
	machineID string
}

// SessionStepExecutor creates real PTY-backed sessions on agents
// and monitors for completion by polling session status.
type SessionStepExecutor struct {
	connMgr    *connmgr.ConnectionManager
	store      storeIface
	logger     *slog.Logger
	runStarter RunStarter // for run_job tasks

	mu            sync.RWMutex
	sessionToStep map[string]*stepTracking

	// sharedSessions tracks active shared sessions keyed by {runID, sessionKey}.
	// Sessions remain alive until the run completes or is cancelled.
	sharedSessionsMu sync.Mutex
	sharedSessions   map[sessionKeyEntry]*sharedSessionEntry
}

// SetRunStarter sets the RunStarter used by run_job tasks. This is separate
// from the constructor to break the circular dependency between the executor
// and the orchestrator (executor needs orchestrator to start runs, orchestrator
// needs executor to execute steps).
func (e *SessionStepExecutor) SetRunStarter(rs RunStarter) {
	e.mu.Lock()
	e.runStarter = rs
	e.mu.Unlock()
}

// NewSessionStepExecutor creates a new SessionStepExecutor.
// If logger is nil, slog.Default() is used.
func NewSessionStepExecutor(
	connMgr *connmgr.ConnectionManager,
	st storeIface,
	logger *slog.Logger,
) *SessionStepExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionStepExecutor{
		connMgr:        connMgr,
		store:          st,
		logger:         logger,
		sessionToStep:  make(map[string]*stepTracking),
		sharedSessions: make(map[sessionKeyEntry]*sharedSessionEntry),
	}
}

// ExecuteStep launches the step on the target agent and begins monitoring the
// resulting session. It is non-blocking; completion is signalled via onComplete.
// Dispatches to executeShellTask or executeClaudeSession based on TaskTypeSnapshot.
func (e *SessionStepExecutor) ExecuteStep(
	ctx context.Context,
	runStep store.RunStep,
	resolveCtx *orchestrator.ResolveContext,
	onComplete func(stepID string, exitCode int),
) {
	switch runStep.TaskTypeSnapshot {
	case "shell":
		e.executeShellTask(ctx, runStep, resolveCtx, onComplete)
	case "run_job":
		e.executeRunJob(ctx, runStep, resolveCtx, onComplete)
	default:
		e.executeClaudeSession(ctx, runStep, resolveCtx, onComplete)
	}
}

// RunStepIDForSession returns the run step ID associated with the given session.
// This is used by the gRPC server to look up where to persist task values.
func (e *SessionStepExecutor) RunStepIDForSession(sessionID string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	t, ok := e.sessionToStep[sessionID]
	if !ok {
		return "", false
	}
	return t.runStepID, true
}

// resolveFields applies template resolution if a ResolveContext is available.
func resolveField(value string, rc *orchestrator.ResolveContext) string {
	if rc == nil || value == "" {
		return value
	}
	return orchestrator.ResolveReferences(value, rc.RunParams, rc.JobMeta, rc.StepValues, rc.StepResults)
}

// executeRunJob triggers a child job run and immediately completes.
// The child run is fire-and-forget: the parent step marks as completed
// regardless of whether the child run succeeds.
func (e *SessionStepExecutor) executeRunJob(
	ctx context.Context,
	runStep store.RunStep,
	resolveCtx *orchestrator.ResolveContext,
	onComplete func(stepID string, exitCode int),
) {
	targetJobID := resolveField(runStep.TargetJobIDSnapshot, resolveCtx)
	if targetJobID == "" {
		e.logger.Error("run_job task has no target_job_id", "step_id", runStep.StepID)
		if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusRunning, "", 0); err != nil {
			e.logger.Warn("failed to update run step status", "error", err, "run_step_id", runStep.RunStepID)
		}
		if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusFailed, "", failureExitCode); err != nil {
			e.logger.Warn("failed to update run step status", "error", err, "run_step_id", runStep.RunStepID)
		}
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	e.mu.RLock()
	rs := e.runStarter
	e.mu.RUnlock()
	if rs == nil {
		e.logger.Error("run_job task cannot execute: no RunStarter configured", "step_id", runStep.StepID)
		if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusRunning, "", 0); err != nil {
			e.logger.Warn("failed to update run step status", "error", err, "run_step_id", runStep.RunStepID)
		}
		if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusFailed, "", failureExitCode); err != nil {
			e.logger.Warn("failed to update run step status", "error", err, "run_step_id", runStep.RunStepID)
		}
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	// Parse job params from snapshot
	params := make(map[string]string)
	if runStep.JobParamsSnapshot != "" {
		if err := json.Unmarshal([]byte(runStep.JobParamsSnapshot), &params); err != nil {
			e.logger.Error("failed to parse job_params", "error", err, "step_id", runStep.StepID)
			if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusRunning, "", 0); err != nil {
				e.logger.Warn("failed to update run step status", "error", err, "run_step_id", runStep.RunStepID)
			}
			if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusFailed, "", failureExitCode); err != nil {
				e.logger.Warn("failed to update run step status", "error", err, "run_step_id", runStep.RunStepID)
			}
			onComplete(runStep.StepID, failureExitCode)
			return
		}
	}

	// Resolve template expressions in param values
	if resolveCtx != nil {
		resolved := make(map[string]string, len(params))
		for k, v := range params {
			resolved[k] = resolveField(v, resolveCtx)
		}
		params = resolved
	}

	// Mark step as running
	if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusRunning, "", 0); err != nil {
		e.logger.Warn("failed to update run step status", "error", err, "run_step_id", runStep.RunStepID)
	}

	// Fire and forget: start the child run
	if err := rs.StartRun(ctx, targetJobID, params); err != nil {
		e.logger.Error("failed to start child job run",
			"error", err,
			"target_job_id", targetJobID,
			"step_id", runStep.StepID,
		)
		if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusFailed, "", failureExitCode); err != nil {
			e.logger.Warn("failed to update run step status", "error", err, "run_step_id", runStep.RunStepID)
		}
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	// Immediately mark as completed
	if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusCompleted, "", 0); err != nil {
		e.logger.Warn("failed to update run step status", "error", err, "run_step_id", runStep.RunStepID)
	}
	onComplete(runStep.StepID, 0)
}

// executeClaudeSession handles the default Claude CLI session execution path.
// If the step has a SessionKeySnapshot, it delegates to the shared session path.
func (e *SessionStepExecutor) executeClaudeSession(
	ctx context.Context,
	runStep store.RunStep,
	resolveCtx *orchestrator.ResolveContext,
	onComplete func(stepID string, exitCode int),
) {
	if runStep.SessionKeySnapshot != "" {
		e.executeSharedSession(ctx, runStep, resolveCtx, onComplete)
		return
	}

	agent := e.connMgr.GetAgent(runStep.MachineIDSnapshot)
	if agent == nil {
		e.logger.Warn("no connected agent for machine",
			"machine_id", runStep.MachineIDSnapshot,
			"run_step_id", runStep.RunStepID,
		)
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	sessionID := uuid.New().String()

	command := runStep.CommandSnapshot
	if command == "" {
		command = defaultCommand
	}
	args := cliutil.ParseArgs(runStep.ArgsSnapshot)

	// Inject --dangerously-skip-permissions if snapshot says so.
	// NOTE: A nil SkipPermissionsSnapshot currently means "skip permissions" by default for jobs,
	// so we treat nil the same as a non-zero (truthy) value here.
	// TODO(kodrun#prefs-in-snapshot): In a future iteration, resolve a nil value against the user's
	// preferences at snapshot time instead of defaulting here in the executor.
	if runStep.SkipPermissionsSnapshot == nil || *runStep.SkipPermissionsSnapshot != 0 {
		args = cliutil.StripFlag(args, "--dangerously-skip-permissions")
		args = append([]string{"--dangerously-skip-permissions"}, args...)
	}

	// Inject --model if set in snapshot, stripping any existing --model from user-supplied args.
	if runStep.ModelSnapshot != "" {
		args = cliutil.StripFlagWithValue(args, "--model")
		args = append(args, "--model", runStep.ModelSnapshot)
	}

	// Resolve template references in the prompt.
	resolvedPrompt := resolveField(runStep.PromptSnapshot, resolveCtx)

	workingDir := runStep.WorkingDirSnapshot

	sess := &store.Session{
		SessionID:  sessionID,
		MachineID:  runStep.MachineIDSnapshot,
		Command:    command,
		WorkingDir: workingDir,
		Status:     store.StatusCreated,
	}
	if err := e.store.CreateSession(sess); err != nil {
		e.logger.Error("failed to create session in store",
			"session_id", sessionID,
			"run_step_id", runStep.RunStepID,
			"error", err,
		)
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	// Link session to run step immediately so the frontend can reference
	// the session even if the command fails to send to the agent.
	if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusRunning, sessionID, 0); err != nil {
		e.logger.Warn("failed to link session to run step",
			"run_step_id", runStep.RunStepID,
			"session_id", sessionID,
			"error", err,
		)
	}

	cmd := &pb.ServerCommand{
		Command: &pb.ServerCommand_CreateSession{
			CreateSession: &pb.CreateSessionCmd{
				SessionId:  sessionID,
				Command:    command,
				Args:       args,
				WorkingDir: workingDir,
				TerminalSize: &pb.TerminalSize{
					Rows: defaultTermRows,
					Cols: defaultTermCols,
				},
				InitialPrompt: resolvedPrompt,
				TaskType:       "claude_session",
			},
		},
	}

	if err := agent.SendCommand(cmd); err != nil {
		e.logger.Error("failed to send CreateSession command to agent",
			"session_id", sessionID,
			"machine_id", runStep.MachineIDSnapshot,
			"error", err,
		)
		// Best-effort cleanup: mark session failed in store.
		if updateErr := e.store.UpdateSessionStatus(sessionID, store.StatusFailed); updateErr != nil {
			e.logger.Warn("failed to mark session as failed after send error",
				"session_id", sessionID,
				"error", updateErr,
			)
		}
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	e.trackAndMonitor(ctx, runStep, sessionID, onComplete)
}

// executeShellTask handles shell task execution. Shell tasks run an arbitrary
// command (not Claude CLI) — no --dangerously-skip-permissions, no --model injection,
// and no initial prompt.
func (e *SessionStepExecutor) executeShellTask(
	ctx context.Context,
	runStep store.RunStep,
	resolveCtx *orchestrator.ResolveContext,
	onComplete func(stepID string, exitCode int),
) {
	if runStep.CommandSnapshot == "" {
		e.logger.Error("shell task has no command",
			"run_step_id", runStep.RunStepID,
			"step_id", runStep.StepID,
		)
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	agent := e.connMgr.GetAgent(runStep.MachineIDSnapshot)
	if agent == nil {
		e.logger.Warn("no connected agent for machine",
			"machine_id", runStep.MachineIDSnapshot,
			"run_step_id", runStep.RunStepID,
		)
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	sessionID := uuid.New().String()

	// Security: do NOT resolve templates in CommandSnapshot. The command binary
	// must be a static string to prevent parameter-injection attacks (e.g. a
	// malicious parameter value replacing the command with an arbitrary binary).
	// Only Args are resolved, which limits the blast radius to argument values.
	resolvedArgsJSON := resolveField(runStep.ArgsSnapshot, resolveCtx)
	args := cliutil.ParseArgs(resolvedArgsJSON)

	workingDir := runStep.WorkingDirSnapshot

	sess := &store.Session{
		SessionID:  sessionID,
		MachineID:  runStep.MachineIDSnapshot,
		Command:    runStep.CommandSnapshot,
		WorkingDir: workingDir,
		Status:     store.StatusCreated,
	}
	if err := e.store.CreateSession(sess); err != nil {
		e.logger.Error("failed to create session in store",
			"session_id", sessionID,
			"run_step_id", runStep.RunStepID,
			"error", err,
		)
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	// Link session to run step immediately so the frontend can reference
	// the session even if the command fails to send to the agent.
	if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusRunning, sessionID, 0); err != nil {
		e.logger.Warn("failed to link session to run step",
			"run_step_id", runStep.RunStepID,
			"session_id", sessionID,
			"error", err,
		)
	}

	cmd := &pb.ServerCommand{
		Command: &pb.ServerCommand_CreateSession{
			CreateSession: &pb.CreateSessionCmd{
				SessionId:  sessionID,
				Command:    runStep.CommandSnapshot,
				Args:       args,
				WorkingDir: workingDir,
				TerminalSize: &pb.TerminalSize{
					Rows: defaultTermRows,
					Cols: defaultTermCols,
				},
				TaskType: "shell",
			},
		},
	}

	if err := agent.SendCommand(cmd); err != nil {
		e.logger.Error("failed to send CreateSession command to agent",
			"session_id", sessionID,
			"machine_id", runStep.MachineIDSnapshot,
			"error", err,
		)
		if updateErr := e.store.UpdateSessionStatus(sessionID, store.StatusFailed); updateErr != nil {
			e.logger.Warn("failed to mark session as failed after send error",
				"session_id", sessionID,
				"error", updateErr,
			)
		}
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	e.trackAndMonitor(ctx, runStep, sessionID, onComplete)
}

// stepTimeout returns the timeout duration for a run step.
// If the step has a positive TimeoutSecondsSnapshot, that value is used;
// otherwise the package-level defaultTimeout is returned.
func stepTimeout(runStep store.RunStep) time.Duration {
	if runStep.TimeoutSecondsSnapshot > 0 {
		return time.Duration(runStep.TimeoutSecondsSnapshot) * time.Second
	}
	return defaultTimeout
}

// trackAndMonitor updates the run step to running, registers tracking state, and
// starts the session monitor goroutine. Shared by both execution paths.
func (e *SessionStepExecutor) trackAndMonitor(
	ctx context.Context,
	runStep store.RunStep,
	sessionID string,
	onComplete func(stepID string, exitCode int),
) {
	if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusRunning, sessionID, 0); err != nil {
		e.logger.Warn("failed to update run step to running",
			"run_step_id", runStep.RunStepID,
			"session_id", sessionID,
			"error", err,
		)
	}

	monitorCtx, cancel := context.WithCancel(ctx)

	tracking := &stepTracking{
		runID:      runStep.RunID,
		runStepID:  runStep.RunStepID,
		stepID:     runStep.StepID,
		onComplete: onComplete,
		cancel:     cancel,
	}

	e.mu.Lock()
	e.sessionToStep[sessionID] = tracking
	e.mu.Unlock()

	timeout := stepTimeout(runStep)
	go e.monitorSessionExit(monitorCtx, sessionID, runStep.MachineIDSnapshot, timeout)
}

// monitorSessionExit polls the session status until it reaches a terminal state,
// the context is cancelled, or the timeout elapses.
func (e *SessionStepExecutor) monitorSessionExit(
	ctx context.Context,
	sessionID string,
	machineID string,
	timeout time.Duration,
) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	deadline := time.Now().Add(timeout)
	notFoundCount := 0

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("session monitor context cancelled, killing session",
				"session_id", sessionID,
				"machine_id", machineID,
			)
			e.sendKill(machineID, sessionID)
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			e.completeStep(cleanupCtx, sessionID, failureExitCode)
			cleanupCancel()
			return

		case <-ticker.C:
			if time.Now().After(deadline) {
				e.logger.Warn("session timed out, sending kill",
					"session_id", sessionID,
					"machine_id", machineID,
					"timeout", timeout,
				)
				e.sendKill(machineID, sessionID)
				e.completeStep(ctx, sessionID, timeoutExitCode)
				return
			}

			sess, err := e.store.GetSession(sessionID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					notFoundCount++
					e.logger.Warn("session not found during polling",
						"session_id", sessionID,
						"attempt", notFoundCount,
						"error", err,
					)
					if notFoundCount >= maxNotFoundRetries {
						e.logger.Error("session persistently not found, failing step",
							"session_id", sessionID,
						)
						e.completeStep(ctx, sessionID, failureExitCode)
						return
					}
				} else {
					e.logger.Warn("transient error polling session",
						"session_id", sessionID,
						"error", err,
					)
				}
				continue
			}
			// Reset counter on successful read.
			notFoundCount = 0

			switch sess.Status {
			case store.StatusCompleted:
				e.logger.Info("session completed", "session_id", sessionID)
				e.completeStep(ctx, sessionID, 0)
				return

			case store.StatusFailed, store.StatusTerminated:
				e.logger.Info("session ended with failure",
					"session_id", sessionID,
					"status", sess.Status,
				)
				e.completeStep(ctx, sessionID, failureExitCode)
				return
			}
			// Non-terminal status (created, starting, running): continue polling.
		}
	}
}

// completeStep looks up tracking info for the session, calls the onComplete
// callback, and removes the tracking entry. For non-zero exit codes, it
// persists an error message on the run step.
func (e *SessionStepExecutor) completeStep(ctx context.Context, sessionID string, exitCode int) {
	e.mu.Lock()
	tracking, ok := e.sessionToStep[sessionID]
	if ok {
		delete(e.sessionToStep, sessionID)
	}
	e.mu.Unlock()

	if !ok {
		e.logger.Warn("completeStep called for unknown session", "session_id", sessionID)
		return
	}

	// Persist a human-readable error message for failed steps.
	if exitCode != 0 {
		msg := fmt.Sprintf("Process exited with code %d", exitCode)
		if err := e.store.UpdateRunStepErrorMessage(ctx, tracking.runStepID, msg); err != nil {
			e.logger.Error("failed to persist error message", "run_step_id", tracking.runStepID, "error", err)
		}
	}

	tracking.cancel()
	tracking.onComplete(tracking.stepID, exitCode)
}

// executeSharedSession handles steps that share a Claude CLI session via session keys.
// The first step with a given key creates the session with keep_alive=true.
// Subsequent steps reuse the existing session by sending an InputDataCmd.
func (e *SessionStepExecutor) executeSharedSession(
	ctx context.Context,
	runStep store.RunStep,
	resolveCtx *orchestrator.ResolveContext,
	onComplete func(stepID string, exitCode int),
) {
	key := sessionKeyEntry{runID: runStep.RunID, sessionKey: runStep.SessionKeySnapshot}

	e.sharedSessionsMu.Lock()
	existing := e.sharedSessions[key]
	e.sharedSessionsMu.Unlock()

	if existing != nil {
		// Subsequent step: reuse existing session.
		e.executeSharedSubsequentStep(ctx, runStep, resolveCtx, onComplete, existing.sessionID, existing.machineID)
		return
	}

	// First step with this key: create a new shared session.
	e.executeSharedFirstStep(ctx, runStep, resolveCtx, onComplete, key)
}

// executeSharedFirstStep creates a new shared session with keep_alive=true.
func (e *SessionStepExecutor) executeSharedFirstStep(
	ctx context.Context,
	runStep store.RunStep,
	resolveCtx *orchestrator.ResolveContext,
	onComplete func(stepID string, exitCode int),
	key sessionKeyEntry,
) {
	agent := e.connMgr.GetAgent(runStep.MachineIDSnapshot)
	if agent == nil {
		e.logger.Warn("no connected agent for shared session",
			"machine_id", runStep.MachineIDSnapshot,
			"run_step_id", runStep.RunStepID,
			"session_key", runStep.SessionKeySnapshot,
		)
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	sessionID := uuid.New().String()

	command := runStep.CommandSnapshot
	if command == "" {
		command = defaultCommand
	}
	args := cliutil.ParseArgs(runStep.ArgsSnapshot)

	if runStep.SkipPermissionsSnapshot == nil || *runStep.SkipPermissionsSnapshot != 0 {
		args = cliutil.StripFlag(args, "--dangerously-skip-permissions")
		args = append([]string{"--dangerously-skip-permissions"}, args...)
	}

	if runStep.ModelSnapshot != "" {
		args = cliutil.StripFlagWithValue(args, "--model")
		args = append(args, "--model", runStep.ModelSnapshot)
	}

	resolvedPrompt := resolveField(runStep.PromptSnapshot, resolveCtx)
	workingDir := runStep.WorkingDirSnapshot

	sess := &store.Session{
		SessionID:  sessionID,
		MachineID:  runStep.MachineIDSnapshot,
		Command:    command,
		WorkingDir: workingDir,
		Status:     store.StatusCreated,
	}
	if err := e.store.CreateSession(sess); err != nil {
		e.logger.Error("failed to create shared session in store",
			"session_id", sessionID,
			"run_step_id", runStep.RunStepID,
			"error", err,
		)
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	// Link session to run step immediately so the frontend can reference
	// the session even if the command fails to send to the agent.
	if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusRunning, sessionID, 0); err != nil {
		e.logger.Warn("failed to link session to run step",
			"run_step_id", runStep.RunStepID,
			"session_id", sessionID,
			"error", err,
		)
	}

	cmd := &pb.ServerCommand{
		Command: &pb.ServerCommand_CreateSession{
			CreateSession: &pb.CreateSessionCmd{
				SessionId:  sessionID,
				Command:    command,
				Args:       args,
				WorkingDir: workingDir,
				TerminalSize: &pb.TerminalSize{
					Rows: defaultTermRows,
					Cols: defaultTermCols,
				},
				InitialPrompt: resolvedPrompt,
				TaskType:      "claude_session",
				KeepAlive:     true,
			},
		},
	}

	if err := agent.SendCommand(cmd); err != nil {
		e.logger.Error("failed to send CreateSession for shared session",
			"session_id", sessionID,
			"machine_id", runStep.MachineIDSnapshot,
			"error", err,
		)
		if updateErr := e.store.UpdateSessionStatus(sessionID, store.StatusFailed); updateErr != nil {
			e.logger.Warn("failed to mark shared session as failed",
				"session_id", sessionID,
				"error", updateErr,
			)
		}
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	// Register in shared sessions map.
	e.sharedSessionsMu.Lock()
	e.sharedSessions[key] = &sharedSessionEntry{
		sessionID: sessionID,
		machineID: runStep.MachineIDSnapshot,
	}
	e.sharedSessionsMu.Unlock()

	e.logger.Info("shared session created",
		"session_id", sessionID,
		"run_id", runStep.RunID,
		"session_key", runStep.SessionKeySnapshot,
		"step_id", runStep.StepID,
	)

	// Track and monitor like a normal step, but the session won't exit on idle
	// because keep_alive=true. The monitor stays alive to detect session crashes.
	e.trackAndMonitor(ctx, runStep, sessionID, onComplete)
}

// executeSharedSubsequentStep sends a new prompt to an existing shared session.
func (e *SessionStepExecutor) executeSharedSubsequentStep(
	ctx context.Context,
	runStep store.RunStep,
	resolveCtx *orchestrator.ResolveContext,
	onComplete func(stepID string, exitCode int),
	sessionID string,
	machineID string,
) {
	agent := e.connMgr.GetAgent(machineID)
	if agent == nil {
		e.logger.Warn("no connected agent for shared session subsequent step",
			"machine_id", machineID,
			"session_id", sessionID,
			"run_step_id", runStep.RunStepID,
		)
		onComplete(runStep.StepID, failureExitCode)
		return
	}

	resolvedPrompt := resolveField(runStep.PromptSnapshot, resolveCtx)

	// Update run step status to running with the shared session ID.
	if err := e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, store.StatusRunning, sessionID, 0); err != nil {
		e.logger.Warn("failed to update run step to running for shared session",
			"run_step_id", runStep.RunStepID,
			"session_id", sessionID,
			"error", err,
		)
	}

	// Register tracking for this step on the shared session.
	// This overwrites any previous tracking entry for the session,
	// which is correct because the previous step already completed via OnStepIdle.
	tracking := &stepTracking{
		runID:      runStep.RunID,
		runStepID:  runStep.RunStepID,
		stepID:     runStep.StepID,
		onComplete: onComplete,
		cancel:     func() {}, // no-op: shared sessions are not cancelled per-step
	}

	e.mu.Lock()
	e.sessionToStep[sessionID] = tracking
	e.mu.Unlock()

	// Send the prompt as input to the existing CLI session.
	inputCmd := &pb.ServerCommand{
		Command: &pb.ServerCommand_InputData{
			InputData: &pb.InputDataCmd{
				SessionId: sessionID,
				Data:      []byte(resolvedPrompt + "\r"),
			},
		},
	}

	if err := agent.SendCommand(inputCmd); err != nil {
		e.logger.Error("failed to send input to shared session",
			"session_id", sessionID,
			"run_step_id", runStep.RunStepID,
			"error", err,
		)
		e.completeSharedStep(ctx, sessionID, failureExitCode)
		return
	}

	e.logger.Info("prompt submitted to shared session",
		"session_id", sessionID,
		"run_id", runStep.RunID,
		"session_key", runStep.SessionKeySnapshot,
		"step_id", runStep.StepID,
		"prompt_len", len(resolvedPrompt),
	)
}

// completeSharedStep signals step completion without killing the shared session.
// Unlike completeStep, this does NOT call tracking.cancel() because the session
// monitor must stay alive for subsequent steps.
func (e *SessionStepExecutor) completeSharedStep(ctx context.Context, sessionID string, exitCode int) {
	e.mu.Lock()
	tracking, ok := e.sessionToStep[sessionID]
	if ok {
		delete(e.sessionToStep, sessionID)
	}
	e.mu.Unlock()

	if !ok {
		e.logger.Warn("completeSharedStep called for unknown session", "session_id", sessionID)
		return
	}

	// Persist error message for failed shared steps, same as completeStep.
	if exitCode != 0 {
		msg := fmt.Sprintf("Process exited with code %d", exitCode)
		if err := e.store.UpdateRunStepErrorMessage(ctx, tracking.runStepID, msg); err != nil {
			e.logger.Error("failed to persist error message", "run_step_id", tracking.runStepID, "error", err)
		}
	}

	// Do NOT call tracking.cancel() — the session stays alive for the next step.
	tracking.onComplete(tracking.stepID, exitCode)
}

// OnStepIdle handles StepIdleEvent from the agent, indicating a shared session
// step has completed (the CLI returned to its prompt). This is called by the
// gRPC server when it receives a StepIdleEvent.
func (e *SessionStepExecutor) OnStepIdle(sessionID string) {
	e.logger.Info("step idle event received", "session_id", sessionID)
	// OnStepIdle is called from the gRPC handler with no parent context.
	// exitCode is always 0 here (idle = success) so no DB write occurs.
	e.completeSharedStep(context.Background(), sessionID, 0)
}

// CleanupRunSessions kills all shared sessions belonging to a run.
// Called when a run is cancelled or completes to release session resources.
func (e *SessionStepExecutor) CleanupRunSessions(runID string) {
	e.sharedSessionsMu.Lock()
	var toClean []sharedSessionEntry
	for key, entry := range e.sharedSessions {
		if key.runID == runID {
			toClean = append(toClean, *entry)
			delete(e.sharedSessions, key)
		}
	}
	e.sharedSessionsMu.Unlock()

	for _, entry := range toClean {
		e.logger.Info("cleaning up shared session for run",
			"run_id", runID,
			"session_id", entry.sessionID,
			"machine_id", entry.machineID,
		)
		e.sendKill(entry.machineID, entry.sessionID)
	}
}

// sendKill sends a SIGTERM kill command to the agent for the given session.
// Errors are logged but not propagated; the caller handles completion regardless.
func (e *SessionStepExecutor) sendKill(machineID, sessionID string) {
	agent := e.connMgr.GetAgent(machineID)
	if agent == nil {
		e.logger.Warn("no agent to send kill command",
			"machine_id", machineID,
			"session_id", sessionID,
		)
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
		e.logger.Warn("failed to send kill command to agent",
			"machine_id", machineID,
			"session_id", sessionID,
			"error", err,
		)
	}
}

