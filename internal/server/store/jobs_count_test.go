package store

import (
	"context"
	"testing"
	"time"
)

func TestCountRunsForJobUpTo(t *testing.T) {
	s := mustNewStore(t)
	ctx := context.Background()

	// Create a user first (foreign key requirement), then a job.
	user := mustCreateUser(t, s, "", "admin")
	job := mustCreateJob(t, s, WithJobName("count-test"), WithJobUserID(user.UserID))

	count, err := s.CountRunsForJobUpTo(ctx, job.JobID, time.Now())
	if err != nil {
		t.Fatalf("CountRunsForJobUpTo: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 runs, got %d", count)
	}

	// Create some runs and verify count increases.
	mustCreateRun(t, s, job.JobID)
	mustCreateRun(t, s, job.JobID)

	count, err = s.CountRunsForJobUpTo(ctx, job.JobID, time.Now())
	if err != nil {
		t.Fatalf("CountRunsForJobUpTo after inserts: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 runs, got %d", count)
	}
}
