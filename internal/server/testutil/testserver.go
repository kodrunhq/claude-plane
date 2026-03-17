// Package testutil provides reusable test infrastructure for integration tests.
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/api"
	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/session"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// TestServer wraps an httptest.Server with the dependencies needed by
// integration tests. Create one with NewTestServer.
type TestServer struct {
	Server *httptest.Server
	Store  *store.Store
	Auth   *auth.Service
}

// NewTestServer creates a test HTTP server backed by a real SQLite database.
// It wires a subset of the production stack (auth, job/template/session/user
// handlers, machine endpoints) but omits gRPC, WebSocket, event bus fanout,
// background workers, and some handlers (webhooks, triggers, schedules,
// credentials, provisioning, runs). Add handlers as needed for new tests.
//
// The caller should call ts.Close() (or use t.Cleanup) when done.
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()

	s := MustNewStore(t)

	blocklist, err := auth.NewBlocklist(s)
	if err != nil {
		t.Fatalf("testutil.NewTestServer: create blocklist: %v", err)
	}

	authSvc := auth.NewService(
		[]byte("test-secret-key-32-bytes-long!!!"),
		15*time.Minute,
		blocklist,
	)

	cm := connmgr.NewConnectionManager(s, nil)

	// Claims getter adapters — mirrors cmd/server/main.go
	sessionClaimsGetter := func(r *http.Request) *session.UserClaims {
		c := api.GetClaims(r)
		if c == nil {
			return nil
		}
		return &session.UserClaims{UserID: c.UserID, Role: c.Role}
	}
	handlerClaimsGetter := func(r *http.Request) *handler.UserClaims {
		c := api.GetClaims(r)
		if c == nil {
			return nil
		}
		return &handler.UserClaims{UserID: c.UserID, Role: c.Role, Scopes: c.Scopes}
	}

	// Domain handlers
	jobHandler := handler.NewJobHandler(s, handlerClaimsGetter)
	templateHandler := handler.NewTemplateHandler(s, handlerClaimsGetter)

	registry := session.NewRegistry(slog.Default())
	sessionHandler := session.NewSessionHandler(s, cm, registry, sessionClaimsGetter, slog.Default())

	userHandler := handler.NewUserHandler(s, handlerClaimsGetter)

	// Core router (auth endpoints, machines, sessions)
	handlers := api.NewHandlers(s, authSvc, cm, "open", "")
	router := api.NewRouter(api.RouterDeps{
		Handlers:       handlers,
		SessionHandler: sessionHandler,
		JobHandler:     jobHandler,
		UserHandler:    userHandler,
	})

	// Template routes — JWT-protected, matching production wiring
	router.Group(func(r chi.Router) {
		r.Use(api.JWTAuthMiddleware(authSvc))
		handler.RegisterTemplateRoutes(r, templateHandler)
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &TestServer{
		Server: srv,
		Store:  s,
		Auth:   authSvc,
	}
}

// Close shuts down the test server. Prefer t.Cleanup via NewTestServer.
func (ts *TestServer) Close() {
	ts.Server.Close()
}

// URL returns the base URL of the test server.
func (ts *TestServer) URL() string {
	return ts.Server.URL
}

// LoginAsAdmin registers an admin user, logs in, and returns the session
// cookies needed for authenticated requests. The admin user is seeded via the
// store so that it has the "admin" role (the register endpoint creates "user"
// role accounts).
func (ts *TestServer) LoginAsAdmin(t *testing.T) []*http.Cookie {
	t.Helper()

	// Register a normal user first (open registration mode).
	email := "admin@integration.test"
	password := "integration-test-pw"

	body, err := json.Marshal(map[string]string{
		"email":        email,
		"password":     password,
		"display_name": "Integration Admin",
	})
	if err != nil {
		t.Fatalf("LoginAsAdmin: marshal register body: %v", err)
	}
	resp, err := http.Post(ts.URL()+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("LoginAsAdmin: register: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("LoginAsAdmin: register returned %d", resp.StatusCode)
	}

	// Login to get the token/cookies.
	var loginResult map[string]string
	body, err = json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	if err != nil {
		t.Fatalf("LoginAsAdmin: marshal login body: %v", err)
	}
	loginResp, err := http.Post(ts.URL()+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("LoginAsAdmin: login: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("LoginAsAdmin: login returned %d", loginResp.StatusCode)
	}

	if err := json.NewDecoder(loginResp.Body).Decode(&loginResult); err != nil {
		t.Fatalf("LoginAsAdmin: decode login response: %v", err)
	}
	userID := loginResult["user_id"]
	if userID == "" {
		t.Fatal("LoginAsAdmin: empty user_id in login response")
	}

	// Promote to admin in the store.
	if err := ts.Store.UpdateUser(context.Background(), userID, "Integration Admin", "admin"); err != nil {
		t.Fatalf("LoginAsAdmin: promote to admin: %v", err)
	}

	// Login again so the JWT contains the admin role.
	body, err = json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	if err != nil {
		t.Fatalf("LoginAsAdmin: marshal second login body: %v", err)
	}
	loginResp2, err := http.Post(ts.URL()+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("LoginAsAdmin: second login: %v", err)
	}
	defer loginResp2.Body.Close()
	if loginResp2.StatusCode != http.StatusOK {
		t.Fatalf("LoginAsAdmin: second login returned %d", loginResp2.StatusCode)
	}

	cookies := loginResp2.Cookies()
	if len(cookies) == 0 {
		t.Fatal("LoginAsAdmin: no cookies returned from login")
	}
	return cookies
}

// AuthRequest makes an authenticated HTTP request using the given cookies.
// The body parameter may be nil for requests without a body.
func (ts *TestServer) AuthRequest(t *testing.T, method, path string, body interface{}, cookies []*http.Cookie) *http.Response {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("AuthRequest: marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, ts.URL()+path, bodyReader)
	if err != nil {
		t.Fatalf("AuthRequest: create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}

	client := &http.Client{
		// Do not follow redirects.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("AuthRequest: do request: %v", err)
	}
	return resp
}
