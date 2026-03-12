package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"github.com/claudeplane/claude-plane/internal/server/auth"
	"github.com/claudeplane/claude-plane/internal/server/connmgr"
	"github.com/claudeplane/claude-plane/internal/server/store"
	pb "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
)

// wsControlMessage is a parsed text WebSocket frame (resize, etc.).
type wsControlMessage struct {
	Type string `json:"type"`
	Cols uint32 `json:"cols"`
	Rows uint32 `json:"rows"`
}

// HandleTerminalWS returns an http.HandlerFunc that bridges a browser WebSocket
// to an agent's terminal session via the gRPC command stream.
//
// Authentication is via query parameter ?token= since WebSocket cannot send
// Authorization headers during the upgrade handshake.
func HandleTerminalWS(st *store.Store, cm *connmgr.ConnectionManager, reg *Registry, authSvc *auth.Service, logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := chi.URLParam(r, "sessionID")

		// Authenticate via query param token
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		claims, err := authSvc.ValidateToken(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		// Look up session
		sess, err := st.GetSession(sessionID)
		if err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		// Authorize: only the session owner or admins can attach
		if claims.Role != "admin" && claims.UserID != sess.UserID {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// Get agent
		agent := cm.GetAgent(sess.MachineID)
		if agent == nil {
			http.Error(w, "agent not connected", http.StatusBadGateway)
			return
		}

		// Upgrade to WebSocket — allow same-origin only by default.
		// The coder/websocket library checks the Origin header against the Host
		// when OriginPatterns is not set (default secure behavior).
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			logger.Error("websocket upgrade failed", "error", err)
			return
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Subscribe to session output
		ch := reg.Subscribe(sessionID)
		defer reg.Unsubscribe(sessionID, ch)

		// Attach session on the agent: replays scrollback and enables live relay.
		if agent.SendCommand != nil {
			_ = agent.SendCommand(&pb.ServerCommand{
				Command: &pb.ServerCommand_AttachSession{
					AttachSession: &pb.AttachSessionCmd{
						SessionId: sessionID,
					},
				},
			})
		}

		// Writer goroutine: reads from subscriber channel and writes to WebSocket.
		// Single goroutine ensures thread-safe WebSocket writes.
		go func() {
			defer cancel() // cancel reader loop if writer exits
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						return
					}
					msgType := websocket.MessageBinary
					if msg.IsControl {
						msgType = websocket.MessageText
					}
					if err := conn.Write(ctx, msgType, msg.Data); err != nil {
						logger.Debug("websocket write error", "session_id", sessionID, "error", err)
						return
					}
				}
			}
		}()

		// Reader loop (main goroutine): reads from WebSocket and dispatches to agent.
		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				// WebSocket closed or error — send DetachSessionCmd (not kill)
				if agent.SendCommand != nil {
					_ = agent.SendCommand(&pb.ServerCommand{
						Command: &pb.ServerCommand_DetachSession{
							DetachSession: &pb.DetachSessionCmd{
								SessionId: sessionID,
							},
						},
					})
				}
				logger.Debug("websocket read error (client disconnected)", "session_id", sessionID, "error", err)
				conn.CloseNow()
				return
			}

			if agent.SendCommand == nil {
				continue
			}

			switch msgType {
			case websocket.MessageBinary:
				// Keystroke input
				_ = agent.SendCommand(&pb.ServerCommand{
					Command: &pb.ServerCommand_InputData{
						InputData: &pb.InputDataCmd{
							SessionId: sessionID,
							Data:      data,
						},
					},
				})
			case websocket.MessageText:
				// Control message (resize)
				var ctrl wsControlMessage
				if err := json.Unmarshal(data, &ctrl); err != nil {
					logger.Debug("invalid control message", "session_id", sessionID, "error", err)
					continue
				}
				switch ctrl.Type {
				case "resize":
					_ = agent.SendCommand(&pb.ServerCommand{
						Command: &pb.ServerCommand_ResizeTerminal{
							ResizeTerminal: &pb.ResizeTerminalCmd{
								SessionId: sessionID,
								Size: &pb.TerminalSize{
									Cols: ctrl.Cols,
									Rows: ctrl.Rows,
								},
							},
						},
					})
				}
			}
		}
	}
}
