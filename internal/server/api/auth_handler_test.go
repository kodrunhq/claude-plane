package api_test

import (
	"bytes"
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

// setupTestAPI creates a full test API server with in-memory SQLite,
// JWT service, and connection manager.
func setupTestAPI(t *testing.T) *httptest.Server {
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
	router := api.NewRouter(handlers, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	return httptest.NewServer(router)
}

// registerUser is a test helper that registers a user and returns the response.
func registerUser(t *testing.T, srv *httptest.Server, email, password, displayName string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":        email,
		"password":     password,
		"display_name": displayName,
	})
	resp, err := http.Post(srv.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("register request: %v", err)
	}
	return resp
}

// loginUser is a test helper that logs in and returns the token.
func loginUser(t *testing.T, srv *httptest.Server, email, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	resp, err := http.Post(srv.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login failed: status %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	return result["token"]
}

func TestRegisterSuccess(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "test@example.com", "password123", "Test User")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	if result["user_id"] == "" {
		t.Error("expected user_id in response")
	}
	if result["email"] != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", result["email"])
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "dupe@example.com", "password123", "User One")
	resp.Body.Close()

	resp2 := registerUser(t, srv, "dupe@example.com", "password456", "User Two")
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp2.StatusCode)
	}
}

func TestRegisterInvalidInput(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	// Empty email
	resp := registerUser(t, srv, "", "password123", "Test")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty email: expected 400, got %d", resp.StatusCode)
	}

	// Short password
	resp = registerUser(t, srv, "test@example.com", "short", "Test")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("short password: expected 400, got %d", resp.StatusCode)
	}
}

func TestLoginSuccess(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "login@example.com", "password123", "Login User")
	resp.Body.Close()

	body, _ := json.Marshal(map[string]string{
		"email":    "login@example.com",
		"password": "password123",
	})
	loginResp, err := http.Post(srv.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", loginResp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(loginResp.Body).Decode(&result)

	if result["token"] == "" {
		t.Error("expected token in response")
	}
	if result["user_id"] == "" {
		t.Error("expected user_id in response")
	}
	if result["email"] != "login@example.com" {
		t.Errorf("expected email login@example.com, got %s", result["email"])
	}
	if result["role"] != "user" {
		t.Errorf("expected role user, got %s", result["role"])
	}
}

func TestLoginSessionCookieSecureFlag(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "cookie@example.com", "password123", "Cookie User")
	resp.Body.Close()

	body, _ := json.Marshal(map[string]string{
		"email":    "cookie@example.com",
		"password": "password123",
	})

	t.Run("HTTP request sets non-secure cookie", func(t *testing.T) {
		loginReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login", bytes.NewReader(body))
		loginReq.Header.Set("Content-Type", "application/json")
		loginResp, err := http.DefaultClient.Do(loginReq)
		if err != nil {
			t.Fatalf("login request: %v", err)
		}
		defer loginResp.Body.Close()

		var sessionCookie *http.Cookie
		for _, c := range loginResp.Cookies() {
			if c.Name == "session_token" {
				sessionCookie = c
				break
			}
		}
		if sessionCookie == nil {
			t.Fatal("expected session_token cookie to be set")
		}
		if sessionCookie.Secure {
			t.Error("expected session_token cookie to be non-secure for HTTP requests")
		}
	})

	t.Run("forwarded HTTPS request sets secure cookie", func(t *testing.T) {
		loginReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login", bytes.NewReader(body))
		loginReq.Header.Set("Content-Type", "application/json")
		loginReq.Header.Set("X-Forwarded-Proto", "https")
		loginResp, err := http.DefaultClient.Do(loginReq)
		if err != nil {
			t.Fatalf("login request: %v", err)
		}
		defer loginResp.Body.Close()

		var sessionCookie *http.Cookie
		for _, c := range loginResp.Cookies() {
			if c.Name == "session_token" {
				sessionCookie = c
				break
			}
		}
		if sessionCookie == nil {
			t.Fatal("expected session_token cookie to be set")
		}
		if !sessionCookie.Secure {
			t.Error("expected session_token cookie to be secure when forwarded proto is https")
		}
	})
}

