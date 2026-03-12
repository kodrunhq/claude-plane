package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRevokeAndLoad(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Revoke two tokens
	exp1 := time.Now().Add(1 * time.Hour)
	exp2 := time.Now().Add(2 * time.Hour)

	if err := s.RevokeToken("jti-1", "user-a", exp1); err != nil {
		t.Fatalf("RevokeToken 1: %v", err)
	}
	if err := s.RevokeToken("jti-2", "user-b", exp2); err != nil {
		t.Fatalf("RevokeToken 2: %v", err)
	}

	// Load active revocations
	revoked, err := s.LoadActiveRevocations()
	if err != nil {
		t.Fatalf("LoadActiveRevocations: %v", err)
	}

	if len(revoked) != 2 {
		t.Fatalf("expected 2 revocations, got %d", len(revoked))
	}

	// Verify fields
	found := map[string]bool{}
	for _, r := range revoked {
		found[r.JTI] = true
		if r.UserID == "" {
			t.Errorf("revocation %s has empty UserID", r.JTI)
		}
		if r.ExpiresAt.IsZero() {
			t.Errorf("revocation %s has zero ExpiresAt", r.JTI)
		}
	}
	if !found["jti-1"] || !found["jti-2"] {
		t.Errorf("missing expected JTIs: %v", found)
	}
}

func TestCleanExpired(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	now := time.Now()

	// Insert one expired and one active revocation
	if err := s.RevokeToken("expired-jti", "user-a", now.Add(-1*time.Hour)); err != nil {
		t.Fatalf("RevokeToken expired: %v", err)
	}
	if err := s.RevokeToken("active-jti", "user-b", now.Add(1*time.Hour)); err != nil {
		t.Fatalf("RevokeToken active: %v", err)
	}

	// Clean expired entries
	if err := s.CleanExpired(now); err != nil {
		t.Fatalf("CleanExpired: %v", err)
	}

	// Only active should remain
	revoked, err := s.LoadActiveRevocations()
	if err != nil {
		t.Fatalf("LoadActiveRevocations: %v", err)
	}

	if len(revoked) != 1 {
		t.Fatalf("expected 1 revocation after cleanup, got %d", len(revoked))
	}
	if revoked[0].JTI != "active-jti" {
		t.Errorf("expected active-jti, got %s", revoked[0].JTI)
	}
}
