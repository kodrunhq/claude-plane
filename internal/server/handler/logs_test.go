package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/logging"
)

func adminClaimsGetter() ClaimsGetter {
	return func(r *http.Request) *UserClaims {
		return &UserClaims{UserID: "admin-1", Role: "admin"}
	}
}

func newTestLogStore(t *testing.T) *logging.LogStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test-logs.db")
	ls, err := logging.NewLogStore(dbPath)
	if err != nil {
		t.Fatalf("NewLogStore: %v", err)
	}
	t.Cleanup(func() { ls.Close() })
	return ls
}

func seedTestLogs(t *testing.T, ls *logging.LogStore) {
	t.Helper()
	now := time.Now().UTC()
	records := []logging.LogRecord{
		{Timestamp: now.Add(-time.Minute), Level: "INFO", Component: "server", Message: "server started", Source: "server"},
		{Timestamp: now.Add(-30 * time.Second), Level: "ERROR", Component: "grpc", Message: "connection failed", Error: "timeout", Source: "server"},
		{Timestamp: now, Level: "WARN", Component: "auth", Message: "invalid token", MachineID: "m1", SessionID: "s1", Source: "agent"},
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
}

func TestListLogs_Empty(t *testing.T) {
	ls := newTestLogStore(t)
	h := NewLogsHandler(ls, adminClaimsGetter())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	w := httptest.NewRecorder()

	h.ListLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	logs, ok := resp["logs"].([]any)
	if !ok {
		t.Fatalf("logs field is not an array")
	}
	if len(logs) != 0 {
		t.Errorf("expected empty logs, got %d", len(logs))
	}
	if total, _ := resp["total"].(float64); total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
}

func TestListLogs_WithData(t *testing.T) {
	ls := newTestLogStore(t)
	seedTestLogs(t, ls)
	h := NewLogsHandler(ls, adminClaimsGetter())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	w := httptest.NewRecorder()

	h.ListLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	logs := resp["logs"].([]any)
	if len(logs) != 3 {
		t.Errorf("len(logs) = %d, want 3", len(logs))
	}
	if total, _ := resp["total"].(float64); total != 3 {
		t.Errorf("total = %v, want 3", total)
	}
}

func TestListLogs_FilterByLevel(t *testing.T) {
	ls := newTestLogStore(t)
	seedTestLogs(t, ls)
	h := NewLogsHandler(ls, adminClaimsGetter())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?level=error", nil)
	w := httptest.NewRecorder()

	h.ListLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	logs := resp["logs"].([]any)
	if len(logs) != 1 {
		t.Errorf("len(logs) = %d, want 1", len(logs))
	}
}

func TestListLogs_FilterByMachineID(t *testing.T) {
	ls := newTestLogStore(t)
	seedTestLogs(t, ls)
	h := NewLogsHandler(ls, adminClaimsGetter())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?machine_id=m1", nil)
	w := httptest.NewRecorder()

	h.ListLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	logs := resp["logs"].([]any)
	if len(logs) != 1 {
		t.Errorf("len(logs) = %d, want 1", len(logs))
	}
}

func TestListLogs_InvalidSince(t *testing.T) {
	ls := newTestLogStore(t)
	h := NewLogsHandler(ls, adminClaimsGetter())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?since=not-a-date", nil)
	w := httptest.NewRecorder()

	h.ListLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetLogStats_Empty(t *testing.T) {
	ls := newTestLogStore(t)
	h := NewLogsHandler(ls, adminClaimsGetter())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/stats", nil)
	w := httptest.NewRecorder()

	h.GetLogStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var stats logging.LogStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.Total != 0 {
		t.Errorf("total = %d, want 0", stats.Total)
	}
}

func TestGetLogStats_WithData(t *testing.T) {
	ls := newTestLogStore(t)
	seedTestLogs(t, ls)
	h := NewLogsHandler(ls, adminClaimsGetter())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/stats", nil)
	w := httptest.NewRecorder()

	h.GetLogStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var stats logging.LogStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.Total != 3 {
		t.Errorf("total = %d, want 3", stats.Total)
	}
	if stats.ByLevel["ERROR"] != 1 {
		t.Errorf("by_level[ERROR] = %d, want 1", stats.ByLevel["ERROR"])
	}
	if stats.ByLevel["INFO"] != 1 {
		t.Errorf("by_level[INFO] = %d, want 1", stats.ByLevel["INFO"])
	}
}

func TestGetLogStats_InvalidSince(t *testing.T) {
	ls := newTestLogStore(t)
	h := NewLogsHandler(ls, adminClaimsGetter())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/stats?since=bad", nil)
	w := httptest.NewRecorder()

	h.GetLogStats(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
