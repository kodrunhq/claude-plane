package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// mockExecutor is a test double for StepExecutor that allows test control
// over step execution and completion.
type mockExecutor struct {
	mu       sync.Mutex
	started  map[string]chan struct{} // step_id -> signal when step starts
	complete map[string]chan int      // step_id -> send exit code to complete
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{
		started:  make(map[string]chan struct{}),
		complete: make(map[string]chan int),
	}
}

func (m *mockExecutor) getOrCreateChans(stepID string) (chan struct{}, chan int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.started[stepID]; !ok {
		m.started[stepID] = make(chan struct{}, 1)
		m.complete[stepID] = make(chan int, 1)
	}
	return m.started[stepID], m.complete[stepID]
}

func (m *mockExecutor) ExecuteStep(ctx context.Context, runStep store.RunStep, resolveCtx *ResolveContext, onComplete func(stepID string, exitCode int)) {
	startCh, completeCh := m.getOrCreateChans(runStep.StepID)

	// Signal that step has started
	select {
	case startCh <- struct{}{}:
	default:
	}

	// Wait for test to signal completion
	go func() {
		select {
		case exitCode := <-completeCh:
			onComplete(runStep.StepID, exitCode)
		case <-ctx.Done():
			onComplete(runStep.StepID, -1)
		}
	}()
}

func (m *mockExecutor) waitForStep(stepID string) {
	startCh, _ := m.getOrCreateChans(stepID)
	select {
	case <-startCh:
	case <-time.After(5 * time.Second):
		panic("timeout waiting for step " + stepID + " to start")
	}
}

func (m *mockExecutor) completeStep(stepID string, exitCode int) {
	_, completeCh := m.getOrCreateChans(stepID)
	completeCh <- exitCode
	// Allow the completion callback to run
	time.Sleep(50 * time.Millisecond)
}

func (m *mockExecutor) isExecuting(stepID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.started[stepID]
	if !ok {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// testStep defines a step for test setup.
type testStep struct {
	id           string
	name         string
	onFailure    string
	delaySeconds int
	runIf        string
}

// testRunner wraps a DAGRunner for testing with completion tracking.
type testRunner struct {
	dag         *DAGRunner
	done        chan struct{}
	finalStatus string
}

func buildTestRunner(t *testing.T, runID string, steps []testStep, deps []store.StepDependency, executor *mockExecutor) *testRunner {
	t.Helper()

	runSteps := make([]store.RunStep, len(steps))
	stepNames := make(map[string]string, len(steps))
	for i, s := range steps {
		runIf := s.runIf
		if runIf == "" {
			runIf = "all_success"
		}
		name := s.name
		if name == "" {
			name = s.id
		}
		runSteps[i] = store.RunStep{
			RunStepID:            "rs-" + s.id,
			RunID:                runID,
			StepID:               s.id,
			Status:               "pending",
			OnFailure:            s.onFailure,
			DelaySecondsSnapshot: s.delaySeconds,
			RunIfSnapshot:        runIf,
		}
		stepNames[s.id] = name
	}

	tr := &testRunner{
		done: make(chan struct{}),
	}

	onComplete := func(runID string, status string) {
		tr.finalStatus = status
		close(tr.done)
	}

	tr.dag = NewDAGRunner(runID, "", runSteps, deps, executor, nil, nil, onComplete, nil, JobMeta{}, stepNames)
	return tr
}

func (tr *testRunner) Start(t *testing.T) {
	t.Helper()
	tr.dag.Start(context.Background())
}

func (tr *testRunner) StartWithContext(t *testing.T, ctx context.Context) {
	t.Helper()
	tr.dag.Start(ctx)
}

func (tr *testRunner) waitForDone() {
	select {
	case <-tr.done:
	case <-time.After(10 * time.Second):
		panic("timeout waiting for DAGRunner to complete")
	}
}
