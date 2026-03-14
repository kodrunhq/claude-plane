package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

var testAPISigningKey = []byte("test-signing-key-32-bytes-for-hmac")

// newAPIKeyRouter creates a test server with API key routes for the given user.
func newAPIKeyRouter(t *testing.T, s *store.Store, userID, role string) *httptest.Server {
	t.Helper()
	seedUser(t, s, userID, role)
	h := handler.NewAPIKeyHandler(s, testAPISigningKey, claimsMiddleware(userID, role))
	r := chi.NewRouter()
	handler.RegisterAPIKeyRoutes(r, h)
	return httptest.NewServer(r)
}

// createAPIKeyViaHTTP posts to /api/v1/api-keys and returns the decoded response.
func createAPIKeyViaHTTP(t *testing.T, srvURL, name string, scopes []string) map[string]interface{} {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"name":   name,
		"scopes": scopes,
	})
	resp, err := http.Post(srvURL+"/api/v1/api-keys", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create api key request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func TestAPIKeyHandler_Create_Valid(t *testing.T) {
	s := newTestStore(t)
	srv := newAPIKeyRouter(t, s, "user-ak-1", "member")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"name":   "My Key",
		"scopes": []string{"sessions"},
	})
	resp, err := http.Post(srv.URL+"/api/v1/api-keys", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// plaintext key must be present and have cpk_ prefix
	key, ok := result["key"].(string)
	if !ok || !strings.HasPrefix(key, "cpk_") {
		t.Errorf("expected plaintext key with cpk_ prefix, got %v", result["key"])
	}

	// key_id must be present
	if result["key_id"] == "" || result["key_id"] == nil {
		t.Error("expected key_id in response")
	}

	// name must match
	if result["name"] != "My Key" {
		t.Errorf("expected name 'My Key', got %v", result["name"])
	}

	// created_at must be present
	if result["created_at"] == nil {
		t.Error("expected created_at in response")
	}
}

func TestAPIKeyHandler_Create_MissingName(t *testing.T) {
	s := newTestStore(t)
	srv := newAPIKeyRouter(t, s, "user-ak-2", "member")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"scopes": []string{"sessions"},
	})
	resp, err := http.Post(srv.URL+"/api/v1/api-keys", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAPIKeyHandler_Create_NoHashInResponse(t *testing.T) {
	s := newTestStore(t)
	srv := newAPIKeyRouter(t, s, "user-ak-3", "member")
	defer srv.Close()

	result := createAPIKeyViaHTTP(t, srv.URL, "No Hash Key", nil)

	// HMAC/hash must not be exposed
	if result["key_hmac"] != nil || result["hmac"] != nil || result["hash"] != nil {
		t.Error("response must not contain hmac or hash fields")
	}
}

func TestAPIKeyHandler_List_UserSeesOwnKeys(t *testing.T) {
	s := newTestStore(t)
	srv := newAPIKeyRouter(t, s, "user-ak-list-1", "member")
	defer srv.Close()

	createAPIKeyViaHTTP(t, srv.URL, "Key A", nil)
	createAPIKeyViaHTTP(t, srv.URL, "Key B", []string{"sessions"})

	resp, err := http.Get(srv.URL + "/api/v1/api-keys")
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result) != 2 {
		t.Errorf("expected 2 keys, got %d", len(result))
	}

	// Neither response object should contain key_hmac or plaintext key
	for _, k := range result {
		if k["key_hmac"] != nil || k["key"] != nil {
			t.Error("list response must not contain key_hmac or plaintext key")
		}
		if k["key_id"] == nil {
			t.Error("list response must contain key_id")
		}
	}
}

func TestAPIKeyHandler_List_AdminSeesAll(t *testing.T) {
	s := newTestStore(t)

	// Create keys as user-1
	srvUser1 := newAPIKeyRouter(t, s, "user-ak-admin-1", "member")
	createAPIKeyViaHTTP(t, srvUser1.URL, "User1 Key", nil)
	srvUser1.Close()

	// Create keys as user-2
	srvUser2 := newAPIKeyRouter(t, s, "user-ak-admin-2", "member")
	createAPIKeyViaHTTP(t, srvUser2.URL, "User2 Key", nil)
	srvUser2.Close()

	// Admin lists all
	srvAdmin := newAPIKeyRouter(t, s, "admin-ak-1", "admin")
	defer srvAdmin.Close()

	resp, err := http.Get(srvAdmin.URL + "/api/v1/api-keys")
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result) < 2 {
		t.Errorf("admin expected at least 2 keys, got %d", len(result))
	}
}

func TestAPIKeyHandler_Delete_Owner(t *testing.T) {
	s := newTestStore(t)
	srv := newAPIKeyRouter(t, s, "user-ak-del-1", "member")
	defer srv.Close()

	created := createAPIKeyViaHTTP(t, srv.URL, "Delete Me", nil)
	keyID, _ := created["key_id"].(string)

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/api-keys/%s", srv.URL, keyID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestAPIKeyHandler_Delete_Admin(t *testing.T) {
	s := newTestStore(t)

	// Create as regular user
	srvUser := newAPIKeyRouter(t, s, "user-ak-del-2", "member")
	created := createAPIKeyViaHTTP(t, srvUser.URL, "Admin Deletes", nil)
	keyID, _ := created["key_id"].(string)
	srvUser.Close()

	// Admin deletes it
	srvAdmin := newAPIKeyRouter(t, s, "admin-ak-del-1", "admin")
	defer srvAdmin.Close()

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/api-keys/%s", srvAdmin.URL, keyID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestAPIKeyHandler_Delete_NonOwner(t *testing.T) {
	s := newTestStore(t)

	// Create as user-1
	srvUser1 := newAPIKeyRouter(t, s, "user-ak-del-3", "member")
	created := createAPIKeyViaHTTP(t, srvUser1.URL, "Not Yours", nil)
	keyID, _ := created["key_id"].(string)
	srvUser1.Close()

	// Attempt delete as user-2 (non-owner, non-admin)
	srvUser2 := newAPIKeyRouter(t, s, "user-ak-del-4", "member")
	defer srvUser2.Close()

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/api-keys/%s", srvUser2.URL, keyID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	defer resp.Body.Close()

	// Must be 404 — don't reveal the key exists
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for non-owner, got %d", resp.StatusCode)
	}
}

func TestAPIKeyHandler_Delete_NotFound(t *testing.T) {
	s := newTestStore(t)
	srv := newAPIKeyRouter(t, s, "user-ak-del-nf", "member")
	defer srv.Close()

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/api-keys/nonexistent-key-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for missing key, got %d", resp.StatusCode)
	}
}
