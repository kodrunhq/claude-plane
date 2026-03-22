package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/logging"
)

const maxIngestEntries = 200

// BridgeIngestHandler handles REST endpoints for ingesting logs and events
// from the bridge binary into the server's logging and event systems.
type BridgeIngestHandler struct {
	logStore *logging.LogStore
	bcast    *logging.LogBroadcaster
	eventBus *event.Bus
	logger   *slog.Logger
}

// NewBridgeIngestHandler creates a new BridgeIngestHandler.
func NewBridgeIngestHandler(logStore *logging.LogStore, bcast *logging.LogBroadcaster, eventBus *event.Bus, logger *slog.Logger) *BridgeIngestHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &BridgeIngestHandler{
		logStore: logStore,
		bcast:    bcast,
		eventBus: eventBus,
		logger:   logger,
	}
}

// RegisterBridgeIngestRoutes mounts bridge ingest routes on the given router.
func RegisterBridgeIngestRoutes(r chi.Router, h *BridgeIngestHandler) {
	r.Post("/api/v1/ingest/logs", h.HandleLogs)
	r.Post("/api/v1/ingest/events", h.HandleEvents)
}

// logsRequest is the JSON body for the log ingestion endpoint.
type logsRequest struct {
	Source  string     `json:"source"`
	Entries []logEntry `json:"entries"`
}

// logEntry is a single log entry in the ingestion request.
type logEntry struct {
	Timestamp  time.Time      `json:"timestamp"`
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	Attributes map[string]any `json:"attributes"`
}

// eventsRequest is the JSON body for the event ingestion endpoint.
type eventsRequest struct {
	Events []eventEntry `json:"events"`
}

// eventEntry is a single event in the ingestion request.
type eventEntry struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// HandleLogs ingests log entries from the bridge, persists them to the log
// store, and broadcasts them to WebSocket subscribers.
func (h *BridgeIngestHandler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	var req logsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Entries) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if len(req.Entries) > maxIngestEntries {
		writeError(w, http.StatusBadRequest, "too many entries (max 200)")
		return
	}

	// Issue 5: Always force source to "bridge" — do not trust caller-supplied value.
	source := "bridge"

	records := make([]logging.LogRecord, 0, len(req.Entries))
	for _, entry := range req.Entries {
		rec := logging.LogRecord{
			Timestamp: entry.Timestamp.UTC(),
			Level:     entry.Level,
			Message:   entry.Message,
			Source:    source,
		}

		// Extract well-known attributes into dedicated fields.
		extra := make(map[string]any, len(entry.Attributes))
		for k, v := range entry.Attributes {
			switch k {
			case "component":
				if s, ok := v.(string); ok {
					rec.Component = s
				}
			case "error", "err":
				if s, ok := v.(string); ok {
					rec.Error = s
				}
			case "machine_id":
				if s, ok := v.(string); ok {
					rec.MachineID = s
				}
			case "session_id":
				if s, ok := v.(string); ok {
					rec.SessionID = s
				}
			default:
				extra[k] = v
			}
		}

		if len(extra) > 0 {
			if data, err := json.Marshal(extra); err == nil {
				rec.Metadata = string(data)
			}
		}

		records = append(records, rec)
	}

	if err := h.logStore.InsertBatch(records); err != nil {
		h.logger.Error("bridge log ingestion failed", "error", err, "count", len(records))
		writeError(w, http.StatusInternalServerError, "failed to store logs")
		return
	}

	// Broadcast to WebSocket subscribers.
	if h.bcast != nil {
		for _, rec := range records {
			h.bcast.Broadcast(rec)
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

// allowedIngestEventTypes is the set of event types permitted through the
// bridge ingest endpoint. Any other type is rejected with 400.
var allowedIngestEventTypes = map[string]bool{
	event.TypeBridgeStarted:          true,
	event.TypeBridgeStopped:          true,
	event.TypeBridgeConnectorStarted: true,
	event.TypeBridgeConnectorError:   true,
	event.TypeBridgeConnectorCommand: true,
}

// HandleEvents ingests events from the bridge and publishes them on the event bus.
func (h *BridgeIngestHandler) HandleEvents(w http.ResponseWriter, r *http.Request) {
	var req eventsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Events) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if len(req.Events) > maxIngestEntries {
		writeError(w, http.StatusBadRequest, "too many events (max 200)")
		return
	}

	// Validate all event types before publishing any.
	for _, entry := range req.Events {
		if !allowedIngestEventTypes[entry.Type] {
			writeError(w, http.StatusBadRequest, "disallowed event type: "+entry.Type)
			return
		}
	}

	if h.eventBus != nil {
		for _, entry := range req.Events {
			evt := event.NewBridgeEvent(entry.Type, entry.Payload)
			if err := h.eventBus.Publish(r.Context(), evt); err != nil {
				h.logger.Error("bridge event publish failed",
					"event_type", entry.Type,
					"error", err,
				)
			}
		}
	}

	w.WriteHeader(http.StatusAccepted)
}
