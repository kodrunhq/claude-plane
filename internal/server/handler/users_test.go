package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// newUserRouter creates a chi router with user routes and a test user for testing.
func newUserRouter(t *testing.T, callerID, callerRole string) (*httptest.Server, *store.Store) {
	t.Helper()
	s := newTestStore(t)

	// Create a test user with a known password.
	hash, err := store.HashPassword("oldpassword1")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	err = s.CreateUser(&store.User{
		UserID:       "user-1",
		Email:        "test@example.com",
		DisplayName:  "Test User",
		PasswordHash: hash,
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}

	// Create an admin user.
	adminHash, err := store.HashPassword("adminpass1")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	err = s.CreateUser(&store.User{
		UserID:       "admin-1",
		Email:        "admin@example.com",
		DisplayName:  "Admin User",
		PasswordHash: adminHash,
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	claims := testClaimsGetter(callerID, callerRole)
	h := handler.NewUserHandler(s, claims)
	r := chi.NewRouter()
	handler.RegisterUserRoutes(r, h)
	return httptest.NewServer(r), s
}

func TestChangePassword_Success(t *testing.T) {
	srv, _ := newUserRouter(t, "user-1", "user")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"current_password": "oldpassword1",
		"new_password":     "newpassword1",
	})
	resp, err := http.Post(srv.URL+"/api/v1/users/me/password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestChangePassword_WrongCurrent(t *testing.T) {
	srv, _ := newUserRouter(t, "user-1", "user")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"current_password": "wrongpassword",
		"new_password":     "newpassword1",
	})
	resp, err := http.Post(srv.URL+"/api/v1/users/me/password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestChangePassword_TooShort(t *testing.T) {
	srv, _ := newUserRouter(t, "user-1", "user")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"current_password": "oldpassword1",
		"new_password":     "short",
	})
	resp, err := http.Post(srv.URL+"/api/v1/users/me/password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestResetPassword_AdminSuccess(t *testing.T) {
	srv, _ := newUserRouter(t, "admin-1", "admin")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"new_password": "resetpass1",
	})
	resp, err := http.Post(srv.URL+"/api/v1/users/user-1/reset-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestResetPassword_NonAdminForbidden(t *testing.T) {
	srv, _ := newUserRouter(t, "user-1", "user")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"new_password": "resetpass1",
	})
	resp, err := http.Post(srv.URL+"/api/v1/users/user-1/reset-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestResetPassword_TooShort(t *testing.T) {
	srv, _ := newUserRouter(t, "admin-1", "admin")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"new_password": "short",
	})
	resp, err := http.Post(srv.URL+"/api/v1/users/user-1/reset-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestResetPassword_UserNotFound(t *testing.T) {
	srv, _ := newUserRouter(t, "admin-1", "admin")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"new_password": "resetpass1",
	})
	resp, err := http.Post(srv.URL+"/api/v1/users/nonexistent/reset-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestUpdateProfile_Success(t *testing.T) {
	srv, s := newUserRouter(t, "user-1", "user")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"display_name": "New Name",
	})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/users/me", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var updated store.User
	json.NewDecoder(resp.Body).Decode(&updated)
	if updated.DisplayName != "New Name" {
		t.Errorf("expected display_name 'New Name', got %q", updated.DisplayName)
	}

	// Verify in the database.
	u, err := s.GetUserByID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u.DisplayName != "New Name" {
		t.Errorf("DB display_name: expected 'New Name', got %q", u.DisplayName)
	}
}
