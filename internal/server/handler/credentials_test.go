package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// testClaimsGetter returns a ClaimsGetter that always returns fixed claims.
func testClaimsGetter(userID, role string) handler.ClaimsGetter {
	return func(_ *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: userID, Role: role}
	}
}

func newCredentialRouter(t *testing.T, encryptionKey []byte) *httptest.Server {
	t.Helper()
	s := newTestStore(t)

	// Create the test user so FK constraints are satisfied.
	err := s.CreateUser(&store.User{
		UserID:      "user-1",
		Email:       "test@example.com",
		DisplayName: "Test User",
		Role:        "admin",
	})
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}

	claims := testClaimsGetter("user-1", "admin")
	h := handler.NewCredentialHandler(s, claims, encryptionKey)
	r := chi.NewRouter()
	handler.RegisterCredentialRoutes(r, h)
	return httptest.NewServer(r)
}

func TestCredentialHandler_StatusWithEncryption(t *testing.T) {
	key := make([]byte, 32) // zero-filled but non-nil 32-byte key
	srv := newCredentialRouter(t, key)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/credentials/status")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var status struct {
		EncryptionEnabled bool `json:"encryption_enabled"`
	}
	json.NewDecoder(resp.Body).Decode(&status)
	if !status.EncryptionEnabled {
		t.Error("expected encryption_enabled=true when key is set")
	}
}

func TestCredentialHandler_StatusWithoutEncryption(t *testing.T) {
	srv := newCredentialRouter(t, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/credentials/status")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var status struct {
		EncryptionEnabled bool `json:"encryption_enabled"`
	}
	json.NewDecoder(resp.Body).Decode(&status)
	if status.EncryptionEnabled {
		t.Error("expected encryption_enabled=false when no key")
	}
}

func TestCredentialHandler_CreateCredential_PublishesEvent(t *testing.T) {
	s := newTestStore(t)

	err := s.CreateUser(&store.User{
		UserID:      "user-pub",
		Email:       "pubuser@example.com",
		DisplayName: "Pub User",
		Role:        "user",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	pub := &mockPublisher{}
	claims := testClaimsGetter("user-pub", "user")
	h := handler.NewCredentialHandler(s, claims, nil)
	h.SetPublisher(pub)
	r := chi.NewRouter()
	handler.RegisterCredentialRoutes(r, h)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"name":  "my-api-key",
		"value": "super-secret",
	})
	resp, err := http.Post(srv.URL+"/api/v1/credentials", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	events := pub.published()
	if len(events) < 1 {
		t.Fatalf("expected at least 1 published event, got %d", len(events))
	}
	evt := events[0]
	if evt.Type != "credential.created" {
		t.Errorf("event type = %q, want %q", evt.Type, "credential.created")
	}
	if evt.Payload["credential_name"] != "my-api-key" {
		t.Errorf("payload credential_name = %v, want %q", evt.Payload["credential_name"], "my-api-key")
	}
}

func TestCredentialHandler_CRUDWithEncryption(t *testing.T) {
	key := []byte("01234567890123456789012345678901") // exactly 32 bytes
	srv := newCredentialRouter(t, key)
	defer srv.Close()

	testCredentialCRUD(t, srv)
}

func TestCredentialHandler_CRUDWithoutEncryption(t *testing.T) {
	srv := newCredentialRouter(t, nil)
	defer srv.Close()

	testCredentialCRUD(t, srv)
}

func testCredentialCRUD(t *testing.T, srv *httptest.Server) {
	t.Helper()

	// Create
	body, _ := json.Marshal(map[string]string{
		"name":  "my-secret",
		"value": "super-secret-value",
	})
	resp, err := http.Post(srv.URL+"/api/v1/credentials", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}

	var created struct {
		CredentialID string `json:"credential_id"`
		Name         string `json:"name"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	if created.CredentialID == "" {
		t.Fatal("expected credential_id in response")
	}
	if created.Name != "my-secret" {
		t.Errorf("expected name 'my-secret', got %q", created.Name)
	}

	// List
	resp, err = http.Get(srv.URL + "/api/v1/credentials")
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}

	var list []struct {
		CredentialID string `json:"credential_id"`
		Name         string `json:"name"`
	}
	json.NewDecoder(resp.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(list))
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/credentials/"+created.CredentialID, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}

	// Verify deleted
	resp, err = http.Get(srv.URL + "/api/v1/credentials")
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	defer resp.Body.Close()

	json.NewDecoder(resp.Body).Decode(&list)
	if len(list) != 0 {
		t.Errorf("expected 0 credentials after delete, got %d", len(list))
	}
}
