package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// StepExecutor launches a step and calls onComplete when it finishes.
type StepExecutor interface {
	ExecuteStep(ctx context.Context, runStep store.RunStep, resolveCtx *ResolveContext, onComplete func(stepID string, exitCode int))
}

// ValidateJobSteps checks step configuration for common errors.
// Returns a slice of errors (empty if all steps are valid).
func ValidateJobSteps(steps []store.Step) []error {
	var errs []error
	sessionKeyMachines := make(map[string]string)
	for _, s := range steps {
		if s.TaskType != "claude_session" && s.TaskType != "shell" {
			errs = append(errs, fmt.Errorf("step %q: task_type must be 'claude_session' or 'shell'", s.Name))
		}
		if s.TaskType == "shell" && s.SessionKey != "" {
			errs = append(errs, fmt.Errorf("step %q: shell tasks cannot share sessions", s.Name))
		}
		if s.TaskType == "shell" && s.Command == "" {
			errs = append(errs, fmt.Errorf("step %q: shell tasks require a command", s.Name))
		}
		if s.SessionKey != "" {
			if existing, ok := sessionKeyMachines[s.SessionKey]; ok {
				if existing != s.MachineID {
					errs = append(errs, fmt.Errorf("steps sharing session key %q must target the same machine", s.SessionKey))
				}
			} else {
				sessionKeyMachines[s.SessionKey] = s.MachineID
			}
		}
		if s.RunIf != "all_success" && s.RunIf != "all_done" {
			errs = append(errs, fmt.Errorf("step %q: run_if must be 'all_success' or 'all_done'", s.Name))
		}
		if s.MaxRetries < 0 || s.MaxRetries > 5 {
			errs = append(errs, fmt.Errorf("step %q: max_retries must be between 0 and 5", s.Name))
		}
		if s.RetryDelaySeconds < 0 || s.RetryDelaySeconds > 3600 {
			errs = append(errs, fmt.Errorf("step %q: retry_delay_seconds must be between 0 and 3600", s.Name))
		}
	}
	return errs
}

