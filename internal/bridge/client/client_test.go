package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
)

const testAPIKey = "cpk_test_key"

// newTestServer creates an httptest server that validates the Bearer token
// and dispatches requests to the provided handler map (method+path -> handler).
func newTestServer(t *testing.T, routes map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, h := range routes {
		handler := h // capture
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+testAPIKey {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			handler(w, r)
		})
	}
	return httptest.NewServer(mux)
}

func writeJSON(t *testing.T, w http.ResponseWriter, v interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

// --- CreateSession ---

func TestCreateSession(t *testing.T) {
	want := client.Session{
		SessionID: "sess-1",
		MachineID: "machine-1",
		Status:    "running",
		Command:   "claude",
	}

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/v1/sessions": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, want)
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	got, err := c.CreateSession(context.Background(), client.CreateSessionRequest{
		MachineID: "machine-1",
		Command:   "claude",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != want.SessionID {
		t.Errorf("session_id = %q, want %q", got.SessionID, want.SessionID)
	}
}

// --- ListSessions ---

func TestListSessions(t *testing.T) {
	want := []client.Session{
		{SessionID: "s1", MachineID: "m1", Status: "running"},
		{SessionID: "s2", MachineID: "m2", Status: "stopped"},
	}

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/sessions": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, want)
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	got, err := c.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(sessions) = %d, want 2", len(got))
	}
}

// --- GetSession ---

func TestGetSession(t *testing.T) {
	want := client.Session{SessionID: "sess-abc", MachineID: "m1", Status: "running"}

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/sessions/sess-abc": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, want)
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	got, err := c.GetSession(context.Background(), "sess-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "sess-abc" {
		t.Errorf("session_id = %q, want %q", got.SessionID, "sess-abc")
	}
}

// --- KillSession ---

func TestKillSession(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"DELETE /api/v1/sessions/sess-xyz": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	if err := c.KillSession(context.Background(), "sess-xyz"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- InjectSession ---

func TestInjectSession(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/v1/sessions/sess-inj/inject": func(w http.ResponseWriter, r *http.Request) {
			var req client.InjectRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if req.Text != "hello" {
				http.Error(w, "unexpected text", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	err := c.InjectSession(context.Background(), "sess-inj", client.InjectRequest{
		Text:   "hello",
		Source: "bridge",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- ListTemplates ---

func TestListTemplates(t *testing.T) {
	want := []client.Template{
		{TemplateID: "tpl-1", Name: "Default"},
	}

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/templates": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, want)
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	got, err := c.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].TemplateID != "tpl-1" {
		t.Errorf("templates = %+v, unexpected", got)
	}
}

// --- ListMachines ---

func TestListMachines(t *testing.T) {
	want := []client.Machine{
		{MachineID: "m-1", DisplayName: "Worker 1", Status: "online"},
	}

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/machines": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, want)
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	got, err := c.ListMachines(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].MachineID != "m-1" {
		t.Errorf("machines = %+v, unexpected", got)
	}
}

// --- PollEvents ---

func TestPollEvents_NoCursor(t *testing.T) {
	feed := client.EventFeedResponse{
		Events:     []client.Event{{EventID: "evt-1", Type: "session.started"}},
		NextCursor: "cursor-abc",
	}

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/events/feed": func(w http.ResponseWriter, r *http.Request) {
			if q := r.URL.Query().Get("after"); q != "" {
				http.Error(w, "unexpected cursor", http.StatusBadRequest)
				return
			}
			writeJSON(t, w, feed)
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	events, cursor, err := c.PollEvents(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0].EventID != "evt-1" {
		t.Errorf("events = %+v, unexpected", events)
	}
	if cursor != "cursor-abc" {
		t.Errorf("cursor = %q, want %q", cursor, "cursor-abc")
	}
}

func TestPollEvents_WithCursor(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/events/feed": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("after") != "cursor-xyz" {
				http.Error(w, "missing cursor", http.StatusBadRequest)
				return
			}
			writeJSON(t, w, client.EventFeedResponse{NextCursor: "cursor-next"})
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	_, cursor, err := c.PollEvents(context.Background(), "cursor-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor != "cursor-next" {
		t.Errorf("cursor = %q, want %q", cursor, "cursor-next")
	}
}

// --- GetConnectorConfigs ---

func TestGetConnectorConfigs(t *testing.T) {
	want := []client.ConnectorConfig{
		{ConnectorID: "cc-1", ConnectorType: "telegram", Name: "My Bot", Enabled: true, ConfigSecret: "bot-token"},
	}

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/bridge/connectors": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, want)
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	got, err := c.GetConnectorConfigs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ConnectorID != "cc-1" {
		t.Errorf("configs = %+v, unexpected", got)
	}
}

// --- CheckRestartSignal ---

func TestCheckRestartSignal_NoRestart(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/bridge/status": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, client.BridgeStatusResponse{RestartRequestedAt: nil})
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	restart, err := c.CheckRestartSignal(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restart {
		t.Error("expected restart=false when RestartRequestedAt is nil")
	}
}

func TestCheckRestartSignal_RestartAfterBoot(t *testing.T) {
	bootTime := time.Now().Add(-5 * time.Minute)
	requestedAt := time.Now().Format(time.RFC3339)

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/bridge/status": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, client.BridgeStatusResponse{RestartRequestedAt: &requestedAt})
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	restart, err := c.CheckRestartSignal(context.Background(), bootTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !restart {
		t.Error("expected restart=true when restart was requested after boot")
	}
}

func TestCheckRestartSignal_RestartBeforeBoot(t *testing.T) {
	bootTime := time.Now()
	requestedAt := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/bridge/status": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, client.BridgeStatusResponse{RestartRequestedAt: &requestedAt})
		},
	})
	defer srv.Close()

	c := client.New(srv.URL, testAPIKey)
	restart, err := c.CheckRestartSignal(context.Background(), bootTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restart {
		t.Error("expected restart=false when restart was requested before boot")
	}
}

// --- Auth failure ---

func TestClientReturnsErrorOnUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := client.New(srv.URL, "wrong-key")
	_, err := c.ListSessions(context.Background())
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
	var apiErr *client.APIError
	if ok := isAPIError(err, &apiErr); !ok || apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected APIError with 401, got %v", err)
	}
}

// isAPIError checks if err wraps an *client.APIError and populates target.
func isAPIError(err error, target **client.APIError) bool {
	for err != nil {
		if ae, ok := err.(*client.APIError); ok {
			*target = ae
			return true
		}
		// unwrap one level
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}
