package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/claudeplane/claude-plane/internal/server/store"
)

// StepExecutor launches a step and calls onComplete when it finishes.
type StepExecutor interface {
	ExecuteStep(ctx context.Context, runStep store.RunStep, onComplete func(stepID string, exitCode int))
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
	steps         map[string]*store.RunStep  // step_id -> run step
	dependents    map[string][]string        // step_id -> step_ids that depend on it
	inDegree      map[string]int             // step_id -> remaining dependency count
	executor      StepExecutor
	store         store.JobStoreIface
	onRunComplete func(runID string, status string)
	completed     int
	total         int
	failed        bool
}

// NewDAGRunner creates a DAGRunner for a specific run.
func NewDAGRunner(runID string, runSteps []store.RunStep, deps []store.StepDependency, executor StepExecutor, jobStore store.JobStoreIface, onComplete func(string, string)) *DAGRunner {
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

	return &DAGRunner{
		runID:         runID,
		steps:         steps,
		dependents:    dependents,
		inDegree:      inDegree,
		executor:      executor,
		store:         jobStore,
		onRunComplete: onComplete,
		total:         len(runSteps),
	}
}

// Start launches all root steps (in-degree 0) concurrently.
// Uses a background context since runs outlive the triggering HTTP request.
func (d *DAGRunner) Start(_ context.Context) {
	d.mu.Lock()
	d.ctx, d.cancel = context.WithCancel(context.Background())

	var toLaunch []store.RunStep
	for stepID, deg := range d.inDegree {
		if deg == 0 {
			rs := d.steps[stepID]
			if rs.Status == "pending" {
				rs.Status = "running"
				d.updateRunStepInDB(rs.RunStepID, "running", "", 0)
				toLaunch = append(toLaunch, *rs)
			}
		}
	}
	ctx := d.ctx
	d.mu.Unlock()

	for _, step := range toLaunch {
		d.executor.ExecuteStep(ctx, step, d.OnStepCompleted)
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

	if exitCode != 0 {
		rs.Status = "failed"
		ec := exitCode
		rs.ExitCode = &ec
		d.updateRunStepInDB(rs.RunStepID, "failed", "", exitCode)

		if rs.OnFailure == "fail_run" {
			d.failed = true
			// Mark remaining pending steps as skipped
			for _, s := range d.steps {
				if s.Status == "pending" {
					s.Status = "skipped"
					d.updateRunStepInDB(s.RunStepID, "skipped", "", 0)
				}
			}
			d.cancel()
			if d.onRunComplete != nil {
				cb := d.onRunComplete
				runID := d.runID
				runComplete = func() { cb(runID, "failed") }
			}
			d.mu.Unlock()
			if runComplete != nil {
				runComplete()
			}
			return
		}
	} else {
		rs.Status = "completed"
		ec := 0
		rs.ExitCode = &ec
		d.updateRunStepInDB(rs.RunStepID, "completed", "", 0)
	}

	d.completed++

	// If the step failed (but on_failure != "fail_run"), skip its dependents
	// rather than launching them — they can't succeed without their dependency.
	stepFailed := exitCode != 0

	// Check if all steps are done
	if d.completed == d.total {
		status := "completed"
		if d.failed || stepFailed {
			status = "failed"
		}
		if d.onRunComplete != nil {
			cb := d.onRunComplete
			runID := d.runID
			s := status
			runComplete = func() { cb(runID, s) }
		}
		d.mu.Unlock()
		if runComplete != nil {
			runComplete()
		}
		return
	}

	// Collect ready dependents
	ctx := d.ctx
	for _, depID := range d.dependents[stepID] {
		d.inDegree[depID]--
		if d.inDegree[depID] == 0 {
			depRS := d.steps[depID]
			if depRS != nil && depRS.Status == "pending" {
				if stepFailed {
					// Skip dependents of failed steps
					depRS.Status = "skipped"
					d.updateRunStepInDB(depRS.RunStepID, "skipped", "", 0)
					d.completed++
				} else {
					depRS.Status = "running"
					d.updateRunStepInDB(depRS.RunStepID, "running", "", 0)
					toLaunch = append(toLaunch, *depRS)
				}
			}
		}
	}
	d.mu.Unlock()

	// Launch outside the lock to prevent deadlocks
	for _, step := range toLaunch {
		d.executor.ExecuteStep(ctx, step, d.OnStepCompleted)
	}
}

// Cancel stops the DAGRunner context. Safe to call before Start().
func (d *DAGRunner) Cancel() {
	if d.cancel != nil {
		d.cancel()
	}
}

// updateRunStepInDB persists run step status changes. No-op if store is nil (unit tests).
func (d *DAGRunner) updateRunStepInDB(runStepID, status, sessionID string, exitCode int) {
	if d.store == nil {
		return
	}
	_ = d.store.UpdateRunStepStatus(context.Background(), runStepID, status, sessionID, exitCode)
}
