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

// testCreateJob is a test helper that creates a job with minimal params.
func testCreateJob(t *testing.T, s *Store, name, description, userID string) *Job {
	t.Helper()
	job, err := s.CreateJob(context.Background(), CreateJobParams{
		Name:        name,
		Description: description,
		UserID:      userID,
	})
	if err != nil {
		t.Fatalf("CreateJob(%q): %v", name, err)
	}
	return job
}

// testCreateRun is a test helper that creates a run with minimal params.
func testCreateRun(t *testing.T, s *Store, jobID, triggerType string) *Run {
	t.Helper()
	run, err := s.CreateRun(context.Background(), CreateRunParams{
		JobID:       jobID,
		TriggerType: triggerType,
	})
	if err != nil {
		t.Fatalf("CreateRun(%q, %q): %v", jobID, triggerType, err)
	}
	return run
}

func TestJobStore_CreateAndGetJob(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, err := s.CreateJob(ctx, CreateJobParams{Name: "Test Job", Description: "A test job"})
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
	if job.MaxConcurrentRuns != 1 {
		t.Errorf("MaxConcurrentRuns = %d, want 1", job.MaxConcurrentRuns)
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

	testCreateJob(t, s, "Job A", "", "")
	testCreateJob(t, s, "Job B", "", "")

	jobs, err := s.ListJobs(context.Background())
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

	job := testCreateJob(t, s, "Original Name", "Original desc", "")

	updated, err := s.UpdateJob(ctx, UpdateJobParams{
		JobID:       job.JobID,
		Name:        "New Name",
		Description: "New desc",
	})
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

	_, err := s.UpdateJob(ctx, UpdateJobParams{
		JobID: "nonexistent-job-id", Name: "Name", Description: "Desc",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestJobStore_DeleteJob(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job := testCreateJob(t, s, "To Delete", "", "")
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

	job := testCreateJob(t, s, "Job", "", "")

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
	if step.TaskType != "claude_session" {
		t.Errorf("TaskType = %q, want %q", step.TaskType, "claude_session")
	}
	if step.RunIf != "all_success" {
		t.Errorf("RunIf = %q, want %q", step.RunIf, "all_success")
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

	job := testCreateJob(t, s, "Job", "", "")
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

	job := testCreateJob(t, s, "Job", "", "")
	stepA, _ := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "do A", MachineID: "machine-a", WorkingDir: "/work", Command: "claude", Args: "--verbose", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "do B", MachineID: "machine-a", WorkingDir: "/work2", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})

	// Create run
	run := testCreateRun(t, s, job.JobID, "manual")
	if run.RunID == "" {
		t.Fatal("expected non-empty RunID")
	}
	if run.Status != StatusPending {
		t.Errorf("Status = %q, want %q", run.Status, StatusPending)
	}

	// Insert run steps with snapshots
	steps := []Step{*stepA, *stepB}
	err := s.InsertRunSteps(ctx, run.RunID, steps)
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
		if rs.TaskTypeSnapshot != "claude_session" {
			t.Errorf("TaskTypeSnapshot = %q, want %q", rs.TaskTypeSnapshot, "claude_session")
		}
		if rs.RunIfSnapshot != "all_success" {
			t.Errorf("RunIfSnapshot = %q, want %q", rs.RunIfSnapshot, "all_success")
		}
		if rs.Attempt != 1 {
			t.Errorf("Attempt = %d, want 1", rs.Attempt)
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

	job := testCreateJob(t, s, "Job", "", "")
	testCreateRun(t, s, job.JobID, "manual")
	testCreateRun(t, s, job.JobID, "manual")

	runs, err := s.ListRuns(context.Background(), job.JobID)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("ListRuns count = %d, want 2", len(runs))
	}
}

func TestJobStore_ListAllRuns(t *testing.T) {
	s := newTestStoreForJobs(t)

	jobA := testCreateJob(t, s, "Job A", "", "")
	jobB := testCreateJob(t, s, "Job B", "", "")
	testCreateRun(t, s, jobA.JobID, "manual")
	testCreateRun(t, s, jobA.JobID, "manual")
	testCreateRun(t, s, jobB.JobID, "scheduled")

	runs, err := s.ListAllRuns(context.Background(), ListRunsOptions{})
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

	job := testCreateJob(t, s, "Job", "", "")
	run1 := testCreateRun(t, s, job.JobID, "manual")
	testCreateRun(t, s, job.JobID, "manual")

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

	jobA := testCreateJob(t, s, "Job A", "", "")
	jobB := testCreateJob(t, s, "Job B", "", "")
	testCreateRun(t, s, jobA.JobID, "manual")
	testCreateRun(t, s, jobA.JobID, "manual")
	testCreateRun(t, s, jobB.JobID, "manual")

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

	job := testCreateJob(t, s, "Job", "", "")
	for i := 0; i < 5; i++ {
		testCreateRun(t, s, job.JobID, "manual")
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

	job := testCreateJob(t, s, "Named Job", "", "")
	testCreateRun(t, s, job.JobID, "manual")

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

	job := testCreateJob(t, s, "Job", "", "")
	// Insert 55 runs -- more than the default limit of 50.
	for i := 0; i < 55; i++ {
		testCreateRun(t, s, job.JobID, "manual")
	}

	runs, err := s.ListAllRuns(context.Background(), ListRunsOptions{})
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

	job := testCreateJob(t, s, "Job", "", "")
	stepA, _ := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "A", Prompt: "prompt", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 0, OnFailure: "fail_run"})
	stepB, _ := s.CreateStep(ctx, CreateStepParams{JobID: job.JobID, Name: "B", Prompt: "prompt", MachineID: "", WorkingDir: "", Command: "claude", Args: "", TimeoutSeconds: 0, SortOrder: 1, OnFailure: "fail_run"})
	_ = s.AddDependency(ctx, stepB.StepID, stepA.StepID)

	run := testCreateRun(t, s, job.JobID, "manual")
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

func TestJobStore_ListJobsWithStats(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	// Seed machines to satisfy FK constraints
	if err := s.UpsertMachine("nuc-01", 5); err != nil {
		t.Fatalf("UpsertMachine nuc-01: %v", err)
	}
	if err := s.UpsertMachine("nuc-02", 5); err != nil {
		t.Fatalf("UpsertMachine nuc-02: %v", err)
	}

	jobA := testCreateJob(t, s, "Job A", "desc A", "")
	testCreateJob(t, s, "Job B", "", "")

	// Add steps with machine_id to job A
	stepA, err := s.CreateStep(ctx, CreateStepParams{
		JobID: jobA.JobID, Name: "step1", MachineID: "nuc-01", SortOrder: 1,
	})
	if err != nil {
		t.Fatalf("CreateStep 1: %v", err)
	}
	if _, err := s.CreateStep(ctx, CreateStepParams{
		JobID: jobA.JobID, Name: "step2", MachineID: "nuc-02", SortOrder: 2,
	}); err != nil {
		t.Fatalf("CreateStep 2: %v", err)
	}

	// Add a run to job A so last_run_status is populated
	run := testCreateRun(t, s, jobA.JobID, "manual")
	if err := s.InsertRunSteps(ctx, run.RunID, []Step{*stepA}); err != nil {
		t.Fatalf("InsertRunSteps: %v", err)
	}
	if err := s.UpdateRunStatus(ctx, run.RunID, StatusCompleted); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}

	// Add a cron schedule to job A
	_, _ = s.CreateSchedule(ctx, CreateScheduleParams{
		JobID: jobA.JobID, CronExpr: "0 * * * *",
	})

	// List all jobs (admin view)
	jobs, err := s.ListJobsWithStats(ctx, "")
	if err != nil {
		t.Fatalf("ListJobsWithStats: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("count = %d, want 2", len(jobs))
	}

	// Jobs are ordered by created_at DESC, so jobB is first
	var jA, jB *JobWithStats
	for i := range jobs {
		if jobs[i].JobID == jobA.JobID {
			jA = &jobs[i]
		} else {
			jB = &jobs[i]
		}
	}

	// Job A: 2 steps, completed, cron trigger, two machines
	if jA.StepCount != 2 {
		t.Errorf("jobA StepCount = %d, want 2", jA.StepCount)
	}
	if jA.LastRunStatus != StatusCompleted {
		t.Errorf("jobA LastRunStatus = %q, want %q", jA.LastRunStatus, StatusCompleted)
	}
	if jA.TriggerType != "cron" {
		t.Errorf("jobA TriggerType = %q, want %q", jA.TriggerType, "cron")
	}
	// machine_ids should contain both nuc-01 and nuc-02
	if jA.MachineIDs == "" {
		t.Error("jobA MachineIDs is empty, want nuc-01,nuc-02")
	}

	// Job B: 0 steps, no runs, manual trigger
	if jB.StepCount != 0 {
		t.Errorf("jobB StepCount = %d, want 0", jB.StepCount)
	}
	if jB.LastRunStatus != "" {
		t.Errorf("jobB LastRunStatus = %q, want empty", jB.LastRunStatus)
	}
	if jB.TriggerType != "manual" {
		t.Errorf("jobB TriggerType = %q, want %q", jB.TriggerType, "manual")
	}

	// User filtering with empty userID returns all
	allJobs, err := s.ListJobsWithStats(ctx, "")
	if err != nil {
		t.Fatalf("ListJobsWithStats empty user: %v", err)
	}
	if len(allJobs) != 2 {
		t.Errorf("empty user filter count = %d, want 2", len(allJobs))
	}
}

func TestJobStore_ListJobsWithStats_TriggerTypes(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	// Job with event trigger only
	jobEvent := testCreateJob(t, s, "Event Job", "", "")
	_, _ = s.CreateJobTrigger(ctx, JobTrigger{
		JobID: jobEvent.JobID, EventType: "run.completed", Enabled: true,
	})

	// Job with both cron and event trigger
	jobMixed := testCreateJob(t, s, "Mixed Job", "", "")
	_, _ = s.CreateSchedule(ctx, CreateScheduleParams{
		JobID: jobMixed.JobID, CronExpr: "0 0 * * *",
	})
	_, _ = s.CreateJobTrigger(ctx, JobTrigger{
		JobID: jobMixed.JobID, EventType: "session.started", Enabled: true,
	})

	jobs, err := s.ListJobsWithStats(ctx, "")
	if err != nil {
		t.Fatalf("ListJobsWithStats: %v", err)
	}

	triggerTypes := make(map[string]string)
	for _, j := range jobs {
		triggerTypes[j.Name] = j.TriggerType
	}

	if triggerTypes["Event Job"] != "event" {
		t.Errorf("Event Job trigger = %q, want %q", triggerTypes["Event Job"], "event")
	}
	if triggerTypes["Mixed Job"] != "mixed" {
		t.Errorf("Mixed Job trigger = %q, want %q", triggerTypes["Mixed Job"], "mixed")
	}
}

func TestJobStore_ListAllRuns_IncludesMachineIDs(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	// Seed machines for FK
	_ = s.UpsertMachine("nuc-01", 5)
	_ = s.UpsertMachine("nuc-02", 5)

	job := testCreateJob(t, s, "Job", "", "")
	step1, err := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s1", MachineID: "nuc-01", SortOrder: 1,
	})
	if err != nil {
		t.Fatalf("CreateStep 1: %v", err)
	}
	step2, err := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s2", MachineID: "nuc-02", SortOrder: 2,
	})
	if err != nil {
		t.Fatalf("CreateStep 2: %v", err)
	}

	run := testCreateRun(t, s, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*step1, *step2})

	runs, err := s.ListAllRuns(ctx, ListRunsOptions{})
	if err != nil {
		t.Fatalf("ListAllRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("count = %d, want 1", len(runs))
	}

	// MachineIDs should be a comma-separated list of distinct machine IDs
	ids := runs[0].MachineIDs
	if ids == "" {
		t.Fatal("MachineIDs is empty, want nuc-01,nuc-02")
	}
	// GROUP_CONCAT order is not guaranteed, so check both are present
	if !(contains(ids, "nuc-01") && contains(ids, "nuc-02")) {
		t.Errorf("MachineIDs = %q, want to contain both nuc-01 and nuc-02", ids)
	}

	// Run without run_steps should have empty MachineIDs
	run2 := testCreateRun(t, s, job.JobID, "manual")

	runs, _ = s.ListAllRuns(ctx, ListRunsOptions{})
	for _, r := range runs {
		if r.RunID == run2.RunID && r.MachineIDs != "" {
			t.Errorf("run without steps: MachineIDs = %q, want empty", r.MachineIDs)
		}
	}
}

