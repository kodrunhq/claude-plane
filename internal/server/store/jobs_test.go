package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestStoreForJobs(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestJobStore_CreateAndGetJob(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, err := s.CreateJob(ctx, "Test Job", "A test job", "")
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if job.JobID == "" {
		t.Fatal("expected non-empty JobID")
	}
	if job.Name != "Test Job" {
		t.Errorf("Name = %q, want %q", job.Name, "Test Job")
	}
	if job.Description != "A test job" {
		t.Errorf("Description = %q, want %q", job.Description, "A test job")
	}

	detail, err := s.GetJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if detail.Job.JobID != job.JobID {
		t.Errorf("GetJob JobID = %q, want %q", detail.Job.JobID, job.JobID)
	}
	if detail.Job.Name != "Test Job" {
		t.Errorf("GetJob Name = %q, want %q", detail.Job.Name, "Test Job")
	}
}

func TestJobStore_ListJobs(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_, err := s.CreateJob(ctx, "Job A", "", "")
	if err != nil {
		t.Fatalf("CreateJob A: %v", err)
	}
	_, err = s.CreateJob(ctx, "Job B", "", "")
	if err != nil {
		t.Fatalf("CreateJob B: %v", err)
	}

	jobs, err := s.ListJobs(ctx)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("ListJobs count = %d, want 2", len(jobs))
	}
}

func TestUpdateJob(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, err := s.CreateJob(ctx, "Original Name", "Original desc", "")
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	updated, err := s.UpdateJob(ctx, job.JobID, "New Name", "New desc")
	if err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("Name = %q, want %q", updated.Name, "New Name")
	}
	if updated.Description != "New desc" {
		t.Errorf("Description = %q, want %q", updated.Description, "New desc")
	}

	// Verify via GetJob
	detail, err := s.GetJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if detail.Job.Name != "New Name" {
		t.Errorf("GetJob Name = %q, want %q", detail.Job.Name, "New Name")
	}
	if detail.Job.Description != "New desc" {
		t.Errorf("GetJob Description = %q, want %q", detail.Job.Description, "New desc")
	}
}

