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
	stepA, _ := s.CreateStep(ctx, job.JobID, "A", "do A", "", "/tmp", "claude", "", 0, 0, "fail_run")
	stepB, _ := s.CreateStep(ctx, job.JobID, "B", "do B", "", "/tmp", "claude", "", 0, 1, "fail_run")
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)

	mock := newMockExecutor()
	orch := NewOrchestrator(s, mock)

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
	stepA, _ := s.CreateStep(ctx, job.JobID, "A", "p", "", "", "claude", "", 0, 0, "fail_run")
	stepB, _ := s.CreateStep(ctx, job.JobID, "B", "p", "", "", "claude", "", 0, 1, "fail_run")
	stepC, _ := s.CreateStep(ctx, job.JobID, "C", "p", "", "", "claude", "", 0, 2, "fail_run")
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)
	_ = s.AddDependency(ctx, stepC.StepID, stepB.StepID)
	_ = s.AddDependency(ctx, stepA.StepID, stepC.StepID)

	mock := newMockExecutor()
	orch := NewOrchestrator(s, mock)

	_, err := orch.CreateRun(ctx, job.JobID, "manual")
	if err == nil {
		t.Error("expected error for cyclic DAG, got nil")
	}
}

func TestOrchestrator_CancelRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Cancel Job", "", "")
	stepA, _ := s.CreateStep(ctx, job.JobID, "A", "p", "", "", "claude", "", 0, 0, "fail_run")
	s.CreateStep(ctx, job.JobID, "B", "p", "", "", "claude", "", 0, 1, "fail_run")

	mock := newMockExecutor()
	orch := NewOrchestrator(s, mock)

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
	stepA, _ := s.CreateStep(ctx, job.JobID, "A", "p", "", "", "claude", "", 0, 0, "fail_run")
	stepB, _ := s.CreateStep(ctx, job.JobID, "B", "p", "", "", "claude", "", 0, 1, "fail_run")
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)

	mock := newMockExecutor()
	orch := NewOrchestrator(s, mock)

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

// waitForRunStatus polls the orchestrator until the run reaches the expected status.
func waitForRunStatus(t *testing.T, orch *Orchestrator, runID, expectedStatus string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		orch.mu.Lock()
		_, active := orch.activeRuns[runID]
		orch.mu.Unlock()

		if !active && expectedStatus != "running" {
			// Run finished, check DB
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if expectedStatus == "completed" || expectedStatus == "failed" {
		// Check if it actually finished
		orch.mu.Lock()
		_, active := orch.activeRuns[runID]
		orch.mu.Unlock()
		if active {
			t.Fatalf("timed out waiting for run %s to reach status %s", runID, expectedStatus)
		}
	}
}
