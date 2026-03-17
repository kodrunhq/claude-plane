package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestStoreForTriggers(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "triggers_test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// insertTestJob inserts a minimal job row so job_triggers FK is satisfied.
func insertTestJob(t *testing.T, s *Store, jobID string) {
	t.Helper()
	_, err := s.writer.ExecContext(context.Background(),
		`INSERT INTO jobs (job_id, name, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		jobID, "test-job",
	)
	if err != nil {
		t.Fatalf("insertTestJob: %v", err)
	}
}

func makeTrigger(jobID, eventType string) JobTrigger {
	return JobTrigger{
		JobID:     jobID,
		EventType: eventType,
		Filter:    "",
		Enabled:   true,
	}
}

// --- CreateJobTrigger ---

func TestCreateJobTrigger(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()
	jobID := "job-001"
	insertTestJob(t, s, jobID)

	trig, err := s.CreateJobTrigger(ctx, makeTrigger(jobID, "run.completed"))
	if err != nil {
		t.Fatalf("CreateJobTrigger: %v", err)
	}

	if trig.TriggerID == "" {
		t.Error("expected non-empty TriggerID")
	}
	if trig.JobID != jobID {
		t.Errorf("JobID = %q, want %q", trig.JobID, jobID)
	}
	if trig.EventType != "run.completed" {
		t.Errorf("EventType = %q, want %q", trig.EventType, "run.completed")
	}
	if !trig.Enabled {
		t.Error("expected Enabled = true")
	}
	if trig.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestCreateJobTrigger_WithFilter(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()
	jobID := "job-002"
	insertTestJob(t, s, jobID)

	trigger := JobTrigger{
		JobID:     jobID,
		EventType: "run.*",
		Filter:    `{"status":"completed"}`,
		Enabled:   true,
	}

	trig, err := s.CreateJobTrigger(ctx, trigger)
	if err != nil {
		t.Fatalf("CreateJobTrigger: %v", err)
	}

	if trig.Filter != `{"status":"completed"}` {
		t.Errorf("Filter = %q, want %q", trig.Filter, `{"status":"completed"}`)
	}
}

// --- ListJobTriggers ---

func TestListJobTriggers_Empty(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()
	jobID := "job-003"
	insertTestJob(t, s, jobID)

	triggers, err := s.ListJobTriggers(ctx, jobID)
	if err != nil {
		t.Fatalf("ListJobTriggers: %v", err)
	}
	if len(triggers) != 0 {
		t.Errorf("expected 0 triggers, got %d", len(triggers))
	}
}

func TestListJobTriggers_ReturnsOnlyForJob(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()

	jobA := "job-A"
	jobB := "job-B"
	insertTestJob(t, s, jobA)
	insertTestJob(t, s, jobB)

	if _, err := s.CreateJobTrigger(ctx, makeTrigger(jobA, "run.completed")); err != nil {
		t.Fatalf("CreateJobTrigger jobA: %v", err)
	}
	if _, err := s.CreateJobTrigger(ctx, makeTrigger(jobA, "run.failed")); err != nil {
		t.Fatalf("CreateJobTrigger jobA 2: %v", err)
	}
	if _, err := s.CreateJobTrigger(ctx, makeTrigger(jobB, "run.completed")); err != nil {
		t.Fatalf("CreateJobTrigger jobB: %v", err)
	}

	triggersA, err := s.ListJobTriggers(ctx, jobA)
	if err != nil {
		t.Fatalf("ListJobTriggers jobA: %v", err)
	}
	if len(triggersA) != 2 {
		t.Errorf("expected 2 triggers for jobA, got %d", len(triggersA))
	}

	triggersB, err := s.ListJobTriggers(ctx, jobB)
	if err != nil {
		t.Fatalf("ListJobTriggers jobB: %v", err)
	}
	if len(triggersB) != 1 {
		t.Errorf("expected 1 trigger for jobB, got %d", len(triggersB))
	}
}

// --- DeleteJobTrigger ---

func TestDeleteJobTrigger(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()
	jobID := "job-del"
	insertTestJob(t, s, jobID)

	trig, err := s.CreateJobTrigger(ctx, makeTrigger(jobID, "run.completed"))
	if err != nil {
		t.Fatalf("CreateJobTrigger: %v", err)
	}

	if err := s.DeleteJobTrigger(ctx, trig.TriggerID); err != nil {
		t.Fatalf("DeleteJobTrigger: %v", err)
	}

	triggers, err := s.ListJobTriggers(ctx, jobID)
	if err != nil {
		t.Fatalf("ListJobTriggers after delete: %v", err)
	}
	if len(triggers) != 0 {
		t.Errorf("expected 0 triggers after delete, got %d", len(triggers))
	}
}

func TestDeleteJobTrigger_NotFound(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()

	err := s.DeleteJobTrigger(ctx, "nonexistent-trigger")
	if err == nil {
		t.Fatal("expected error for nonexistent trigger")
	}
}

// --- UpdateJobTrigger ---

func TestUpdateJobTrigger(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()
	jobID := "job-upd"
	insertTestJob(t, s, jobID)

	trig, err := s.CreateJobTrigger(ctx, makeTrigger(jobID, "run.completed"))
	if err != nil {
		t.Fatalf("CreateJobTrigger: %v", err)
	}

	updated, err := s.UpdateJobTrigger(ctx, trig.TriggerID, "run.failed", `{"job_id":"xyz"}`)
	if err != nil {
		t.Fatalf("UpdateJobTrigger: %v", err)
	}

	if updated.EventType != "run.failed" {
		t.Errorf("EventType = %q, want %q", updated.EventType, "run.failed")
	}
	if updated.Filter != `{"job_id":"xyz"}` {
		t.Errorf("Filter = %q, want %q", updated.Filter, `{"job_id":"xyz"}`)
	}
	if !updated.Enabled {
		t.Error("expected Enabled unchanged (true)")
	}
	if !updated.UpdatedAt.After(trig.UpdatedAt) {
		t.Error("expected UpdatedAt to advance")
	}
}

func TestUpdateJobTrigger_NotFound(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()

	_, err := s.UpdateJobTrigger(ctx, "nonexistent", "run.completed", "")
	if err == nil {
		t.Fatal("expected error for nonexistent trigger")
	}
}

// --- ToggleJobTrigger ---

func TestToggleJobTrigger(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()
	jobID := "job-toggle"
	insertTestJob(t, s, jobID)

	trig, err := s.CreateJobTrigger(ctx, makeTrigger(jobID, "run.completed"))
	if err != nil {
		t.Fatalf("CreateJobTrigger: %v", err)
	}
	if !trig.Enabled {
		t.Fatal("expected initial Enabled = true")
	}

	toggled, err := s.ToggleJobTrigger(ctx, trig.TriggerID)
	if err != nil {
		t.Fatalf("ToggleJobTrigger: %v", err)
	}
	if toggled.Enabled {
		t.Error("expected Enabled = false after toggle")
	}

	toggled2, err := s.ToggleJobTrigger(ctx, trig.TriggerID)
	if err != nil {
		t.Fatalf("ToggleJobTrigger 2: %v", err)
	}
	if !toggled2.Enabled {
		t.Error("expected Enabled = true after second toggle")
	}
}

func TestToggleJobTrigger_NotFound(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()

	_, err := s.ToggleJobTrigger(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent trigger")
	}
}

// --- ListEnabledTriggers ---

func TestListEnabledTriggers(t *testing.T) {
	s := newTestStoreForTriggers(t)
	ctx := context.Background()

	jobA := "job-ena-A"
	jobB := "job-ena-B"
	insertTestJob(t, s, jobA)
	insertTestJob(t, s, jobB)

	// Create one enabled and one disabled trigger for jobA.
	if _, err := s.CreateJobTrigger(ctx, makeTrigger(jobA, "run.completed")); err != nil {
		t.Fatalf("CreateJobTrigger enabled: %v", err)
	}
	disabled := JobTrigger{JobID: jobA, EventType: "run.failed", Enabled: false}
	if _, err := s.CreateJobTrigger(ctx, disabled); err != nil {
		t.Fatalf("CreateJobTrigger disabled: %v", err)
	}
	// One enabled trigger for jobB.
	if _, err := s.CreateJobTrigger(ctx, makeTrigger(jobB, "session.started")); err != nil {
		t.Fatalf("CreateJobTrigger jobB: %v", err)
	}

	enabled, err := s.ListEnabledTriggers(ctx)
	if err != nil {
		t.Fatalf("ListEnabledTriggers: %v", err)
	}
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled triggers, got %d", len(enabled))
	}
	for _, tr := range enabled {
		if !tr.Enabled {
			t.Errorf("trigger %s should be enabled", tr.TriggerID)
		}
	}
}
