package reaper

import (
	"context"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestReaper(t *testing.T) (*Reaper, *store.Store) {
	t.Helper()
	s := newTestStore(t)

	// Seed parent records required by FK constraints.
	if err := s.UpsertMachine("m1", 10, "/tmp"); err != nil {
		t.Fatalf("seed machine: %v", err)
	}
	if err := s.CreateUser(&store.User{
		UserID:       "u1",
		Email:        "test@test.com",
		DisplayName:  "Test",
		PasswordHash: "fake",
		Role:         "admin",
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	r := New(s, nil, nil, nil)
	return r, s
}

func TestReaper_DoesNotTerminateFreshSessions(t *testing.T) {
	r, s := newTestReaper(t)

	sess := &store.Session{
		SessionID: "fresh-1",
		MachineID: "m1",
		UserID:    "u1",
		Command:   "claude",
	}
	if err := s.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	s.UpdateSessionStatus("fresh-1", store.StatusWaitingForInput)

	r.sweep(context.Background())

	result, _ := s.GetSession("fresh-1")
	if result.Status != store.StatusWaitingForInput {
		t.Fatalf("expected fresh session to remain waiting_for_input, got %s", result.Status)
	}
}

func TestReaper_DoesNotTerminateRunning(t *testing.T) {
	r, s := newTestReaper(t)

	sess := &store.Session{
		SessionID: "running-1",
		MachineID: "m1",
		UserID:    "u1",
		Command:   "claude",
	}
	if err := s.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	s.UpdateSessionStatus("running-1", store.StatusRunning)

	r.sweep(context.Background())

	result, _ := s.GetSession("running-1")
	if result.Status != store.StatusRunning {
		t.Fatalf("expected running session to remain running, got %s", result.Status)
	}
}

func TestReaper_RespectsDisabledTimeout(t *testing.T) {
	r, s := newTestReaper(t)

	sess := &store.Session{
		SessionID: "s1",
		MachineID: "m1",
		UserID:    "u1",
		Command:   "claude",
	}
	if err := s.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	s.UpdateSessionStatus("s1", store.StatusWaitingForInput)

	// Set user timeout to 0 (disabled).
	s.UpsertUserPreferences(context.Background(), "u1", `{"session_stale_timeout": 0}`)

	r.sweep(context.Background())

	result, _ := s.GetSession("s1")
	if result.Status != store.StatusWaitingForInput {
		t.Fatalf("expected session to remain waiting_for_input with timeout=0, got %s", result.Status)
	}
}

func TestReaper_LoadUserTimeout_Default(t *testing.T) {
	r, _ := newTestReaper(t)
	timeout := r.loadUserTimeout(context.Background(), "nonexistent-user")
	if timeout != DefaultStaleTimeout {
		t.Fatalf("expected default timeout %d, got %d", DefaultStaleTimeout, timeout)
	}
}

func TestReaper_LoadUserTimeout_CustomValue(t *testing.T) {
	r, s := newTestReaper(t)
	s.UpsertUserPreferences(context.Background(), "u1", `{"session_stale_timeout": 60}`)

	timeout := r.loadUserTimeout(context.Background(), "u1")
	if timeout != 60 {
		t.Fatalf("expected timeout 60, got %d", timeout)
	}
}

func TestReaper_LoadUserTimeout_NullValue(t *testing.T) {
	r, s := newTestReaper(t)
	s.UpsertUserPreferences(context.Background(), "u1", `{"skip_permissions": true}`)

	timeout := r.loadUserTimeout(context.Background(), "u1")
	if timeout != DefaultStaleTimeout {
		t.Fatalf("expected default timeout when field absent, got %d", timeout)
	}
}

func TestReaper_SweepInterval(t *testing.T) {
	r, _ := newTestReaper(t)
	if r.interval != DefaultSweepInterval {
		t.Fatalf("expected interval %v, got %v", DefaultSweepInterval, r.interval)
	}
	if DefaultSweepInterval != 15*time.Minute {
		t.Fatalf("expected default sweep interval to be 15 minutes")
	}
}
