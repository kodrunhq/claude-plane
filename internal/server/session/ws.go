package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"github.com/claudeplane/claude-plane/internal/server/auth"
	"github.com/claudeplane/claude-plane/internal/server/connmgr"
	"github.com/claudeplane/claude-plane/internal/server/store"
	pb "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
)

// wsControlMessage is a parsed text WebSocket frame (resize, auth, etc.).
type wsControlMessage struct {
	Type  string `json:"type"`
	Token string `json:"token,omitempty"`
	Cols  uint32 `json:"cols,omitempty"`
	Rows  uint32 `json:"rows,omitempty"`
}

// authTimeout is the maximum time allowed for the first-message auth handshake.
const authTimeout = 5 * time.Second

// HandleTerminalWS returns an http.HandlerFunc that bridges a browser WebSocket
// to an agent's terminal session via the gRPC command stream.
//
// Authentication supports two modes (backwards compatible):
//  1. Query parameter: ?token=<jwt> (deprecated, will be removed in a future version)
//  2. First-message auth: connect without token, send {"type":"auth","token":"<jwt>"} as first message
//
// If a query param token is provided, it is used immediately. Otherwise the
// handler upgrades the WebSocket and waits up to 5 seconds for an auth message.
func HandleTerminalWS(st *store.Store, cm *connmgr.ConnectionManager, reg *Registry, authSvc *auth.Service, logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := chi.URLParam(r, "sessionID")

		// --- Query-param auth (deprecated, backwards compat) ---
		token := r.URL.Query().Get("token")
		if token != "" {
			claims, err := authSvc.ValidateToken(token)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			sess, err := st.GetSession(sessionID)
			if err != nil {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}

			if claims.Role != "admin" && claims.UserID != sess.UserID {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				logger.Error("websocket upgrade failed", "error", err)
				return
			}

			runSession(conn, r.Context(), sessionID, sess.MachineID, cm, reg, logger)
			return
		}

		// --- First-message auth ---
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			logger.Error("websocket upgrade failed", "error", err)
			return
		}

		authCtx, authCancel := context.WithTimeout(r.Context(), authTimeout)
		defer authCancel()

		msgType, data, err := conn.Read(authCtx)
		if err != nil {
			logger.Debug("auth message read failed", "session_id", sessionID, "error", err)
			conn.Close(websocket.StatusPolicyViolation, "auth timeout or read error")
			return
		}
		if msgType != websocket.MessageText {
			conn.Close(websocket.StatusPolicyViolation, "expected text auth message")
			return
		}

		var authMsg wsControlMessage
		if err := json.Unmarshal(data, &authMsg); err != nil || authMsg.Type != "auth" || authMsg.Token == "" {
			conn.Close(websocket.StatusPolicyViolation, "invalid auth message")
			return
		}

		claims, err := authSvc.ValidateToken(authMsg.Token)
		if err != nil {
			conn.Close(websocket.StatusPolicyViolation, "invalid token")
			return
		}

		sess, err := st.GetSession(sessionID)
		if err != nil {
			conn.Close(websocket.StatusPolicyViolation, "session not found")
			return
		}

		if claims.Role != "admin" && claims.UserID != sess.UserID {
			conn.Close(websocket.StatusPolicyViolation, "forbidden")
			return
		}

		runSession(conn, r.Context(), sessionID, sess.MachineID, cm, reg, logger)
	}
}

// sendToAgent returns the current agent for machineID and sends a command.
// Re-fetches the agent on each call to handle agent reconnections (B10 fix).
func sendToAgent(cm *connmgr.ConnectionManager, machineID string, cmd *pb.ServerCommand) error {
	agent := cm.GetAgent(machineID)
	if agent == nil || agent.SendCommand == nil {
		return nil
	}
	return agent.SendCommand(cmd)
}

// runSession handles the bidirectional WebSocket<->agent relay after auth is complete.
func runSession(conn *websocket.Conn, reqCtx context.Context, sessionID, machineID string, cm *connmgr.ConnectionManager, reg *Registry, logger *slog.Logger) {
	ctx, cancel := context.WithCancel(reqCtx)
	defer cancel()

	// Subscribe to session output
	ch := reg.Subscribe(sessionID)
	defer reg.Unsubscribe(sessionID, ch)

	// Attach session on the agent: replays scrollback and enables live relay.
	_ = sendToAgent(cm, machineID, &pb.ServerCommand{
		Command: &pb.ServerCommand_AttachSession{
			AttachSession: &pb.AttachSessionCmd{
				SessionId: sessionID,
			},
		},
	})

	// Writer goroutine: reads from subscriber channel and writes to WebSocket.
	go func() {
		defer cancel()
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
			_ = sendToAgent(cm, machineID, &pb.ServerCommand{
				Command: &pb.ServerCommand_DetachSession{
					DetachSession: &pb.DetachSessionCmd{
						SessionId: sessionID,
					},
				},
			})
			logger.Debug("websocket read error (client disconnected)", "session_id", sessionID, "error", err)
			conn.CloseNow()
			return
		}

		switch msgType {
		case websocket.MessageBinary:
			// Keystroke input
			_ = sendToAgent(cm, machineID, &pb.ServerCommand{
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
				_ = sendToAgent(cm, machineID, &pb.ServerCommand{
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
