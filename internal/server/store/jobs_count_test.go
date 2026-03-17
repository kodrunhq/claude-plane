package store

import (
	"context"
	"testing"
)

func TestCountRunsForJob(t *testing.T) {
	s := mustNewStore(t)
	ctx := context.Background()

	// Create a user first (foreign key requirement), then a job.
	user := mustCreateUser(t, s, "", "admin")
	job := mustCreateJob(t, s, WithJobName("count-test"), WithJobUserID(user.UserID))

	count, err := s.CountRunsForJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("CountRunsForJob: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 runs, got %d", count)
	}

	// Create some runs and verify count increases.
	mustCreateRun(t, s, job.JobID)
	mustCreateRun(t, s, job.JobID)

	count, err = s.CountRunsForJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("CountRunsForJob after inserts: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 runs, got %d", count)
	}
}
