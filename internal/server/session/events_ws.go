package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/kodrunhq/claude-plane/internal/server/auth"
)

// HandleEventsWS returns an http.HandlerFunc that accepts a WebSocket
// connection for the /ws/events endpoint. It authenticates via cookie
// (preferred, pre-upgrade) or first-message auth, and sends periodic
// heartbeat pings to keep the connection alive.
//
// This is a minimal stub: a full implementation would fan-out server-side
// events (session status changes, job progress, etc.) to connected clients.
func HandleEventsWS(authSvc *auth.Service, logger *slog.Logger) http.HandlerFunc {
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

			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				logger.Error("events websocket upgrade failed", "error", err)
				return
			}

			runEventsLoop(conn, r.Context(), logger)
			return
		}

		// --- First-message auth ---
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			logger.Error("events websocket upgrade failed", "error", err)
			return
		}

		authCtx, authCancel := context.WithTimeout(r.Context(), 5*time.Second)
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

		runEventsLoop(conn, r.Context(), logger)
	}
}

// runEventsLoop runs the heartbeat ping loop for an authenticated events WebSocket.
func runEventsLoop(conn *websocket.Conn, reqCtx context.Context, logger *slog.Logger) {
	ctx, cancel := context.WithCancel(reqCtx)
	defer cancel()
	defer conn.CloseNow()

	logger.Debug("events websocket connected")

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
