package session_test

import (
	"bytes"
	"context"
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
	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/session"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// noopSubscriber is a minimal event.Subscriber for handler tests.
type noopSubscriber struct{}

func (n *noopSubscriber) Subscribe(_ string, _ event.HandlerFunc, _ event.SubscriberOptions) func() {
	return func() {}
}

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

func (cr *commandRecorder) last() *pb.ServerCommand {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if len(cr.commands) == 0 {
		return nil
	}
	return cr.commands[len(cr.commands)-1]
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
	if err := cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	}); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

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
	if err := cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	}); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

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
	if err := cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	}); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

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
	if err := cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	}); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

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
	if err := cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	}); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

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
	if err := cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	}); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

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

// setupTemplateTestEnv creates a test environment with auth, a connected agent, and a user.
// Returns all components needed for template-aware session creation tests.
func setupTemplateTestEnv(t *testing.T, userID string) (
	*store.Store,
	*connmgr.ConnectionManager,
	*commandRecorder,
	*session.Registry,
	session.ClaimsGetter,
) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	createTestUser(t, st, userID)

	cm := connmgr.NewConnectionManager(&mockMachineStore{}, nil)
	reg := session.NewRegistry(slog.Default())
	recorder := &commandRecorder{}

	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	if err := cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	}); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	getClaims := func(r *http.Request) *session.UserClaims {
		return &session.UserClaims{UserID: userID, Role: "user"}
	}

	return st, cm, recorder, reg, getClaims
}

