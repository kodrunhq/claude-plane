package session

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// ErrAgentNotConnected is returned when no agent is available for a machine.
var ErrAgentNotConnected = errors.New("agent not connected")

// wsAcceptOptions restricts WebSocket upgrades to same-origin requests.
var wsAcceptOptions = &websocket.AcceptOptions{
	InsecureSkipVerify: false,
}

// wsControlMessage is a parsed text WebSocket frame (resize, auth, etc.).
type wsControlMessage struct {
	Type  string `json:"type"`
	Token string `json:"token,omitempty"`
	Cols  uint32 `json:"cols,omitempty"`
	Rows  uint32 `json:"rows,omitempty"`
}

// authTimeout is the maximum time allowed for the first-message auth handshake.
const authTimeout = 5 * time.Second

// sessionCookieName references the canonical cookie name from the auth package.
const sessionCookieName = auth.SessionCookieName

// HandleTerminalWS returns an http.HandlerFunc that bridges a browser WebSocket
// to an agent's terminal session via the gRPC command stream.
//
// Authentication is checked in this order:
//  1. httpOnly cookie named "session_token" (preferred, pre-upgrade)
//  2. First-message auth: send {"type":"auth","token":"<jwt>"} as first WebSocket message
func HandleTerminalWS(st *store.Store, cm *connmgr.ConnectionManager, reg *Registry, authSvc *auth.Service, logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := chi.URLParam(r, "sessionID")

		// --- Cookie auth (preferred, pre-upgrade) ---
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			claims, err := authSvc.ValidateToken(cookie.Value)
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

			conn, err := websocket.Accept(w, r, wsAcceptOptions)
			if err != nil {
				logger.Error("websocket upgrade failed", "error", err)
				return
			}

			logger.Info("websocket upgrade success", "session_id", sessionID, "remote_addr", r.RemoteAddr)
			logger.Info("websocket auth success", "session_id", sessionID, "user_id", claims.UserID, "auth_method", "cookie")

			runSession(conn, r.Context(), sessionID, sess.MachineID, cm, reg, logger)
			return
		}

		// --- First-message auth ---
		conn, err := websocket.Accept(w, r, wsAcceptOptions)
		if err != nil {
			logger.Error("websocket upgrade failed", "error", err)
			return
		}

		logger.Info("websocket upgrade success", "session_id", sessionID, "remote_addr", r.RemoteAddr)

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

		logger.Info("websocket auth success", "session_id", sessionID, "user_id", claims.UserID, "auth_method", "bearer")

		runSession(conn, r.Context(), sessionID, sess.MachineID, cm, reg, logger)
	}
}

// sendToAgent returns the current agent for machineID and sends a command.
// Re-fetches the agent on each call to handle agent reconnections (B10 fix).
// Returns ErrAgentNotConnected if the agent is not available.
func sendToAgent(cm *connmgr.ConnectionManager, machineID string, cmd *pb.ServerCommand) error {
	agent := cm.GetAgent(machineID)
	if agent == nil || agent.SendCommand == nil {
		return ErrAgentNotConnected
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
	logger.Info("session attach initiated", "session_id", sessionID, "machine_id", machineID)
	if err := sendToAgent(cm, machineID, &pb.ServerCommand{
		Command: &pb.ServerCommand_AttachSession{
			AttachSession: &pb.AttachSessionCmd{
				SessionId: sessionID,
			},
		},
	}); err != nil {
		logger.Warn("failed to attach session on agent", "session_id", sessionID, "machine_id", machineID, "error", err)
		// Notify the browser so it doesn't hang in "Connecting" state.
		// Send scrollback_end first (so the client transitions out of replaying)
		// then session_ended (so it knows the session can't be viewed).
		reg.PublishControl(sessionID, []byte(`{"type":"scrollback_end"}`))
		reg.PublishControl(sessionID, []byte(`{"type":"session_ended","status":"disconnected"}`))
		// Drain the subscriber channel to deliver the control messages, then close.
		go func() {
			for msg := range ch {
				msgType := websocket.MessageBinary
				if msg.IsControl {
					msgType = websocket.MessageText
				}
				if writeErr := conn.Write(ctx, msgType, msg.Data); writeErr != nil {
					break
				}
			}
			conn.Close(websocket.StatusNormalClosure, "agent not connected")
		}()
		return
	}

	logger.Info("session relay started", "session_id", sessionID)

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
			logger.Info("websocket close/detach", "session_id", sessionID, "reason", err.Error())
			// WebSocket closed or error — send DetachSessionCmd (not kill)
			if err := sendToAgent(cm, machineID, &pb.ServerCommand{
				Command: &pb.ServerCommand_DetachSession{
					DetachSession: &pb.DetachSessionCmd{
						SessionId: sessionID,
					},
				},
			}); err != nil {
				logger.Warn("failed to send detach session command to agent", "error", err, "session_id", sessionID, "machine_id", machineID)
			}
			logger.Debug("websocket read error (client disconnected)", "session_id", sessionID, "error", err)
			conn.CloseNow()
			return
		}

		switch msgType {
		case websocket.MessageBinary:
			// Keystroke input
			if err := sendToAgent(cm, machineID, &pb.ServerCommand{
				Command: &pb.ServerCommand_InputData{
					InputData: &pb.InputDataCmd{
						SessionId: sessionID,
						Data:      data,
					},
				},
			}); err != nil {
				logger.Warn("failed to send input data to agent", "error", err, "session_id", sessionID, "machine_id", machineID)
			}
		case websocket.MessageText:
			// Control message (resize)
			var ctrl wsControlMessage
			if err := json.Unmarshal(data, &ctrl); err != nil {
				logger.Debug("invalid control message", "session_id", sessionID, "error", err)
				continue
			}
			switch ctrl.Type {
			case "resize":
				if err := sendToAgent(cm, machineID, &pb.ServerCommand{
					Command: &pb.ServerCommand_ResizeTerminal{
						ResizeTerminal: &pb.ResizeTerminalCmd{
							SessionId: sessionID,
							Size: &pb.TerminalSize{
								Cols: ctrl.Cols,
								Rows: ctrl.Rows,
							},
						},
					},
				}); err != nil {
					logger.Warn("failed to send resize command to agent", "error", err, "session_id", sessionID, "machine_id", machineID)
				}
			}
		}
	}
}
