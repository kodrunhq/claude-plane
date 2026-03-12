package session

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/claudeplane/claude-plane/internal/server/connmgr"
	"github.com/claudeplane/claude-plane/internal/server/store"
	pb "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
)

// ClaimsGetter extracts user claims from a request context.
// This decouples the session handler from the api package to avoid import cycles.
type ClaimsGetter func(r *http.Request) (userID string)

// SessionHandler provides REST handlers for session lifecycle management.
type SessionHandler struct {
	store       *store.Store
	connMgr     *connmgr.ConnectionManager
	registry    *Registry
	getClaims   ClaimsGetter
	logger      *slog.Logger
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
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MachineID == "" {
		writeError(w, http.StatusBadRequest, "machine_id is required")
		return
	}

	// Defaults
	if req.Command == "" {
		req.Command = "claude"
	}
	if req.WorkingDir == "" {
		req.WorkingDir = "~"
	}
	if req.TerminalSize == nil {
		req.TerminalSize = &terminalSize{Rows: 24, Cols: 80}
	}

	// Verify agent is connected
	agent := h.connMgr.GetAgent(req.MachineID)
	if agent == nil {
		writeError(w, http.StatusNotFound, "machine not connected")
		return
	}

	sessionID := uuid.New().String()
	userID := ""
	if h.getClaims != nil {
		userID = h.getClaims(r)
	}

	// Persist to store
	sess := &store.Session{
		SessionID:  sessionID,
		MachineID:  req.MachineID,
		UserID:     userID,
		Command:    req.Command,
		WorkingDir: req.WorkingDir,
		Status:     "created",
	}
	if err := h.store.CreateSession(sess); err != nil {
		h.logger.Error("failed to create session", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create session")
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
			_ = h.store.UpdateSessionStatus(sessionID, "failed")
			writeError(w, http.StatusInternalServerError, "failed to dispatch session to agent")
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"session_id": sessionID,
		"machine_id": req.MachineID,
		"status":     "created",
		"command":    req.Command,
	})
}

// ListSessions handles GET /api/v1/sessions.
func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.store.ListSessions()
	if err != nil {
		h.logger.Error("failed to list sessions", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

// GetSession handles GET /api/v1/sessions/{sessionID}.
func (h *SessionHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	sess, err := h.store.GetSession(sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// TerminateSession handles DELETE /api/v1/sessions/{sessionID}.
func (h *SessionHandler) TerminateSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	sess, err := h.store.GetSession(sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
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

	if err := h.store.UpdateSessionStatus(sessionID, "terminated"); err != nil {
		h.logger.Error("failed to update session status", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "terminated"})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
