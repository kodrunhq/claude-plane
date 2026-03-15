package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/provision"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	"github.com/kodrunhq/claude-plane/internal/shared/tlsutil"
)

// --- test helpers ---

func newProvisionTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "provision_handler_test.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newProvisionTestCA(t *testing.T) string {
	t.Helper()
	caDir := t.TempDir()
	if err := tlsutil.GenerateCA(caDir); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	return caDir
}

func newProvisionHandlerFixture(t *testing.T, claims *handler.UserClaims) (*handler.ProvisionHandler, *store.Store) {
	t.Helper()
	s := newProvisionTestStore(t)
	caDir := newProvisionTestCA(t)
	svc := provision.NewService(s, caDir, "http://test.example.com", "test.example.com:9090")
	getClaims := func(_ *http.Request) *handler.UserClaims { return claims }
	h := handler.NewProvisionHandler(svc, s, getClaims)
	return h, s
}

// --- CreateProvision tests ---

func TestCreateProvision_AdminSuccess(t *testing.T) {
	adminClaims := &handler.UserClaims{UserID: "user-1", Role: "admin"}
	h, _ := newProvisionHandlerFixture(t, adminClaims)

	body := `{"machine_id":"test-machine","os":"linux","arch":"amd64"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provision/agent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateProvision(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var result provision.ProvisionResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Token == "" {
		t.Error("expected non-empty token in response")
	}
	if result.ExpiresAt.IsZero() {
		t.Error("expected non-zero expires_at")
	}
	if !strings.Contains(result.CurlCommand, result.Token) {
		t.Errorf("curl_command %q does not contain token %q", result.CurlCommand, result.Token)
	}
}

func TestCreateProvision_NonAdminForbidden(t *testing.T) {
	userClaims := &handler.UserClaims{UserID: "user-2", Role: "user"}
	h, _ := newProvisionHandlerFixture(t, userClaims)

	body := `{"machine_id":"test-machine"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provision/agent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateProvision(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestCreateProvision_NilClaimsForbidden(t *testing.T) {
	h, _ := newProvisionHandlerFixture(t, nil)

	body := `{"machine_id":"test-machine"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provision/agent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateProvision(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestCreateProvision_MissingMachineID(t *testing.T) {
	adminClaims := &handler.UserClaims{UserID: "user-1", Role: "admin"}
	h, _ := newProvisionHandlerFixture(t, adminClaims)

	body := `{"os":"linux","arch":"amd64"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provision/agent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateProvision(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateProvision_InvalidBody(t *testing.T) {
	adminClaims := &handler.UserClaims{UserID: "user-1", Role: "admin"}
	h, _ := newProvisionHandlerFixture(t, adminClaims)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provision/agent", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateProvision(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- ServeScript tests ---

// insertTestToken inserts a provisioning token directly into the store for testing.
func insertTestToken(t *testing.T, s *store.Store, tokenID, machineID string, ttl time.Duration) {
	t.Helper()
	now := time.Now().UTC()
	pt := store.ProvisioningToken{
		Token:         tokenID,
		MachineID:     machineID,
		TargetOS:      "linux",
		TargetArch:    "amd64",
		CACertPEM:     "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n",
		AgentCertPEM:  "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n",
		AgentKeyPEM:   "-----BEGIN EC PRIVATE KEY-----\nfake\n-----END EC PRIVATE KEY-----\n",
		ServerAddress: "http://test.example.com",
		GRPCAddress:   "test.example.com:9090",
		CreatedBy:     "admin",
		CreatedAt:     now,
		ExpiresAt:     now.Add(ttl),
	}
	if err := s.CreateProvisioningToken(t.Context(), pt); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}
}

// serveScriptViaRouter routes ServeScript through a real chi router
// so chi.URLParam resolves correctly.
func serveScriptViaRouter(h *handler.ProvisionHandler) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/provision/{token}/script", h.ServeScript)
	return r
}

func TestServeScript_ValidToken(t *testing.T) {
	adminClaims := &handler.UserClaims{UserID: "user-1", Role: "admin"}
	h, s := newProvisionHandlerFixture(t, adminClaims)

	tokenID := "valid-token-abc123"
	insertTestToken(t, s, tokenID, "my-machine", time.Hour)

	router := serveScriptViaRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provision/"+tokenID+"/script", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/x-shellscript" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/x-shellscript")
	}
	body := w.Body.String()
	if !strings.Contains(body, "#!/usr/bin/env bash") {
		t.Errorf("response does not look like a shell script: %q", body[:min(200, len(body))])
	}
	if !strings.Contains(body, "my-machine") {
		t.Errorf("script does not contain machine ID")
	}
}

func TestServeScript_TokenNotFound(t *testing.T) {
	adminClaims := &handler.UserClaims{UserID: "user-1", Role: "admin"}
	h, _ := newProvisionHandlerFixture(t, adminClaims)

	router := serveScriptViaRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provision/nonexistent-token/script", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestServeScript_ExpiredToken(t *testing.T) {
	adminClaims := &handler.UserClaims{UserID: "user-1", Role: "admin"}
	h, s := newProvisionHandlerFixture(t, adminClaims)

	tokenID := "expired-token-xyz"
	insertTestToken(t, s, tokenID, "exp-machine", -time.Second)

	router := serveScriptViaRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provision/"+tokenID+"/script", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("status = %d, want %d", w.Code, http.StatusGone)
	}
}

func TestServeScript_AlreadyRedeemed(t *testing.T) {
	adminClaims := &handler.UserClaims{UserID: "user-1", Role: "admin"}
	h, s := newProvisionHandlerFixture(t, adminClaims)

	tokenID := "redeemed-token-qrs"
	insertTestToken(t, s, tokenID, "red-machine", time.Hour)
	if err := s.RedeemProvisioningToken(t.Context(), tokenID); err != nil {
		t.Fatalf("RedeemProvisioningToken: %v", err)
	}

	router := serveScriptViaRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provision/"+tokenID+"/script", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("status = %d, want %d", w.Code, http.StatusGone)
	}
}

func TestServeScript_SingleUse(t *testing.T) {
	adminClaims := &handler.UserClaims{UserID: "user-1", Role: "admin"}
	h, s := newProvisionHandlerFixture(t, adminClaims)

	tokenID := "single-use-token"
	insertTestToken(t, s, tokenID, "once-machine", time.Hour)

	router := serveScriptViaRouter(h)

	// First request should succeed.
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/provision/"+tokenID+"/script", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", w1.Code, http.StatusOK)
	}

	// Second request should fail because the token is now redeemed.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/provision/"+tokenID+"/script", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusGone {
		t.Errorf("second request status = %d, want %d (token should be single-use)", w2.Code, http.StatusGone)
	}
}

