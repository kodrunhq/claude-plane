package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// Orchestrator manages active DAGRunners for job runs.
type Orchestrator struct {
	mu         sync.Mutex
	rootCtx    context.Context
	activeRuns map[string]*DAGRunner
	store      store.JobStoreIface
	executor   StepExecutor
	publisher  event.Publisher
}

// SetPublisher sets the event publisher used to emit run lifecycle events.
func (o *Orchestrator) SetPublisher(p event.Publisher) {
	o.publisher = p
}

// publishEvent publishes an event if a publisher is configured. Errors are
// logged at Warn level and do not affect the caller.
func (o *Orchestrator) publishEvent(ctx context.Context, e event.Event) {
	if o.publisher != nil {
		if err := o.publisher.Publish(ctx, e); err != nil {
			slog.Warn("failed to publish event", "event_type", e.Type, "error", err)
		}
	}
}

// NewOrchestrator creates an Orchestrator. The provided context is used as the
// parent for all DAGRunner contexts, tying their lifetime to the server.
func NewOrchestrator(ctx context.Context, s store.JobStoreIface, executor StepExecutor) *Orchestrator {
	return &Orchestrator{
		rootCtx:    ctx,
		activeRuns: make(map[string]*DAGRunner),
		store:      s,
		executor:   executor,
	}
}

// CreateRun validates the DAG, creates a run in the DB, snapshots steps
// into run_steps, builds a DAGRunner, and starts execution.
// An optional triggerDetail string provides extra context about what triggered the run.
func (o *Orchestrator) CreateRun(ctx context.Context, jobID string, triggerType string, triggerDetail ...string) (*store.Run, error) {
	// Get steps and dependencies
	steps, deps, err := o.store.GetStepsWithDeps(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get steps: %w", err)
	}

	// Validate DAG
	if err := ValidateDAG(steps, deps); err != nil {
		return nil, fmt.Errorf("validate DAG: %w", err)
	}

	// Create run in DB
	run, err := o.store.CreateRun(ctx, jobID, triggerType, triggerDetail...)
	if err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}
	o.publishEvent(ctx, event.NewRunEvent(event.TypeRunCreated, run.RunID, jobID, store.StatusPending, triggerType))

	// Snapshot steps into run_steps
	if err := o.store.InsertRunSteps(ctx, run.RunID, steps); err != nil {
		return nil, fmt.Errorf("insert run steps: %w", err)
	}

	// Read back the run steps to get generated IDs
	detail, err := o.store.GetRunWithSteps(ctx, run.RunID)
	if err != nil {
		return nil, fmt.Errorf("get run steps: %w", err)
	}

	// OnFailure is now populated from on_failure_snapshot by GetRunWithSteps.

	// Build and start DAGRunner
	capturedJobID := jobID
	capturedTriggerType := triggerType
	onComplete := func(runID string, status string) {
		if err := o.store.UpdateRunStatus(o.rootCtx, runID, status); err != nil {
			slog.Warn("failed to update run status on completion", "error", err, "run_id", runID, "status", status)
		}
		o.mu.Lock()
		delete(o.activeRuns, runID)
		o.mu.Unlock()
		evType := event.TypeRunCompleted
		if status == store.StatusFailed {
			evType = event.TypeRunFailed
		}
		o.publishEvent(o.rootCtx, event.NewRunEvent(evType, runID, capturedJobID, status, capturedTriggerType))
	}

	runner := NewDAGRunner(run.RunID, detail.RunSteps, deps, o.executor, o.store, o.publisher, onComplete)

	o.mu.Lock()
	o.activeRuns[run.RunID] = runner
	o.mu.Unlock()

	// Mark run as running
	if err := o.store.UpdateRunStatus(ctx, run.RunID, store.StatusRunning); err != nil {
		slog.Warn("failed to mark run as running", "error", err, "run_id", run.RunID)
	}
	o.publishEvent(ctx, event.NewRunEvent(event.TypeRunStarted, run.RunID, jobID, store.StatusRunning, triggerType))

	runner.Start(o.rootCtx)

	return run, nil
}

