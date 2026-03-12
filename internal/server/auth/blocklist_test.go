package auth

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/claudeplane/claude-plane/internal/server/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewBlocklistLoadsFromDB(t *testing.T) {
	s := newTestStore(t)

	// Persist 2 revocations directly to the store
	exp := time.Now().Add(1 * time.Hour)
	if err := s.RevokeToken("jti-from-db-1", "user-a", exp); err != nil {
		t.Fatalf("RevokeToken 1: %v", err)
	}
	if err := s.RevokeToken("jti-from-db-2", "user-b", exp); err != nil {
		t.Fatalf("RevokeToken 2: %v", err)
	}

	// Create blocklist — should load both from DB
	bl, err := NewBlocklist(s)
	if err != nil {
		t.Fatalf("NewBlocklist: %v", err)
	}

	if !bl.IsRevoked("jti-from-db-1") {
		t.Error("jti-from-db-1 should be revoked")
	}
	if !bl.IsRevoked("jti-from-db-2") {
		t.Error("jti-from-db-2 should be revoked")
	}
}

func TestBlocklistAdd(t *testing.T) {
	s := newTestStore(t)
	bl, err := NewBlocklist(s)
	if err != nil {
		t.Fatalf("NewBlocklist: %v", err)
	}

	exp := time.Now().Add(1 * time.Hour)
	if err := bl.Add("new-jti", exp); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if !bl.IsRevoked("new-jti") {
		t.Error("new-jti should be revoked after Add")
	}

	// Verify it was persisted to DB
	revoked, err := s.LoadActiveRevocations()
	if err != nil {
		t.Fatalf("LoadActiveRevocations: %v", err)
	}
	found := false
	for _, r := range revoked {
		if r.JTI == "new-jti" {
			found = true
			break
		}
	}
	if !found {
		t.Error("new-jti should be persisted in DB")
	}
}

func TestBlocklistNotRevoked(t *testing.T) {
	s := newTestStore(t)
	bl, err := NewBlocklist(s)
	if err != nil {
		t.Fatalf("NewBlocklist: %v", err)
	}

	if bl.IsRevoked("unknown-jti") {
		t.Error("unknown-jti should not be revoked")
	}
}

func TestBlocklistCleanup(t *testing.T) {
	s := newTestStore(t)
	bl, err := NewBlocklist(s)
	if err != nil {
		t.Fatalf("NewBlocklist: %v", err)
	}

	// Add a JTI with a past expiry
	pastExp := time.Now().Add(-1 * time.Second)
	if err := bl.Add("expired-jti", pastExp); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Add a JTI with a future expiry
	futureExp := time.Now().Add(1 * time.Hour)
	if err := bl.Add("future-jti", futureExp); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Start cleanup with very short interval
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bl.StartCleanup(ctx, 10*time.Millisecond)

	// Wait for cleanup to run
	time.Sleep(50 * time.Millisecond)
	cancel()

	// expired-jti should be cleaned, future-jti should remain
	if bl.IsRevoked("expired-jti") {
		t.Error("expired-jti should have been cleaned up")
	}
	if !bl.IsRevoked("future-jti") {
		t.Error("future-jti should still be revoked")
	}
}

func TestBlocklistConcurrency(t *testing.T) {
	s := newTestStore(t)
	bl, err := NewBlocklist(s)
	if err != nil {
		t.Fatalf("NewBlocklist: %v", err)
	}

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n * 2)

	// Launch n goroutines doing Add
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			jti := "concurrent-" + time.Now().Format("150405.000000000") + "-" + string(rune('a'+i%26))
			_ = bl.Add(jti, time.Now().Add(1*time.Hour))
		}(i)
	}

	// Launch n goroutines doing IsRevoked
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_ = bl.IsRevoked("some-jti")
		}(i)
	}

	wg.Wait()
	// If we get here without race detector triggering, the test passes.
}
