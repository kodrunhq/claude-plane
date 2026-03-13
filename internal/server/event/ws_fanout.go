package event

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/coder/websocket"
)

// wsClient holds a connected WebSocket and its subscribed event patterns.
type wsClient struct {
	conn     *websocket.Conn
	patterns []string
}

// wsEventMsg is the wire format for events sent to WebSocket clients.
type wsEventMsg struct {
	Type      string         `json:"type"`
	EventID   string         `json:"event_id"`
	EventType string         `json:"event_type"`
	Timestamp string         `json:"timestamp"`
	Source    string         `json:"source"`
	Payload   map[string]any `json:"payload"`
}

// WSFanout subscribes to the event bus and fans out matching events to all
// registered WebSocket clients. Each client declares its own set of patterns.
type WSFanout struct {
	bus         *Bus
	logger      *slog.Logger
	mu          sync.RWMutex
	clients     map[*wsClient]struct{}
	unsubscribe func()
	startOnce   sync.Once
}

// NewWSFanout constructs a WSFanout. Call Start() to begin receiving events.
// If logger is nil, slog.Default() is used.
func NewWSFanout(bus *Bus, logger *slog.Logger) *WSFanout {
	if logger == nil {
		logger = slog.Default()
	}
	return &WSFanout{
		bus:     bus,
		logger:  logger,
		clients: make(map[*wsClient]struct{}),
	}
}

// Start subscribes the fan-out to all events on the bus ("*" pattern).
// It is idempotent: only the first call has any effect; subsequent calls are
// silently ignored, preventing a double-subscription from leaking a subscriber.
func (f *WSFanout) Start() {
	f.startOnce.Do(func() {
		f.unsubscribe = f.bus.Subscribe("*", f.handle, SubscriberOptions{
			BufferSize:  512,
			Concurrency: 1,
		})
	})
}

// AddClient registers a WebSocket connection with the given event patterns.
// If patterns is empty, the client receives all events ("*").
func (f *WSFanout) AddClient(conn *websocket.Conn, patterns []string) {
	if len(patterns) == 0 {
		patterns = []string{"*"}
	}
	c := &wsClient{conn: conn, patterns: patterns}
	f.mu.Lock()
	f.clients[c] = struct{}{}
	f.mu.Unlock()
}

// RemoveClient unregisters the WebSocket connection from the fan-out.
func (f *WSFanout) RemoveClient(conn *websocket.Conn) {
	f.mu.Lock()
	for c := range f.clients {
		if c.conn == conn {
			delete(f.clients, c)
			break
		}
	}
	f.mu.Unlock()
}

// Close unsubscribes from the bus and closes all registered client connections.
func (f *WSFanout) Close() {
	if f.unsubscribe != nil {
		f.unsubscribe()
	}

	f.mu.Lock()
	clients := make([]*wsClient, 0, len(f.clients))
	for c := range f.clients {
		clients = append(clients, c)
	}
	f.clients = make(map[*wsClient]struct{})
	f.mu.Unlock()

	for _, c := range clients {
		c.conn.Close(websocket.StatusGoingAway, "server shutting down")
	}
}

// handle is the bus handler that delivers the event to matching clients.
func (f *WSFanout) handle(_ context.Context, ev Event) error {
	data, err := json.Marshal(wsEventMsg{
		Type:      "event",
		EventID:   ev.EventID,
		EventType: ev.Type,
		Timestamp: ev.Timestamp.UTC().Format("2006-01-02T15:04:05.999999999Z"),
		Source:    ev.Source,
		Payload:   ev.Payload,
	})
	if err != nil {
		f.logger.Warn("ws_fanout: failed to marshal event", "event_id", ev.EventID, "error", err)
		return nil
	}

	f.mu.RLock()
	clients := make([]*wsClient, 0, len(f.clients))
	for c := range f.clients {
		clients = append(clients, c)
	}
	f.mu.RUnlock()

	for _, c := range clients {
		if !clientMatchesEvent(c.patterns, ev.Type) {
			continue
		}
		if err := c.conn.Write(context.Background(), websocket.MessageText, data); err != nil {
			f.logger.Debug("ws_fanout: write to client failed, removing",
				"event_id", ev.EventID,
				"error", err,
			)
			f.RemoveClient(c.conn)
		}
	}
	return nil
}

// clientMatchesEvent returns true if any of the client's patterns matches the event type.
func clientMatchesEvent(patterns []string, eventType string) bool {
	for _, p := range patterns {
		if MatchPattern(p, eventType) {
			return true
		}
	}
	return false
}
