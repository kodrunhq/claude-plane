package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/api"
	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

func TestRequestBodySizeLimit(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.NewStore(tmpDir + "/test.db")
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
	h := api.NewHandlers(s, authSvc, cm, "open", "")
	router := api.NewRouter(h, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	// 2MB body — should exceed 1MB limit
	largeBody := strings.NewReader(strings.Repeat("x", 2*1024*1024))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", largeBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge && w.Code != http.StatusBadRequest {
		t.Errorf("expected 413 or 400 for oversized body, got %d", w.Code)
	}
}