// ValidateDAG checks for cycles in step dependencies using Kahn's algorithm.
// Returns an error if a cycle is detected.
func ValidateDAG(steps []store.Step, deps []store.StepDependency) error {
	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	for _, s := range steps {
		inDegree[s.StepID] = 0
	}
	for _, d := range deps {
		adj[d.DependsOn] = append(adj[d.DependsOn], d.StepID)
		inDegree[d.StepID]++
	}

	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adj[node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if visited != len(steps) {
		return fmt.Errorf("cycle detected in job DAG")
	}
	return nil
}

// DAGRunner executes a job run by tracking step dependencies and launching
// ready steps as their dependencies complete. Thread-safe via mutex.
type DAGRunner struct {
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	runID         string
	jobID         string
	steps         map[string]*store.RunStep  // step_id -> run step
	stepNames     map[string]string          // step_id -> step name (for reference resolution)
	dependents    map[string][]string        // step_id -> step_ids that depend on it
	inDegree      map[string]int             // step_id -> remaining dependency count
	executor      StepExecutor
	store         store.JobStoreIface
	publisher     event.Publisher
	onRunComplete func(runID string, status string)
	completed     int
	total         int
	failed        bool
	done          chan struct{}              // closed when run completes

	// Template resolution state
	runParams   map[string]string
	jobMeta     JobMeta
	stepValues  map[string]map[string]string // stepName -> key -> value
	stepResults map[string]StepResult        // stepName -> result

	// Session key serialization
	activeSessionKeys map[string]bool   // session_key -> in-use
	deferredSteps     []string          // step_ids waiting for session key release

	// run_if tracking
	hasFailedUpstream map[string]bool   // stepID -> any upstream failed
}

// NewDAGRunner creates a DAGRunner for a specific run.
func NewDAGRunner(
	runID string,
	jobID string,
	runSteps []store.RunStep,
	deps []store.StepDependency,
	executor StepExecutor,
	jobStore store.JobStoreIface,
	publisher event.Publisher,
	onComplete func(string, string),
	runParams map[string]string,
	jobMeta JobMeta,
	stepNames map[string]string,
) *DAGRunner {
	steps := make(map[string]*store.RunStep, len(runSteps))
	dependents := make(map[string][]string)
	inDegree := make(map[string]int)

	for i := range runSteps {
		rs := runSteps[i]
		steps[rs.StepID] = &rs
		inDegree[rs.StepID] = 0
	}

	for _, d := range deps {
		dependents[d.DependsOn] = append(dependents[d.DependsOn], d.StepID)
		inDegree[d.StepID]++
	}

	if stepNames == nil {
		stepNames = make(map[string]string)
	}

	return &DAGRunner{
		runID:             runID,
		jobID:             jobID,
		steps:             steps,
		stepNames:         stepNames,
		dependents:        dependents,
		inDegree:          inDegree,
		executor:          executor,
		store:             jobStore,
		publisher:         publisher,
		onRunComplete:     onComplete,
		total:             len(runSteps),
		done:              make(chan struct{}),
		runParams:         runParams,
		jobMeta:           jobMeta,
		stepValues:        make(map[string]map[string]string),
		stepResults:       make(map[string]StepResult),
		activeSessionKeys: make(map[string]bool),
		hasFailedUpstream: make(map[string]bool),
	}
}

// Done returns a channel that is closed when the run completes (or is cancelled).
func (d *DAGRunner) Done() <-chan struct{} {
	return d.done
}

// SetStepValues records task values produced by a step, keyed by step name.
// Called externally (e.g., by the gRPC layer) after task value extraction.
func (d *DAGRunner) SetStepValues(stepName string, values map[string]string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stepValues[stepName] = values
}

// copyStepValues returns a snapshot of step values for safe use outside the lock.
func (d *DAGRunner) copyStepValues() map[string]map[string]string {
	cp := make(map[string]map[string]string, len(d.stepValues))
	for k, v := range d.stepValues {
		inner := make(map[string]string, len(v))
		for ik, iv := range v {
			inner[ik] = iv
		}
		cp[k] = inner
	}
	return cp
}

// copyStepResults returns a snapshot of step results for safe use outside the lock.
func (d *DAGRunner) copyStepResults() map[string]StepResult {
	cp := make(map[string]StepResult, len(d.stepResults))
	for k, v := range d.stepResults {
		cp[k] = v
	}
	return cp
}

// closeDone safely closes the done channel (idempotent).
func (d *DAGRunner) closeDone() {
	select {
	case <-d.done:
		// Already closed
	default:
		close(d.done)
	}
}

// Start launches all root steps (in-degree 0) concurrently.
// Uses parentCtx as the parent so that runs are tied to the server lifecycle.
func (d *DAGRunner) Start(parentCtx context.Context) {
	d.mu.Lock()
	d.ctx, d.cancel = context.WithCancel(parentCtx)

	var toLaunch []store.RunStep
	for stepID, deg := range d.inDegree {
		if deg == 0 {
			rs := d.steps[stepID]
			if rs.Status == store.StatusPending {
				rs.Status = store.StatusRunning
				d.updateRunStepInDB(rs.RunStepID, store.StatusRunning, "", 0)
				toLaunch = append(toLaunch, *rs)
			}
		}
	}
	ctx := d.ctx
	d.mu.Unlock()

	for _, step := range toLaunch {
		d.launchStep(ctx, step)
	}
}

// launchStep starts a step, applying any configured delay before execution.
// If DelaySecondsSnapshot > 0, the step waits in a goroutine before calling
// ExecuteStep, respecting context cancellation during the wait.
func (d *DAGRunner) launchStep(ctx context.Context, rs store.RunStep) {
	// Build ResolveContext snapshot under lock
	d.mu.Lock()
	resolveCtx := &ResolveContext{
		RunParams:   d.runParams,
		JobMeta:     d.jobMeta,
		StepValues:  d.copyStepValues(),
		StepResults: d.copyStepResults(),
	}
	d.mu.Unlock()

	delay := time.Duration(rs.DelaySecondsSnapshot) * time.Second
	if delay > 0 {
		go func() {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
				d.executor.ExecuteStep(ctx, rs, resolveCtx, d.OnStepCompleted)
			case <-ctx.Done():
				// Context was cancelled (run cancelled or server shutdown).
				// Do NOT call OnStepCompleted — CancelRun already handles marking
				// pending/running steps as cancelled in the DB. Calling OnStepCompleted
				// here would race with CancelRun's DB updates and could incorrectly
				// mark the step as completed/failed or trigger dependent launches.
				return
			}
		}()
	} else {
		go d.executor.ExecuteStep(ctx, rs, resolveCtx, d.OnStepCompleted)
	}
}