// RetryStep resets the target run_step and downstream skipped/cancelled steps
// to pending, rebuilds the DAGRunner from DB state, and re-launches.
func (o *Orchestrator) RetryStep(ctx context.Context, runID string, stepID string) error {
	// Get current run state
	detail, err := o.store.GetRunWithSteps(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}

	// Get the job's dependencies
	_, deps, err := o.store.GetStepsWithDeps(ctx, detail.Run.JobID)
	if err != nil {
		return fmt.Errorf("get deps: %w", err)
	}

	// Build dependency graph to find downstream steps
	dependents := make(map[string][]string)
	for _, d := range deps {
		dependents[d.DependsOn] = append(dependents[d.DependsOn], d.StepID)
	}

	// Find all downstream steps from the target step
	toReset := map[string]bool{stepID: true}
	queue := []string{stepID}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		for _, dep := range dependents[curr] {
			if !toReset[dep] {
				toReset[dep] = true
				queue = append(queue, dep)
			}
		}
	}

	// Reset target and downstream skipped/cancelled/failed steps to pending
	for _, rs := range detail.RunSteps {
		if toReset[rs.StepID] && (rs.Status == store.StatusFailed || rs.Status == store.StatusSkipped || rs.Status == store.StatusCancelled) {
			if err := o.store.UpdateRunStepStatus(ctx, rs.RunStepID, store.StatusPending, "", 0); err != nil {
				return fmt.Errorf("reset step %s: %w", rs.StepID, err)
			}
		}
	}

	// Update run status back to running
	if err := o.store.UpdateRunStatus(ctx, runID, store.StatusRunning); err != nil {
		return fmt.Errorf("update run status: %w", err)
	}

	// Re-read run state
	detail, err = o.store.GetRunWithSteps(ctx, runID)
	if err != nil {
		return fmt.Errorf("re-read run: %w", err)
	}

	// OnFailure is now populated from on_failure_snapshot by GetRunWithSteps.

	// Build new DAGRunner from current DB state
	retryJobID := detail.Run.JobID
	onComplete := func(runID string, status string) {
		if err := o.store.UpdateRunStatus(o.rootCtx, runID, status); err != nil {
			slog.Warn("failed to update run status on completion", "error", err, "run_id", runID, "status", status)
		}
		o.mu.Lock()
		delete(o.activeRuns, runID)
		o.mu.Unlock()
		evType := event.TypeRunCompleted
		if status == store.StatusFailed {
			evType = event.TypeRunFailed
		}
		o.publishEvent(o.rootCtx, event.NewRunEvent(evType, runID, retryJobID, status, "manual"))
	}

	runner := NewDAGRunner(runID, detail.RunSteps, deps, o.executor, o.store, o.publisher, onComplete)

	// Pre-process completed steps: count them and pre-decrement in-degrees
	// of their dependents so pending steps with completed upstream deps can launch.
	runner.mu.Lock()
	for _, rs := range detail.RunSteps {
		if rs.Status == store.StatusCompleted {
			runner.completed++
			// Decrement in-degree for all steps that depend on this completed step
			for _, depID := range runner.dependents[rs.StepID] {
				runner.inDegree[depID]--
			}
		}
	}
	runner.mu.Unlock()

	// Cancel any existing active runner for this run to prevent conflicts
	o.mu.Lock()
	if existing, ok := o.activeRuns[runID]; ok {
		existing.Cancel()
	}
	o.activeRuns[runID] = runner
	o.mu.Unlock()

	runner.Start(o.rootCtx)

	return nil
}

// CancelRun stops executing steps and marks remaining as cancelled.
func (o *Orchestrator) CancelRun(ctx context.Context, runID string) error {
	o.mu.Lock()
	runner, ok := o.activeRuns[runID]
	delete(o.activeRuns, runID)
	o.mu.Unlock()

	if ok {
		runner.Cancel()
	}

	// Mark pending/running run steps as cancelled
	detail, err := o.store.GetRunWithSteps(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}

	for _, rs := range detail.RunSteps {
		if rs.Status == store.StatusPending || rs.Status == store.StatusRunning {
			if err := o.store.UpdateRunStepStatus(ctx, rs.RunStepID, store.StatusCancelled, "", 0); err != nil {
				return fmt.Errorf("cancel step %s: %w", rs.StepID, err)
			}
		}
	}

	// Mark run as cancelled
	if err := o.store.UpdateRunStatus(ctx, runID, store.StatusCancelled); err != nil {
		return err
	}
	cancelDetail, err := o.store.GetRunWithSteps(ctx, runID)
	if err == nil {
		o.publishEvent(ctx, event.NewRunEvent(event.TypeRunCancelled, runID, cancelDetail.Run.JobID, store.StatusCancelled, cancelDetail.Run.TriggerType))
	}
	return nil
}

// CreateRunErr is a thin wrapper around CreateRun that discards the returned
// *store.Run, satisfying the event.OrchestratorIface interface.
func (o *Orchestrator) CreateRunErr(ctx context.Context, jobID string, triggerType string) error {
	_, err := o.CreateRun(ctx, jobID, triggerType)
	return err
}

// OnStepCompleted routes step completion to the correct DAGRunner.
func (o *Orchestrator) OnStepCompleted(runID string, stepID string, exitCode int) {
	o.mu.Lock()
	runner, ok := o.activeRuns[runID]
	o.mu.Unlock()

	if ok {
		runner.OnStepCompleted(stepID, exitCode)
	}
}
