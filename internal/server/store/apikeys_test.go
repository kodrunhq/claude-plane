package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStoreForAPIKeys(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func createTestUserForAPIKeys(t *testing.T, s *Store, email string) string {
	t.Helper()
	id := "user-apikeys-" + email
	hash, err := HashPassword("password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	err = s.CreateUser(&User{
		UserID:       id,
		Email:        email,
		DisplayName:  "Test User",
		PasswordHash: hash,
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return id
}

var testSigningKey = []byte("test-signing-key-for-hmac-sha256")

func TestCreateAPIKey(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()
	userID := createTestUserForAPIKeys(t, s, "alice@apikeys.com")

	plaintextKey, keyID, err := s.CreateAPIKey(ctx, userID, "my-key", []string{"read", "write"}, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	if !strings.HasPrefix(plaintextKey, "cpk_") {
		t.Errorf("plaintext key = %q, want prefix cpk_", plaintextKey)
	}
	if keyID == "" {
		t.Error("keyID should not be empty")
	}

	// Verify the key ID is embedded in the plaintext key.
	// Format: cpk_{8-char-keyid}_{random}
	parts := strings.SplitN(plaintextKey, "_", 3)
	if len(parts) != 3 {
		t.Fatalf("plaintext key format invalid: %q (expected 3 parts separated by _)", plaintextKey)
	}
	if parts[0] != "cpk" {
		t.Errorf("prefix = %q, want cpk", parts[0])
	}
	if len(parts[1]) != 8 {
		t.Errorf("key ID prefix length = %d, want 8", len(parts[1]))
	}
	if parts[2] == "" {
		t.Error("random part should not be empty")
	}
}

func TestGetAPIKeyByID(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()
	userID := createTestUserForAPIKeys(t, s, "bob@apikeys.com")

	_, keyID, err := s.CreateAPIKey(ctx, userID, "get-test-key", []string{"read"}, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	key, err := s.GetAPIKeyByID(ctx, keyID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID: %v", err)
	}

	if key.KeyID != keyID {
		t.Errorf("KeyID = %q, want %q", key.KeyID, keyID)
	}
	if key.UserID != userID {
		t.Errorf("UserID = %q, want %q", key.UserID, userID)
	}
	if key.Name != "get-test-key" {
		t.Errorf("Name = %q, want %q", key.Name, "get-test-key")
	}
	if len(key.Scopes) != 1 || key.Scopes[0] != "read" {
		t.Errorf("Scopes = %v, want [read]", key.Scopes)
	}
	if key.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if key.ExpiresAt != nil {
		t.Error("ExpiresAt should be nil when not set")
	}
	if key.LastUsedAt != nil {
		t.Error("LastUsedAt should be nil initially")
	}
}

func TestGetAPIKeyByID_NotFound(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()

	_, err := s.GetAPIKeyByID(ctx, "nonexistent-key-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestValidateAPIKey(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()
	userID := createTestUserForAPIKeys(t, s, "carol@apikeys.com")

	plaintextKey, keyID, err := s.CreateAPIKey(ctx, userID, "validate-key", []string{"admin"}, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	apiKey, err := s.ValidateAPIKey(ctx, plaintextKey, testSigningKey)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}

	if apiKey.KeyID != keyID {
		t.Errorf("KeyID = %q, want %q", apiKey.KeyID, keyID)
	}
	if apiKey.UserID != userID {
		t.Errorf("UserID = %q, want %q", apiKey.UserID, userID)
	}
	if len(apiKey.Scopes) != 1 || apiKey.Scopes[0] != "admin" {
		t.Errorf("Scopes = %v, want [admin]", apiKey.Scopes)
	}
}

func TestValidateAPIKey_Invalid(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()
	userID := createTestUserForAPIKeys(t, s, "dave@apikeys.com")

	plaintextKey, _, err := s.CreateAPIKey(ctx, userID, "tampered-key", nil, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Tamper with the random part of the key.
	parts := strings.SplitN(plaintextKey, "_", 3)
	tamperedKey := parts[0] + "_" + parts[1] + "_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	_, err = s.ValidateAPIKey(ctx, tamperedKey, testSigningKey)
	if err == nil {
		t.Fatal("expected error for tampered key, got nil")
	}
}

func TestValidateAPIKey_WrongFormat(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()

	_, err := s.ValidateAPIKey(ctx, "not-a-valid-key", testSigningKey)
	if err == nil {
		t.Fatal("expected error for invalid key format, got nil")
	}
}

func TestValidateAPIKey_NonExistentKey(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()

	// Valid format but key doesn't exist in DB.
	_, err := s.ValidateAPIKey(ctx, "cpk_abcdef12_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", testSigningKey)
	if err == nil {
		t.Fatal("expected error for non-existent key, got nil")
	}
}

func TestListAPIKeys(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()
	userA := createTestUserForAPIKeys(t, s, "listA@apikeys.com")
	userB := createTestUserForAPIKeys(t, s, "listB@apikeys.com")

	_, _, err := s.CreateAPIKey(ctx, userA, "key-1", []string{"read"}, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey 1: %v", err)
	}
	_, _, err = s.CreateAPIKey(ctx, userA, "key-2", nil, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey 2: %v", err)
	}
	_, _, err = s.CreateAPIKey(ctx, userB, "key-3", []string{"write"}, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey 3: %v", err)
	}

	keysA, err := s.ListAPIKeys(ctx, userA)
	if err != nil {
		t.Fatalf("ListAPIKeys userA: %v", err)
	}
	if len(keysA) != 2 {
		t.Errorf("userA key count = %d, want 2", len(keysA))
	}

	keysB, err := s.ListAPIKeys(ctx, userB)
	if err != nil {
		t.Fatalf("ListAPIKeys userB: %v", err)
	}
	if len(keysB) != 1 {
		t.Errorf("userB key count = %d, want 1", len(keysB))
	}
}

func TestDeleteAPIKey(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()
	userID := createTestUserForAPIKeys(t, s, "eve@apikeys.com")

	plaintextKey, keyID, err := s.CreateAPIKey(ctx, userID, "delete-me", nil, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	err = s.DeleteAPIKey(ctx, keyID)
	if err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}

	// Subsequent validate should fail.
	_, err = s.ValidateAPIKey(ctx, plaintextKey, testSigningKey)
	if err == nil {
		t.Fatal("expected error after deleting key, got nil")
	}

	// GetAPIKeyByID should return ErrNotFound.
	_, err = s.GetAPIKeyByID(ctx, keyID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestDeleteAPIKey_NotFound(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()

	err := s.DeleteAPIKey(ctx, "nonexistent-key-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestUpdateAPIKeyLastUsed(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()
	userID := createTestUserForAPIKeys(t, s, "frank@apikeys.com")

	_, keyID, err := s.CreateAPIKey(ctx, userID, "last-used-key", nil, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Initially last_used_at should be nil.
	key, err := s.GetAPIKeyByID(ctx, keyID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID: %v", err)
	}
	if key.LastUsedAt != nil {
		t.Error("LastUsedAt should be nil before first use")
	}

	err = s.UpdateAPIKeyLastUsed(ctx, keyID)
	if err != nil {
		t.Fatalf("UpdateAPIKeyLastUsed: %v", err)
	}

	key, err = s.GetAPIKeyByID(ctx, keyID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID after update: %v", err)
	}
	if key.LastUsedAt == nil {
		t.Error("LastUsedAt should be set after UpdateAPIKeyLastUsed")
	}
}

func TestValidateAPIKey_Expired(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()
	userID := createTestUserForAPIKeys(t, s, "grace@apikeys.com")

	// Create a key, then manually set its expiry to the past via direct DB access.
	plaintextKey, keyID, err := s.CreateAPIKey(ctx, userID, "expiring-key", nil, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Set expires_at to a time in the past.
	pastTime := time.Now().UTC().Add(-1 * time.Hour)
	_, err = s.writer.ExecContext(ctx,
		`UPDATE api_keys SET expires_at = ? WHERE key_id = ?`,
		pastTime, keyID,
	)
	if err != nil {
		t.Fatalf("set expires_at: %v", err)
	}

	_, err = s.ValidateAPIKey(ctx, plaintextKey, testSigningKey)
	if err == nil {
		t.Fatal("expected error for expired key, got nil")
	}
}

func TestAPIKey_NilScopes(t *testing.T) {
	s := newTestStoreForAPIKeys(t)
	ctx := context.Background()
	userID := createTestUserForAPIKeys(t, s, "hank@apikeys.com")

	_, keyID, err := s.CreateAPIKey(ctx, userID, "no-scopes-key", nil, testSigningKey)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	key, err := s.GetAPIKeyByID(ctx, keyID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID: %v", err)
	}
	if key.Scopes != nil {
		t.Errorf("Scopes = %v, want nil", key.Scopes)
	}
}
