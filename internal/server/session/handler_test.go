package session_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/session"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// mockMachineStore implements connmgr.MachineStore for tests.
type mockMachineStore struct{}

func (m *mockMachineStore) UpsertMachine(string, int32) error                   { return nil }
func (m *mockMachineStore) UpdateMachineStatus(string, string, time.Time) error { return nil }

// commandRecorder records commands sent to a mock agent.
type commandRecorder struct {
	mu       sync.Mutex
	commands []*pb.ServerCommand
}

func (cr *commandRecorder) send(cmd *pb.ServerCommand) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.commands = append(cr.commands, cmd)
	return nil
}

func (cr *commandRecorder) count() int {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	return len(cr.commands)
}

func setupTestHandler(t *testing.T) (*session.SessionHandler, *connmgr.ConnectionManager, *commandRecorder, *store.Store, chi.Router) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cm := connmgr.NewConnectionManager(&mockMachineStore{}, nil)
	reg := session.NewRegistry(slog.Default())
	recorder := &commandRecorder{}

	// nil getClaims = unauthenticated mode (no auth configured)
	handler := session.NewSessionHandler(st, cm, reg, nil, slog.Default())

	r := chi.NewRouter()
	r.Post("/api/v1/sessions", handler.CreateSession)
	r.Get("/api/v1/sessions", handler.ListSessions)
	r.Get("/api/v1/sessions/{sessionID}", handler.GetSession)
	r.Delete("/api/v1/sessions/{sessionID}", handler.TerminateSession)

	return handler, cm, recorder, st, r
}

func TestCreateSession(t *testing.T) {
	_, cm, recorder, st, router := setupTestHandler(t)

	// Register a connected agent with machine
	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	})

	body := `{"machine_id":"machine-a","command":"claude","working_dir":"/tmp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["session_id"] == "" {
		t.Error("expected non-empty session_id")
	}
	if resp["status"] != "created" {
		t.Errorf("status = %v, want created", resp["status"])
	}

	// Verify command was sent to agent
	if recorder.count() != 1 {
		t.Errorf("commands sent = %d, want 1", recorder.count())
	}
}

func TestCreateSessionMachineNotConnected(t *testing.T) {
	_, _, _, _, router := setupTestHandler(t)

	body := `{"machine_id":"unknown-machine"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestListSessions(t *testing.T) {
	_, cm, recorder, st, router := setupTestHandler(t)

	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	})

	// Create 2 sessions
	for i := range 2 {
		body := fmt.Sprintf(`{"machine_id":"machine-a","command":"claude-%d"}`, i)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create session %d: status = %d", i, w.Code)
		}
	}

	// List sessions
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}

	var sessions []interface{}
	if err := json.NewDecoder(w.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("sessions count = %d, want 2", len(sessions))
	}
}

func TestTerminateSession(t *testing.T) {
	_, cm, recorder, st, router := setupTestHandler(t)

	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	})

	// Create a session
	body := `{"machine_id":"machine-a"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	sessionID := resp["session_id"].(string)

	// Terminate
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+sessionID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("terminate status = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify KillSessionCmd was sent (2 commands: 1 create + 1 kill)
	if recorder.count() != 2 {
		t.Errorf("commands sent = %d, want 2", recorder.count())
	}

	// Verify session status is terminated
	sess, err := st.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.Status != "terminated" {
		t.Errorf("status = %q, want %q", sess.Status, "terminated")
	}
}

// createTestUser is a helper to ensure a user exists in the DB for FK constraints.
func createTestUser(t *testing.T, st *store.Store, userID string) {
	t.Helper()
	_ = st.CreateUser(&store.User{
		UserID:       userID,
		Email:        userID + "@test.com",
		DisplayName:  userID,
		PasswordHash: "not-used",
		Role:         "user",
	})
}

// setupAuthTestEnv sets up a shared test environment for authorization tests.
func setupAuthTestEnv(t *testing.T) (*connmgr.ConnectionManager, *commandRecorder, *store.Store, *session.Registry) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cm := connmgr.NewConnectionManager(&mockMachineStore{}, nil)
	reg := session.NewRegistry(slog.Default())
	recorder := &commandRecorder{}

	return cm, recorder, st, reg
}

func TestGetSession_AuthorizationNonOwner(t *testing.T) {
	cm, recorder, st, reg := setupAuthTestEnv(t)

	// Create users for FK constraints
	createTestUser(t, st, "user-owner")
	createTestUser(t, st, "user-other")

	// Register agent
	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	})

	// Create a session as user-owner
	ownerClaims := func(r *http.Request) *session.UserClaims {
		return &session.UserClaims{UserID: "user-owner", Role: "user"}
	}
	ownerHandler := session.NewSessionHandler(st, cm, reg, ownerClaims, slog.Default())
	ownerRouter := chi.NewRouter()
	ownerRouter.Post("/api/v1/sessions", ownerHandler.CreateSession)

	body := `{"machine_id":"machine-a"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ownerRouter.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	sessionID := resp["session_id"].(string)

	// Now try to GET/DELETE with a different non-admin user
	otherClaims := func(r *http.Request) *session.UserClaims {
		return &session.UserClaims{UserID: "user-other", Role: "user"}
	}
	otherHandler := session.NewSessionHandler(st, cm, reg, otherClaims, slog.Default())
	otherRouter := chi.NewRouter()
	otherRouter.Get("/api/v1/sessions/{sessionID}", otherHandler.GetSession)
	otherRouter.Delete("/api/v1/sessions/{sessionID}", otherHandler.TerminateSession)

	// GET should return 404 (not 403)
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID, nil)
	getW := httptest.NewRecorder()
	otherRouter.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusNotFound {
		t.Errorf("GET by non-owner: status = %d, want 404", getW.Code)
	}

	// DELETE should return 404 (not 403)
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+sessionID, nil)
	delW := httptest.NewRecorder()
	otherRouter.ServeHTTP(delW, delReq)
	if delW.Code != http.StatusNotFound {
		t.Errorf("DELETE by non-owner: status = %d, want 404", delW.Code)
	}
}

