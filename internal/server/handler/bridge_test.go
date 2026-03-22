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
	h := handler.NewBridgeHandler(s, claimsMiddleware(userID, role), testEncKey, nil)
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

// claimsMiddlewareWithScopes injects a ClaimsGetter that returns fixed claims with scopes.
func claimsMiddlewareWithScopes(userID, role string, scopes []string) handler.ClaimsGetter {
	return func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: userID, Role: role, Scopes: scopes}
	}
}

func TestBridgeHandler_ListConnectors_APIKeyAuth_WithScope_IncludesSecrets(t *testing.T) {
	s := newTestStore(t)
	userID := "bridge-user-5"
	seedUser(t, s, userID, "admin")
	h := handler.NewBridgeHandler(s, claimsMiddlewareWithScopes(userID, "admin", []string{"connectors:read_secret"}), testEncKey, nil)
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
		t.Error("expected config_secret in response for API key auth with connectors:read_secret scope")
	}
}

func TestBridgeHandler_ListConnectors_APIKeyAuth_WithoutScope_HidesSecrets(t *testing.T) {
	s := newTestStore(t)
	userID := "bridge-user-5b"
	seedUser(t, s, userID, "admin")
	// API key auth but no connectors:read_secret scope
	h := handler.NewBridgeHandler(s, claimsMiddlewareWithScopes(userID, "admin", []string{"jobs:read"}), testEncKey, nil)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := httputil.SetAPIKeyAuth(r.Context())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	handler.RegisterBridgeRoutes(r, h)
	srv := httptest.NewServer(r)
	defer srv.Close()

	c := &store.BridgeConnector{
		ConnectorType: "telegram",
		Name:          "No Scope Bot",
		Enabled:       true,
		Config:        "{}",
		CreatedBy:     userID,
	}
	_, err := s.CreateConnector(t.Context(), c, []byte(`{"bot_token":"hidden-token"}`), testEncKey)
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

	if result[0]["config_secret"] != nil {
		t.Error("config_secret must not appear for API key without connectors:read_secret scope")
	}
}