// --- JoinByCode tests ---

func TestJoinHandler_ValidCode(t *testing.T) {
	h, s := newProvisionHandlerFixture(t, &handler.UserClaims{UserID: "admin-id", Role: "admin"})
	ctx := context.Background()

	// Create a token with a short code
	tok := store.ProvisioningToken{
		Token:         "test-token-join",
		ShortCode:     "A3X9K2",
		MachineID:     "worker-join",
		TargetOS:      "linux",
		TargetArch:    "amd64",
		CACertPEM:     "ca-data",
		AgentCertPEM:  "cert-data",
		AgentKeyPEM:   "key-data",
		ServerAddress: "http://test.example.com",
		GRPCAddress:   "test.example.com:9090",
		CreatedBy:     "admin",
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(1 * time.Hour),
	}
	if err := s.CreateProvisioningToken(ctx, tok); err != nil {
		t.Fatalf("setup: %v", err)
	}

	body := `{"code":"A3X9K2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provision/join", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Post("/api/v1/provision/join", h.JoinByCode)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["machine_id"] != "worker-join" {
		t.Errorf("machine_id = %q, want %q", resp["machine_id"], "worker-join")
	}
	if resp["grpc_address"] != "test.example.com:9090" {
		t.Errorf("grpc_address = %q, want %q", resp["grpc_address"], "test.example.com:9090")
	}
}

func TestJoinHandler_InvalidCode(t *testing.T) {
	h, _ := newProvisionHandlerFixture(t, &handler.UserClaims{UserID: "admin-id", Role: "admin"})

	tests := []struct {
		name string
		body string
		code int
	}{
		{"empty body", `{}`, http.StatusBadRequest},
		{"missing code", `{"code":""}`, http.StatusBadRequest},
		{"too short", `{"code":"A3X9K"}`, http.StatusBadRequest},
		{"too long", `{"code":"A3X9K2B"}`, http.StatusBadRequest},
		{"invalid chars", `{"code":"a3x9k2"}`, http.StatusBadRequest},
		{"ambiguous O", `{"code":"A3XOK2"}`, http.StatusBadRequest},
		{"not found", `{"code":"ZZZZZZ"}`, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/provision/join", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r := chi.NewRouter()
			r.Post("/api/v1/provision/join", h.JoinByCode)
			r.ServeHTTP(w, req)

			if w.Code != tt.code {
				t.Errorf("status = %d, want %d; body = %s", w.Code, tt.code, w.Body.String())
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