func TestGetSession_AdminCanAccessAny(t *testing.T) {
	cm, recorder, st, reg := setupAuthTestEnv(t)

	createTestUser(t, st, "user-owner")

	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	})

	// Create a session as user-owner
	ownerClaims := func(r *http.Request) *session.UserClaims {
		return &session.UserClaims{UserID: "user-owner", Role: "user"}
	}
	ownerHandler := session.NewSessionHandler(st, cm, reg, ownerClaims, slog.Default())
	ownerRouter := chi.NewRouter()
	ownerRouter.Post("/api/v1/sessions", ownerHandler.CreateSession)

	body := `{"machine_id":"machine-a"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ownerRouter.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	sessionID := resp["session_id"].(string)

	// Admin should be able to access
	adminClaims := func(r *http.Request) *session.UserClaims {
		return &session.UserClaims{UserID: "admin-user", Role: "admin"}
	}
	adminHandler := session.NewSessionHandler(st, cm, reg, adminClaims, slog.Default())
	adminRouter := chi.NewRouter()
	adminRouter.Get("/api/v1/sessions/{sessionID}", adminHandler.GetSession)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID, nil)
	getW := httptest.NewRecorder()
	adminRouter.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Errorf("GET by admin: status = %d, want 200", getW.Code)
	}
}

func TestListSessions_FiltersByOwnership(t *testing.T) {
	cm, recorder, st, reg := setupAuthTestEnv(t)

	createTestUser(t, st, "user-a")
	createTestUser(t, st, "user-b")

	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	})

	// Create session as user-a
	claimsA := func(r *http.Request) *session.UserClaims {
		return &session.UserClaims{UserID: "user-a", Role: "user"}
	}
	hA := session.NewSessionHandler(st, cm, reg, claimsA, slog.Default())
	rA := chi.NewRouter()
	rA.Post("/api/v1/sessions", hA.CreateSession)
	rA.Get("/api/v1/sessions", hA.ListSessions)

	body := `{"machine_id":"machine-a"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rA.ServeHTTP(w, req)

	// Create session as user-b
	claimsB := func(r *http.Request) *session.UserClaims {
		return &session.UserClaims{UserID: "user-b", Role: "user"}
	}
	hB := session.NewSessionHandler(st, cm, reg, claimsB, slog.Default())
	rB := chi.NewRouter()
	rB.Post("/api/v1/sessions", hB.CreateSession)
	rB.Get("/api/v1/sessions", hB.ListSessions)

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	rB.ServeHTTP(w2, req2)

	// user-a should only see 1 session
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	listW := httptest.NewRecorder()
	rA.ServeHTTP(listW, listReq)

	var sessions []interface{}
	json.NewDecoder(listW.Body).Decode(&sessions)
	if len(sessions) != 1 {
		t.Errorf("user-a sessions count = %d, want 1", len(sessions))
	}

	// Admin should see both
	adminClaims := func(r *http.Request) *session.UserClaims {
		return &session.UserClaims{UserID: "admin-user", Role: "admin"}
	}
	hAdmin := session.NewSessionHandler(st, cm, reg, adminClaims, slog.Default())
	rAdmin := chi.NewRouter()
	rAdmin.Get("/api/v1/sessions", hAdmin.ListSessions)

	adminListReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	adminListW := httptest.NewRecorder()
	rAdmin.ServeHTTP(adminListW, adminListReq)

	var adminSessions []interface{}
	json.NewDecoder(adminListW.Body).Decode(&adminSessions)
	if len(adminSessions) != 2 {
		t.Errorf("admin sessions count = %d, want 2", len(adminSessions))
	}
}

func TestAuthorizeSession_NilClaimsDenied(t *testing.T) {
	cm, recorder, st, reg := setupAuthTestEnv(t)

	createTestUser(t, st, "user-owner")

	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	})

	// Create a session directly in the DB
	if err := st.CreateSession(&store.Session{
		SessionID: "sess-nil-claims",
		MachineID: "machine-a",
		UserID:    "user-owner",
		Command:   "claude",
		Status:    store.StatusCreated,
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// getClaims is non-nil (auth is configured) but returns nil claims
	// (simulates misconfigured middleware or missing auth header)
	nilClaims := func(r *http.Request) *session.UserClaims { return nil }
	h := session.NewSessionHandler(st, cm, reg, nilClaims, slog.Default())
	r := chi.NewRouter()
	r.Get("/api/v1/sessions/{sessionID}", h.GetSession)
	r.Get("/api/v1/sessions", h.ListSessions)

	// GET session should be denied (404, not 200)
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-nil-claims", nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusNotFound {
		t.Errorf("GET with nil claims: status = %d, want 404", getW.Code)
	}

	// LIST sessions should return 401
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	listW := httptest.NewRecorder()
	r.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusUnauthorized {
		t.Errorf("LIST with nil claims: status = %d, want 401", listW.Code)
	}
}
