package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

func TestValidateDAG_Linear(t *testing.T) {
	// A -> B -> C (valid)
	steps := []store.Step{
		{StepID: "a"}, {StepID: "b"}, {StepID: "c"},
	}
	deps := []store.StepDependency{
		{StepID: "b", DependsOn: "a"},
		{StepID: "c", DependsOn: "b"},
	}
	if err := ValidateDAG(steps, deps); err != nil {
		t.Errorf("ValidateDAG linear: unexpected error: %v", err)
	}
}

func TestValidateDAG_Diamond(t *testing.T) {
	// A -> B, A -> C, B -> D, C -> D (valid)
	steps := []store.Step{
		{StepID: "a"}, {StepID: "b"}, {StepID: "c"}, {StepID: "d"},
	}
	deps := []store.StepDependency{
		{StepID: "b", DependsOn: "a"},
		{StepID: "c", DependsOn: "a"},
		{StepID: "d", DependsOn: "b"},
		{StepID: "d", DependsOn: "c"},
	}
	if err := ValidateDAG(steps, deps); err != nil {
		t.Errorf("ValidateDAG diamond: unexpected error: %v", err)
	}
}

func TestValidateDAG_Parallel(t *testing.T) {
	// A, B, C (no dependencies - all parallel, valid)
	steps := []store.Step{
		{StepID: "a"}, {StepID: "b"}, {StepID: "c"},
	}
	if err := ValidateDAG(steps, nil); err != nil {
		t.Errorf("ValidateDAG parallel: unexpected error: %v", err)
	}
}

func TestValidateDAG_Cycle(t *testing.T) {
	// A -> B -> C -> A (cycle)
	steps := []store.Step{
		{StepID: "a"}, {StepID: "b"}, {StepID: "c"},
	}
	deps := []store.StepDependency{
		{StepID: "b", DependsOn: "a"},
		{StepID: "c", DependsOn: "b"},
		{StepID: "a", DependsOn: "c"},
	}
	if err := ValidateDAG(steps, deps); err == nil {
		t.Error("ValidateDAG cycle: expected error, got nil")
	}
}

func TestDAGRunner_LinearExecution(t *testing.T) {
	// A -> B -> C: steps execute in order
	mock := newMockExecutor()
	runner := buildTestRunner(t, "run-1",
		[]testStep{
			{id: "a", onFailure: "fail_run"},
			{id: "b", onFailure: "fail_run"},
			{id: "c", onFailure: "fail_run"},
		},
		[]store.StepDependency{
			{StepID: "b", DependsOn: "a"},
			{StepID: "c", DependsOn: "b"},
		},
		mock,
	)

	runner.Start(t)

	// Only A should be executing
	mock.waitForStep("a")
	if mock.isExecuting("b") || mock.isExecuting("c") {
		t.Error("B or C should not be executing before A completes")
	}

	mock.completeStep("a", 0)
	mock.waitForStep("b")
	if mock.isExecuting("c") {
		t.Error("C should not be executing before B completes")
	}

	mock.completeStep("b", 0)
	mock.waitForStep("c")

	mock.completeStep("c", 0)
	runner.waitForDone()

	if runner.finalStatus != "completed" {
		t.Errorf("final status = %q, want %q", runner.finalStatus, "completed")
	}
}

func TestDAGRunner_DiamondExecution(t *testing.T) {
	// A -> B, A -> C, B -> D, C -> D
	mock := newMockExecutor()
	runner := buildTestRunner(t, "run-2",
		[]testStep{
			{id: "a", onFailure: "fail_run"},
			{id: "b", onFailure: "fail_run"},
			{id: "c", onFailure: "fail_run"},
			{id: "d", onFailure: "fail_run"},
		},
		[]store.StepDependency{
			{StepID: "b", DependsOn: "a"},
			{StepID: "c", DependsOn: "a"},
			{StepID: "d", DependsOn: "b"},
			{StepID: "d", DependsOn: "c"},
		},
		mock,
	)

	runner.Start(t)

	// A should start first
	mock.waitForStep("a")
	mock.completeStep("a", 0)

	// B and C should both start
	mock.waitForStep("b")
	mock.waitForStep("c")

	// D should not start yet
	if mock.isExecuting("d") {
		t.Error("D should not execute until both B and C complete")
	}

	mock.completeStep("b", 0)
	mock.completeStep("c", 0)

	mock.waitForStep("d")
	mock.completeStep("d", 0)

	runner.waitForDone()

	if runner.finalStatus != "completed" {
		t.Errorf("final status = %q, want %q", runner.finalStatus, "completed")
	}
}

func TestDAGRunner_StepFailure(t *testing.T) {
	// A -> B -> C: B fails, C should be skipped, run should fail
	mock := newMockExecutor()
	runner := buildTestRunner(t, "run-3",
		[]testStep{
			{id: "a", onFailure: "fail_run"},
			{id: "b", onFailure: "fail_run"},
			{id: "c", onFailure: "fail_run"},
		},
		[]store.StepDependency{
			{StepID: "b", DependsOn: "a"},
			{StepID: "c", DependsOn: "b"},
		},
		mock,
	)

	runner.Start(t)

	mock.waitForStep("a")
	mock.completeStep("a", 0)

	mock.waitForStep("b")
	mock.completeStep("b", 1) // fail

	runner.waitForDone()

	if runner.finalStatus != store.StatusFailed {
		t.Errorf("final status = %q, want %q", runner.finalStatus, store.StatusFailed)
	}
}

