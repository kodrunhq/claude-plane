package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertMachine(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertMachine("m-001", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}

	m, err := s.GetMachine("m-001")
	if err != nil {
		t.Fatalf("GetMachine: %v", err)
	}
	if m == nil {
		t.Fatal("GetMachine returned nil")
	}
	if m.MachineID != "m-001" {
		t.Errorf("MachineID = %q, want %q", m.MachineID, "m-001")
	}
	if m.MaxSessions != 5 {
		t.Errorf("MaxSessions = %d, want %d", m.MaxSessions, 5)
	}
	if m.Status != "disconnected" {
		t.Errorf("Status = %q, want %q", m.Status, "disconnected")
	}
}

func TestUpsertMachineUpdate(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertMachine("m-001", 5); err != nil {
		t.Fatalf("UpsertMachine (first): %v", err)
	}
	if err := s.UpsertMachine("m-001", 10); err != nil {
		t.Fatalf("UpsertMachine (second): %v", err)
	}

	m, err := s.GetMachine("m-001")
	if err != nil {
		t.Fatalf("GetMachine: %v", err)
	}
	if m.MaxSessions != 10 {
		t.Errorf("MaxSessions = %d, want %d after update", m.MaxSessions, 10)
	}
}

func TestUpdateMachineStatus(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertMachine("m-001", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	if err := s.UpdateMachineStatus("m-001", "connected", now); err != nil {
		t.Fatalf("UpdateMachineStatus: %v", err)
	}

	m, err := s.GetMachine("m-001")
	if err != nil {
		t.Fatalf("GetMachine: %v", err)
	}
	if m.Status != "connected" {
		t.Errorf("Status = %q, want %q", m.Status, "connected")
	}
	if m.LastSeenAt == nil {
		t.Fatal("LastSeenAt is nil after update")
	}
	if !m.LastSeenAt.Truncate(time.Second).Equal(now) {
		t.Errorf("LastSeenAt = %v, want %v", m.LastSeenAt.Truncate(time.Second), now)
	}
}

func TestListMachines(t *testing.T) {
	s := newTestStore(t)

	for _, id := range []string{"m-003", "m-001", "m-002"} {
		if err := s.UpsertMachine(id, 5); err != nil {
			t.Fatalf("UpsertMachine(%q): %v", id, err)
		}
	}

	machines, err := s.ListMachines()
	if err != nil {
		t.Fatalf("ListMachines: %v", err)
	}
	if len(machines) != 3 {
		t.Fatalf("len(machines) = %d, want 3", len(machines))
	}
	// Verify ordering by machine_id
	if machines[0].MachineID != "m-001" {
		t.Errorf("machines[0].MachineID = %q, want %q", machines[0].MachineID, "m-001")
	}
	if machines[1].MachineID != "m-002" {
		t.Errorf("machines[1].MachineID = %q, want %q", machines[1].MachineID, "m-002")
	}
	if machines[2].MachineID != "m-003" {
		t.Errorf("machines[2].MachineID = %q, want %q", machines[2].MachineID, "m-003")
	}
}

func TestUpdateMachineDisplayName(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertMachine("m-001", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}

	if err := s.UpdateMachineDisplayName("m-001", "My Worker"); err != nil {
		t.Fatalf("UpdateMachineDisplayName: %v", err)
	}

	m, err := s.GetMachine("m-001")
	if err != nil {
		t.Fatalf("GetMachine: %v", err)
	}
	if m.DisplayName != "My Worker" {
		t.Errorf("DisplayName = %q, want %q", m.DisplayName, "My Worker")
	}
}

func TestUpdateMachineDisplayNameNotFound(t *testing.T) {
	s := newTestStore(t)

	err := s.UpdateMachineDisplayName("nonexistent", "Name")
	if err == nil {
		t.Fatal("expected error for non-existent machine, got nil")
	}
	if err != ErrMachineNotFound {
		t.Errorf("expected ErrMachineNotFound, got %v", err)
	}
}

func TestGetMachineNotFound(t *testing.T) {
	s := newTestStore(t)

	m, err := s.GetMachine("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent machine, got nil")
	}
	if m != nil {
		t.Errorf("expected nil machine, got %+v", m)
	}
}
