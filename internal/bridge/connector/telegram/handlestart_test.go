package telegram

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"path/filepath"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
	"github.com/kodrunhq/claude-plane/internal/bridge/state"
)

// newTestTelegram creates a Telegram connector backed by a test HTTP server
// that returns the given templates from ListTemplates and the given session
// from CreateSession.
func newTestTelegram(t *testing.T, templates []client.Template, session *client.Session) *Telegram {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/templates"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(templates)

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/sessions"):
			if session == nil {
				http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(session)

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	apiClient := client.New(srv.URL, "test-key")
	stateStore := state.New(filepath.Join(t.TempDir(), "state.json"))
	logger := slog.Default()

	return New("test-telegram", Config{}, apiClient, stateStore, logger)
}

func TestHandleStart_ExplicitMachine(t *testing.T) {
	templates := []client.Template{
		{TemplateID: "tmpl-1", Name: "template1", MachineID: "default-machine"},
	}
	session := &client.Session{
		SessionID: "sess-123",
		MachineID: "machine1",
		Status:    "starting",
	}
	tg := newTestTelegram(t, templates, session)

	cmd := &Command{
		Name: "start",
		Args: []string{"template1", "machine1"},
		Vars: map[string]string{},
	}

	result := tg.handleStart(context.Background(), cmd)

	if !strings.Contains(result, "sess-123") {
		t.Errorf("expected session ID in result, got: %s", result)
	}
	if !strings.Contains(result, "machine1") {
		t.Errorf("expected machine ID in result, got: %s", result)
	}
	if !strings.HasPrefix(result, "\u2705") {
		t.Errorf("expected success prefix, got: %s", result)
	}
}

func TestHandleStart_FallbackToTemplateDefaultMachine(t *testing.T) {
	templates := []client.Template{
		{TemplateID: "tmpl-1", Name: "template1", MachineID: "default-machine"},
	}
	session := &client.Session{
		SessionID: "sess-456",
		MachineID: "default-machine",
		Status:    "starting",
	}
	tg := newTestTelegram(t, templates, session)

	cmd := &Command{
		Name: "start",
		Args: []string{"template1"},
		Vars: map[string]string{},
	}

	result := tg.handleStart(context.Background(), cmd)

	if !strings.Contains(result, "sess-456") {
		t.Errorf("expected session ID in result, got: %s", result)
	}
	if !strings.Contains(result, "default-machine") {
		t.Errorf("expected default machine ID in result, got: %s", result)
	}
	if !strings.HasPrefix(result, "\u2705") {
		t.Errorf("expected success prefix, got: %s", result)
	}
}

func TestHandleStart_NoDefaultMachine_Error(t *testing.T) {
	templates := []client.Template{
		{TemplateID: "tmpl-1", Name: "template1", MachineID: ""},
	}
	tg := newTestTelegram(t, templates, nil)

	cmd := &Command{
		Name: "start",
		Args: []string{"template1"},
		Vars: map[string]string{},
	}

	result := tg.handleStart(context.Background(), cmd)

	if !strings.Contains(result, "no default machine") {
		t.Errorf("expected 'no default machine' error, got: %s", result)
	}
	if !strings.HasPrefix(result, "\u274c") {
		t.Errorf("expected error prefix, got: %s", result)
	}
}

func TestHandleStart_TemplateNotFound(t *testing.T) {
	templates := []client.Template{
		{TemplateID: "tmpl-1", Name: "other-template"},
	}
	tg := newTestTelegram(t, templates, nil)

	cmd := &Command{
		Name: "start",
		Args: []string{"nonexistent"},
		Vars: map[string]string{},
	}

	result := tg.handleStart(context.Background(), cmd)

	if !strings.Contains(result, "not found") {
		t.Errorf("expected 'not found' error, got: %s", result)
	}
}

func TestHandleStart_NoArgs_Error(t *testing.T) {
	tg := newTestTelegram(t, nil, nil)

	cmd := &Command{
		Name: "start",
		Args: []string{},
		Vars: map[string]string{},
	}

	result := tg.handleStart(context.Background(), cmd)

	if !strings.Contains(result, "Usage") {
		t.Errorf("expected usage message, got: %s", result)
	}
}
