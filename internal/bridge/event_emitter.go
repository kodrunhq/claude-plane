package bridge

import (
	"context"
	"log/slog"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
)

// EventEmitter sends bridge lifecycle events to the server.
type EventEmitter struct {
	client *client.Client
	logger *slog.Logger
}

// NewEventEmitter creates a new EventEmitter that sends events via the given client.
func NewEventEmitter(c *client.Client, logger *slog.Logger) *EventEmitter {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventEmitter{client: c, logger: logger}
}

// Emit sends a single lifecycle event to the server. Best-effort: errors are
// logged but never returned to the caller.
func (e *EventEmitter) Emit(eventType string, payload map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := client.EventIngestionRequest{
		Events: []client.EventEntry{{Type: eventType, Payload: payload}},
	}
	if err := e.client.PostEvents(ctx, req); err != nil {
		e.logger.Error("failed to emit event", "type", eventType, "error", err)
	}
}