func TestJobStore_RunWithParameters(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	job, err := s.CreateJob(ctx, CreateJobParams{
		Name:              "Param Job",
		Parameters:        `{"env": "staging"}`,
		TimeoutSeconds:    300,
		MaxConcurrentRuns: 3,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if job.Parameters != `{"env": "staging"}` {
		t.Errorf("Parameters = %q, want %q", job.Parameters, `{"env": "staging"}`)
	}
	if job.TimeoutSeconds != 300 {
		t.Errorf("TimeoutSeconds = %d, want 300", job.TimeoutSeconds)
	}
	if job.MaxConcurrentRuns != 3 {
		t.Errorf("MaxConcurrentRuns = %d, want 3", job.MaxConcurrentRuns)
	}

	// Create run with parameters
	run, err := s.CreateRun(ctx, CreateRunParams{
		JobID:         job.JobID,
		TriggerType:   "manual",
		TriggerDetail: "test",
		Parameters:    `{"env": "prod"}`,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.Parameters != `{"env": "prod"}` {
		t.Errorf("Run.Parameters = %q, want %q", run.Parameters, `{"env": "prod"}`)
	}

	// Update run parameters
	err = s.UpdateRunParameters(ctx, run.RunID, `{"env": "updated"}`)
	if err != nil {
		t.Fatalf("UpdateRunParameters: %v", err)
	}

	detail, err := s.GetRunWithSteps(ctx, run.RunID)
	if err != nil {
		t.Fatalf("GetRunWithSteps: %v", err)
	}
	if detail.Run.Parameters != `{"env": "updated"}` {
		t.Errorf("Updated Parameters = %q, want %q", detail.Run.Parameters, `{"env": "updated"}`)
	}
}

func TestJobStore_UpdateRunStepAttempt(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_ = s.UpsertMachine("m1", 5)

	job := testCreateJob(t, s, "Job", "", "")
	step, _ := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s1", MachineID: "m1", Prompt: "do it",
		OnFailure: "fail_run", MaxRetries: 3,
	})
	run := testCreateRun(t, s, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*step})

	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	if detail.RunSteps[0].Attempt != 1 {
		t.Errorf("initial Attempt = %d, want 1", detail.RunSteps[0].Attempt)
	}

	err := s.UpdateRunStepAttempt(ctx, detail.RunSteps[0].RunStepID, 2)
	if err != nil {
		t.Fatalf("UpdateRunStepAttempt: %v", err)
	}

	detail, _ = s.GetRunWithSteps(ctx, run.RunID)
	if detail.RunSteps[0].Attempt != 2 {
		t.Errorf("updated Attempt = %d, want 2", detail.RunSteps[0].Attempt)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
