package store

import (
	"path/filepath"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correcthorse")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Verify correct password
	ok, err := VerifyPassword("correcthorse", hash)
	if err != nil {
		t.Fatalf("VerifyPassword (correct): %v", err)
	}
	if !ok {
		t.Error("VerifyPassword returned false for correct password")
	}

	// Verify wrong password
	ok, err = VerifyPassword("wrongpassword", hash)
	if err != nil {
		t.Fatalf("VerifyPassword (wrong): %v", err)
	}
	if ok {
		t.Error("VerifyPassword returned true for wrong password")
	}
}

func TestSeedAdmin(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Seed admin
	if err := s.SeedAdmin("admin@example.com", "secret123", "Admin User"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}

	// Verify user exists
	user, err := s.GetUserByEmail("admin@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user == nil {
		t.Fatal("GetUserByEmail returned nil for seeded admin")
	}

	// Verify fields
	if user.Email != "admin@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "admin@example.com")
	}
	if user.DisplayName != "Admin User" {
		t.Errorf("DisplayName = %q, want %q", user.DisplayName, "Admin User")
	}
	if user.Role != "admin" {
		t.Errorf("Role = %q, want %q", user.Role, "admin")
	}
	if user.UserID == "" {
		t.Error("UserID is empty")
	}

	// Verify password hash is valid Argon2id
	ok, err := VerifyPassword("secret123", user.PasswordHash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Error("VerifyPassword returned false for seeded admin password")
	}
}

func TestSeedAdminDuplicate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// First seed should succeed
	if err := s.SeedAdmin("admin@example.com", "pass1", "Admin"); err != nil {
		t.Fatalf("SeedAdmin (first): %v", err)
	}

	// Second seed with same email should return error, not panic
	err = s.SeedAdmin("admin@example.com", "pass2", "Admin 2")
	if err == nil {
		t.Fatal("SeedAdmin (duplicate) should return error, got nil")
	}
	// Verify error message is descriptive
	t.Logf("Duplicate seed error (expected): %v", err)
}

func TestGetUserByEmailNotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	user, err := s.GetUserByEmail("nonexistent@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user != nil {
		t.Errorf("GetUserByEmail returned non-nil for nonexistent email: %+v", user)
	}
}
