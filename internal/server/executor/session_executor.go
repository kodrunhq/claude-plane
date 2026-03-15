// Package executor provides StepExecutor implementations for the orchestrator.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/orchestrator"
	"github.com/kodrunhq/claude-plane/internal/server/store"
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
	connMgr *connmgr.ConnectionManager
	store   storeIface
	logger  *slog.Logger

	mu            sync.RWMutex
	sessionToStep map[string]*stepTracking

	// sharedSessions tracks active shared sessions keyed by {runID, sessionKey}.
	// Sessions remain alive until the run completes or is cancelled.
	sharedSessionsMu sync.Mutex
	sharedSessions   map[sessionKeyEntry]*sharedSessionEntry
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
	args := parseArgs(runStep.ArgsSnapshot)

	// Inject --dangerously-skip-permissions if snapshot says so.
	// NOTE: A nil SkipPermissionsSnapshot currently means "skip permissions" by default for jobs,
	// so we treat nil the same as a non-zero (truthy) value here.
	// TODO(kodrun#prefs-in-snapshot): In a future iteration, resolve a nil value against the user's
	// preferences at snapshot time instead of defaulting here in the executor.
	if runStep.SkipPermissionsSnapshot == nil || *runStep.SkipPermissionsSnapshot != 0 {
		args = stripFlag(args, "--dangerously-skip-permissions")
		args = append([]string{"--dangerously-skip-permissions"}, args...)
	}

	// Inject --model if set in snapshot, stripping any existing --model from user-supplied args.
	if runStep.ModelSnapshot != "" {
		args = stripFlagWithValue(args, "--model")
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
	args := parseArgs(resolvedArgsJSON)

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

	go e.monitorSessionExit(monitorCtx, sessionID, runStep.MachineIDSnapshot)
}

// monitorSessionExit polls the session status until it reaches a terminal state,
// the context is cancelled, or the timeout elapses.
func (e *SessionStepExecutor) monitorSessionExit(
	ctx context.Context,
	sessionID string,
	machineID string,
) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	deadline := time.Now().Add(defaultTimeout)
	notFoundCount := 0

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("session monitor context cancelled, killing session",
				"session_id", sessionID,
				"machine_id", machineID,
			)
			e.sendKill(machineID, sessionID)
			e.completeStep(sessionID, failureExitCode)
			return

		case <-ticker.C:
			if time.Now().After(deadline) {
				e.logger.Warn("session timed out, sending kill",
					"session_id", sessionID,
					"machine_id", machineID,
					"timeout", defaultTimeout,
				)
				e.sendKill(machineID, sessionID)
				e.completeStep(sessionID, timeoutExitCode)
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
						e.completeStep(sessionID, failureExitCode)
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
				e.completeStep(sessionID, 0)
				return

			case store.StatusFailed, store.StatusTerminated:
				e.logger.Info("session ended with failure",
					"session_id", sessionID,
					"status", sess.Status,
				)
				e.completeStep(sessionID, failureExitCode)
				return
			}
			// Non-terminal status (created, starting, running): continue polling.
		}
	}
}

// completeStep looks up tracking info for the session, calls the onComplete
// callback, and removes the tracking entry.
func (e *SessionStepExecutor) completeStep(sessionID string, exitCode int) {
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
	args := parseArgs(runStep.ArgsSnapshot)

	if runStep.SkipPermissionsSnapshot == nil || *runStep.SkipPermissionsSnapshot != 0 {
		args = stripFlag(args, "--dangerously-skip-permissions")
		args = append([]string{"--dangerously-skip-permissions"}, args...)
	}

	if runStep.ModelSnapshot != "" {
		args = stripFlagWithValue(args, "--model")
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
		e.completeSharedStep(sessionID, failureExitCode)
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
func (e *SessionStepExecutor) completeSharedStep(sessionID string, exitCode int) {
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

	// Do NOT call tracking.cancel() — the session stays alive for the next step.
	tracking.onComplete(tracking.stepID, exitCode)
}

// OnStepIdle handles StepIdleEvent from the agent, indicating a shared session
// step has completed (the CLI returned to its prompt). This is called by the
// gRPC server when it receives a StepIdleEvent.
func (e *SessionStepExecutor) OnStepIdle(sessionID string) {
	e.logger.Info("step idle event received", "session_id", sessionID)
	e.completeSharedStep(sessionID, 0)
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

// sendInput sends input data to a session via the agent.
func (e *SessionStepExecutor) sendInput(machineID, sessionID string, data []byte) {
	agent := e.connMgr.GetAgent(machineID)
	if agent == nil {
		e.logger.Warn("no agent to send input",
			"machine_id", machineID,
			"session_id", sessionID,
		)
		return
	}

	cmd := &pb.ServerCommand{
		Command: &pb.ServerCommand_InputData{
			InputData: &pb.InputDataCmd{
				SessionId: sessionID,
				Data:      data,
			},
		},
	}
	if err := agent.SendCommand(cmd); err != nil {
		e.logger.Warn("failed to send input to agent",
			"machine_id", machineID,
			"session_id", sessionID,
			"error", err,
		)
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

// stripFlag removes all occurrences of a standalone flag from args.
func stripFlag(args []string, flag string) []string {
	result := make([]string, 0, len(args))
	for _, a := range args {
		if a != flag {
			result = append(result, a)
		}
	}
	return result
}

// stripFlagWithValue removes a flag and its following value from args (e.g., --model opus).
func stripFlagWithValue(args []string, flag string) []string {
	result := make([]string, 0, len(args))
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == flag {
			skip = true
			continue
		}
		result = append(result, a)
	}
	return result
}

// parseArgs parses a JSON-encoded array string into a []string.
// Returns an empty slice on empty input or any parse error.
func parseArgs(argsSnapshot string) []string {
	trimmed := strings.TrimSpace(argsSnapshot)
	if trimmed == "" {
		return []string{}
	}
	var args []string
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return []string{}
	}
	return args
}
