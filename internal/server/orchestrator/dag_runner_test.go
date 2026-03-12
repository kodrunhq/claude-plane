package orchestrator

import (
	"testing"

	"github.com/claudeplane/claude-plane/internal/server/store"
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

	if runner.finalStatus != "failed" {
		t.Errorf("final status = %q, want %q", runner.finalStatus, "failed")
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

func TestDAGRunner_CancelBeforeStart(t *testing.T) {
	runner := NewDAGRunner(
		"run-1",
		[]store.RunStep{{RunStepID: "rs-1", StepID: "s-1", Status: "pending"}},
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
