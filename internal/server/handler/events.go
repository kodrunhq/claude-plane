package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

const (
	defaultEventsLimit = 50
	maxEventsLimit     = 500
)

// EventQueryStore is the read interface required by EventHandler.
type EventQueryStore interface {
	ListEvents(ctx context.Context, filter store.EventFilter) ([]event.Event, error)
}

// EventHandler handles REST endpoints for querying the event audit trail.
type EventHandler struct {
	store EventQueryStore
}

// NewEventHandler creates a new EventHandler backed by store.
func NewEventHandler(store EventQueryStore) *EventHandler {
	return &EventHandler{store: store}
}

// ListEvents handles GET /api/v1/events.
//
// Query parameters:
//   - type   — event type pattern, e.g. "run.*" or exact "run.created" (optional)
//   - since  — ISO 8601 timestamp lower bound, inclusive (optional)
//   - limit  — max results to return (default 50, max 500)
//   - offset — pagination offset (default 0)
func (h *EventHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	typePattern := q.Get("type")

	var since time.Time
	if sinceStr := q.Get("since"); sinceStr != "" {
		var err error
		since, err = time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid 'since' parameter: must be RFC3339 format")
			return
		}
	}

	limit := defaultEventsLimit
	if limitStr := q.Get("limit"); limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil || v < 1 {
			writeError(w, http.StatusBadRequest, "invalid 'limit' parameter: must be a positive integer")
			return
		}
		if v > maxEventsLimit {
			v = maxEventsLimit
		}
		limit = v
	}

	offset := 0
	if offsetStr := q.Get("offset"); offsetStr != "" {
		v, err := strconv.Atoi(offsetStr)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "invalid 'offset' parameter: must be a non-negative integer")
			return
		}
		offset = v
	}

	filter := store.EventFilter{
		TypePattern: typePattern,
		Since:       since,
		Limit:       limit,
		Offset:      offset,
	}

	events, err := h.store.ListEvents(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Return empty slice instead of null for JSON consumers.
	if events == nil {
		events = []event.Event{}
	}

	writeJSON(w, http.StatusOK, events)
}