func TestDAGRunner_ConcurrentCompletions(t *testing.T) {
	// Test thread safety: A -> C, B -> C
	// A and B are roots, complete concurrently
	mock := newMockExecutor()
	runner := buildTestRunner(t, "run-4",
		[]testStep{
			{id: "a", onFailure: "fail_run"},
			{id: "b", onFailure: "fail_run"},
			{id: "c", onFailure: "fail_run"},
		},
		[]store.StepDependency{
			{StepID: "c", DependsOn: "a"},
			{StepID: "c", DependsOn: "b"},
		},
		mock,
	)

	runner.Start(t)

	mock.waitForStep("a")
	mock.waitForStep("b")

	// Complete both concurrently
	done := make(chan struct{})
	go func() {
		mock.completeStep("a", 0)
		done <- struct{}{}
	}()
	go func() {
		mock.completeStep("b", 0)
		done <- struct{}{}
	}()
	<-done
	<-done

	mock.waitForStep("c")
	mock.completeStep("c", 0)

	runner.waitForDone()
	if runner.finalStatus != "completed" {
		t.Errorf("final status = %q, want %q", runner.finalStatus, "completed")
	}
}

func TestDAGRunner_DeepLinearChainSkipPropagation(t *testing.T) {
	// Build a deep linear chain: step-0 -> step-1 -> step-2 -> ... -> step-199
	// step-0 fails, all 199 dependents should be skipped transitively.
	// With a recursive implementation this would risk a stack overflow.
	const chainLen = 200

	steps := make([]testStep, chainLen)
	for i := 0; i < chainLen; i++ {
		steps[i] = testStep{
			id:        fmt.Sprintf("step-%d", i),
			onFailure: "continue",
		}
	}

	deps := make([]store.StepDependency, chainLen-1)
	for i := 1; i < chainLen; i++ {
		deps[i-1] = store.StepDependency{
			StepID:    fmt.Sprintf("step-%d", i),
			DependsOn: fmt.Sprintf("step-%d", i-1),
		}
	}

	mock := newMockExecutor()
	runner := buildTestRunner(t, "run-deep", steps, deps, mock)

	runner.Start(t)

	// Only step-0 should start (it's the root)
	mock.waitForStep("step-0")

	// Fail step-0 — all dependents should be skipped transitively
	mock.completeStep("step-0", 1)

	runner.waitForDone()

	if runner.finalStatus != store.StatusFailed {
		t.Errorf("final status = %q, want %q", runner.finalStatus, store.StatusFailed)
	}

	// Verify all downstream steps were skipped
	for i := 1; i < chainLen; i++ {
		stepID := fmt.Sprintf("step-%d", i)
		rs := runner.dag.steps[stepID]
		if rs.Status != store.StatusSkipped {
			t.Errorf("step %s status = %q, want %q", stepID, rs.Status, store.StatusSkipped)
		}
	}
}

func TestDAGRunner_CancelBeforeStart(t *testing.T) {
	runner := NewDAGRunner(
		"run-1",
		[]store.RunStep{{RunStepID: "rs-1", StepID: "s-1", Status: store.StatusPending}},
		nil,
		nil,
		nil,
		nil,
		func(runID, status string) {},
	)

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Cancel() panicked: %v", r)
		}
	}()
	runner.Cancel()
}

func TestDAGRunner_StepDelay(t *testing.T) {
	// A single step with delay_seconds_snapshot=1 should wait ~1s before executing.
	mock := newMockExecutor()
	runner := buildTestRunner(t, "run-delay",
		[]testStep{
			{id: "a", onFailure: "fail_run", delaySeconds: 1},
		},
		nil,
		mock,
	)

	startTime := time.Now()
	runner.Start(t)

	mock.waitForStep("a")
	elapsed := time.Since(startTime)

	if elapsed < 900*time.Millisecond {
		t.Errorf("step started after %v, expected at least ~1s delay", elapsed)
	}

	mock.completeStep("a", 0)
	runner.waitForDone()

	if runner.finalStatus != "completed" {
		t.Errorf("final status = %q, want %q", runner.finalStatus, "completed")
	}
}

