package store

import (
	"testing"
	"time"
)

func TestSessionCRUD(t *testing.T) {
	s := mustNewStore(t)
	machineID := mustCreateMachine(t, s)

	sess := mustCreateSession(t, s, machineID,
		WithSessionID("sess-001"),
		WithSessionCommand("claude"),
		WithSessionWorkingDir("/tmp"),
		WithSessionStatus(StatusCreated),
	)

	// Get
	got, err := s.GetSession(sess.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.SessionID != "sess-001" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "sess-001")
	}
	if got.MachineID != machineID {
		t.Errorf("MachineID = %q, want %q", got.MachineID, machineID)
	}
	if got.UserID != "" {
		t.Errorf("UserID = %q, want %q", got.UserID, "")
	}
	if got.Command != "claude" {
		t.Errorf("Command = %q, want %q", got.Command, "claude")
	}
	if got.WorkingDir != "/tmp" {
		t.Errorf("WorkingDir = %q, want %q", got.WorkingDir, "/tmp")
	}
	if got.Status != StatusCreated {
		t.Errorf("Status = %q, want %q", got.Status, StatusCreated)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	// Update status
	if err := s.UpdateSessionStatus("sess-001", StatusRunning); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	got, err = s.GetSession("sess-001")
	if err != nil {
		t.Fatalf("GetSession after update: %v", err)
	}
	if got.Status != StatusRunning {
		t.Errorf("Status after update = %q, want %q", got.Status, StatusRunning)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero after status update")
	}
	if got.UpdatedAt.Before(got.CreatedAt) {
		t.Errorf("UpdatedAt %v should not be before CreatedAt %v", got.UpdatedAt, got.CreatedAt)
	}

	// Delete
	if err := s.DeleteSession("sess-001"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	_, err = s.GetSession("sess-001")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestListSessions(t *testing.T) {
	s := mustNewStore(t)
	machineID := mustCreateMachine(t, s)

	for range 3 {
		mustCreateSession(t, s, machineID)
	}

	sessions, err := s.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("ListSessions count = %d, want 3", len(sessions))
	}
}

func TestCreateSession_PersistsMetadata(t *testing.T) {
	s := mustNewStore(t)
	machineID := mustCreateMachine(t, s)

	sess := mustCreateSession(t, s, machineID,
		WithSessionModel("opus"),
		WithSessionSkipPerms("true"),
		WithSessionEnvVars(`{"ANTHROPIC_API_KEY":"test-placeholder-not-a-real-key","DEBUG":"1"}`),
		WithSessionArgs(`["--verbose","--no-cache"]`),
		WithSessionInitialPrompt("Fix the login bug"),
		WithSessionWorkingDir("/home/user/project"),
	)

	got, err := s.GetSession(sess.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	if got.Model != "opus" {
		t.Errorf("Model = %q, want %q", got.Model, "opus")
	}
	if got.SkipPerms != "true" {
		t.Errorf("SkipPerms = %q, want %q", got.SkipPerms, "true")
	}
	if got.EnvVars != `{"ANTHROPIC_API_KEY":"test-placeholder-not-a-real-key","DEBUG":"1"}` {
		t.Errorf("EnvVars = %q, want %q", got.EnvVars, `{"ANTHROPIC_API_KEY":"test-placeholder-not-a-real-key","DEBUG":"1"}`)
	}
	if got.Args != `["--verbose","--no-cache"]` {
		t.Errorf("Args = %q, want %q", got.Args, `["--verbose","--no-cache"]`)
	}
	if got.InitialPrompt != "Fix the login bug" {
		t.Errorf("InitialPrompt = %q, want %q", got.InitialPrompt, "Fix the login bug")
	}

	// Verify metadata also comes through in ListSessions
	sessions, err := s.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("ListSessions count = %d, want 1", len(sessions))
	}
	if sessions[0].Model != "opus" {
		t.Errorf("ListSessions[0].Model = %q, want %q", sessions[0].Model, "opus")
	}
	if sessions[0].Args != `["--verbose","--no-cache"]` {
		t.Errorf("ListSessions[0].Args = %q, want %q", sessions[0].Args, `["--verbose","--no-cache"]`)
	}
}

func TestListSessionsByMachine(t *testing.T) {
	s := mustNewStore(t)
	machineA := mustCreateMachine(t, s, WithMachineID("machine-a"))
	machineB := mustCreateMachine(t, s, WithMachineID("machine-b"))

	// Create 2 sessions on machine-a, 1 on machine-b
	mustCreateSession(t, s, machineA)
	mustCreateSession(t, s, machineA)
	mustCreateSession(t, s, machineB)

	sessions, err := s.ListSessionsByMachine("machine-a")
	if err != nil {
		t.Fatalf("ListSessionsByMachine: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("ListSessionsByMachine count = %d, want 2", len(sessions))
	}

	sessions, err = s.ListSessionsByMachine("machine-b")
	if err != nil {
		t.Fatalf("ListSessionsByMachine: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("ListSessionsByMachine count = %d, want 1", len(sessions))
	}
}

func TestUpdateSessionStatus_SetsUpdatedAt(t *testing.T) {
	s := mustNewStore(t)
	machineID := mustCreateMachine(t, s)

	sess := &Session{
		SessionID: "sess-upd-001",
		MachineID: machineID,
		Command:   "claude",
		Status:    StatusRunning,
	}
	if err := s.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// CURRENT_TIMESTAMP in SQLite has second-level granularity, so we
	// must sleep long enough to land in a different second.
	time.Sleep(1100 * time.Millisecond)

	if err := s.UpdateSessionStatus("sess-upd-001", StatusWaitingForInput); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSession("sess-upd-001")
	if err != nil {
		t.Fatal(err)
	}

	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("UpdatedAt (%v) should be after CreatedAt (%v)", got.UpdatedAt, got.CreatedAt)
	}
	if got.EndedAt != nil {
		t.Error("EndedAt should be nil for non-terminal status")
	}

	time.Sleep(1100 * time.Millisecond)
	if err := s.UpdateSessionStatus("sess-upd-001", StatusCompleted); err != nil {
		t.Fatal(err)
	}

	got, err = s.GetSession("sess-upd-001")
	if err != nil {
		t.Fatal(err)
	}

	if got.EndedAt == nil {
		t.Error("EndedAt should be set for terminal status")
	}
}
