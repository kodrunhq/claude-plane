package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/httputil"
	"github.com/kodrunhq/claude-plane/internal/server/logging"
)

// LogsHandler handles log-related REST endpoints.
type LogsHandler struct {
	store *logging.LogStore
}

// NewLogsHandler creates a new LogsHandler.
func NewLogsHandler(store *logging.LogStore) *LogsHandler {
	return &LogsHandler{store: store}
}

// ListLogs handles GET /api/v1/logs
func (h *LogsHandler) ListLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var since, until time.Time
	if s := q.Get("since"); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid since parameter: expected RFC3339 format")
			return
		}
		since = parsed
	}
	if u := q.Get("until"); u != "" {
		parsed, err := time.Parse(time.RFC3339, u)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid until parameter: expected RFC3339 format")
			return
		}
		until = parsed
	}

	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}

	filter := logging.LogFilter{
		Level:     q.Get("level"),
		Component: q.Get("component"),
		Source:    q.Get("source"),
		MachineID: q.Get("machine_id"),
		SessionID: q.Get("session_id"),
		Search:    q.Get("search"),
		Since:     since,
		Until:     until,
		Limit:     limit,
		Offset:    offset,
	}

	logs, total, err := h.store.Query(filter)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query logs")
		return
	}

	if logs == nil {
		logs = []logging.LogRecord{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"logs":  logs,
		"total": total,
	})
}

// GetLogStats handles GET /api/v1/logs/stats
func (h *LogsHandler) GetLogStats(w http.ResponseWriter, r *http.Request) {
	since := time.Now().UTC().Add(-24 * time.Hour)
	if s := r.URL.Query().Get("since"); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid since parameter: expected RFC3339 format")
			return
		}
		since = parsed
	}

	stats, err := h.store.Stats(since)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get log stats")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, stats)
}
