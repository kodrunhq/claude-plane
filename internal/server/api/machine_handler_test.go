package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/api"
	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// testAPIEnv bundles the test server with underlying store for direct seeding.
type testAPIEnv struct {
	Server *httptest.Server
	Store  *store.Store
}

// setupTestAPIWithStore creates a test API server and exposes the store for seeding.
func setupTestAPIWithStore(t *testing.T) *testAPIEnv {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	blocklist, err := auth.NewBlocklist(s)
	if err != nil {
		t.Fatalf("create blocklist: %v", err)
	}

	authSvc := auth.NewService([]byte("test-secret-key-32-bytes-long!!!"), 15*time.Minute, blocklist)
	cm := connmgr.NewConnectionManager(s, nil)

	handlers := api.NewHandlers(s, authSvc, cm, "open", "")
	router := api.NewRouter(api.RouterDeps{Handlers: handlers})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &testAPIEnv{Server: srv, Store: s}
}

// registerAndLoginAdmin registers a user, promotes them to admin, and returns the token.
func registerAndLoginAdmin(t *testing.T, env *testAPIEnv, email, password, name string) string {
	t.Helper()
	resp := registerUser(t, env.Server, email, password, name)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register admin user: expected 201, got %d", resp.StatusCode)
	}
	var regResult map[string]string
	json.NewDecoder(resp.Body).Decode(&regResult)
	userID := regResult["user_id"]

	if err := env.Store.UpdateUser(context.Background(), userID, name, "admin"); err != nil {
		t.Fatalf("promote user to admin: %v", err)
	}

	return loginUser(t, env.Server, email, password)
}

func TestListMachinesAuthenticated(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "machines@example.com", "password123", "Machine User")
	resp.Body.Close()

	token := loginUser(t, srv, "machines@example.com", "password123")

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/machines", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	machinesResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("machines request: %v", err)
	}
	defer machinesResp.Body.Close()

	if machinesResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", machinesResp.StatusCode)
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(machinesResp.Body).Decode(&result); err != nil {
		t.Fatalf("decoding machines response: %v", err)
	}

	if result == nil {
		t.Fatal("expected JSON array, got null")
	}
}

func TestUpdateMachine_NotFound(t *testing.T) {
	env := setupTestAPIWithStore(t)
	token := registerAndLoginAdmin(t, env, "admin-notfound@example.com", "password123", "Admin User")

	body, _ := json.Marshal(map[string]string{"display_name": "My Worker"})
	req, _ := http.NewRequest("PUT", env.Server.URL+"/api/v1/machines/nonexistent", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent machine, got %d", putResp.StatusCode)
	}
}

func TestUpdateMachine_HappyPath(t *testing.T) {
	env := setupTestAPIWithStore(t)
	token := registerAndLoginAdmin(t, env, "admin-happy@example.com", "password123", "Admin User")

	// Seed a machine in the store.
	if err := env.Store.UpsertMachine("test-machine-1", 5, ""); err != nil {
		t.Fatalf("seed machine: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"display_name": "My Worker"})
	req, _ := http.NewRequest("PUT", env.Server.URL+"/api/v1/machines/test-machine-1", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", putResp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(putResp.Body).Decode(&result); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if result["display_name"] != "My Worker" {
		t.Errorf("expected display_name 'My Worker', got %v", result["display_name"])
	}
	if result["machine_id"] != "test-machine-1" {
		t.Errorf("expected machine_id 'test-machine-1', got %v", result["machine_id"])
	}
}

func TestUpdateMachine_ForbiddenForNonAdmin(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "regular@example.com", "password123", "Regular User")
	resp.Body.Close()
	token := loginUser(t, srv, "regular@example.com", "password123")

	body, _ := json.Marshal(map[string]string{"display_name": "Hacked"})
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/machines/test-machine", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin user, got %d", putResp.StatusCode)
	}
}

func TestUpdateMachineInvalidBody(t *testing.T) {
	env := setupTestAPIWithStore(t)
	token := registerAndLoginAdmin(t, env, "admin-invalid@example.com", "password123", "Admin User")

	req, _ := http.NewRequest("PUT", env.Server.URL+"/api/v1/machines/test-machine", bytes.NewReader([]byte("not json")))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", putResp.StatusCode)
	}
}

func TestUpdateMachineMissingDisplayName(t *testing.T) {
	env := setupTestAPIWithStore(t)
	token := registerAndLoginAdmin(t, env, "admin-missing@example.com", "password123", "Admin User")

	body, _ := json.Marshal(map[string]string{})
	req, _ := http.NewRequest("PUT", env.Server.URL+"/api/v1/machines/test-machine", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing display_name, got %d", putResp.StatusCode)
	}
}

func TestUpdateMachineEmptyDisplayName(t *testing.T) {
	env := setupTestAPIWithStore(t)
	token := registerAndLoginAdmin(t, env, "admin-empty@example.com", "password123", "Admin User")

	body, _ := json.Marshal(map[string]string{"display_name": "   "})
	req, _ := http.NewRequest("PUT", env.Server.URL+"/api/v1/machines/test-machine", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for whitespace-only display_name, got %d", putResp.StatusCode)
	}
}

func TestDeleteMachine_Disconnected(t *testing.T) {
	env := setupTestAPIWithStore(t)
	token := registerAndLoginAdmin(t, env, "admin-del@example.com", "password123", "Admin User")

	if err := env.Store.UpsertMachine("del-machine", 5, ""); err != nil {
		t.Fatalf("seed machine: %v", err)
	}

	req, _ := http.NewRequest("DELETE", env.Server.URL+"/api/v1/machines/del-machine", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 for disconnected machine, got %d", resp.StatusCode)
	}

	// Verify machine is no longer listed.
	getReq, _ := http.NewRequest("GET", env.Server.URL+"/api/v1/machines/del-machine", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("GET request: %v", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for deleted machine, got %d", getResp.StatusCode)
	}
}

func TestDeleteMachine_ForbiddenForNonAdmin(t *testing.T) {
	env := setupTestAPIWithStore(t)

	resp := registerUser(t, env.Server, "regular-del@example.com", "password123", "Regular User")
	resp.Body.Close()
	token := loginUser(t, env.Server, "regular-del@example.com", "password123")

	if err := env.Store.UpsertMachine("del-machine-2", 5, ""); err != nil {
		t.Fatalf("seed machine: %v", err)
	}

	req, _ := http.NewRequest("DELETE", env.Server.URL+"/api/v1/machines/del-machine-2", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE request: %v", err)
	}
	defer delResp.Body.Close()

	if delResp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d", delResp.StatusCode)
	}
}

func TestDeleteMachine_NotFound(t *testing.T) {
	env := setupTestAPIWithStore(t)
	token := registerAndLoginAdmin(t, env, "admin-del-nf@example.com", "password123", "Admin User")

	req, _ := http.NewRequest("DELETE", env.Server.URL+"/api/v1/machines/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestListMachinesUnauthenticated(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/machines")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}
