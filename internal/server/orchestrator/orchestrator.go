package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// ErrMaxConcurrentRuns is returned when a job has reached its maximum number
// of concurrent active runs.
var ErrMaxConcurrentRuns = errors.New("max concurrent runs reached for this job")

// ErrInvalidRunState is returned when a run is not in a terminal state
// (failed or cancelled) and cannot be repaired.
var ErrInvalidRunState = errors.New("run is not in a terminal state")

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

// countActiveRunsForJob counts how many active runners belong to a given job.
// Must be called with o.mu held.
func (o *Orchestrator) countActiveRunsForJob(jobID string) int {
	count := 0
	for _, runner := range o.activeRuns {
		if runner.jobID == jobID {
			count++
		}
	}
	return count
}

// CreateRun validates the DAG, creates a run in the DB, snapshots steps
// into run_steps, builds a DAGRunner, and starts execution.
// params provides runtime parameter overrides (merged with job defaults).
// An optional triggerDetail string provides extra context about what triggered the run.
func (o *Orchestrator) CreateRun(ctx context.Context, jobID string, triggerType string, params map[string]string, triggerDetail ...string) (*store.Run, error) {
	// Get job to check max concurrent runs and parameters
	job, err := o.store.GetJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}

	// Check max concurrent runs
	o.mu.Lock()
	activeCount := o.countActiveRunsForJob(jobID)
	if job.Job.MaxConcurrentRuns > 0 && activeCount >= job.Job.MaxConcurrentRuns {
		o.mu.Unlock()
		return nil, ErrMaxConcurrentRuns
	}
	o.mu.Unlock()

	// Get steps and dependencies
	steps, deps, err := o.store.GetStepsWithDeps(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get steps: %w", err)
	}

	// Validate DAG
	if err := ValidateDAG(steps, deps); err != nil {
		return nil, fmt.Errorf("validate DAG: %w", err)
	}

	// Resolve parameters: merge job defaults with runtime overrides
	resolvedParams := resolveParameters(job.Job.Parameters, params)
	paramsJSON, err := json.Marshal(resolvedParams)
	if err != nil {
		return nil, fmt.Errorf("marshal parameters: %w", err)
	}

	// Create run in DB
	detail0 := ""
	if len(triggerDetail) > 0 {
		detail0 = triggerDetail[0]
	}
	run, err := o.store.CreateRun(ctx, store.CreateRunParams{
		JobID:         jobID,
		TriggerType:   triggerType,
		TriggerDetail: detail0,
		Parameters:    string(paramsJSON),
	})
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

	// Build and start DAGRunner
	capturedJobID := jobID
	capturedTriggerType := triggerType
	onComplete := func(runID string, status string) {
		// Clean up shared sessions when the run finishes.
		if cleanup, ok := o.executor.(interface{ CleanupRunSessions(string) }); ok {
			cleanup.CleanupRunSessions(runID)
		}
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

	// Build step name map from steps for template resolution
	stepNameMap := make(map[string]string, len(steps))
	for _, s := range steps {
		stepNameMap[s.StepID] = s.Name
	}

	jobMeta := JobMeta{
		Name:        job.Job.Name,
		RunID:       run.RunID,
		TriggerType: triggerType,
		StartTime:   run.CreatedAt.Format(time.RFC3339),
	}
	// Populate RunNumber by counting existing runs for this job.
	if existingRuns, err := o.store.ListRuns(ctx, jobID); err == nil {
		jobMeta.RunNumber = len(existingRuns)
	}

	runner := NewDAGRunner(run.RunID, jobID, detail.RunSteps, deps, o.executor, o.store, o.publisher, onComplete, resolvedParams, jobMeta, stepNameMap)

	o.mu.Lock()
	o.activeRuns[run.RunID] = runner
	o.mu.Unlock()

	// Mark run as running
	if err := o.store.UpdateRunStatus(ctx, run.RunID, store.StatusRunning); err != nil {
		slog.Warn("failed to mark run as running", "error", err, "run_id", run.RunID)
	}
	o.publishEvent(ctx, event.NewRunEvent(event.TypeRunStarted, run.RunID, jobID, store.StatusRunning, triggerType))

	runner.Start(o.rootCtx)

	// Job-level timeout
	if job.Job.TimeoutSeconds > 0 {
		timeout := time.Duration(job.Job.TimeoutSeconds) * time.Second
		go func() {
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case <-timer.C:
				slog.Warn("job run timed out", "run_id", run.RunID, "timeout", timeout)
				o.CancelRun(o.rootCtx, run.RunID)
			case <-runner.Done():
				// Run completed before timeout
			}
		}()
	}

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
			if err := o.store.UpdateRunStepAttempt(ctx, rs.RunStepID, 1); err != nil {
				return fmt.Errorf("reset attempt for step %s: %w", rs.StepID, err)
			}
		}
	}

	return o.rebuildAndStartRun(ctx, runID)
}

