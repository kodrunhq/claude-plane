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

	"github.com/claudeplane/claude-plane/internal/server/connmgr"
	"github.com/claudeplane/claude-plane/internal/server/session"
	"github.com/claudeplane/claude-plane/internal/server/store"
)

// mockMachineStore implements connmgr.MachineStore for tests.
type mockMachineStore struct{}

func (m *mockMachineStore) UpsertMachine(string, int32) error                         { return nil }
func (m *mockMachineStore) UpdateMachineStatus(string, string, time.Time) error { return nil }

// commandRecorder records commands sent to a mock agent.
type commandRecorder struct {
	mu       sync.Mutex
	commands []interface{}
}

func (cr *commandRecorder) send(cmd interface{}) error {
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

	getClaims := func(r *http.Request) string { return "" }
	handler := session.NewSessionHandler(st, cm, reg, getClaims, slog.Default())

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