func TestLoginInvalidCredentials(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "valid@example.com", "password123", "Valid User")
	resp.Body.Close()

	// Wrong password
	body, _ := json.Marshal(map[string]string{
		"email":    "valid@example.com",
		"password": "wrongpassword",
	})
	loginResp, err := http.Post(srv.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong password: expected 401, got %d", loginResp.StatusCode)
	}

	// Non-existent email
	body, _ = json.Marshal(map[string]string{
		"email":    "nonexistent@example.com",
		"password": "password123",
	})
	loginResp, err = http.Post(srv.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("non-existent user: expected 401, got %d", loginResp.StatusCode)
	}
}

func TestLogoutSuccess(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "logout@example.com", "password123", "Logout User")
	resp.Body.Close()

	token := loginUser(t, srv, "logout@example.com", "password123")

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	logoutResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("logout request: %v", err)
	}
	defer logoutResp.Body.Close()

	if logoutResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", logoutResp.StatusCode)
	}
}

func TestLogoutRevokesToken(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	resp := registerUser(t, srv, "revoke@example.com", "password123", "Revoke User")
	resp.Body.Close()

	token := loginUser(t, srv, "revoke@example.com", "password123")

	// Logout
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	logoutResp, _ := http.DefaultClient.Do(req)
	logoutResp.Body.Close()

	// Try to use the revoked token
	req, _ = http.NewRequest("GET", srv.URL+"/api/v1/machines", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	machinesResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("machines request: %v", err)
	}
	defer machinesResp.Body.Close()

	if machinesResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("revoked token: expected 401, got %d", machinesResp.StatusCode)
	}
}

// setupTestAPIWithMode creates a test API server with the given registration mode.
func setupTestAPIWithMode(t *testing.T, mode, inviteCode string) *httptest.Server {
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

	handlers := api.NewHandlers(s, authSvc, cm, mode, inviteCode)
	router := api.NewRouter(handlers, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	return httptest.NewServer(router)
}

func TestRegisterClosedMode(t *testing.T) {
	srv := setupTestAPIWithMode(t, "closed", "")
	defer srv.Close()

	resp := registerUser(t, srv, "test@example.com", "password123", "Test User")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for closed registration, got %d", resp.StatusCode)
	}
}

func TestRegisterDefaultMode(t *testing.T) {
	// Default (empty string) should behave as "closed"
	srv := setupTestAPIWithMode(t, "", "")
	defer srv.Close()

	resp := registerUser(t, srv, "test@example.com", "password123", "Test User")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for default (closed) registration, got %d", resp.StatusCode)
	}
}

func TestRegisterInviteModeValid(t *testing.T) {
	srv := setupTestAPIWithMode(t, "invite", "secret-invite-code")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"email":        "invited@example.com",
		"password":     "password123",
		"display_name": "Invited User",
		"invite_code":  "secret-invite-code",
	})
	resp, err := http.Post(srv.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("register request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 with valid invite code, got %d", resp.StatusCode)
	}
}

func TestRegisterInviteModeInvalidCode(t *testing.T) {
	srv := setupTestAPIWithMode(t, "invite", "secret-invite-code")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"email":        "bad@example.com",
		"password":     "password123",
		"display_name": "Bad User",
		"invite_code":  "wrong-code",
	})
	resp, err := http.Post(srv.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("register request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 with invalid invite code, got %d", resp.StatusCode)
	}
}

func TestRegisterInviteModeMissingCode(t *testing.T) {
	srv := setupTestAPIWithMode(t, "invite", "secret-invite-code")
	defer srv.Close()

	// No invite_code in request body
	resp := registerUser(t, srv, "no-code@example.com", "password123", "No Code User")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 with missing invite code, got %d", resp.StatusCode)
	}
}

func TestProtectedEndpointNoAuth(t *testing.T) {
	srv := setupTestAPI(t)
	defer srv.Close()

	// No header
	resp, err := http.Get(srv.URL + "/api/v1/machines")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no header: expected 401, got %d", resp.StatusCode)
	}

	// Invalid token
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/machines", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("invalid token: expected 401, got %d", resp.StatusCode)
	}
}