func TestUpdateJob_NotFound(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_, err := s.UpdateJob(ctx, "nonexistent-job-id", "Name", "Desc")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestJobStore_DeleteJob(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "To Delete", "", "")
	if err := s.DeleteJob(ctx, job.JobID); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	_, err := s.GetJob(ctx, job.JobID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestJobStore_StepCRUD(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Job", "", "")

	step, err := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "Step 1", Prompt: "Do something", MachineID: "", WorkingDir: "/tmp", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	if err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	if step.StepID == "" {
		t.Fatal("expected non-empty StepID")
	}
	if step.Name != "Step 1" {
		t.Errorf("Name = %q, want %q", step.Name, "Step 1")
	}

	// Update step
	err = s.UpdateStep(ctx, UpdateStepParams{StepID: step.StepID, Name: "Step 1 Updated", Prompt: "New prompt", MachineID: "", WorkingDir: "/home", Command: "claude", Args: "--flag", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	if err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}

	detail, _ := s.GetJob(ctx, job.JobID)
	if len(detail.Steps) != 1 {
		t.Fatalf("Steps count = %d, want 1", len(detail.Steps))
	}
	if detail.Steps[0].Name != "Step 1 Updated" {
		t.Errorf("Updated Name = %q, want %q", detail.Steps[0].Name, "Step 1 Updated")
	}

	// Delete step
	err = s.DeleteStep(ctx, step.StepID)
	if err != nil {
		t.Fatalf("DeleteStep: %v", err)
	}
	detail, _ = s.GetJob(ctx, job.JobID)
	if len(detail.Steps) != 0 {
		t.Errorf("Steps count after delete = %d, want 0", len(detail.Steps))
	}
}

func TestJobStore_Dependencies(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Job", "", "")
	stepA, _ := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "prompt", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "prompt", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})

	// Add dependency: B depends on A
	err := s.AddDependency(ctx, stepB.StepID, stepA.StepID)
	if err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	// Reject self-reference
	err = s.AddDependency(ctx, stepA.StepID, stepA.StepID)
	if err == nil {
		t.Error("expected error for self-reference dependency")
	}

	// Get steps with deps
	steps, deps, err := s.GetStepsWithDeps(ctx, job.JobID)
	if err != nil {
		t.Fatalf("GetStepsWithDeps: %v", err)
	}
	if len(steps) != 2 {
		t.Errorf("Steps count = %d, want 2", len(steps))
	}
	if len(deps) != 1 {
		t.Errorf("Deps count = %d, want 1", len(deps))
	}
	if deps[0].StepID != stepB.StepID || deps[0].DependsOn != stepA.StepID {
		t.Errorf("Dep = (%s, %s), want (%s, %s)", deps[0].StepID, deps[0].DependsOn, stepB.StepID, stepA.StepID)
	}

	// Remove dependency
	err = s.RemoveDependency(ctx, stepB.StepID, stepA.StepID)
	if err != nil {
		t.Fatalf("RemoveDependency: %v", err)
	}
	_, deps, _ = s.GetStepsWithDeps(ctx, job.JobID)
	if len(deps) != 0 {
		t.Errorf("Deps after remove = %d, want 0", len(deps))
	}
}

func TestJobStore_RunWithSnapshots(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	// Create machine for FK constraint
	if err := s.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}

	job, _ := s.CreateJob(ctx, "Job", "", "")
	stepA, _ := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "do A", MachineID: "machine-a", WorkingDir: "/work", Command: "claude", Args: "--verbose", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "do B", MachineID: "machine-a", WorkingDir: "/work2", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})

	// Create run
	run, err := s.CreateRun(ctx, job.JobID, "manual")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.RunID == "" {
		t.Fatal("expected non-empty RunID")
	}
	if run.Status != StatusPending {
		t.Errorf("Status = %q, want %q", run.Status, StatusPending)
	}

	// Insert run steps with snapshots
	steps := []Step{*stepA, *stepB}
	err = s.InsertRunSteps(ctx, run.RunID, steps)
	if err != nil {
		t.Fatalf("InsertRunSteps: %v", err)
	}

	// Get run with steps
	detail, err := s.GetRunWithSteps(ctx, run.RunID)
	if err != nil {
		t.Fatalf("GetRunWithSteps: %v", err)
	}
	if len(detail.RunSteps) != 2 {
		t.Fatalf("RunSteps count = %d, want 2", len(detail.RunSteps))
	}

	// Verify snapshot fields
	for _, rs := range detail.RunSteps {
		if rs.PromptSnapshot == "" {
			t.Error("PromptSnapshot should not be empty")
		}
		if rs.MachineIDSnapshot == "" {
			t.Error("MachineIDSnapshot should not be empty")
		}
		if rs.Status != StatusPending {
			t.Errorf("RunStep Status = %q, want %q", rs.Status, StatusPending)
		}
	}

	// Update run step status
	err = s.UpdateRunStepStatus(ctx, detail.RunSteps[0].RunStepID, StatusRunning, "", 0)
	if err != nil {
		t.Fatalf("UpdateRunStepStatus: %v", err)
	}
	err = s.UpdateRunStepStatus(ctx, detail.RunSteps[0].RunStepID, StatusCompleted, "sess-1", 0)
	if err != nil {
		t.Fatalf("UpdateRunStepStatus (completed): %v", err)
	}

	detail, _ = s.GetRunWithSteps(ctx, run.RunID)
	if detail.RunSteps[0].Status != StatusCompleted {
		t.Errorf("RunStep status = %q, want %q", detail.RunSteps[0].Status, StatusCompleted)
	}

	// Update run status
	err = s.UpdateRunStatus(ctx, run.RunID, StatusCompleted)
	if err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}
	detail, _ = s.GetRunWithSteps(ctx, run.RunID)
	if detail.Run.Status != StatusCompleted {
		t.Errorf("Run status = %q, want %q", detail.Run.Status, StatusCompleted)
	}
}

func TestJobStore_ListRuns(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Job", "", "")
	_, _ = s.CreateRun(ctx, job.JobID, "manual")
	_, _ = s.CreateRun(ctx, job.JobID, "manual")

	runs, err := s.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("ListRuns count = %d, want 2", len(runs))
	}
}

