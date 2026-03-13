package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/event"
)

// subscriptionTimeout is the maximum time to wait for an optional subscribe
// message from the client before defaulting to "*".
const subscriptionTimeout = 5 * time.Second

// maxSubscriptionPatterns is the maximum number of event patterns a client may
// subscribe to. Requests exceeding this cap fall back to ["*"].
const maxSubscriptionPatterns = 20

// heartbeatInterval is the interval between WebSocket ping frames.
const heartbeatInterval = 30 * time.Second

// subscribeMsg is the optional message a client sends right after auth to
// declare which event patterns it is interested in.
type subscribeMsg struct {
	Type   string   `json:"type"`
	Events []string `json:"events"`
}

// HandleEventsWS returns an http.HandlerFunc that accepts a WebSocket
// connection for the /ws/events endpoint.
//
// Authentication order:
//  1. httpOnly cookie (pre-upgrade, preferred)
//  2. First WebSocket message: {"type":"auth","token":"<jwt>"}
//
// After authentication the client may optionally send:
//
//	{"type":"subscribe","events":["run.*","machine.*"]}
//
// within 5 seconds. If no subscription arrives the client defaults to receiving
// all events ("*").
func HandleEventsWS(authSvc *auth.Service, fanout *event.WSFanout, logger *slog.Logger) http.HandlerFunc {
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

			runEventsLoop(conn, r.Context(), fanout, logger)
			return
		}

		// --- First-message auth ---
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			logger.Error("events websocket upgrade failed", "error", err)
			return
		}

		authCtx, authCancel := context.WithTimeout(r.Context(), subscriptionTimeout)
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

		runEventsLoop(conn, r.Context(), fanout, logger)
	}
}

// runEventsLoop manages the lifecycle of an authenticated events WebSocket:
//   - waits up to 5s for an optional subscribe message; defaults to ["*"]
//   - registers the connection with the WSFanout (which delivers real events)
//   - sends periodic heartbeat pings
//   - cleans up on disconnect
func runEventsLoop(conn *websocket.Conn, reqCtx context.Context, fanout *event.WSFanout, logger *slog.Logger) {
	ctx, cancel := context.WithCancel(reqCtx)
	defer cancel()
	defer conn.CloseNow()

	logger.Debug("events websocket connected")

	// --- Optional subscribe message ---
	patterns := negotiatePatterns(conn, ctx, logger)

	// Register with fanout (no-op when fanout is nil to ease integration order).
	if fanout != nil {
		fanout.AddClient(conn, patterns)
		defer fanout.RemoveClient(conn)
	}

	// Heartbeat ticker — keeps idle connections alive and detects dropped clients.
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	// Reader goroutine: drain incoming frames (client may send future control messages).
	// Signals ctx cancellation when the connection closes.
	go func() {
		defer cancel()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("events websocket context done")
			return
		case <-ticker.C:
			if err := conn.Ping(ctx); err != nil {
				logger.Debug("events websocket ping failed", "error", err)
				return
			}
		}
	}
}

// negotiatePatterns reads an optional subscribe message within subscriptionTimeout.
// If none arrives, it returns the default wildcard pattern.
//
// The read runs in a background goroutine so that a timeout does NOT cancel the
// context passed to conn.Read — which would close the underlying connection with
// the coder/websocket library. The goroutine is bounded by the parent ctx (i.e.
// it exits when the connection is eventually torn down by runEventsLoop).
func negotiatePatterns(conn *websocket.Conn, ctx context.Context, logger *slog.Logger) []string {
	ch := make(chan []string, 1)

	go func() {
		// Use the parent ctx so cancelling it does not close the connection
		// prematurely; the timeout is handled via time.After below.
		_, msg, err := conn.Read(ctx)
		if err != nil {
			ch <- []string{"*"}
			return
		}
		var sub subscribeMsg
		if err := json.Unmarshal(msg, &sub); err != nil || sub.Type != "subscribe" || len(sub.Events) == 0 {
			logger.Debug("events websocket: ignoring non-subscribe first message, defaulting to '*'")
			ch <- []string{"*"}
			return
		}
		if len(sub.Events) > maxSubscriptionPatterns {
			logger.Debug("events websocket: too many subscription patterns, defaulting to '*'",
				"count", len(sub.Events), "max", maxSubscriptionPatterns)
			ch <- []string{"*"}
			return
		}
		ch <- sub.Events
	}()

	select {
	case patterns := <-ch:
		logger.Debug("events websocket: negotiated patterns", "patterns", patterns)
		return patterns
	case <-time.After(subscriptionTimeout):
		// The goroutine remains alive until the next read or connection close,
		// which is driven by the reader goroutine in runEventsLoop.
		logger.Debug("events websocket: subscription timeout, defaulting to '*'")
		return []string{"*"}
	case <-ctx.Done():
		return []string{"*"}
	}
}
