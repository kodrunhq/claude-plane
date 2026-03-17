package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
)

func newSettingsRouter(getClaims handler.ClaimsGetter) *httptest.Server {
	s := newTestStore(&testing.T{})
	h := handler.NewSettingsHandler(s, getClaims)
	r := chi.NewRouter()
	handler.RegisterSettingsRoutes(r, h)
	return httptest.NewServer(r)
}

func newSettingsRouterWithCleanup(t *testing.T, getClaims handler.ClaimsGetter) *httptest.Server {
	t.Helper()
	s := newTestStore(t)
	h := handler.NewSettingsHandler(s, getClaims)
	r := chi.NewRouter()
	handler.RegisterSettingsRoutes(r, h)
	return httptest.NewServer(r)
}

func TestSettingsHandler_NonAdminForbidden(t *testing.T) {
	getClaims := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "user-1", Role: "member"}
	}

	srv := newSettingsRouterWithCleanup(t, getClaims)
	defer srv.Close()

	body := map[string]string{"retention_days": "30"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/settings", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d", resp.StatusCode)
	}
}

func TestSettingsHandler_GetSettings_NonAdminForbidden(t *testing.T) {
	getClaims := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "user-1", Role: "member"}
	}

	srv := newSettingsRouterWithCleanup(t, getClaims)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/settings")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d", resp.StatusCode)
	}
}

func TestSettingsHandler_NilClaimsForbidden(t *testing.T) {
	getClaims := func(r *http.Request) *handler.UserClaims {
		return nil
	}

	srv := newSettingsRouterWithCleanup(t, getClaims)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/settings")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for nil claims, got %d", resp.StatusCode)
	}
}

func TestSettingsHandler_UnknownKeyRejected(t *testing.T) {
	getClaims := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "admin-1", Role: "admin"}
	}

	srv := newSettingsRouterWithCleanup(t, getClaims)
	defer srv.Close()

	body := map[string]string{"unknown_key": "value"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/settings", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown key, got %d", resp.StatusCode)
	}
}

func TestSettingsHandler_ValidRetentionDays(t *testing.T) {
	getClaims := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "admin-1", Role: "admin"}
	}

	srv := newSettingsRouterWithCleanup(t, getClaims)
	defer srv.Close()

	body := map[string]string{"retention_days": "30"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/settings", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for valid retention_days, got %d", resp.StatusCode)
	}
}

func TestSettingsHandler_InvalidRetentionDays(t *testing.T) {
	getClaims := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "admin-1", Role: "admin"}
	}

	srv := newSettingsRouterWithCleanup(t, getClaims)
	defer srv.Close()

	body := map[string]string{"retention_days": "42"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/settings", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid retention_days, got %d", resp.StatusCode)
	}
}
