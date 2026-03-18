package store

import (
	"context"
	"strings"
	"testing"
)

func TestTaskValues_SetAndGet(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_ = s.UpsertMachine("m1", 5, "")
	job := testCreateJob(t, s, "Job", "", "")
	step, _ := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s1", MachineID: "m1", Prompt: "do it",
		OnFailure: "fail_run",
	})
	run := testCreateRun(t, s, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*step})

	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	rsID := detail.RunSteps[0].RunStepID

	// Set a value
	err := s.SetTaskValue(ctx, rsID, "result", "hello world")
	if err != nil {
		t.Fatalf("SetTaskValue: %v", err)
	}

	// Get values
	values, err := s.GetTaskValues(ctx, rsID)
	if err != nil {
		t.Fatalf("GetTaskValues: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("values count = %d, want 1", len(values))
	}
	if values[0].Key != "result" {
		t.Errorf("Key = %q, want %q", values[0].Key, "result")
	}
	if values[0].Value != "hello world" {
		t.Errorf("Value = %q, want %q", values[0].Value, "hello world")
	}
}

func TestTaskValues_MultipleKeys(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_ = s.UpsertMachine("m1", 5, "")
	job := testCreateJob(t, s, "Job", "", "")
	step, _ := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s1", MachineID: "m1", Prompt: "do it",
		OnFailure: "fail_run",
	})
	run := testCreateRun(t, s, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*step})

	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	rsID := detail.RunSteps[0].RunStepID

	_ = s.SetTaskValue(ctx, rsID, "alpha", "1")
	_ = s.SetTaskValue(ctx, rsID, "beta", "2")
	_ = s.SetTaskValue(ctx, rsID, "gamma", "3")

	values, err := s.GetTaskValues(ctx, rsID)
	if err != nil {
		t.Fatalf("GetTaskValues: %v", err)
	}
	if len(values) != 3 {
		t.Fatalf("values count = %d, want 3", len(values))
	}
	// Ordered by key
	if values[0].Key != "alpha" || values[1].Key != "beta" || values[2].Key != "gamma" {
		t.Errorf("keys = [%s, %s, %s], want [alpha, beta, gamma]",
			values[0].Key, values[1].Key, values[2].Key)
	}
}

func TestTaskValues_GetForUpstreamSteps(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_ = s.UpsertMachine("m1", 5, "")
	job := testCreateJob(t, s, "Job", "", "")
	step1, _ := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s1", MachineID: "m1", Prompt: "p1",
		OnFailure: "fail_run", SortOrder: 0,
	})
	step2, _ := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s2", MachineID: "m1", Prompt: "p2",
		OnFailure: "fail_run", SortOrder: 1,
	})
	run := testCreateRun(t, s, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*step1, *step2})

	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	rs1ID := detail.RunSteps[0].RunStepID
	rs2ID := detail.RunSteps[1].RunStepID

	_ = s.SetTaskValue(ctx, rs1ID, "from_step1", "value1")
	_ = s.SetTaskValue(ctx, rs2ID, "from_step2", "value2")

	values, err := s.GetTaskValuesForSteps(ctx, []string{rs1ID, rs2ID})
	if err != nil {
		t.Fatalf("GetTaskValuesForSteps: %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("values count = %d, want 2", len(values))
	}
}

func TestTaskValues_GetForEmptySteps(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	values, err := s.GetTaskValuesForSteps(ctx, []string{})
	if err != nil {
		t.Fatalf("GetTaskValuesForSteps empty: %v", err)
	}
	if values != nil {
		t.Errorf("expected nil for empty input, got %v", values)
	}
}

func TestTaskValues_KeyValidation(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_ = s.UpsertMachine("m1", 5, "")
	job := testCreateJob(t, s, "Job", "", "")
	step, _ := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s1", MachineID: "m1", Prompt: "p",
		OnFailure: "fail_run",
	})
	run := testCreateRun(t, s, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*step})

	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	rsID := detail.RunSteps[0].RunStepID

	// Empty key
	if err := s.SetTaskValue(ctx, rsID, "", "val"); err == nil {
		t.Error("expected error for empty key")
	}

	// Key starting with number
	if err := s.SetTaskValue(ctx, rsID, "1bad", "val"); err == nil {
		t.Error("expected error for key starting with number")
	}

	// Key with special chars
	if err := s.SetTaskValue(ctx, rsID, "bad-key", "val"); err == nil {
		t.Error("expected error for key with hyphens")
	}

	// Key too long
	longKey := strings.Repeat("a", maxTaskValueKeyLen+1)
	if err := s.SetTaskValue(ctx, rsID, longKey, "val"); err == nil {
		t.Error("expected error for key exceeding max length")
	}

	// Valid keys should work
	if err := s.SetTaskValue(ctx, rsID, "valid_key", "val"); err != nil {
		t.Errorf("expected valid_key to succeed: %v", err)
	}
	if err := s.SetTaskValue(ctx, rsID, "A", "val"); err != nil {
		t.Errorf("expected single char key to succeed: %v", err)
	}
	if err := s.SetTaskValue(ctx, rsID, "camelCase123", "val"); err != nil {
		t.Errorf("expected camelCase key to succeed: %v", err)
	}
}

