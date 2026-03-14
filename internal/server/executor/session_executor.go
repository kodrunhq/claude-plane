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
	stepID     string
	onComplete func(stepID string, exitCode int)
	cancel     context.CancelFunc
}

// SessionStepExecutor creates real PTY-backed sessions on agents
// and monitors for completion by polling session status.
type SessionStepExecutor struct {
	connMgr *connmgr.ConnectionManager
	store   storeIface
	logger  *slog.Logger

	mu            sync.RWMutex
	sessionToStep map[string]*stepTracking
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
		connMgr:       connMgr,
		store:         st,
		logger:        logger,
		sessionToStep: make(map[string]*stepTracking),
	}
}

// ExecuteStep launches the step on the target agent and begins monitoring the
// resulting session. It is non-blocking; completion is signalled via onComplete.
func (e *SessionStepExecutor) ExecuteStep(
	ctx context.Context,
	runStep store.RunStep,
	onComplete func(stepID string, exitCode int),
) {
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

	// Inject --dangerously-skip-permissions if snapshot says so (nil defaults to true for jobs).
	if runStep.SkipPermissionsSnapshot == nil || *runStep.SkipPermissionsSnapshot != 0 {
		args = append([]string{"--dangerously-skip-permissions"}, args...)
	}

	// Inject --model if set in snapshot.
	if runStep.ModelSnapshot != "" {
		args = append(args, "--model", runStep.ModelSnapshot)
	}

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
				InitialPrompt: runStep.PromptSnapshot,
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