// RepairRun resets all failed/skipped steps in a terminal run to pending,
// optionally merges parameter overrides (only existing keys), clears task
// values for reset steps, and rebuilds the DAG.
func (o *Orchestrator) RepairRun(ctx context.Context, runID string, paramOverrides map[string]string) error {
	detail, err := o.store.GetRunWithSteps(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}
	if detail.Run.Status != store.StatusFailed && detail.Run.Status != store.StatusCancelled {
		return ErrInvalidRunState
	}

	// Merge parameter overrides (only existing keys)
	if len(paramOverrides) > 0 && detail.Run.Parameters != "" {
		existingParams := make(map[string]string)
		if err := json.Unmarshal([]byte(detail.Run.Parameters), &existingParams); err != nil {
			return fmt.Errorf("parse run parameters: %w", err)
		}
		for k, v := range paramOverrides {
			if _, exists := existingParams[k]; exists {
				existingParams[k] = v
			}
		}
		paramsJSON, err := json.Marshal(existingParams)
		if err != nil {
			return fmt.Errorf("marshal parameters: %w", err)
		}
		if err := o.store.UpdateRunParameters(ctx, runID, string(paramsJSON)); err != nil {
			return fmt.Errorf("update run parameters: %w", err)
		}
	}

	// Reset failed/skipped/cancelled steps to pending, clear their task values and attempt counters
	for _, rs := range detail.RunSteps {
		if rs.Status == store.StatusFailed || rs.Status == store.StatusSkipped || rs.Status == store.StatusCancelled {
			if err := o.store.UpdateRunStepStatus(ctx, rs.RunStepID, store.StatusPending, "", 0); err != nil {
				return fmt.Errorf("reset step %s: %w", rs.StepID, err)
			}
			if err := o.store.UpdateRunStepAttempt(ctx, rs.RunStepID, 1); err != nil {
				return fmt.Errorf("reset attempt for step %s: %w", rs.StepID, err)
			}
			if err := o.store.DeleteTaskValuesForStep(ctx, rs.RunStepID); err != nil {
				return fmt.Errorf("clear task values for step %s: %w", rs.StepID, err)
			}
		}
	}

	return o.rebuildAndStartRun(ctx, runID)
}

// rebuildAndStartRun re-reads the run state, builds a new DAGRunner
// pre-configured with completed steps, and starts execution. Shared by
// RetryStep and RepairRun.
func (o *Orchestrator) rebuildAndStartRun(ctx context.Context, runID string) error {
	// Update run status back to running
	if err := o.store.UpdateRunStatus(ctx, runID, store.StatusRunning); err != nil {
		return fmt.Errorf("update run status: %w", err)
	}

	// Re-read run state
	detail, err := o.store.GetRunWithSteps(ctx, runID)
	if err != nil {
		return fmt.Errorf("re-read run: %w", err)
	}

	// Get the job's dependencies
	_, deps, err := o.store.GetStepsWithDeps(ctx, detail.Run.JobID)
	if err != nil {
		return fmt.Errorf("get deps: %w", err)
	}

	// Build new DAGRunner from current DB state
	rebuildJobID := detail.Run.JobID
	rebuildTriggerType := detail.Run.TriggerType
	onComplete := func(runID string, status string) {
		// Clean up shared sessions when the run finishes.
		if cleanup, ok := o.executor.(interface{ CleanupRunSessions(string) }); ok {
			cleanup.CleanupRunSessions(runID)
		}
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
		o.publishEvent(o.rootCtx, event.NewRunEvent(evType, runID, rebuildJobID, status, rebuildTriggerType))
	}

	// Build step name map
	stepNames := make(map[string]string)
	for _, rs := range detail.RunSteps {
		stepNames[rs.StepID] = rs.StepID
	}
	if steps, _, err := o.store.GetStepsWithDeps(ctx, detail.Run.JobID); err == nil {
		for _, s := range steps {
			stepNames[s.StepID] = s.Name
		}
	}

	// Parse run parameters so retried/repaired runs retain template resolution
	var runParams map[string]string
	if detail.Run.Parameters != "" {
		if err := json.Unmarshal([]byte(detail.Run.Parameters), &runParams); err != nil {
			slog.Warn("failed to parse run parameters for rebuild", "error", err, "run_id", runID)
		}
	}

	// Reconstruct job metadata from run data
	jobMeta := JobMeta{
		RunID:       runID,
		TriggerType: detail.Run.TriggerType,
		StartTime:   detail.Run.CreatedAt.Format(time.RFC3339),
	}
	if jobDetail, err := o.store.GetJob(ctx, detail.Run.JobID); err == nil {
		jobMeta.Name = jobDetail.Job.Name
	}
	// Populate RunNumber by counting existing runs for this job.
	if existingRuns, err := o.store.ListRuns(ctx, detail.Run.JobID); err == nil {
		for i, r := range existingRuns {
			if r.RunID == runID {
				jobMeta.RunNumber = len(existingRuns) - i
				break
			}
		}
	}

	runner := NewDAGRunner(runID, detail.Run.JobID, detail.RunSteps, deps, o.executor, o.store, o.publisher, onComplete, runParams, jobMeta, stepNames)

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

	// Clean up shared sessions belonging to this run.
	if cleanup, ok := o.executor.(interface{ CleanupRunSessions(string) }); ok {
		cleanup.CleanupRunSessions(runID)
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
	_, err := o.CreateRun(ctx, jobID, triggerType, nil)
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