func TestCreateSession_WithTemplateID(t *testing.T) {
	st, cm, recorder, reg, getClaims := setupTemplateTestEnv(t, "user-tmpl")

	// Create a template
	tmpl, err := st.CreateTemplate(context.Background(), &store.SessionTemplate{
		UserID:        "user-tmpl",
		Name:          "my-template",
		Command:       "claude-code",
		Args:          []string{"--model", "opus"},
		WorkingDir:    "/projects/foo",
		InitialPrompt: "Hello world",
		EnvVars:       map[string]string{"FOO": "bar"},
		TerminalRows:  40,
		TerminalCols:  120,
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	handler := session.NewSessionHandler(st, cm, reg, getClaims, slog.Default())
	router := chi.NewRouter()
	router.Post("/api/v1/sessions", handler.CreateSession)

	body := fmt.Sprintf(`{"machine_id":"machine-a","template_id":%q}`, tmpl.TemplateID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	// Command should come from template
	if resp["command"] != "claude-code" {
		t.Errorf("command = %v, want claude-code", resp["command"])
	}

	// Verify proto command includes template fields
	cmd := recorder.last()
	if cmd == nil {
		t.Fatal("expected command to be sent")
	}
	createCmd := cmd.GetCreateSession()
	if createCmd == nil {
		t.Fatal("expected CreateSession command")
	}
	if createCmd.Command != "claude-code" {
		t.Errorf("proto command = %q, want claude-code", createCmd.Command)
	}
	if createCmd.WorkingDir != "/projects/foo" {
		t.Errorf("proto working_dir = %q, want /projects/foo", createCmd.WorkingDir)
	}
	if createCmd.InitialPrompt != "Hello world" {
		t.Errorf("proto initial_prompt = %q, want Hello world", createCmd.InitialPrompt)
	}
	if createCmd.EnvVars["FOO"] != "bar" {
		t.Errorf("proto env_vars[FOO] = %q, want bar", createCmd.EnvVars["FOO"])
	}
	if len(createCmd.Args) != 2 || createCmd.Args[0] != "--model" || createCmd.Args[1] != "opus" {
		t.Errorf("proto args = %v, want [--model opus]", createCmd.Args)
	}

	// Verify session in DB has template_id
	sessionID := resp["session_id"].(string)
	sess, err := st.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.TemplateID != tmpl.TemplateID {
		t.Errorf("session template_id = %q, want %q", sess.TemplateID, tmpl.TemplateID)
	}
}

func TestCreateSession_WithTemplateName(t *testing.T) {
	st, cm, recorder, reg, getClaims := setupTemplateTestEnv(t, "user-tmpl-name")

	_, err := st.CreateTemplate(context.Background(), &store.SessionTemplate{
		UserID:  "user-tmpl-name",
		Name:    "named-template",
		Command: "special-command",
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	handler := session.NewSessionHandler(st, cm, reg, getClaims, slog.Default())
	router := chi.NewRouter()
	router.Post("/api/v1/sessions", handler.CreateSession)

	body := `{"machine_id":"machine-a","template_name":"named-template"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	cmd := recorder.last()
	createCmd := cmd.GetCreateSession()
	if createCmd.Command != "special-command" {
		t.Errorf("command = %q, want special-command", createCmd.Command)
	}
}

func TestCreateSession_TemplateWithExplicitOverride(t *testing.T) {
	st, cm, recorder, reg, getClaims := setupTemplateTestEnv(t, "user-override")

	_, err := st.CreateTemplate(context.Background(), &store.SessionTemplate{
		UserID:     "user-override",
		Name:       "override-test",
		Command:    "template-cmd",
		WorkingDir: "/template/dir",
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	handler := session.NewSessionHandler(st, cm, reg, getClaims, slog.Default())
	router := chi.NewRouter()
	router.Post("/api/v1/sessions", handler.CreateSession)

	// Explicit command should override template
	body := `{"machine_id":"machine-a","template_name":"override-test","command":"explicit-cmd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	cmd := recorder.last()
	createCmd := cmd.GetCreateSession()
	if createCmd.Command != "explicit-cmd" {
		t.Errorf("command = %q, want explicit-cmd (explicit override)", createCmd.Command)
	}
	// WorkingDir should come from template since not overridden
	if createCmd.WorkingDir != "/template/dir" {
		t.Errorf("working_dir = %q, want /template/dir (from template)", createCmd.WorkingDir)
	}
}

func TestCreateSession_TemplateVariableSubstitution(t *testing.T) {
	st, cm, recorder, reg, getClaims := setupTemplateTestEnv(t, "user-vars")

	_, err := st.CreateTemplate(context.Background(), &store.SessionTemplate{
		UserID:        "user-vars",
		Name:          "pr-review",
		Command:       "claude",
		InitialPrompt: "Review this PR: ${PR_URL} and focus on ${FOCUS_AREA}",
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	handler := session.NewSessionHandler(st, cm, reg, getClaims, slog.Default())
	router := chi.NewRouter()
	router.Post("/api/v1/sessions", handler.CreateSession)

	body := `{
		"machine_id": "machine-a",
		"template_name": "pr-review",
		"variables": {
			"PR_URL": "https://github.com/org/repo/pull/42",
			"FOCUS_AREA": "security"
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	cmd := recorder.last()
	createCmd := cmd.GetCreateSession()
	expected := "Review this PR: https://github.com/org/repo/pull/42 and focus on security"
	if createCmd.InitialPrompt != expected {
		t.Errorf("initial_prompt = %q, want %q", createCmd.InitialPrompt, expected)
	}
}

func TestCreateSession_TemplateEnvVarsInProto(t *testing.T) {
	st, cm, recorder, reg, getClaims := setupTemplateTestEnv(t, "user-env")

	_, err := st.CreateTemplate(context.Background(), &store.SessionTemplate{
		UserID:  "user-env",
		Name:    "env-template",
		Command: "claude",
		EnvVars: map[string]string{"API_KEY": "secret123", "DEBUG": "true"},
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	handler := session.NewSessionHandler(st, cm, reg, getClaims, slog.Default())
	router := chi.NewRouter()
	router.Post("/api/v1/sessions", handler.CreateSession)

	body := `{"machine_id":"machine-a","template_name":"env-template"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	cmd := recorder.last()
	createCmd := cmd.GetCreateSession()
	if len(createCmd.EnvVars) != 2 {
		t.Fatalf("env_vars count = %d, want 2", len(createCmd.EnvVars))
	}
	if createCmd.EnvVars["API_KEY"] != "secret123" {
		t.Errorf("env_vars[API_KEY] = %q, want secret123", createCmd.EnvVars["API_KEY"])
	}
	if createCmd.EnvVars["DEBUG"] != "true" {
		t.Errorf("env_vars[DEBUG] = %q, want true", createCmd.EnvVars["DEBUG"])
	}
}

func TestCreateSession_InvalidTemplateID(t *testing.T) {
	st, cm, _, reg, getClaims := setupTemplateTestEnv(t, "user-invalid")

	handler := session.NewSessionHandler(st, cm, reg, getClaims, slog.Default())
	router := chi.NewRouter()
	router.Post("/api/v1/sessions", handler.CreateSession)

	body := `{"machine_id":"machine-a","template_id":"nonexistent-id"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestCreateSession_WithoutTemplate_UnchangedBehavior(t *testing.T) {
	st, cm, recorder, reg, getClaims := setupTemplateTestEnv(t, "user-notemplate")

	handler := session.NewSessionHandler(st, cm, reg, getClaims, slog.Default())
	router := chi.NewRouter()
	router.Post("/api/v1/sessions", handler.CreateSession)

	body := `{"machine_id":"machine-a","command":"my-cli","working_dir":"/home"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["command"] != "my-cli" {
		t.Errorf("command = %v, want my-cli", resp["command"])
	}

	cmd := recorder.last()
	createCmd := cmd.GetCreateSession()
	if createCmd.Command != "my-cli" {
		t.Errorf("proto command = %q, want my-cli", createCmd.Command)
	}
	if createCmd.WorkingDir != "/home" {
		t.Errorf("proto working_dir = %q, want /home", createCmd.WorkingDir)
	}
	// No template → default terminal size
	if createCmd.TerminalSize.Rows != 24 || createCmd.TerminalSize.Cols != 80 {
		t.Errorf("terminal_size = %dx%d, want 24x80", createCmd.TerminalSize.Rows, createCmd.TerminalSize.Cols)
	}

	// Session should have empty template_id
	sessionID := resp["session_id"].(string)
	sess, err := st.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.TemplateID != "" {
		t.Errorf("session template_id = %q, want empty", sess.TemplateID)
	}
}

func TestAuthorizeSession_NilClaimsDenied(t *testing.T) {
	cm, recorder, st, reg := setupAuthTestEnv(t)

	createTestUser(t, st, "user-owner")

	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	if err := cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	}); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

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

// --- Inject + Injection History Tests ---

// setupInjectTestEnv creates a handler with inject routes, a running session, and a connected agent.
func setupInjectTestEnv(t *testing.T) (chi.Router, *store.Store, string) {
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

	createTestUser(t, st, "inject-user")

	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	if err := cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	}); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	getClaims := func(r *http.Request) *session.UserClaims {
		return &session.UserClaims{UserID: "inject-user", Role: "user"}
	}
	handler := session.NewSessionHandler(st, cm, reg, getClaims, slog.Default())

	// Wire up the injection queue so InjectSession doesn't return 503.
	injQueue := session.NewInjectionQueue(cm, st, st, &noopSubscriber{}, slog.Default())
	t.Cleanup(func() { injQueue.Close() })
	handler.SetInjectionQueue(injQueue)

	// Create a running session directly in the DB.
	sessionID := "sess-inject-test"
	if err := st.CreateSession(&store.Session{
		SessionID: sessionID,
		MachineID: "machine-a",
		UserID:    "inject-user",
		Command:   "claude",
		Status:    store.StatusRunning,
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	r := chi.NewRouter()
	r.Post("/api/v1/sessions/{sessionID}/inject", handler.InjectSession)
	r.Get("/api/v1/sessions/{sessionID}/injections", handler.ListInjections)

	return r, st, sessionID
}

func TestInjectSession_ValidReturns202(t *testing.T) {
	router, _, sessionID := setupInjectTestEnv(t)

	body := `{"text":"hello world"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/inject", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["injection_id"] == nil || resp["injection_id"] == "" {
		t.Error("expected non-empty injection_id in response")
	}
	if resp["queued_at"] == nil || resp["queued_at"] == "" {
		t.Error("expected non-empty queued_at in response")
	}
}

func TestInjectSession_NonExistentSession404(t *testing.T) {
	router, _, _ := setupInjectTestEnv(t)

	body := `{"text":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/nonexistent-id/inject", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestInjectSession_TerminatedSession409(t *testing.T) {
	router, st, sessionID := setupInjectTestEnv(t)

	// Terminate the session.
	if err := st.UpdateSessionStatus(sessionID, store.StatusTerminated); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}

	body := `{"text":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/inject", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusConflict, w.Body.String())
	}
}

func TestInjectSession_EmptyText400(t *testing.T) {
	router, _, sessionID := setupInjectTestEnv(t)

	body := `{"text":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/inject", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestListInjections_ReturnsInjectionList(t *testing.T) {
	router, st, sessionID := setupInjectTestEnv(t)

	// Create two injection records directly in the store.
	for i := range 2 {
		_, err := st.CreateInjection(context.Background(), &store.Injection{
			SessionID:  sessionID,
			UserID:     "inject-user",
			TextLength: 10 + i,
			Source:     "api",
		})
		if err != nil {
			t.Fatalf("CreateInjection %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/injections", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var injections []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&injections); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(injections) != 2 {
		t.Errorf("injections count = %d, want 2", len(injections))
	}
}

func TestListInjections_EmptyListForNoInjections(t *testing.T) {
	router, _, sessionID := setupInjectTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/injections", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var injections []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&injections); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(injections) != 0 {
		t.Errorf("injections count = %d, want 0", len(injections))
	}
}

func TestInjectSession_NonOwnerReturns404(t *testing.T) {
	cm, recorder, st, reg := setupAuthTestEnv(t)

	createTestUser(t, st, "owner-user")
	createTestUser(t, st, "other-user")

	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	if err := cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	}); err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	// Create a running session owned by owner-user.
	sessionID := "sess-inject-auth"
	if err := st.CreateSession(&store.Session{
		SessionID: sessionID,
		MachineID: "machine-a",
		UserID:    "owner-user",
		Command:   "claude",
		Status:    store.StatusRunning,
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Attempt inject as other-user (non-admin).
	otherClaims := func(r *http.Request) *session.UserClaims {
		return &session.UserClaims{UserID: "other-user", Role: "user"}
	}
	otherHandler := session.NewSessionHandler(st, cm, reg, otherClaims, slog.Default())
	otherRouter := chi.NewRouter()
	otherRouter.Post("/api/v1/sessions/{sessionID}/inject", otherHandler.InjectSession)

	body := `{"text":"should not work"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/inject", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	otherRouter.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("inject by non-owner: status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}
