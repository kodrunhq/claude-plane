package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestStoreForSchedules(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "schedules_test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// insertScheduleTestJob inserts a minimal job row so cron_schedules FK is satisfied.
func insertScheduleTestJob(t *testing.T, s *Store, jobID string) {
	t.Helper()
	_, err := s.writer.ExecContext(context.Background(),
		`INSERT INTO jobs (job_id, name, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		jobID, "test-job",
	)
	if err != nil {
		t.Fatalf("insertScheduleTestJob: %v", err)
	}
}

func makeScheduleParams(jobID, cronExpr, tz string) CreateScheduleParams {
	return CreateScheduleParams{
		JobID:    jobID,
		CronExpr: cronExpr,
		Timezone: tz,
	}
}

// --- CreateSchedule ---

func TestCreateSchedule(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-001"
	insertScheduleTestJob(t, s, jobID)

	sc, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "0 * * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	if sc.ScheduleID == "" {
		t.Error("expected non-empty ScheduleID")
	}
	if sc.JobID != jobID {
		t.Errorf("JobID = %q, want %q", sc.JobID, jobID)
	}
	if sc.CronExpr != "0 * * * *" {
		t.Errorf("CronExpr = %q, want %q", sc.CronExpr, "0 * * * *")
	}
	if sc.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want %q", sc.Timezone, "UTC")
	}
	if !sc.Enabled {
		t.Error("expected Enabled = true")
	}
	if sc.NextRunAt != nil {
		t.Error("expected NextRunAt = nil on creation")
	}
	if sc.LastTriggeredAt != nil {
		t.Error("expected LastTriggeredAt = nil on creation")
	}
	if sc.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestCreateSchedule_DefaultTimezone(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-002"
	insertScheduleTestJob(t, s, jobID)

	sc, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "0 0 * * *", ""))
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	if sc.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want %q (default)", sc.Timezone, "UTC")
	}
}

func TestCreateSchedule_CustomTimezone(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-003"
	insertScheduleTestJob(t, s, jobID)

	sc, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "0 9 * * 1-5", "America/New_York"))
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	if sc.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want %q", sc.Timezone, "America/New_York")
	}
}

// --- GetSchedule ---

func TestGetSchedule(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-get"
	insertScheduleTestJob(t, s, jobID)

	created, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "*/5 * * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	got, err := s.GetSchedule(ctx, created.ScheduleID)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}

	if got.ScheduleID != created.ScheduleID {
		t.Errorf("ScheduleID = %q, want %q", got.ScheduleID, created.ScheduleID)
	}
	if got.CronExpr != "*/5 * * * *" {
		t.Errorf("CronExpr = %q, want %q", got.CronExpr, "*/5 * * * *")
	}
}

func TestGetSchedule_NotFound(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()

	_, err := s.GetSchedule(ctx, "nonexistent-schedule")
	if err == nil {
		t.Fatal("expected error for nonexistent schedule")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- ListSchedulesByJob ---

func TestListSchedulesByJob_Empty(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-list-empty"
	insertScheduleTestJob(t, s, jobID)

	schedules, err := s.ListSchedulesByJob(ctx, jobID)
	if err != nil {
		t.Fatalf("ListSchedulesByJob: %v", err)
	}
	if len(schedules) != 0 {
		t.Errorf("expected 0 schedules, got %d", len(schedules))
	}
}

func TestListSchedulesByJob_ReturnsOnlyForJob(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()

	jobA := "job-list-A"
	jobB := "job-list-B"
	insertScheduleTestJob(t, s, jobA)
	insertScheduleTestJob(t, s, jobB)

	if _, err := s.CreateSchedule(ctx, makeScheduleParams(jobA, "0 * * * *", "UTC")); err != nil {
		t.Fatalf("CreateSchedule jobA 1: %v", err)
	}
	if _, err := s.CreateSchedule(ctx, makeScheduleParams(jobA, "0 0 * * *", "UTC")); err != nil {
		t.Fatalf("CreateSchedule jobA 2: %v", err)
	}
	if _, err := s.CreateSchedule(ctx, makeScheduleParams(jobB, "*/30 * * * *", "UTC")); err != nil {
		t.Fatalf("CreateSchedule jobB: %v", err)
	}

	schedulesA, err := s.ListSchedulesByJob(ctx, jobA)
	if err != nil {
		t.Fatalf("ListSchedulesByJob jobA: %v", err)
	}
	if len(schedulesA) != 2 {
		t.Errorf("expected 2 schedules for jobA, got %d", len(schedulesA))
	}

	schedulesB, err := s.ListSchedulesByJob(ctx, jobB)
	if err != nil {
		t.Fatalf("ListSchedulesByJob jobB: %v", err)
	}
	if len(schedulesB) != 1 {
		t.Errorf("expected 1 schedule for jobB, got %d", len(schedulesB))
	}
}

// --- ListEnabledSchedules ---

func TestListEnabledSchedules(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()

	jobA := "job-ena-sched-A"
	jobB := "job-ena-sched-B"
	insertScheduleTestJob(t, s, jobA)
	insertScheduleTestJob(t, s, jobB)

	sc1, err := s.CreateSchedule(ctx, makeScheduleParams(jobA, "0 * * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule enabled: %v", err)
	}
	sc2, err := s.CreateSchedule(ctx, makeScheduleParams(jobA, "0 0 * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule 2: %v", err)
	}
	if _, err := s.CreateSchedule(ctx, makeScheduleParams(jobB, "*/5 * * * *", "UTC")); err != nil {
		t.Fatalf("CreateSchedule jobB: %v", err)
	}

	// Disable sc2.
	if err := s.SetScheduleEnabled(ctx, sc2.ScheduleID, false); err != nil {
		t.Fatalf("SetScheduleEnabled false: %v", err)
	}
	// Verify sc1 is not affected.
	_ = sc1

	enabled, err := s.ListEnabledSchedules(ctx)
	if err != nil {
		t.Fatalf("ListEnabledSchedules: %v", err)
	}
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled schedules, got %d", len(enabled))
	}
	for _, sc := range enabled {
		if !sc.Enabled {
			t.Errorf("schedule %s should be enabled", sc.ScheduleID)
		}
	}
}

// --- UpdateSchedule ---

func TestUpdateSchedule(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-upd"
	insertScheduleTestJob(t, s, jobID)

	sc, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "0 * * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	updated, err := s.UpdateSchedule(ctx, UpdateScheduleParams{
		ScheduleID: sc.ScheduleID,
		CronExpr:   "0 12 * * *",
		Timezone:   "Europe/London",
	})
	if err != nil {
		t.Fatalf("UpdateSchedule: %v", err)
	}

	if updated.CronExpr != "0 12 * * *" {
		t.Errorf("CronExpr = %q, want %q", updated.CronExpr, "0 12 * * *")
	}
	if updated.Timezone != "Europe/London" {
		t.Errorf("Timezone = %q, want %q", updated.Timezone, "Europe/London")
	}
	if !updated.UpdatedAt.After(sc.CreatedAt) && !updated.UpdatedAt.Equal(sc.CreatedAt) {
		t.Error("expected UpdatedAt to be >= CreatedAt after update")
	}
}

func TestUpdateSchedule_NotFound(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()

	_, err := s.UpdateSchedule(ctx, UpdateScheduleParams{
		ScheduleID: "nonexistent",
		CronExpr:   "0 * * * *",
		Timezone:   "UTC",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent schedule")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- SetScheduleEnabled ---

func TestSetScheduleEnabled_Disable(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-dis"
	insertScheduleTestJob(t, s, jobID)

	sc, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "0 * * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	if err := s.SetScheduleEnabled(ctx, sc.ScheduleID, false); err != nil {
		t.Fatalf("SetScheduleEnabled false: %v", err)
	}

	got, err := s.GetSchedule(ctx, sc.ScheduleID)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if got.Enabled {
		t.Error("expected Enabled = false after disabling")
	}
	if got.NextRunAt != nil {
		t.Error("expected NextRunAt = nil after disabling")
	}
}

func TestSetScheduleEnabled_Enable(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-en"
	insertScheduleTestJob(t, s, jobID)

	sc, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "0 * * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	if err := s.SetScheduleEnabled(ctx, sc.ScheduleID, false); err != nil {
		t.Fatalf("SetScheduleEnabled false: %v", err)
	}
	if err := s.SetScheduleEnabled(ctx, sc.ScheduleID, true); err != nil {
		t.Fatalf("SetScheduleEnabled true: %v", err)
	}

	got, err := s.GetSchedule(ctx, sc.ScheduleID)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if !got.Enabled {
		t.Error("expected Enabled = true after re-enabling")
	}
}

func TestSetScheduleEnabled_NotFound(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()

	err := s.SetScheduleEnabled(ctx, "nonexistent", false)
	if err == nil {
		t.Fatal("expected error for nonexistent schedule")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- UpdateScheduleTimestamps ---

func TestUpdateScheduleTimestamps(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-ts"
	insertScheduleTestJob(t, s, jobID)

	sc, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "0 * * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	lastTriggered := time.Now().UTC().Add(-time.Minute)
	nextRun := time.Now().UTC().Add(time.Hour)

	if err := s.UpdateScheduleTimestamps(ctx, sc.ScheduleID, lastTriggered, nextRun); err != nil {
		t.Fatalf("UpdateScheduleTimestamps: %v", err)
	}

	got, err := s.GetSchedule(ctx, sc.ScheduleID)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if got.LastTriggeredAt == nil {
		t.Fatal("expected LastTriggeredAt to be set")
	}
	if got.NextRunAt == nil {
		t.Fatal("expected NextRunAt to be set")
	}
	// Compare with 1-second tolerance for DB time precision.
	if got.LastTriggeredAt.Unix() != lastTriggered.Unix() {
		t.Errorf("LastTriggeredAt = %v, want ~%v", got.LastTriggeredAt, lastTriggered)
	}
	if got.NextRunAt.Unix() != nextRun.Unix() {
		t.Errorf("NextRunAt = %v, want ~%v", got.NextRunAt, nextRun)
	}
}

func TestUpdateScheduleTimestamps_NotFound(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()

	err := s.UpdateScheduleTimestamps(ctx, "nonexistent", time.Now(), time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected error for nonexistent schedule")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- DeleteSchedule ---

func TestDeleteSchedule(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-del"
	insertScheduleTestJob(t, s, jobID)

	sc, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "0 * * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	if err := s.DeleteSchedule(ctx, sc.ScheduleID); err != nil {
		t.Fatalf("DeleteSchedule: %v", err)
	}

	_, err = s.GetSchedule(ctx, sc.ScheduleID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteSchedule_NotFound(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()

	err := s.DeleteSchedule(ctx, "nonexistent-schedule")
	if err == nil {
		t.Fatal("expected error for nonexistent schedule")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- ListSchedulesByJob ordering ---

func TestListSchedulesByJob_OrderedByCreatedAtDesc(t *testing.T) {
	s := newTestStoreForSchedules(t)
	ctx := context.Background()
	jobID := "job-sched-order"
	insertScheduleTestJob(t, s, jobID)

	sc1, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "0 1 * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule 1: %v", err)
	}
	sc2, err := s.CreateSchedule(ctx, makeScheduleParams(jobID, "0 2 * * *", "UTC"))
	if err != nil {
		t.Fatalf("CreateSchedule 2: %v", err)
	}

	schedules, err := s.ListSchedulesByJob(ctx, jobID)
	if err != nil {
		t.Fatalf("ListSchedulesByJob: %v", err)
	}
	if len(schedules) != 2 {
		t.Fatalf("expected 2 schedules, got %d", len(schedules))
	}
	// Newest first: sc2 before sc1.
	if schedules[0].ScheduleID != sc2.ScheduleID {
		t.Errorf("expected sc2 first, got %q", schedules[0].ScheduleID)
	}
	if schedules[1].ScheduleID != sc1.ScheduleID {
		t.Errorf("expected sc1 second, got %q", schedules[1].ScheduleID)
	}
}
