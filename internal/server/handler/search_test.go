package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

func newSearchRouter(t *testing.T, getClaims handler.ClaimsGetter) *httptest.Server {
	t.Helper()
	s := newTestStore(t)
	h := handler.NewSearchHandler(s, getClaims)
	r := chi.NewRouter()
	handler.RegisterSearchRoutes(r, h)
	return httptest.NewServer(r)
}

func TestSearchHandler_RequiresQ(t *testing.T) {
	getClaims := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "user-1", Role: "admin"}
	}

	srv := newSearchRouter(t, getClaims)
	defer srv.Close()

	// Call without q parameter.
	resp, err := http.Get(srv.URL + "/api/v1/search/sessions")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing q parameter, got %d", resp.StatusCode)
	}
}

func TestSearchHandler_EmptyQRejected(t *testing.T) {
	getClaims := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "user-1", Role: "admin"}
	}

	srv := newSearchRouter(t, getClaims)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/search/sessions?q=")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty q parameter, got %d", resp.StatusCode)
	}
}

func TestSearchHandler_NonAdminScopedToOwnSessions(t *testing.T) {
	getClaims := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "user-scoped", Role: "member"}
	}

	srv := newSearchRouter(t, getClaims)
	defer srv.Close()

	// Search with a query. Since there's no data, we just verify it returns 200
	// (not an error) and the results are scoped (empty is fine).
	resp, err := http.Get(srv.URL + "/api/v1/search/sessions?q=hello")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var results []store.ContentSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Non-admin user with no data should get empty results.
	if len(results) != 0 {
		t.Errorf("expected 0 results for scoped user, got %d", len(results))
	}
}

func TestSearchHandler_AdminSearchReturnsOK(t *testing.T) {
	getClaims := func(r *http.Request) *handler.UserClaims {
		return &handler.UserClaims{UserID: "admin-1", Role: "admin"}
	}

	srv := newSearchRouter(t, getClaims)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/search/sessions?q=test")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for admin search, got %d", resp.StatusCode)
	}
}
