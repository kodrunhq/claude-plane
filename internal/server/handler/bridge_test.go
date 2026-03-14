package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/httputil"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// testEncKey is a 32-byte AES-256 encryption key for bridge handler tests.
var testEncKey = func() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}()

// newBridgeRouter creates a test server with bridge routes and a seeded admin user.
func newBridgeRouter(t *testing.T, s *store.Store, userID, role string) *httptest.Server {
	t.Helper()
	seedUser(t, s, userID, role)
	h := handler.NewBridgeHandler(s, claimsMiddleware(userID, role), testEncKey)
	r := chi.NewRouter()
	handler.RegisterBridgeRoutes(r, h)
	return httptest.NewServer(r)
}

// createConnectorViaHTTP posts to /api/v1/bridge/connectors and returns the decoded response.
func createConnectorViaHTTP(t *testing.T, srvURL string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(srvURL+"/api/v1/bridge/connectors", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("create connector request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

// --- Tests ---

func TestBridgeHandler_CreateConnector(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-1", "admin")
	defer srv.Close()

	body := map[string]interface{}{
		"connector_type": "telegram",
		"name":           "My Telegram Bot",
		"config":         `{"webhook_url":"https://example.com"}`,
		"config_secret":  `{"bot_token":"secret-token-123"}`,
		"enabled":        true,
	}

	result := createConnectorViaHTTP(t, srv.URL, body)

	if result["connector_id"] == nil || result["connector_id"] == "" {
		t.Error("expected connector_id in response")
	}
	if result["connector_type"] != "telegram" {
		t.Errorf("connector_type = %v, want telegram", result["connector_type"])
	}
	if result["name"] != "My Telegram Bot" {
		t.Errorf("name = %v, want 'My Telegram Bot'", result["name"])
	}
	// Secrets must not be returned on create
	if result["config_secret"] != nil {
		t.Error("config_secret must not be returned in create response")
	}
}

func TestBridgeHandler_CreateConnector_MissingType(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-2", "admin")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"name":   "No Type Bot",
		"config": "{}",
	})
	resp, err := http.Post(srv.URL+"/api/v1/bridge/connectors", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBridgeHandler_CreateConnector_MissingName(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-3", "admin")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"connector_type": "telegram",
		"config":         "{}",
	})
	resp, err := http.Post(srv.URL+"/api/v1/bridge/connectors", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBridgeHandler_ListConnectors(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-4", "admin")
	defer srv.Close()

	// Create two connectors
	for _, name := range []string{"Bot A", "Bot B"} {
		createConnectorViaHTTP(t, srv.URL, map[string]interface{}{
			"connector_type": "telegram",
			"name":           name,
			"config":         "{}",
		})
	}

	resp, err := http.Get(srv.URL + "/api/v1/bridge/connectors")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result) != 2 {
		t.Errorf("expected 2 connectors, got %d", len(result))
	}

	// JWT auth — secrets must not be present
	for _, c := range result {
		if c["config_secret"] != nil {
			t.Error("config_secret must not appear for JWT auth list")
		}
	}
}

func TestBridgeHandler_ListConnectors_APIKeyAuth_IncludesSecrets(t *testing.T) {
	s := newTestStore(t)
	userID := "bridge-user-5"
	seedUser(t, s, userID, "admin")
	h := handler.NewBridgeHandler(s, claimsMiddleware(userID, "admin"), testEncKey)
	r := chi.NewRouter()
	// Simulate API key auth by setting the context flag the middleware would set.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := httputil.SetAPIKeyAuth(r.Context())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	handler.RegisterBridgeRoutes(r, h)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Create a connector with a secret (via direct store call to bypass HTTP auth for setup)
	c := &store.BridgeConnector{
		ConnectorType: "telegram",
		Name:          "Secret Bot",
		Enabled:       true,
		Config:        "{}",
		CreatedBy:     userID,
	}
	_, err := s.CreateConnector(t.Context(), c, []byte(`{"bot_token":"super-secret"}`), testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	resp, err := http.Get(srv.URL + "/api/v1/bridge/connectors")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result) != 1 {
		t.Fatalf("expected 1 connector, got %d", len(result))
	}

	secret, ok := result[0]["config_secret"].(string)
	if !ok || secret == "" {
		t.Error("expected config_secret in response for API key auth")
	}
}

func TestBridgeHandler_GetConnector(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-6", "admin")
	defer srv.Close()

	created := createConnectorViaHTTP(t, srv.URL, map[string]interface{}{
		"connector_type": "telegram",
		"name":           "Get Me",
		"config":         `{"key":"value"}`,
	})
	connectorID := fmt.Sprintf("%v", created["connector_id"])

	resp, err := http.Get(srv.URL + "/api/v1/bridge/connectors/" + connectorID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["connector_id"] != connectorID {
		t.Errorf("connector_id = %v, want %v", result["connector_id"], connectorID)
	}
	if result["name"] != "Get Me" {
		t.Errorf("name = %v, want 'Get Me'", result["name"])
	}
}

func TestBridgeHandler_GetConnector_NotFound(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-7", "admin")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/bridge/connectors/nonexistent-id")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestBridgeHandler_UpdateConnector(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-8", "admin")
	defer srv.Close()

	created := createConnectorViaHTTP(t, srv.URL, map[string]interface{}{
		"connector_type": "telegram",
		"name":           "Old Name",
		"config":         "{}",
	})
	connectorID := fmt.Sprintf("%v", created["connector_id"])

	updateBody, _ := json.Marshal(map[string]interface{}{
		"connector_type": "telegram",
		"name":           "New Name",
		"config":         `{"updated":true}`,
		"enabled":        false,
	})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/bridge/connectors/"+connectorID, bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["name"] != "New Name" {
		t.Errorf("name = %v, want 'New Name'", result["name"])
	}
	if result["enabled"] != false {
		t.Errorf("enabled = %v, want false", result["enabled"])
	}
	// Secrets must not appear in update response
	if result["config_secret"] != nil {
		t.Error("config_secret must not appear in update response")
	}
}

func TestBridgeHandler_UpdateConnector_NotFound(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-9", "admin")
	defer srv.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"connector_type": "telegram",
		"name":           "Ghost",
		"config":         "{}",
	})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/bridge/connectors/does-not-exist", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestBridgeHandler_DeleteConnector(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-10", "admin")
	defer srv.Close()

	created := createConnectorViaHTTP(t, srv.URL, map[string]interface{}{
		"connector_type": "telegram",
		"name":           "Delete Me",
		"config":         "{}",
	})
	connectorID := fmt.Sprintf("%v", created["connector_id"])

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/bridge/connectors/"+connectorID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Confirm it's gone
	getResp, _ := http.Get(srv.URL + "/api/v1/bridge/connectors/" + connectorID)
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestBridgeHandler_DeleteConnector_NotFound(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-11", "admin")
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/bridge/connectors/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestBridgeHandler_Restart(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-12", "admin")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/bridge/restart", "application/json", nil)
	if err != nil {
		t.Fatalf("POST restart: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["message"] != "restart signal sent" {
		t.Errorf("message = %v, want 'restart signal sent'", result["message"])
	}
}

func TestBridgeHandler_Status_BeforeRestart(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-13", "admin")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/bridge/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// restart_requested_at must be present but null when never set
	val, exists := result["restart_requested_at"]
	if !exists {
		t.Error("expected restart_requested_at key in status response")
	}
	if val != nil {
		t.Errorf("expected null restart_requested_at before restart, got %v", val)
	}
}

func TestBridgeHandler_Status_AfterRestart(t *testing.T) {
	s := newTestStore(t)
	srv := newBridgeRouter(t, s, "bridge-user-14", "admin")
	defer srv.Close()

	// Send restart signal first
	restartResp, err := http.Post(srv.URL+"/api/v1/bridge/restart", "application/json", nil)
	if err != nil {
		t.Fatalf("POST restart: %v", err)
	}
	restartResp.Body.Close()

	// Now check status
	resp, err := http.Get(srv.URL + "/api/v1/bridge/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	val, exists := result["restart_requested_at"]
	if !exists {
		t.Error("expected restart_requested_at key in status response")
	}
	if val == nil || val == "" {
		t.Error("expected non-nil restart_requested_at after restart signal")
	}
}