func TestJobStore_ListAllRuns(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	jobA, _ := s.CreateJob(ctx, "Job A", "", "")
	jobB, _ := s.CreateJob(ctx, "Job B", "", "")
	_, _ = s.CreateRun(ctx, jobA.JobID, "manual")
	_, _ = s.CreateRun(ctx, jobA.JobID, "manual")
	_, _ = s.CreateRun(ctx, jobB.JobID, "scheduled")

	runs, err := s.ListAllRuns(ctx, ListRunsOptions{})
	if err != nil {
		t.Fatalf("ListAllRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("ListAllRuns count = %d, want 3", len(runs))
	}
}

func TestJobStore_ListAllRuns_FilterByStatus(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Job", "", "")
	run1, _ := s.CreateRun(ctx, job.JobID, "manual")
	_, _ = s.CreateRun(ctx, job.JobID, "manual")

	_ = s.UpdateRunStatus(ctx, run1.RunID, StatusCompleted)

	runs, err := s.ListAllRuns(ctx, ListRunsOptions{Status: StatusCompleted})
	if err != nil {
		t.Fatalf("ListAllRuns filter by status: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("ListAllRuns completed count = %d, want 1", len(runs))
	}
	if runs[0].Status != StatusCompleted {
		t.Errorf("Status = %q, want %q", runs[0].Status, StatusCompleted)
	}
}

func TestJobStore_ListAllRuns_FilterByJobID(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	jobA, _ := s.CreateJob(ctx, "Job A", "", "")
	jobB, _ := s.CreateJob(ctx, "Job B", "", "")
	_, _ = s.CreateRun(ctx, jobA.JobID, "manual")
	_, _ = s.CreateRun(ctx, jobA.JobID, "manual")
	_, _ = s.CreateRun(ctx, jobB.JobID, "manual")

	runs, err := s.ListAllRuns(ctx, ListRunsOptions{JobID: jobA.JobID})
	if err != nil {
		t.Fatalf("ListAllRuns filter by job: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("ListAllRuns job A count = %d, want 2", len(runs))
	}
	for _, r := range runs {
		if r.JobID != jobA.JobID {
			t.Errorf("unexpected JobID %q, want %q", r.JobID, jobA.JobID)
		}
	}
}

func TestJobStore_ListAllRuns_Pagination(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Job", "", "")
	for i := 0; i < 5; i++ {
		_, _ = s.CreateRun(ctx, job.JobID, "manual")
	}

	page1, err := s.ListAllRuns(ctx, ListRunsOptions{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("ListAllRuns page 1: %v", err)
	}
	if len(page1) != 3 {
		t.Errorf("page 1 count = %d, want 3", len(page1))
	}

	page2, err := s.ListAllRuns(ctx, ListRunsOptions{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("ListAllRuns page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page 2 count = %d, want 2", len(page2))
	}

	// Ensure pages don't overlap
	seen := map[string]bool{}
	for _, r := range page1 {
		seen[r.RunID] = true
	}
	for _, r := range page2 {
		if seen[r.RunID] {
			t.Errorf("run %q appeared in both pages", r.RunID)
		}
	}
}

func TestJobStore_ListAllRuns_IncludesJobName(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Named Job", "", "")
	_, _ = s.CreateRun(ctx, job.JobID, "manual")

	runs, err := s.ListAllRuns(ctx, ListRunsOptions{})
	if err != nil {
		t.Fatalf("ListAllRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListAllRuns count = %d, want 1", len(runs))
	}
	if runs[0].JobName != "Named Job" {
		t.Errorf("JobName = %q, want %q", runs[0].JobName, "Named Job")
	}
}

func TestJobStore_ListAllRuns_DefaultLimit(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Job", "", "")
	// Insert 55 runs — more than the default limit of 50.
	for i := 0; i < 55; i++ {
		_, _ = s.CreateRun(ctx, job.JobID, "manual")
	}

	runs, err := s.ListAllRuns(ctx, ListRunsOptions{})
	if err != nil {
		t.Fatalf("ListAllRuns default limit: %v", err)
	}
	if len(runs) != 50 {
		t.Errorf("default limit count = %d, want 50", len(runs))
	}
}

func TestJobStore_ListAllRuns_EmptyResult(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	runs, err := s.ListAllRuns(ctx, ListRunsOptions{})
	if err != nil {
		t.Fatalf("ListAllRuns empty: %v", err)
	}
	if runs == nil {
		t.Error("expected non-nil slice for empty result")
	}
	if len(runs) != 0 {
		t.Errorf("empty count = %d, want 0", len(runs))
	}
}

func TestJobStore_DeleteJobCascades(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, _ := s.CreateJob(ctx, "Job", "", "")
	stepA, _ := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "prompt", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "prompt", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)

	run, _ := s.CreateRun(ctx, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*stepA, *stepB})

	// Delete job -- should cascade transactionally
	err := s.DeleteJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}

	// Verify job is gone
	_, err = s.GetJob(ctx, job.JobID)
	if err == nil {
		t.Error("expected error fetching deleted job")
	}

	// Verify steps are gone
	var count int
	s.reader.QueryRow("SELECT COUNT(*) FROM steps WHERE job_id = ?", job.JobID).Scan(&count)
	if count != 0 {
		t.Errorf("steps count = %d, want 0", count)
	}

	// Verify runs are gone
	s.reader.QueryRow("SELECT COUNT(*) FROM runs WHERE job_id = ?", job.JobID).Scan(&count)
	if count != 0 {
		t.Errorf("runs count = %d, want 0", count)
	}

	// Verify run steps are gone
	s.reader.QueryRow("SELECT COUNT(*) FROM run_steps WHERE run_id = ?", run.RunID).Scan(&count)
	if count != 0 {
		t.Errorf("run_steps count = %d, want 0", count)
	}
}