func TestDAGRunner_DiamondDependency(t *testing.T) {
	// A -> B, A -> C, B+C -> D: all succeed, D runs after both B and C.
	mock := newMockExecutor()
	runner := buildTestRunner(t, "run-diamond-dep",
		[]testStep{
			{id: "a", onFailure: "fail_run"},
			{id: "b", onFailure: "fail_run"},
			{id: "c", onFailure: "fail_run"},
			{id: "d", onFailure: "fail_run"},
		},
		[]store.StepDependency{
			{StepID: "b", DependsOn: "a"},
			{StepID: "c", DependsOn: "a"},
			{StepID: "d", DependsOn: "b"},
			{StepID: "d", DependsOn: "c"},
		},
		mock,
	)

	runner.Start(t)

	mock.waitForStep("a")
	mock.completeStep("a", 0)

	// B and C should both be launched
	mock.waitForStep("b")
	mock.waitForStep("c")

	// D must not start yet
	if mock.isExecuting("d") {
		t.Error("D should not execute until both B and C complete")
	}

	// Complete B first — D should still wait for C
	mock.completeStep("b", 0)
	time.Sleep(50 * time.Millisecond)
	if mock.isExecuting("d") {
		t.Error("D should not execute until C completes")
	}

	mock.completeStep("c", 0)
	mock.waitForStep("d")
	mock.completeStep("d", 0)

	runner.waitForDone()

	if runner.finalStatus != "completed" {
		t.Errorf("final status = %q, want %q", runner.finalStatus, "completed")
	}
}

func TestDAGRunner_FailurePropagation_Continue(t *testing.T) {
	// A fails with on_failure=continue, dependent B is skipped, independent C still runs.
	// Topology: A -> B, C (independent root)
	mock := newMockExecutor()
	runner := buildTestRunner(t, "run-continue",
		[]testStep{
			{id: "a", onFailure: "continue"},
			{id: "b", onFailure: "continue"},
			{id: "c", onFailure: "continue"},
		},
		[]store.StepDependency{
			{StepID: "b", DependsOn: "a"},
		},
		mock,
	)

	runner.Start(t)

	// A and C are roots; both should start
	mock.waitForStep("a")
	mock.waitForStep("c")

	// Fail A — B should be skipped, C should keep running
	mock.completeStep("a", 1)
	time.Sleep(100 * time.Millisecond)

	// B should be skipped (never started)
	if mock.isExecuting("b") {
		t.Error("B should be skipped, not executing")
	}

	// Complete C
	mock.completeStep("c", 0)
	runner.waitForDone()

	if runner.finalStatus != store.StatusFailed {
		t.Errorf("final status = %q, want %q", runner.finalStatus, store.StatusFailed)
	}

	// Check B status after run completes (no concurrent writers)
	rs := runner.dag.steps["b"]
	if rs.Status != store.StatusSkipped {
		t.Errorf("step b status = %q, want %q", rs.Status, store.StatusSkipped)
	}
}

func TestDAGRunner_SkipCascade(t *testing.T) {
	// A -> B -> C, A fails (continue), B skipped, C also skipped.
	mock := newMockExecutor()
	runner := buildTestRunner(t, "run-skip-cascade",
		[]testStep{
			{id: "a", onFailure: "continue"},
			{id: "b", onFailure: "continue"},
			{id: "c", onFailure: "continue"},
		},
		[]store.StepDependency{
			{StepID: "b", DependsOn: "a"},
			{StepID: "c", DependsOn: "b"},
		},
		mock,
	)

	runner.Start(t)

	mock.waitForStep("a")
	mock.completeStep("a", 1)

	runner.waitForDone()

	if runner.finalStatus != store.StatusFailed {
		t.Errorf("final status = %q, want %q", runner.finalStatus, store.StatusFailed)
	}

	// Both B and C should be skipped
	for _, stepID := range []string{"b", "c"} {
		rs := runner.dag.steps[stepID]
		if rs.Status != store.StatusSkipped {
			t.Errorf("step %s status = %q, want %q", stepID, rs.Status, store.StatusSkipped)
		}
	}
}

func TestDAGRunner_DelayedStepCancellation(t *testing.T) {
	// A delayed step that is cancelled during its wait should NOT call
	// OnStepCompleted (CancelRun handles DB cleanup), and its dependent
	// should never be launched.
	mock := newMockExecutor()
	runner := buildTestRunner(t, "run-delay-cancel",
		[]testStep{
			{id: "a", onFailure: "fail_run", delaySeconds: 60}, // long delay
			{id: "b", onFailure: "fail_run"},                   // depends on a
		},
		[]store.StepDependency{
			{StepID: "b", DependsOn: "a"},
		},
		mock,
	)

	ctx, cancel := context.WithCancel(context.Background())
	runner.StartWithContext(t, ctx)

	// A is a root step with a 60s delay — executor should NOT have been called yet
	time.Sleep(100 * time.Millisecond)
	if mock.isExecuting("a") {
		t.Error("step A should not have started executing during delay")
	}

	// Cancel the run
	cancel()

	// Give goroutine time to process ctx.Done()
	time.Sleep(100 * time.Millisecond)

	// B should never have been launched
	if mock.isExecuting("b") {
		t.Error("step B should not have been launched after cancellation")
	}

	// Step A's in-memory status should still be running (CancelRun handles DB transition).
	// The key assertion: no OnStepCompleted was called, so no dependents triggered.
	rs := runner.dag.steps["a"]
	if rs.Status == store.StatusCompleted {
		t.Error("step A should NOT be marked completed after cancellation")
	}
}
