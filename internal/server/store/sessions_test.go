package store

import (
	"path/filepath"
	"testing"
)

func TestSessionCRUD(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Create machine for foreign key constraint
	if err := s.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}

	sess := &Session{
		SessionID:  "sess-001",
		MachineID:  "machine-a",
		UserID:     "",
		Command:    "claude",
		WorkingDir: "/tmp",
		Status:     StatusCreated,
	}

	// Create
	if err := s.CreateSession(sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Get
	got, err := s.GetSession("sess-001")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.SessionID != "sess-001" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "sess-001")
	}
	if got.MachineID != "machine-a" {
		t.Errorf("MachineID = %q, want %q", got.MachineID, "machine-a")
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
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Need a machine for foreign key
	if err := s.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}

	for i := range 3 {
		sess := &Session{
			SessionID:  "sess-" + string(rune('a'+i)),
			MachineID:  "machine-a",
			UserID:     "",
			Command:    "claude",
			WorkingDir: "/tmp",
			Status:     StatusCreated,
		}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("CreateSession %d: %v", i, err)
		}
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
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Create machine for foreign key constraint
	if err := s.UpsertMachine("machine-meta", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}

	sess := &Session{
		SessionID:     "sess-meta-001",
		MachineID:     "machine-meta",
		UserID:        "",
		Command:       "claude",
		WorkingDir:    "/home/user/project",
		Status:        StatusCreated,
		Model:         "opus",
		SkipPerms:     "true",
		EnvVars:       `{"ANTHROPIC_API_KEY":"sk-test","DEBUG":"1"}`,
		Args:          `["--verbose","--no-cache"]`,
		InitialPrompt: "Fix the login bug",
	}

	if err := s.CreateSession(sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := s.GetSession("sess-meta-001")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	if got.Model != "opus" {
		t.Errorf("Model = %q, want %q", got.Model, "opus")
	}
	if got.SkipPerms != "true" {
		t.Errorf("SkipPerms = %q, want %q", got.SkipPerms, "true")
	}
	if got.EnvVars != `{"ANTHROPIC_API_KEY":"sk-test","DEBUG":"1"}` {
		t.Errorf("EnvVars = %q, want %q", got.EnvVars, `{"ANTHROPIC_API_KEY":"sk-test","DEBUG":"1"}`)
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
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Create two machines
	if err := s.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine a: %v", err)
	}
	if err := s.UpsertMachine("machine-b", 5); err != nil {
		t.Fatalf("UpsertMachine b: %v", err)
	}

	// Create sessions on both machines
	for i, mid := range []string{"machine-a", "machine-a", "machine-b"} {
		sess := &Session{
			SessionID:  "sess-" + string(rune('a'+i)),
			MachineID:  mid,
			UserID:     "",
			Command:    "claude",
			WorkingDir: "/tmp",
			Status:     StatusCreated,
		}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("CreateSession %d: %v", i, err)
		}
	}

	// Filter by machine-a
	sessions, err := s.ListSessionsByMachine("machine-a")
	if err != nil {
		t.Fatalf("ListSessionsByMachine: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("ListSessionsByMachine count = %d, want 2", len(sessions))
	}

	// Filter by machine-b
	sessions, err = s.ListSessionsByMachine("machine-b")
	if err != nil {
		t.Fatalf("ListSessionsByMachine: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("ListSessionsByMachine count = %d, want 1", len(sessions))
	}
}