func TestTaskValues_SizeLimit(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_ = s.UpsertMachine("m1", 5, "")
	job := testCreateJob(t, s, "Job", "", "")
	step, _ := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s1", MachineID: "m1", Prompt: "p",
		OnFailure: "fail_run",
	})
	run := testCreateRun(t, s, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*step})

	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	rsID := detail.RunSteps[0].RunStepID

	// Value at max size should work
	maxVal := strings.Repeat("x", maxTaskValueSize)
	if err := s.SetTaskValue(ctx, rsID, "atmax", maxVal); err != nil {
		t.Errorf("expected value at max size to succeed: %v", err)
	}

	// Value over max size should fail
	overVal := strings.Repeat("x", maxTaskValueSize+1)
	if err := s.SetTaskValue(ctx, rsID, "overmax", overVal); err == nil {
		t.Error("expected error for value exceeding max size")
	}
}

func TestTaskValues_MaxPerStep(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_ = s.UpsertMachine("m1", 5, "")
	job := testCreateJob(t, s, "Job", "", "")
	step, _ := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s1", MachineID: "m1", Prompt: "p",
		OnFailure: "fail_run",
	})
	run := testCreateRun(t, s, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*step})

	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	rsID := detail.RunSteps[0].RunStepID

	// Insert max number of values
	for i := 0; i < maxTaskValuesPerStep; i++ {
		key := "key" + strings.Repeat("x", i) // unique keys
		if err := s.SetTaskValue(ctx, rsID, key, "val"); err != nil {
			t.Fatalf("SetTaskValue %d: %v", i, err)
		}
	}

	// One more should fail
	err := s.SetTaskValue(ctx, rsID, "overflow", "val")
	if err == nil {
		t.Error("expected error when exceeding max task values per step")
	}
}

func TestTaskValues_DeleteForStep(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_ = s.UpsertMachine("m1", 5, "")
	job := testCreateJob(t, s, "Job", "", "")
	step, _ := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s1", MachineID: "m1", Prompt: "p",
		OnFailure: "fail_run",
	})
	run := testCreateRun(t, s, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*step})

	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	rsID := detail.RunSteps[0].RunStepID

	_ = s.SetTaskValue(ctx, rsID, "k1", "v1")
	_ = s.SetTaskValue(ctx, rsID, "k2", "v2")

	err := s.DeleteTaskValuesForStep(ctx, rsID)
	if err != nil {
		t.Fatalf("DeleteTaskValuesForStep: %v", err)
	}

	values, _ := s.GetTaskValues(ctx, rsID)
	if len(values) != 0 {
		t.Errorf("values after delete = %d, want 0", len(values))
	}
}

func TestTaskValues_UpsertExistingKey(t *testing.T) {
	s := newTestStoreForJobs(t)
	ctx := context.Background()

	_ = s.UpsertMachine("m1", 5, "")
	job := testCreateJob(t, s, "Job", "", "")
	step, _ := s.CreateStep(ctx, CreateStepParams{
		JobID: job.JobID, Name: "s1", MachineID: "m1", Prompt: "p",
		OnFailure: "fail_run",
	})
	run := testCreateRun(t, s, job.JobID, "manual")
	_ = s.InsertRunSteps(ctx, run.RunID, []Step{*step})

	detail, _ := s.GetRunWithSteps(ctx, run.RunID)
	rsID := detail.RunSteps[0].RunStepID

	// Set initial value
	_ = s.SetTaskValue(ctx, rsID, "mykey", "original")

	// Upsert with new value
	err := s.SetTaskValue(ctx, rsID, "mykey", "updated")
	if err != nil {
		t.Fatalf("SetTaskValue upsert: %v", err)
	}

	values, _ := s.GetTaskValues(ctx, rsID)
	if len(values) != 1 {
		t.Fatalf("values count after upsert = %d, want 1", len(values))
	}
	if values[0].Value != "updated" {
		t.Errorf("Value = %q, want %q", values[0].Value, "updated")
	}
}
