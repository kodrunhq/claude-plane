package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStoreForInjections(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "injections_test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedInjectionDeps creates a user, machine, and session for FK constraints
// and returns the session_id and user_id.
func seedInjectionDeps(t *testing.T, s *Store, suffix string) (sessionID, userID string) {
	t.Helper()

	userID = "user-inj-" + suffix
	hash, err := HashPassword("password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	err = s.CreateUser(&User{
		UserID:       userID,
		Email:        suffix + "@test.com",
		DisplayName:  "Test User",
		PasswordHash: hash,
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	machineID := "machine-inj-" + suffix
	err = s.UpsertMachine(machineID, 5, "")
	if err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}

	sessionID = "session-inj-" + suffix
	err = s.CreateSession(&Session{
		SessionID: sessionID,
		MachineID: machineID,
		UserID:    userID,
		Command:   "claude",
		Status:    "running",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	return sessionID, userID
}

func TestCreateInjection(t *testing.T) {
	s := newTestStoreForInjections(t)
	ctx := context.Background()
	sessionID, userID := seedInjectionDeps(t, s, "create")

	inj := &Injection{
		SessionID:  sessionID,
		UserID:     userID,
		TextLength: 42,
		Metadata:   `{"key":"value"}`,
		Source:     "workbench",
	}

	created, err := s.CreateInjection(ctx, inj)
	if err != nil {
		t.Fatalf("CreateInjection: %v", err)
	}
	if created.InjectionID == "" {
		t.Fatal("expected non-empty InjectionID")
	}
	if created.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", created.SessionID, sessionID)
	}
	if created.UserID != userID {
		t.Errorf("UserID = %q, want %q", created.UserID, userID)
	}
	if created.TextLength != 42 {
		t.Errorf("TextLength = %d, want 42", created.TextLength)
	}
	if created.Metadata != `{"key":"value"}` {
		t.Errorf("Metadata = %q, want %q", created.Metadata, `{"key":"value"}`)
	}
	if created.Source != "workbench" {
		t.Errorf("Source = %q, want %q", created.Source, "workbench")
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if created.DeliveredAt != nil {
		t.Error("DeliveredAt should be nil")
	}
}

func TestUpdateInjectionDelivered(t *testing.T) {
	s := newTestStoreForInjections(t)
	ctx := context.Background()
	sessionID, userID := seedInjectionDeps(t, s, "delivered")

	created, err := s.CreateInjection(ctx, &Injection{
		SessionID:  sessionID,
		UserID:     userID,
		TextLength: 10,
		Source:     "api",
	})
	if err != nil {
		t.Fatalf("CreateInjection: %v", err)
	}

	deliveredAt := time.Now().UTC().Truncate(time.Second)
	err = s.UpdateInjectionDelivered(ctx, created.InjectionID, deliveredAt)
	if err != nil {
		t.Fatalf("UpdateInjectionDelivered: %v", err)
	}

	// Verify delivered_at persists by listing
	list, err := s.ListInjectionsBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListInjectionsBySession: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 injection, got %d", len(list))
	}
	if list[0].DeliveredAt == nil {
		t.Fatal("DeliveredAt should not be nil after update")
	}
	if !list[0].DeliveredAt.Truncate(time.Second).Equal(deliveredAt) {
		t.Errorf("DeliveredAt = %v, want %v", list[0].DeliveredAt, deliveredAt)
	}
}

func TestUpdateInjectionDelivered_NotFound(t *testing.T) {
	s := newTestStoreForInjections(t)
	ctx := context.Background()

	err := s.UpdateInjectionDelivered(ctx, "nonexistent-id", time.Now().UTC())
	if err == nil {
		t.Fatal("expected error for nonexistent injection, got nil")
	}
}

func TestListInjectionsBySession(t *testing.T) {
	s := newTestStoreForInjections(t)
	ctx := context.Background()

	sessionA, userID := seedInjectionDeps(t, s, "listA")
	sessionB, _ := seedInjectionDeps(t, s, "listB")

	// Create 3 injections for session A with staggered times
	for i := 0; i < 3; i++ {
		_, err := s.CreateInjection(ctx, &Injection{
			SessionID:  sessionA,
			UserID:     userID,
			TextLength: (i + 1) * 10,
			Source:     "workbench",
		})
		if err != nil {
			t.Fatalf("CreateInjection A[%d]: %v", i, err)
		}
		// Small pause to ensure distinct created_at ordering
		time.Sleep(10 * time.Millisecond)
	}

	// Create 1 injection for session B
	_, err := s.CreateInjection(ctx, &Injection{
		SessionID:  sessionB,
		UserID:     userID,
		TextLength: 99,
		Source:     "api",
	})
	if err != nil {
		t.Fatalf("CreateInjection B: %v", err)
	}

	// List session A — should return 3, ordered by created_at DESC
	listA, err := s.ListInjectionsBySession(ctx, sessionA)
	if err != nil {
		t.Fatalf("ListInjectionsBySession A: %v", err)
	}
	if len(listA) != 3 {
		t.Fatalf("session A count = %d, want 3", len(listA))
	}

	// Verify DESC ordering: most recent first (highest TextLength last created)
	if listA[0].TextLength != 30 {
		t.Errorf("listA[0].TextLength = %d, want 30 (most recent)", listA[0].TextLength)
	}
	if listA[2].TextLength != 10 {
		t.Errorf("listA[2].TextLength = %d, want 10 (oldest)", listA[2].TextLength)
	}

	// List session B — should return 1
	listB, err := s.ListInjectionsBySession(ctx, sessionB)
	if err != nil {
		t.Fatalf("ListInjectionsBySession B: %v", err)
	}
	if len(listB) != 1 {
		t.Errorf("session B count = %d, want 1", len(listB))
	}

	// List non-existent session — should return empty
	listNone, err := s.ListInjectionsBySession(ctx, "nonexistent-session")
	if err != nil {
		t.Fatalf("ListInjectionsBySession nonexistent: %v", err)
	}
	if len(listNone) != 0 {
		t.Errorf("nonexistent session count = %d, want 0", len(listNone))
	}
}
