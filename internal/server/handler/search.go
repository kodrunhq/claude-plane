package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// SearchHandler handles REST endpoints for searching session content.
type SearchHandler struct {
	store     *store.Store
	getClaims ClaimsGetter
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(s *store.Store, getClaims ClaimsGetter) *SearchHandler {
	return &SearchHandler{store: s, getClaims: getClaims}
}

// RegisterSearchRoutes mounts all search routes on the given router.
func RegisterSearchRoutes(r chi.Router, h *SearchHandler) {
	r.Get("/api/v1/search/sessions", h.SearchSessions)
}

// SearchSessions handles GET /api/v1/search/sessions?q=<query>&limit=50&offset=0.
func (h *SearchHandler) SearchSessions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	if len(query) > 500 {
		writeError(w, http.StatusBadRequest, "search query too long (max 500 characters)")
		return
	}

	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}

	offset := 0
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	// Scope search results to the current user unless they are an admin.
	var userID string
	claims := h.getClaims(r)
	if claims != nil && claims.Role != "admin" {
		userID = claims.UserID
	}

	results, err := h.store.SearchContent(r.Context(), query, limit, offset, userID)
	if err != nil {
		if strings.Contains(err.Error(), "fts5: syntax error") ||
			strings.Contains(err.Error(), "no such column") {
			writeError(w, http.StatusBadRequest, "invalid search query syntax")
			return
		}
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	// Fetch context lines for each result
	for i := range results {
		before, after, err := h.store.FetchContextLines(r.Context(),
			results[i].SessionID, results[i].LineNumber, 2, 2)
		if err != nil {
			continue // skip context on error, still return the match
		}
		results[i].ContextBefore = before
		results[i].ContextAfter = after
	}

	writeJSON(w, http.StatusOK, results)
}
