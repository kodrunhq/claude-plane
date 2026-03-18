package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/logging"
)

// logsHeartbeatInterval is the interval between WebSocket ping frames on the
// logs endpoint.
const logsHeartbeatInterval = 30 * time.Second

// logsFilterTimeout is the maximum time to wait for an optional filter
// message from the client before defaulting to all logs.
const logsFilterTimeout = 5 * time.Second

// logsSubscribeMsg is the message a client sends to set or update the log
// filter. Accepted as the first post-auth message or at any time during the
// connection.
type logsSubscribeMsg struct {
	Type      string `json:"type"`                 // "subscribe" or "update_filter"
	Level     string `json:"level,omitempty"`      // e.g. "WARN", "ERROR"
	Component string `json:"component,omitempty"`  // e.g. "grpc"
	Source    string `json:"source,omitempty"`     // e.g. "server", "agent"
	MachineID string `json:"machine_id,omitempty"` // filter by machine
}

// logsOutMsg is the envelope sent to the client for each log record.
type logsOutMsg struct {
	Type  string           `json:"type"`
	Entry logging.LogRecord `json:"entry,omitempty"`
}

// HandleLogsWS returns an http.HandlerFunc for the /ws/logs WebSocket endpoint.
// Clients connect, optionally send a filter message, and receive matching
// log records in real-time from the LogBroadcaster.
//
// Authentication order:
//  1. httpOnly cookie (pre-upgrade, preferred)
//  2. First WebSocket message: {"type":"auth","token":"<jwt>"}
func HandleLogsWS(authSvc *auth.Service, broadcaster *logging.LogBroadcaster, logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// --- Cookie auth (preferred, pre-upgrade) ---
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			if _, err := authSvc.ValidateToken(cookie.Value); err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			conn, err := websocket.Accept(w, r, wsAcceptOptions)
			if err != nil {
				logger.Error("logs websocket upgrade failed", "error", err)
				return
			}

			runLogsLoop(conn, r.Context(), broadcaster, logger)
			return
		}

		// --- First-message auth ---
		conn, err := websocket.Accept(w, r, wsAcceptOptions)
		if err != nil {
			logger.Error("logs websocket upgrade failed", "error", err)
			return
		}

		authCtx, authCancel := context.WithTimeout(r.Context(), authTimeout)
		defer authCancel()

		_, msg, err := conn.Read(authCtx)
		if err != nil {
			conn.Close(websocket.StatusPolicyViolation, "auth timeout or read error")
			return
		}

		var authMsg struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}
		if err := json.Unmarshal(msg, &authMsg); err != nil || authMsg.Type != "auth" || authMsg.Token == "" {
			conn.Close(websocket.StatusPolicyViolation, "first message must be auth")
			return
		}
		if _, err := authSvc.ValidateToken(authMsg.Token); err != nil {
			conn.Close(websocket.StatusPolicyViolation, "invalid token")
			return
		}

		runLogsLoop(conn, r.Context(), broadcaster, logger)
	}
}

// runLogsLoop manages the lifecycle of an authenticated logs WebSocket:
//   - subscribes to the broadcaster with a default (all-pass) filter
//   - starts a reader goroutine for filter updates
//   - writes matching log records to the client
//   - sends periodic heartbeat pings
//   - cleans up on disconnect
func runLogsLoop(conn *websocket.Conn, reqCtx context.Context, broadcaster *logging.LogBroadcaster, logger *slog.Logger) {
	ctx, cancel := context.WithCancel(reqCtx)
	defer cancel()
	defer conn.CloseNow()

	logger.Debug("logs websocket connected")

	// Subscribe with default filter (all levels, all sources).
	sub := broadcaster.Subscribe(logging.LogFilter{})
	defer broadcaster.Unsubscribe(sub)

	// Reader goroutine: handles initial filter negotiation and subsequent
	// filter updates. Only one goroutine ever calls conn.Read.
	filterReady := make(chan struct{}, 1)
	go func() {
		defer cancel()
		first := true
		for {
			_, msg, err := conn.Read(ctx)
			if err != nil {
				if first {
					// Signal that no initial filter arrived.
					close(filterReady)
				}
				return
			}

			var filterMsg logsSubscribeMsg
			if err := json.Unmarshal(msg, &filterMsg); err != nil {
				logger.Debug("logs websocket: invalid message", "error", err)
				if first {
					first = false
					close(filterReady)
				}
				continue
			}

			switch filterMsg.Type {
			case "subscribe", "update_filter":
				newFilter := logging.LogFilter{
					Level:     filterMsg.Level,
					Component: filterMsg.Component,
					Source:    filterMsg.Source,
					MachineID: filterMsg.MachineID,
				}
				sub.UpdateFilter(newFilter)
				logger.Debug("logs websocket: filter updated",
					"level", filterMsg.Level,
					"component", filterMsg.Component,
					"source", filterMsg.Source,
				)

				// Send filter_ack.
				ack, _ := json.Marshal(logsOutMsg{Type: "filter_ack"})
				if err := conn.Write(ctx, websocket.MessageText, ack); err != nil {
					logger.Debug("logs websocket: failed to write filter_ack", "error", err)
					return
				}
			default:
				logger.Debug("logs websocket: unknown message type", "type", filterMsg.Type)
			}

			if first {
				first = false
				close(filterReady)
			}
		}
	}()

	// Wait briefly for an optional initial filter message.
	select {
	case <-filterReady:
	case <-time.After(logsFilterTimeout):
	case <-ctx.Done():
		return
	}

	// Heartbeat ticker.
	ticker := time.NewTicker(logsHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("logs websocket context done")
			return

		case rec, ok := <-sub.Ch:
			if !ok {
				return
			}
			out, err := json.Marshal(logsOutMsg{Type: "log", Entry: rec})
			if err != nil {
				logger.Debug("logs websocket: marshal error", "error", err)
				continue
			}
			if err := conn.Write(ctx, websocket.MessageText, out); err != nil {
				logger.Debug("logs websocket: write error", "error", err)
				return
			}

		case <-ticker.C:
			if err := conn.Ping(ctx); err != nil {
				logger.Debug("logs websocket ping failed", "error", err)
				return
			}
		}
	}
}
