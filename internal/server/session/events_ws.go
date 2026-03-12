package session

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/claudeplane/claude-plane/internal/server/auth"
)

// HandleEventsWS returns an http.HandlerFunc that accepts a WebSocket
// connection for the /ws/events endpoint. It authenticates via query param
// token and sends periodic heartbeat pings to keep the connection alive.
//
// This is a minimal stub: a full implementation would fan-out server-side
// events (session status changes, job progress, etc.) to connected clients.
func HandleEventsWS(authSvc *auth.Service, logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Authenticate via query param token
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		if _, err := authSvc.ValidateToken(token); err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			logger.Error("events websocket upgrade failed", "error", err)
			return
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		defer conn.CloseNow()

		logger.Debug("events websocket connected")

		// Send periodic heartbeat pings to keep the connection alive
		// and prevent the frontend from silently reconnecting.
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := conn.Ping(ctx); err != nil {
					logger.Debug("events websocket ping failed", "error", err)
					return
				}
			}
		}
	}
}
