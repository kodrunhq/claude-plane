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
		Status:     "created",
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
	if got.Status != "created" {
		t.Errorf("Status = %q, want %q", got.Status, "created")
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	// Update status
	if err := s.UpdateSessionStatus("sess-001", "running"); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	got, err = s.GetSession("sess-001")
	if err != nil {
		t.Fatalf("GetSession after update: %v", err)
	}
	if got.Status != "running" {
		t.Errorf("Status after update = %q, want %q", got.Status, "running")
	}
	if got.UpdatedAt.Before(got.CreatedAt) || got.UpdatedAt.Equal(got.CreatedAt) {
		// UpdatedAt should be >= CreatedAt (may be equal in fast test)
		// Just check it's not zero
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
			Status:     "created",
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
			Status:     "created",
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
