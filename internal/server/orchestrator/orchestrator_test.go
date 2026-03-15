package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/store"
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
	job, _ := s.CreateJob(ctx, store.CreateJobParams{Name: "Test Job", Description: "desc"})
	stepA, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "do A", MachineID: "", WorkingDir: "/tmp", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "do B", MachineID: "", WorkingDir: "/tmp", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	run, err := orch.CreateRun(ctx, job.JobID, "manual", nil)
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

	job, _ := s.CreateJob(ctx, store.CreateJobParams{Name: "Cycle Job"})
	stepA, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})
	stepC, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "C", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 2, OnFailure: "fail_run"})
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)
	_ = s.AddDependency(ctx, stepC.StepID, stepB.StepID)
	_ = s.AddDependency(ctx, stepA.StepID, stepC.StepID)

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	_, err := orch.CreateRun(ctx, job.JobID, "manual", nil)
	if err == nil {
		t.Error("expected error for cyclic DAG, got nil")
	}
}

func TestOrchestrator_CancelRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, store.CreateJobParams{Name: "Cancel Job"})
	stepA, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	run, _ := orch.CreateRun(ctx, job.JobID, "manual", nil)
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

	job, _ := s.CreateJob(ctx, store.CreateJobParams{Name: "Retry Job"})
	stepA, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "p", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	run, _ := orch.CreateRun(ctx, job.JobID, "manual", nil)

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
		job, _ := s.CreateJob(ctx, store.CreateJobParams{Name: "OnStepCompleted Job"})
		stepA, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "do A", MachineID: "", WorkingDir: "/tmp", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
		stepB, _ := s.CreateStep(ctx, store.CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "do B", MachineID: "", WorkingDir: "/tmp", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})
		_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)

		mock := newMockExecutor()
		orch := NewOrchestrator(context.Background(), s, mock)

		run, err := orch.CreateRun(ctx, job.JobID, "manual", nil)
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

func TestOrchestrator_MaxConcurrentRuns(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a job with max_concurrent_runs = 1
	job, _ := s.CreateJob(ctx, store.CreateJobParams{Name: "Max1 Job", MaxConcurrentRuns: 1})
	s.CreateStep(ctx, store.CreateStepParams{
		JobID: job.JobID, Name: "A", Prompt: "p", Command: "claude",
		OnFailure: "fail_run",
	})

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	// First run should succeed
	run1, err := orch.CreateRun(ctx, job.JobID, "manual", nil)
	if err != nil {
		t.Fatalf("first CreateRun: %v", err)
	}

	// Second run should be rejected
	_, err = orch.CreateRun(ctx, job.JobID, "manual", nil)
	if !errors.Is(err, ErrMaxConcurrentRuns) {
		t.Fatalf("expected ErrMaxConcurrentRuns, got: %v", err)
	}

	// Complete the first run
	detail, _ := s.GetRunWithSteps(ctx, run1.RunID)
	for _, rs := range detail.RunSteps {
		mock.waitForStep(rs.StepID)
		mock.completeStep(rs.StepID, 0)
	}
	waitForRunStatus(t, orch, run1.RunID, "completed", 5*time.Second)

	// Now a new run should be allowed
	_, err = orch.CreateRun(ctx, job.JobID, "manual", nil)
	if err != nil {
		t.Fatalf("third CreateRun (after completion): %v", err)
	}
}

func TestOrchestrator_MaxConcurrentRuns_DefaultOne(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Default max_concurrent_runs should be 1
	job, _ := s.CreateJob(ctx, store.CreateJobParams{Name: "Default Job"})
	s.CreateStep(ctx, store.CreateStepParams{
		JobID: job.JobID, Name: "A", Prompt: "p", Command: "claude",
		OnFailure: "fail_run",
	})

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	_, err := orch.CreateRun(ctx, job.JobID, "manual", nil)
	if err != nil {
		t.Fatalf("first CreateRun: %v", err)
	}

	_, err = orch.CreateRun(ctx, job.JobID, "manual", nil)
	if !errors.Is(err, ErrMaxConcurrentRuns) {
		t.Fatalf("expected ErrMaxConcurrentRuns for default max=1, got: %v", err)
	}
}

func TestOrchestrator_JobTimeout(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create job with 1-second timeout
	job, _ := s.CreateJob(ctx, store.CreateJobParams{Name: "Timeout Job", TimeoutSeconds: 1})
	s.CreateStep(ctx, store.CreateStepParams{
		JobID: job.JobID, Name: "A", Prompt: "p", Command: "claude",
		OnFailure: "fail_run",
	})

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	run, err := orch.CreateRun(ctx, job.JobID, "manual", nil)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Step starts but we don't complete it — timeout should cancel
	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	for _, rs := range detail.RunSteps {
		mock.waitForStep(rs.StepID)
	}

	// Wait for timeout to cancel the run
	waitForRunStatus(t, orch, run.RunID, "cancelled", 5*time.Second)
}

func TestOrchestrator_ParameterResolution(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create job with default parameters
	job, _ := s.CreateJob(ctx, store.CreateJobParams{
		Name:       "Param Job",
		Parameters: `{"ENV":"staging","VERSION":"1.0"}`,
	})
	s.CreateStep(ctx, store.CreateStepParams{
		JobID: job.JobID, Name: "A", Prompt: "p", Command: "claude",
		OnFailure: "fail_run",
	})

	mock := newMockExecutor()
	orch := NewOrchestrator(context.Background(), s, mock)

	// Override ENV but keep VERSION default
	overrides := map[string]string{"ENV": "production"}
	run, err := orch.CreateRun(ctx, job.JobID, "manual", overrides)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Check that parameters were resolved and stored
	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	if detail.Run.Parameters == "" {
		t.Fatal("expected parameters to be stored on run")
	}

	var params map[string]string
	if err := json.Unmarshal([]byte(detail.Run.Parameters), &params); err != nil {
		t.Fatalf("unmarshal run params: %v", err)
	}
	if params["ENV"] != "production" {
		t.Errorf("ENV = %q, want %q", params["ENV"], "production")
	}
	if params["VERSION"] != "1.0" {
		t.Errorf("VERSION = %q, want %q", params["VERSION"], "1.0")
	}
}
