package orchestrator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/claudeplane/claude-plane/internal/server/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOrchestrator_CreateRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Set up job with steps and dependencies
	job, _ := s.CreateJob(ctx, "Test Job", "desc", "")
	stepA, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "do A", MachineID: "", WorkingDir: "/tmp", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "do B", MachineID: "", WorkingDir: "/tmp", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	run, err := orch.CreateRun(ctx, job.JobID, "manual")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.RunID == "" {
		t.Fatal("expected non-empty RunID")
	}
	if run.Status != "pending" {
		t.Errorf("Status = %q, want %q", run.Status, "pending")
	}

	// Step A should be executing (root step)
	mock.waitForStep(stepA.StepID)

	// Complete A, then B should start
	mock.completeStep(stepA.StepID, 0)
	mock.waitForStep(stepB.StepID)
	mock.completeStep(stepB.StepID, 0)

	// Wait for run to complete
	waitForRunStatus(t, orch, run.RunID, "completed", 5*time.Second)
}

func TestOrchestrator_CreateRun_CycleRejected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Cycle Job", "", "")
	stepA, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})
	stepC, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "C", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 2, OnFailure: "fail_run"})
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)
	_ = s.AddDependency(ctx, stepC.StepID, stepB.StepID)
	_ = s.AddDependency(ctx, stepA.StepID, stepC.StepID)

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	_, err := orch.CreateRun(ctx, job.JobID, "manual")
	if err == nil {
		t.Error("expected error for cyclic DAG, got nil")
	}
}

func TestOrchestrator_CancelRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Cancel Job", "", "")
	stepA, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	run, _ := orch.CreateRun(ctx, job.JobID, "manual")
	mock.waitForStep(stepA.StepID)

	// Cancel the run
	err := orch.CancelRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}

	// Verify run status is cancelled
	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	if detail.Run.Status != "cancelled" {
		t.Errorf("Run status = %q, want %q", detail.Run.Status, "cancelled")
	}
}

func TestOrchestrator_RetryStep(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Retry Job", "", "")
	stepA, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	run, _ := orch.CreateRun(ctx, job.JobID, "manual")

	// A starts and fails
	mock.waitForStep(stepA.StepID)
	mock.completeStep(stepA.StepID, 1) // fail

	waitForRunStatus(t, orch, run.RunID, "failed", 5*time.Second)

	// Retry step A
	err := orch.RetryStep(ctx, run.RunID, stepA.StepID)
	if err != nil {
		t.Fatalf("RetryStep: %v", err)
	}

	// A should re-execute
	mock.waitForStep(stepA.StepID)
	mock.completeStep(stepA.StepID, 0) // succeed this time

	// B should now execute
	mock.waitForStep(stepB.StepID)
	mock.completeStep(stepB.StepID, 0)

	waitForRunStatus(t, orch, run.RunID, "completed", 5*time.Second)
}

func TestOrchestrator_OnStepCompleted_ExternalAPI(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Test 1: Calling OnStepCompleted on a nonexistent run is a safe no-op
	t.Run("nonexistent_run", func(t *testing.T) {
		mock := newMockExecutor()
		orch := NewOrchestrator(context.Background(), s, mock)
		// Should not panic
		orch.OnStepCompleted("nonexistent-run-id", "nonexistent-step-id", 0)
	})

	// Test 2: OnStepCompleted routes to the correct active DAGRunner
	t.Run("routes_to_active_runner", func(t *testing.T) {
		job, _ := s.CreateJob(ctx, "OnStepCompleted Job", "", "")
		stepA, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "do A", MachineID: "", WorkingDir: "/tmp", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
		stepB, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "do B", MachineID: "", WorkingDir: "/tmp", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})
		_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)

		mock := newMockExecutor()
		orch := NewOrchestrator(context.Background(), s, mock)

		run, err := orch.CreateRun(ctx, job.JobID, "manual")
		if err != nil {
			t.Fatalf("CreateRun: %v", err)
		}

		// Verify the run is tracked as active
		orch.mu.Lock()
		_, active := orch.activeRuns[run.RunID]
		orch.mu.Unlock()
		if !active {
			t.Fatal("expected run to be active after CreateRun")
		}

		// Complete steps through the standard mock path
		mock.waitForStep(stepA.StepID)
		mock.completeStep(stepA.StepID, 0)
		mock.waitForStep(stepB.StepID)
		mock.completeStep(stepB.StepID, 0)

		waitForRunStatus(t, orch, run.RunID, "completed", 5*time.Second)

		// After completion, the run should no longer be active
		orch.mu.Lock()
		_, active = orch.activeRuns[run.RunID]
		orch.mu.Unlock()
		if active {
			t.Error("expected run to be removed from activeRuns after completion")
		}

		// Calling OnStepCompleted on a completed run is a safe no-op
		orch.OnStepCompleted(run.RunID, stepA.StepID, 0)
	})
}

// waitForRunStatus polls the orchestrator until the run reaches the expected status,
// then verifies the persisted status in the database matches.
func waitForRunStatus(t *testing.T, orch *Orchestrator, runID, expectedStatus string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		orch.mu.Lock()
		_, active := orch.activeRuns[runID]
		orch.mu.Unlock()

		if !active && expectedStatus != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Verify the persisted status matches
	detail, err := orch.store.GetRunWithSteps(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRunWithSteps: %v", err)
	}
	if detail.Run.Status != expectedStatus {
		t.Fatalf("run %s: persisted status = %q, want %q", runID, detail.Run.Status, expectedStatus)
	}
}
