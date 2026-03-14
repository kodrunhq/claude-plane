package store

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// newTestStoreForBridge creates a temp-dir Store and registers cleanup.
func newTestStoreForBridge(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// createTestUserForBridge inserts a user and returns its user_id.
func createTestUserForBridge(t *testing.T, s *Store, email string) string {
	t.Helper()
	id := "user-bridge-" + email
	hash, err := HashPassword("password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	err = s.CreateUser(&User{
		UserID:       id,
		Email:        email,
		DisplayName:  "Bridge Test User",
		PasswordHash: hash,
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return id
}

// testEncKey is a 32-byte key for AES-256 tests.
var testEncKey = []byte("12345678901234567890123456789012")

// ---- CreateConnector ----

func TestCreateConnector(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()
	userID := createTestUserForBridge(t, s, "create@bridge.test")

	in := &BridgeConnector{
		ConnectorType: "telegram",
		Name:          "my-telegram",
		Enabled:       true,
		Config:        `{"chat_id":"123"}`,
		CreatedBy:     userID,
	}
	secret := []byte(`{"bot_token":"secret-token"}`)

	got, err := s.CreateConnector(ctx, in, secret, testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	if got.ConnectorID == "" {
		t.Error("ConnectorID should not be empty")
	}
	if got.ConnectorType != in.ConnectorType {
		t.Errorf("ConnectorType = %q, want %q", got.ConnectorType, in.ConnectorType)
	}
	if got.Name != in.Name {
		t.Errorf("Name = %q, want %q", got.Name, in.Name)
	}
	if !got.Enabled {
		t.Error("Enabled should be true")
	}
	if got.Config != in.Config {
		t.Errorf("Config = %q, want %q", got.Config, in.Config)
	}
	if got.CreatedBy != userID {
		t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, userID)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestCreateConnector_NoSecret(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()
	userID := createTestUserForBridge(t, s, "nosecret@bridge.test")

	in := &BridgeConnector{
		ConnectorType: "webhook",
		Name:          "no-secret-connector",
		Enabled:       true,
		Config:        `{"url":"https://example.com"}`,
		CreatedBy:     userID,
	}

	got, err := s.CreateConnector(ctx, in, nil, testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector (no secret): %v", err)
	}
	if got.ConnectorID == "" {
		t.Error("ConnectorID should not be empty")
	}
}

// ---- GetConnector ----

func TestGetConnector(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()
	userID := createTestUserForBridge(t, s, "get@bridge.test")

	in := &BridgeConnector{
		ConnectorType: "telegram",
		Name:          "get-test",
		Enabled:       true,
		Config:        `{"chat_id":"456"}`,
		CreatedBy:     userID,
	}
	created, err := s.CreateConnector(ctx, in, []byte(`{"bot_token":"tok"}`), testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	got, err := s.GetConnector(ctx, created.ConnectorID)
	if err != nil {
		t.Fatalf("GetConnector: %v", err)
	}

	if got.ConnectorID != created.ConnectorID {
		t.Errorf("ConnectorID = %q, want %q", got.ConnectorID, created.ConnectorID)
	}
	if got.Name != in.Name {
		t.Errorf("Name = %q, want %q", got.Name, in.Name)
	}
}

func TestGetConnector_NotFound(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()

	_, err := s.GetConnector(ctx, "nonexistent-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ---- GetConnectorWithSecret ----

func TestGetConnectorWithSecret(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()
	userID := createTestUserForBridge(t, s, "secret@bridge.test")

	rawSecret := []byte(`{"bot_token":"my-secret-bot-token"}`)
	in := &BridgeConnector{
		ConnectorType: "telegram",
		Name:          "secret-test",
		Enabled:       true,
		Config:        `{"chat_id":"789"}`,
		CreatedBy:     userID,
	}
	created, err := s.CreateConnector(ctx, in, rawSecret, testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	got, secretJSON, err := s.GetConnectorWithSecret(ctx, created.ConnectorID, testEncKey)
	if err != nil {
		t.Fatalf("GetConnectorWithSecret: %v", err)
	}

	if got.ConnectorID != created.ConnectorID {
		t.Errorf("ConnectorID = %q, want %q", got.ConnectorID, created.ConnectorID)
	}
	if !bytes.Equal(secretJSON, rawSecret) {
		t.Errorf("secretJSON = %q, want %q", secretJSON, rawSecret)
	}
}

func TestGetConnectorWithSecret_NoSecret(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()
	userID := createTestUserForBridge(t, s, "nosecret2@bridge.test")

	in := &BridgeConnector{
		ConnectorType: "webhook",
		Name:          "no-secret",
		Enabled:       true,
		Config:        `{}`,
		CreatedBy:     userID,
	}
	created, err := s.CreateConnector(ctx, in, nil, testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	got, secretJSON, err := s.GetConnectorWithSecret(ctx, created.ConnectorID, testEncKey)
	if err != nil {
		t.Fatalf("GetConnectorWithSecret (no secret): %v", err)
	}
	if got.ConnectorID != created.ConnectorID {
		t.Errorf("ConnectorID = %q, want %q", got.ConnectorID, created.ConnectorID)
	}
	if secretJSON != nil {
		t.Errorf("secretJSON should be nil when no secret stored, got: %q", secretJSON)
	}
}

func TestGetConnectorWithSecret_NotFound(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()

	_, _, err := s.GetConnectorWithSecret(ctx, "nonexistent-id", testEncKey)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ---- ListConnectors ----

func TestListConnectors(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()
	userID := createTestUserForBridge(t, s, "list@bridge.test")

	names := []string{"alpha", "beta", "gamma"}
	for _, name := range names {
		_, err := s.CreateConnector(ctx, &BridgeConnector{
			ConnectorType: "telegram",
			Name:          name,
			Enabled:       true,
			Config:        `{}`,
			CreatedBy:     userID,
		}, nil, testEncKey)
		if err != nil {
			t.Fatalf("CreateConnector %q: %v", name, err)
		}
	}

	list, err := s.ListConnectors(ctx)
	if err != nil {
		t.Fatalf("ListConnectors: %v", err)
	}
	if len(list) != len(names) {
		t.Errorf("len = %d, want %d", len(list), len(names))
	}
}

func TestListConnectors_Empty(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()

	list, err := s.ListConnectors(ctx)
	if err != nil {
		t.Fatalf("ListConnectors (empty): %v", err)
	}
	if list == nil {
		t.Error("ListConnectors should return empty slice, not nil")
	}
	if len(list) != 0 {
		t.Errorf("len = %d, want 0", len(list))
	}
}

// ---- UpdateConnector ----

func TestUpdateConnector(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()
	userID := createTestUserForBridge(t, s, "update@bridge.test")

	in := &BridgeConnector{
		ConnectorType: "telegram",
		Name:          "before-update",
		Enabled:       true,
		Config:        `{"chat_id":"111"}`,
		CreatedBy:     userID,
	}
	created, err := s.CreateConnector(ctx, in, []byte(`{"bot_token":"old"}`), testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	updated := &BridgeConnector{
		ConnectorType: "telegram",
		Name:          "after-update",
		Enabled:       false,
		Config:        `{"chat_id":"222"}`,
		CreatedBy:     userID,
	}
	newSecret := []byte(`{"bot_token":"new"}`)

	got, err := s.UpdateConnector(ctx, created.ConnectorID, updated, newSecret, testEncKey)
	if err != nil {
		t.Fatalf("UpdateConnector: %v", err)
	}

	if got.Name != "after-update" {
		t.Errorf("Name = %q, want after-update", got.Name)
	}
	if got.Enabled {
		t.Error("Enabled should be false after update")
	}
	if got.Config != `{"chat_id":"222"}` {
		t.Errorf("Config = %q, want {\"chat_id\":\"222\"}", got.Config)
	}

	// Verify the new secret can be decrypted correctly.
	_, secretJSON, err := s.GetConnectorWithSecret(ctx, created.ConnectorID, testEncKey)
	if err != nil {
		t.Fatalf("GetConnectorWithSecret after update: %v", err)
	}
	if !bytes.Equal(secretJSON, newSecret) {
		t.Errorf("secretJSON after update = %q, want %q", secretJSON, newSecret)
	}
}

func TestUpdateConnector_NotFound(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()

	_, err := s.UpdateConnector(ctx, "nonexistent-id", &BridgeConnector{
		ConnectorType: "telegram",
		Name:          "x",
		Config:        `{}`,
	}, nil, testEncKey)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ---- DeleteConnector ----

func TestDeleteConnector(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()
	userID := createTestUserForBridge(t, s, "delete@bridge.test")

	created, err := s.CreateConnector(ctx, &BridgeConnector{
		ConnectorType: "telegram",
		Name:          "delete-me",
		Enabled:       true,
		Config:        `{}`,
		CreatedBy:     userID,
	}, nil, testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	if err := s.DeleteConnector(ctx, created.ConnectorID); err != nil {
		t.Fatalf("DeleteConnector: %v", err)
	}

	_, err = s.GetConnector(ctx, created.ConnectorID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestDeleteConnector_NotFound(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()

	err := s.DeleteConnector(ctx, "nonexistent-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ---- Bridge Control ----

func TestSetBridgeControl(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()

	if err := s.SetBridgeControl(ctx, "restart_requested_at", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("SetBridgeControl: %v", err)
	}

	val, err := s.GetBridgeControl(ctx, "restart_requested_at")
	if err != nil {
		t.Fatalf("GetBridgeControl: %v", err)
	}
	if val != "2026-01-01T00:00:00Z" {
		t.Errorf("value = %q, want 2026-01-01T00:00:00Z", val)
	}
}

func TestSetBridgeControl_Upsert(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()

	if err := s.SetBridgeControl(ctx, "my_key", "first"); err != nil {
		t.Fatalf("SetBridgeControl (first): %v", err)
	}
	if err := s.SetBridgeControl(ctx, "my_key", "second"); err != nil {
		t.Fatalf("SetBridgeControl (second): %v", err)
	}

	val, err := s.GetBridgeControl(ctx, "my_key")
	if err != nil {
		t.Fatalf("GetBridgeControl: %v", err)
	}
	if val != "second" {
		t.Errorf("value = %q, want second", val)
	}
}

func TestGetBridgeControl_NotFound(t *testing.T) {
	s := newTestStoreForBridge(t)
	ctx := context.Background()

	_, err := s.GetBridgeControl(ctx, "nonexistent_key")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}