func TestBridgeHandler_GetConnector_APIKeyAuth_WithScope_IncludesSecrets(t *testing.T) {
	s := newTestStore(t)
	userID := "bridge-user-5c"
	seedUser(t, s, userID, "admin")
	h := handler.NewBridgeHandler(s, claimsMiddlewareWithScopes(userID, "admin", []string{"connectors:read_secret"}), testEncKey, nil)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := httputil.SetAPIKeyAuth(r.Context())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	handler.RegisterBridgeRoutes(r, h)
	srv := httptest.NewServer(r)
	defer srv.Close()

	c := &store.BridgeConnector{
		ConnectorType: "github",
		Name:          "Scoped GitHub",
		Enabled:       true,
		Config:        "{}",
		CreatedBy:     userID,
	}
	created, err := s.CreateConnector(t.Context(), c, []byte(`{"pat":"ghp_secret123"}`), testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	resp, err := http.Get(srv.URL + "/api/v1/bridge/connectors/" + created.ConnectorID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	secret, ok := result["config_secret"].(string)
	if !ok || secret == "" {
		t.Error("expected config_secret in response for API key auth with connectors:read_secret scope")
	}
}

func TestBridgeHandler_GetConnector_APIKeyAuth_WithoutScope_HidesSecrets(t *testing.T) {
	s := newTestStore(t)
	userID := "bridge-user-5d"
	seedUser(t, s, userID, "admin")
	h := handler.NewBridgeHandler(s, claimsMiddlewareWithScopes(userID, "admin", nil), testEncKey, nil)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := httputil.SetAPIKeyAuth(r.Context())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	handler.RegisterBridgeRoutes(r, h)
	srv := httptest.NewServer(r)
	defer srv.Close()

	c := &store.BridgeConnector{
		ConnectorType: "github",
		Name:          "No Scope GitHub",
		Enabled:       true,
		Config:        "{}",
		CreatedBy:     userID,
	}
	created, err := s.CreateConnector(t.Context(), c, []byte(`{"pat":"ghp_hidden"}`), testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	resp, err := http.Get(srv.URL + "/api/v1/bridge/connectors/" + created.ConnectorID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["config_secret"] != nil {
		t.Error("config_secret must not appear for API key without connectors:read_secret scope")
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

// --- Notification channel auto-sync tests ---

// newBridgeRouterWithNotifStore creates a test server that also passes the store as NotifStore.
func newBridgeRouterWithNotifStore(t *testing.T, s *store.Store, userID, role string) *httptest.Server {
	t.Helper()
	seedUser(t, s, userID, role)
	h := handler.NewBridgeHandler(s, claimsMiddleware(userID, role), testEncKey, s)
	r := chi.NewRouter()
	handler.RegisterBridgeRoutes(r, h)
	return httptest.NewServer(r)
}

func TestBridgeHandler_CreateTelegramConnector_AutoCreatesNotifChannel(t *testing.T) {
	s := newTestStore(t)
	userID := "bridge-notif-1"
	srv := newBridgeRouterWithNotifStore(t, s, userID, "admin")
	defer srv.Close()

	body := map[string]interface{}{
		"connector_type": "telegram",
		"name":           "TG Notif Bot",
		"config":         `{"group_id":-1001234567890,"events_topic_id":5}`,
		"enabled":        true,
	}
	result := createConnectorViaHTTP(t, srv.URL, body)
	connectorID := fmt.Sprintf("%v", result["connector_id"])

	// Verify a notification channel was auto-created
	ch, err := s.GetChannelByConnectorID(t.Context(), connectorID)
	if err != nil {
		t.Fatalf("expected notification channel for connector %s, got error: %v", connectorID, err)
	}
	if ch.ChannelType != "telegram" {
		t.Errorf("channel type = %q, want telegram", ch.ChannelType)
	}
	if ch.Name != "TG Notif Bot" {
		t.Errorf("channel name = %q, want 'TG Notif Bot'", ch.Name)
	}
	if ch.ConnectorID == nil || *ch.ConnectorID != connectorID {
		t.Errorf("channel connector_id = %v, want %s", ch.ConnectorID, connectorID)
	}
	if !ch.Enabled {
		t.Error("channel should be enabled")
	}

	// Verify config contains chat_id and topic_id
	var chCfg map[string]interface{}
	if err := json.Unmarshal([]byte(ch.Config), &chCfg); err != nil {
		t.Fatalf("unmarshal channel config: %v", err)
	}
	if chCfg["chat_id"] != "-1001234567890" {
		t.Errorf("chat_id = %v, want '-1001234567890'", chCfg["chat_id"])
	}
	if chCfg["topic_id"] != float64(5) {
		t.Errorf("topic_id = %v, want 5", chCfg["topic_id"])
	}
}

func TestBridgeHandler_CreateGitHubConnector_NoNotifChannel(t *testing.T) {
	s := newTestStore(t)
	userID := "bridge-notif-2"
	srv := newBridgeRouterWithNotifStore(t, s, userID, "admin")
	defer srv.Close()

	body := map[string]interface{}{
		"connector_type": "github",
		"name":           "GH Connector",
		"config":         `{"owner":"test","repo":"repo"}`,
		"enabled":        true,
	}
	result := createConnectorViaHTTP(t, srv.URL, body)
	connectorID := fmt.Sprintf("%v", result["connector_id"])

	// Verify no notification channel was created
	_, err := s.GetChannelByConnectorID(t.Context(), connectorID)
	if err == nil {
		t.Error("expected no notification channel for GitHub connector")
	}
}

func TestBridgeHandler_UpdateTelegramConnector_SyncsNotifChannel(t *testing.T) {
	s := newTestStore(t)
	userID := "bridge-notif-3"
	srv := newBridgeRouterWithNotifStore(t, s, userID, "admin")
	defer srv.Close()

	// Create a telegram connector
	body := map[string]interface{}{
		"connector_type": "telegram",
		"name":           "Original Name",
		"config":         `{"group_id":-100111,"events_topic_id":1}`,
		"enabled":        true,
	}
	result := createConnectorViaHTTP(t, srv.URL, body)
	connectorID := fmt.Sprintf("%v", result["connector_id"])

	// Verify initial channel
	ch, err := s.GetChannelByConnectorID(t.Context(), connectorID)
	if err != nil {
		t.Fatalf("expected initial channel: %v", err)
	}
	if ch.Name != "Original Name" {
		t.Fatalf("initial channel name = %q, want 'Original Name'", ch.Name)
	}

	// Update the connector
	updateBody, _ := json.Marshal(map[string]interface{}{
		"connector_type": "telegram",
		"name":           "Updated Name",
		"config":         `{"group_id":-100222,"events_topic_id":10}`,
	})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/bridge/connectors/"+connectorID, bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify channel was updated
	ch, err = s.GetChannelByConnectorID(t.Context(), connectorID)
	if err != nil {
		t.Fatalf("expected channel after update: %v", err)
	}
	if ch.Name != "Updated Name" {
		t.Errorf("channel name = %q, want 'Updated Name'", ch.Name)
	}

	var chCfg map[string]interface{}
	if err := json.Unmarshal([]byte(ch.Config), &chCfg); err != nil {
		t.Fatalf("unmarshal channel config: %v", err)
	}
	if chCfg["chat_id"] != "-100222" {
		t.Errorf("chat_id = %v, want '-100222'", chCfg["chat_id"])
	}
	if chCfg["topic_id"] != float64(10) {
		t.Errorf("topic_id = %v, want 10", chCfg["topic_id"])
	}
}

func TestBridgeHandler_DeleteTelegramConnector_DeletesNotifChannel(t *testing.T) {
	s := newTestStore(t)
	userID := "bridge-notif-4"
	srv := newBridgeRouterWithNotifStore(t, s, userID, "admin")
	defer srv.Close()

	// Create a telegram connector
	body := map[string]interface{}{
		"connector_type": "telegram",
		"name":           "Delete Notif Bot",
		"config":         `{"group_id":-100333}`,
		"enabled":        true,
	}
	result := createConnectorViaHTTP(t, srv.URL, body)
	connectorID := fmt.Sprintf("%v", result["connector_id"])

	// Verify channel exists
	_, err := s.GetChannelByConnectorID(t.Context(), connectorID)
	if err != nil {
		t.Fatalf("expected channel before delete: %v", err)
	}

	// Delete the connector
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/bridge/connectors/"+connectorID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	// Verify channel is gone
	_, err = s.GetChannelByConnectorID(t.Context(), connectorID)
	if err == nil {
		t.Error("expected notification channel to be deleted with connector")
	}
}