// OnStepCompleted is called when a step finishes execution.
// Thread-safe: uses mutex to serialize state mutations.
// Executor calls are made outside the lock to prevent deadlocks.
func (d *DAGRunner) OnStepCompleted(stepID string, exitCode int) {
	d.mu.Lock()

	rs, ok := d.steps[stepID]
	if !ok {
		d.mu.Unlock()
		return
	}

	var toLaunch []store.RunStep
	var runComplete func()

	// Capture for post-lock publishing.
	capturedRunStepID := rs.RunStepID
	capturedStepID := stepID

	// Record step result for template resolution
	stepName := d.stepNames[stepID]
	if stepName != "" {
		status := store.StatusCompleted
		if exitCode != 0 {
			status = store.StatusFailed
		}
		d.stepResults[stepName] = StepResult{Status: status, ExitCode: exitCode}
	}

	if exitCode != 0 {
		rs.Status = store.StatusFailed
		ec := exitCode
		rs.ExitCode = &ec
		d.failed = true
		d.updateRunStepInDB(rs.RunStepID, store.StatusFailed, "", exitCode)

		if rs.OnFailure == "fail_run" {
			// Mark remaining pending steps as skipped
			for _, s := range d.steps {
				if s.Status == store.StatusPending {
					s.Status = store.StatusSkipped
					d.updateRunStepInDB(s.RunStepID, store.StatusSkipped, "", 0)
				}
			}
			d.cancel()
			ctx := d.ctx
			if d.onRunComplete != nil {
				cb := d.onRunComplete
				runID := d.runID
				runComplete = func() { cb(runID, store.StatusFailed) }
			}
			d.closeDone()
			d.mu.Unlock()
			d.publishStepEvent(ctx, event.TypeJobRunStepFailed, capturedRunStepID, capturedStepID, store.StatusFailed)
			if runComplete != nil {
				runComplete()
			}
			return
		}
	} else {
		rs.Status = store.StatusCompleted
		ec := 0
		rs.ExitCode = &ec
		d.updateRunStepInDB(rs.RunStepID, store.StatusCompleted, "", 0)
	}

	d.completed++

	// If the step failed (but on_failure != "fail_run"), skip its dependents
	// rather than launching them — they can't succeed without their dependency.
	stepFailed := exitCode != 0

	// Collect ready dependents; propagate skips transitively.
	ctx := d.ctx
	d.processReadyDependents(stepID, stepFailed, &toLaunch)

	// Check if all steps are done (after processing dependents/skips)
	if d.completed == d.total {
		status := store.StatusCompleted
		if d.failed || stepFailed {
			status = store.StatusFailed
		}
		if d.onRunComplete != nil {
			cb := d.onRunComplete
			runID := d.runID
			s := status
			runComplete = func() { cb(runID, s) }
		}
		d.closeDone()
		d.mu.Unlock()
		if stepFailed {
			d.publishStepEvent(ctx, event.TypeJobRunStepFailed, capturedRunStepID, capturedStepID, store.StatusFailed)
		} else {
			d.publishStepEvent(ctx, event.TypeJobRunStepCompleted, capturedRunStepID, capturedStepID, store.StatusCompleted)
		}
		if runComplete != nil {
			runComplete()
		}
		return
	}
	d.mu.Unlock()

	if stepFailed {
		d.publishStepEvent(ctx, event.TypeJobRunStepFailed, capturedRunStepID, capturedStepID, store.StatusFailed)
	} else {
		d.publishStepEvent(ctx, event.TypeJobRunStepCompleted, capturedRunStepID, capturedStepID, store.StatusCompleted)
	}

	// Launch outside the lock to prevent deadlocks
	for _, step := range toLaunch {
		d.launchStep(ctx, step)
	}
}

// Cancel stops the DAGRunner context. Safe to call before Start() and
// concurrently with Start().
func (d *DAGRunner) Cancel() {
	d.mu.Lock()
	cancel := d.cancel
	d.closeDone()
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// updateRunStepInDB persists run step status changes. No-op if store is nil (unit tests).
func (d *DAGRunner) updateRunStepInDB(runStepID, status, sessionID string, exitCode int) {
	if d.store == nil {
		return
	}
	if err := d.store.UpdateRunStepStatus(d.ctx, runStepID, status, sessionID, exitCode); err != nil {
		slog.Warn("failed to update run step status", "error", err, "run_step_id", runStepID, "status", status)
	}
}

// processReadyDependents decrements in-degree for dependents of stepID.
// If stepFailed, skips ready dependents and propagates transitively.
// Uses an iterative work queue instead of recursion to avoid stack overflow on deep
// dependency chains. This is a deliberate design choice and should be preserved when
// modifying this function.
// Must be called with d.mu held.
func (d *DAGRunner) processReadyDependents(stepID string, stepFailed bool, toLaunch *[]store.RunStep) {
	type workItem struct {
		stepID string
		failed bool
	}

	queue := []workItem{{stepID: stepID, failed: stepFailed}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		for _, depID := range d.dependents[item.stepID] {
			// Track failed upstream for run_if decisions
			if item.failed {
				d.hasFailedUpstream[depID] = true
			}

			d.inDegree[depID]--
			if d.inDegree[depID] == 0 {
				depRS := d.steps[depID]
				if depRS != nil && depRS.Status == store.StatusPending {
					anyUpstreamFailed := d.hasFailedUpstream[depID]

					if anyUpstreamFailed && depRS.RunIfSnapshot != "all_done" {
						// all_success (default): skip when upstream failed
						depRS.Status = store.StatusSkipped
						d.updateRunStepInDB(depRS.RunStepID, store.StatusSkipped, "", 0)
						d.completed++
						queue = append(queue, workItem{stepID: depID, failed: true})
					} else {
						// Either no upstream failed, or run_if=all_done (launch regardless)
						depRS.Status = store.StatusRunning
						d.updateRunStepInDB(depRS.RunStepID, store.StatusRunning, "", 0)
						*toLaunch = append(*toLaunch, *depRS)
					}
				}
			}
		}
	}
}

// publishStepEvent publishes a step lifecycle event. No-op if publisher is nil.
// Must be called outside the mutex to avoid blocking.
func (d *DAGRunner) publishStepEvent(ctx context.Context, eventType, runStepID, stepID, status string) {
	if d.publisher == nil {
		return
	}
	if err := d.publisher.Publish(ctx, event.NewRunStepEvent(eventType, d.runID, runStepID, stepID, status)); err != nil {
		slog.Warn("failed to publish step event", "event_type", eventType, "run_step_id", runStepID, "error", err)
	}
}
