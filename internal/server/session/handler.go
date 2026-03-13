package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/httputil"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// UserClaims holds the minimal user identity needed for authorization.
type UserClaims struct {
	UserID string
	Role   string
}

// ClaimsGetter extracts user claims from a request context.
// This decouples the session handler from the api package to avoid import cycles.
type ClaimsGetter func(r *http.Request) *UserClaims

// SessionHandler provides REST handlers for session lifecycle management.
type SessionHandler struct {
	store     *store.Store
	connMgr   *connmgr.ConnectionManager
	registry  *Registry
	getClaims ClaimsGetter
	logger    *slog.Logger
	publisher event.Publisher
}

// SetPublisher sets the event publisher used to emit session lifecycle events.
func (h *SessionHandler) SetPublisher(p event.Publisher) {
	h.publisher = p
}

// publishEvent emits an event if a publisher is configured.
func (h *SessionHandler) publishEvent(ctx context.Context, e event.Event) {
	if h.publisher != nil {
		if err := h.publisher.Publish(ctx, e); err != nil {
			h.logger.Warn("failed to publish event", "event_type", e.Type, "error", err)
		}
	}
}

// NewSessionHandler creates a new SessionHandler.
func NewSessionHandler(s *store.Store, cm *connmgr.ConnectionManager, r *Registry, getClaims ClaimsGetter, logger *slog.Logger) *SessionHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionHandler{
		store:     s,
		connMgr:   cm,
		registry:  r,
		getClaims: getClaims,
		logger:    logger,
	}
}

// createSessionRequest is the JSON body for POST /api/v1/sessions.
type createSessionRequest struct {
	MachineID    string        `json:"machine_id"`
	Command      string        `json:"command"`
	Args         []string      `json:"args"`
	WorkingDir   string        `json:"working_dir"`
	TerminalSize *terminalSize `json:"terminal_size"`
}

type terminalSize struct {
	Rows uint32 `json:"rows"`
	Cols uint32 `json:"cols"`
}

// CreateSession handles POST /api/v1/sessions.
func (h *SessionHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MachineID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "machine_id is required")
		return
	}

	// Defaults
	if req.Command == "" {
		req.Command = "claude"
	}
	// WorkingDir left empty intentionally — the agent inherits its own cwd.
	if req.TerminalSize == nil {
		req.TerminalSize = &terminalSize{Rows: 24, Cols: 80}
	}

	// Verify agent is connected
	agent := h.connMgr.GetAgent(req.MachineID)
	if agent == nil {
		httputil.WriteError(w, http.StatusNotFound, "machine not connected")
		return
	}

	sessionID := uuid.New().String()
	userID := ""
	if h.getClaims != nil {
		if claims := h.getClaims(r); claims != nil {
			userID = claims.UserID
		}
	}

	// Persist to store
	sess := &store.Session{
		SessionID:  sessionID,
		MachineID:  req.MachineID,
		UserID:     userID,
		Command:    req.Command,
		WorkingDir: req.WorkingDir,
		Status:     store.StatusCreated,
	}
	if err := h.store.CreateSession(sess); err != nil {
		h.logger.Error("failed to create session", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	// Send CreateSessionCmd to agent
	if agent.SendCommand != nil {
		cmd := &pb.ServerCommand{
			Command: &pb.ServerCommand_CreateSession{
				CreateSession: &pb.CreateSessionCmd{
					SessionId:  sessionID,
					Command:    req.Command,
					Args:       req.Args,
					WorkingDir: req.WorkingDir,
					TerminalSize: &pb.TerminalSize{
						Rows: req.TerminalSize.Rows,
						Cols: req.TerminalSize.Cols,
					},
				},
			},
		}
		if err := agent.SendCommand(cmd); err != nil {
			h.logger.Error("failed to send create session command", "error", err)
			// Session was created in DB but command failed; update status
			if err := h.store.UpdateSessionStatus(sessionID, store.StatusFailed); err != nil {
				h.logger.Warn("failed to update session status after command dispatch failure", "error", err, "session_id", sessionID)
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to dispatch session to agent")
			return
		}
	}

	h.publishEvent(r.Context(), event.NewSessionEvent(event.TypeSessionStarted, sessionID, req.MachineID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"session_id": sessionID,
		"machine_id": req.MachineID,
		"status":     store.StatusCreated,
		"command":    req.Command,
	})
}

// ListSessions handles GET /api/v1/sessions.
func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.store.ListSessions()
	if err != nil {
		h.logger.Error("failed to list sessions", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	// Non-admin users only see their own sessions.
	// When auth is configured but claims are nil, deny access entirely.
	if h.getClaims != nil {
		claims := h.getClaims(r)
		if claims == nil {
			httputil.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if claims.Role != "admin" {
			filtered := make([]store.Session, 0, len(sessions))
			for _, s := range sessions {
				if s.UserID == claims.UserID {
					filtered = append(filtered, s)
				}
			}
			sessions = filtered
		}
	}

	httputil.WriteJSON(w, http.StatusOK, sessions)
}

// GetSession handles GET /api/v1/sessions/{sessionID}.
func (h *SessionHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	sess, err := h.store.GetSession(sessionID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if !h.authorizeSession(r, sess) {
		httputil.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sess)
}

// TerminateSession handles DELETE /api/v1/sessions/{sessionID}.
func (h *SessionHandler) TerminateSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	sess, err := h.store.GetSession(sessionID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if !h.authorizeSession(r, sess) {
		httputil.WriteError(w, http.StatusNotFound, "session not found")
		return
	}

	agent := h.connMgr.GetAgent(sess.MachineID)
	if agent != nil && agent.SendCommand != nil {
		cmd := &pb.ServerCommand{
			Command: &pb.ServerCommand_KillSession{
				KillSession: &pb.KillSessionCmd{
					SessionId: sessionID,
					Signal:    "SIGTERM",
				},
			},
		}
		if err := agent.SendCommand(cmd); err != nil {
			h.logger.Error("failed to send kill session command", "error", err)
		}
	}

	if err := h.store.UpdateSessionStatus(sessionID, store.StatusTerminated); err != nil {
		h.logger.Error("failed to update session status", "error", err)
	}
	h.publishEvent(r.Context(), event.NewSessionEvent(event.TypeSessionExited, sessionID, sess.MachineID))

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": store.StatusTerminated})
}

// authorizeSession returns true if the current user is allowed to access the session.
// Admins can access any session; regular users can only access their own.
// Returns true only when no claims getter is configured (explicitly unauthenticated mode).
// When a claims getter is configured but returns nil claims (e.g. misconfigured middleware),
// access is denied to prevent silent security bypass.
func (h *SessionHandler) authorizeSession(r *http.Request, sess *store.Session) bool {
	if h.getClaims == nil {
		return true
	}
	claims := h.getClaims(r)
	if claims == nil {
		return false
	}
	return claims.Role == "admin" || claims.UserID == sess.UserID
}
