package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
)

// SearchHandler handles REST endpoints for searching session logs.
type SearchHandler struct {
	connMgr *connmgr.ConnectionManager
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(connMgr *connmgr.ConnectionManager) *SearchHandler {
	return &SearchHandler{connMgr: connMgr}
}

// RegisterSearchRoutes mounts all search routes on the given router.
func RegisterSearchRoutes(r chi.Router, h *SearchHandler) {
	r.Get("/api/v1/search/sessions", h.SearchSessions)
}

// SearchResult represents a single search match from an agent's scrollback.
type SearchResult struct {
	SessionID     string `json:"session_id"`
	MachineID     string `json:"machine_id"`
	Line          string `json:"line"`
	ContextBefore string `json:"context_before"`
	ContextAfter  string `json:"context_after"`
	TimestampMs   int64  `json:"timestamp_ms"`
	SessionStatus string `json:"session_status,omitempty"`
}

// SearchSessions handles GET /api/v1/search/sessions?q=<query>&limit=50.
// Fans out search to connected agents via the CommandStream and aggregates results.
func (h *SearchHandler) SearchSessions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	// TODO: Fan out SearchScrollbackCmd to connected agents via CommandStream.
	// For now, return an empty result set. The full implementation requires:
	// 1. Adding SearchScrollbackCmd/SearchScrollbackResultEvent to the proto
	// 2. Implementing request/response correlation on AgentConnection
	// 3. Agent-side .cast file parsing and search
	_ = limit
	_ = query

	results := make([]SearchResult, 0)
	writeJSON(w, http.StatusOK, results)
}
