package session

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/httputil"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	"github.com/kodrunhq/claude-plane/internal/shared/cliutil"
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
	store          *store.Store
	connMgr        *connmgr.ConnectionManager
	registry       *Registry
	getClaims      ClaimsGetter
	logger         *slog.Logger
	publisher      event.Publisher
	injectionQueue *InjectionQueue
}

// SetPublisher sets the event publisher used to emit session lifecycle events.
func (h *SessionHandler) SetPublisher(p event.Publisher) {
	h.publisher = p
}

// SetInjectionQueue sets the injection queue used by the InjectSession handler.
func (h *SessionHandler) SetInjectionQueue(q *InjectionQueue) {
	h.injectionQueue = q
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
	MachineID       string            `json:"machine_id"`
	Command         string            `json:"command"`
	Args            []string          `json:"args"`
	WorkingDir      string            `json:"working_dir"`
	TerminalSize    *terminalSize     `json:"terminal_size"`
	EnvVars         map[string]string `json:"env_vars"`
	InitialPrompt   string            `json:"initial_prompt"`
	Model           string            `json:"model"`
	SkipPermissions *bool             `json:"skip_permissions"`
	// Template fields
	TemplateID   string            `json:"template_id"`
	TemplateName string            `json:"template_name"`
	Variables    map[string]string `json:"variables"`
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

	// Resolve template if specified
	var templateID string
	if req.TemplateID != "" || req.TemplateName != "" {
		var tmpl *store.SessionTemplate
		var err error

		if req.TemplateID != "" {
			tmpl, err = h.store.GetTemplate(r.Context(), req.TemplateID)
		} else {
			// Need user ID for name lookup
			userID := ""
			if h.getClaims != nil {
				if claims := h.getClaims(r); claims != nil {
					userID = claims.UserID
				}
			}
			tmpl, err = h.store.GetTemplateByName(r.Context(), userID, req.TemplateName)
		}

		if err != nil {
			httputil.WriteError(w, http.StatusNotFound, "template not found")
			return
		}

		// Ownership check: non-admin users can only use their own templates.
		if h.getClaims != nil {
			claims := h.getClaims(r)
			if claims != nil && claims.Role != "admin" && tmpl.UserID != claims.UserID {
				httputil.WriteError(w, http.StatusNotFound, "template not found")
				return
			}
		}

		templateID = tmpl.TemplateID

		// Merge: template provides defaults, request overrides
		if req.Command == "" {
			req.Command = tmpl.Command
		}
		if len(req.Args) == 0 && len(tmpl.Args) > 0 {
			req.Args = tmpl.Args
		}
		if req.WorkingDir == "" {
			req.WorkingDir = tmpl.WorkingDir
		}
		if req.InitialPrompt == "" {
			req.InitialPrompt = tmpl.InitialPrompt
		}
		if req.EnvVars == nil && len(tmpl.EnvVars) > 0 {
			req.EnvVars = tmpl.EnvVars
		}
		if req.TerminalSize == nil && (tmpl.TerminalRows > 0 || tmpl.TerminalCols > 0) {
			req.TerminalSize = &terminalSize{
				Rows: uint32(tmpl.TerminalRows),
				Cols: uint32(tmpl.TerminalCols),
			}
		}

		// Variable substitution in InitialPrompt
		if req.InitialPrompt != "" && len(req.Variables) > 0 {
			for k, v := range req.Variables {
				req.InitialPrompt = strings.ReplaceAll(req.InitialPrompt, "${"+k+"}", v)
			}
		}
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

	// Inject --model flag into args when model is specified.
	if req.Model != "" {
		req.Args = cliutil.StripFlagWithValue(req.Args, "--model")
		req.Args = append(req.Args, "--model", req.Model)
	}

	// Always strip --dangerously-skip-permissions from user-supplied args to prevent
	// bypass via the freeform args field, then re-add only when explicitly requested.
	req.Args = cliutil.StripFlag(req.Args, "--dangerously-skip-permissions")
	if req.SkipPermissions != nil && *req.SkipPermissions {
		req.Args = append([]string{"--dangerously-skip-permissions"}, req.Args...)
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

	h.logger.Info("session creation started", "session_id", sessionID, "machine_id", req.MachineID, "user_id", userID, "command", req.Command)

	// Persist to store
	sess := &store.Session{
		SessionID:     sessionID,
		MachineID:     req.MachineID,
		UserID:        userID,
		TemplateID:    templateID,
		Command:       req.Command,
		WorkingDir:    req.WorkingDir,
		Status:        store.StatusCreated,
		Model:         req.Model,
		SkipPerms:     boolToSkipPerms(req.SkipPermissions),
		EnvVars:       marshalJSON(req.EnvVars),
		Args:          marshalJSON(req.Args),
		InitialPrompt: req.InitialPrompt,
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
					SessionId:     sessionID,
					Command:       req.Command,
					Args:          req.Args,
					WorkingDir:    req.WorkingDir,
					EnvVars:       req.EnvVars,
					InitialPrompt: req.InitialPrompt,
					TerminalSize: &pb.TerminalSize{
						Rows: req.TerminalSize.Rows,
						Cols: req.TerminalSize.Cols,
					},
				},
			},
		}
		if err := agent.SendCommand(cmd); err != nil {
			h.logger.Error("failed to send create session command",
				"error", err,
				"machine_id", req.MachineID,
				"session_id", sessionID,
			)
			if err := h.store.UpdateSessionStatus(sessionID, store.StatusFailed); err != nil {
				h.logger.Warn("failed to update session status after command dispatch failure", "error", err, "session_id", sessionID)
			}
			h.publishEvent(r.Context(), event.NewDispatchFailedEvent(sessionID, req.MachineID, "", "session dispatch failed"))
			httputil.WriteError(w, http.StatusInternalServerError, "failed to dispatch session to agent")
			return
		}
	}

	h.logger.Info("session creation complete", "session_id", sessionID)
	h.publishEvent(r.Context(), event.NewSessionEvent(event.TypeSessionStarted, sessionID, req.MachineID, "", req.Command))
	httputil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"session_id": sessionID,
		"machine_id": req.MachineID,
		"status":     store.StatusCreated,
		"command":    req.Command,
	})
}

// ListSessions handles GET /api/v1/sessions.
// Supports optional query params: ?status=running&machine_id=xyz
func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	statusFilter := q.Get("status")
	machineFilter := q.Get("machine_id")

	listUserID := ""
	if h.getClaims != nil {
		if claims := h.getClaims(r); claims != nil {
			listUserID = claims.UserID
		}
	}
	h.logger.Info("session list requested", "user_id", listUserID, "status_filter", statusFilter, "machine_filter", machineFilter)

	var sessions []store.Session
	var err error

	if machineFilter != "" {
		sessions, err = h.store.ListSessionsByMachine(machineFilter)
	} else {
		sessions, err = h.store.ListSessions()
	}
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

	// Apply status filter after authorization.
	if statusFilter != "" {
		filtered := make([]store.Session, 0, len(sessions))
		for _, s := range sessions {
			if s.Status == statusFilter {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	// Strip sensitive fields from list responses to avoid leaking secrets.
	for i := range sessions {
		sessions[i].EnvVars = ""
		sessions[i].Args = ""
		sessions[i].InitialPrompt = ""
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
	// Strip sensitive fields to avoid leaking secrets.
	sess.EnvVars = ""
	sess.Args = ""
	sess.InitialPrompt = ""
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

	terminateUserID := ""
	if h.getClaims != nil {
		if claims := h.getClaims(r); claims != nil {
			terminateUserID = claims.UserID
		}
	}
	h.logger.Info("session termination started", "session_id", sessionID, "user_id", terminateUserID)

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
	h.logger.Info("session termination complete", "session_id", sessionID)
	h.publishEvent(r.Context(), event.NewSessionEvent(event.TypeSessionTerminated, sessionID, sess.MachineID, "", ""))

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": store.StatusTerminated})
}

// injectRequest is the JSON body for POST /api/v1/sessions/{sessionID}/inject.
type injectRequest struct {
	Text     string         `json:"text"`
	Raw      bool           `json:"raw"`
	DelayMs  int            `json:"delay_ms"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// InjectSession handles POST /api/v1/sessions/{sessionID}/inject.
// It enqueues text to be sent to a running session's PTY via the injection queue.
func (h *SessionHandler) InjectSession(w http.ResponseWriter, r *http.Request) {
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

	if sess.Status != store.StatusRunning {
		httputil.WriteError(w, http.StatusConflict, "session is not running")
		return
	}

	agent := h.connMgr.GetAgent(sess.MachineID)
	if agent == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "agent disconnected")
		return
	}

	var req injectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Text == "" {
		httputil.WriteError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.DelayMs < 0 {
		httputil.WriteError(w, http.StatusBadRequest, "delay_ms must be non-negative")
		return
	}
	if req.DelayMs > 30000 {
		httputil.WriteError(w, http.StatusBadRequest, "delay_ms must not exceed 30000")
		return
	}

	data := []byte(req.Text)
	if !req.Raw {
		data = append(data, '\n')
	}

	userID := ""
	if h.getClaims != nil {
		if claims := h.getClaims(r); claims != nil {
			userID = claims.UserID
		}
	}
	if userID == "" {
		userID = sess.UserID
	}

	metadataJSON := ""
	if req.Metadata != nil {
		if b, err := json.Marshal(req.Metadata); err == nil {
			metadataJSON = string(b)
		}
	}

	injectionID := uuid.New().String()

	if h.injectionQueue == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "injection service not available")
		return
	}

	// Create audit record BEFORE enqueue to avoid a delivery race: the drainer
	// can deliver and call UpdateInjectionDelivered before CreateInjection runs.
	inj := &store.Injection{
		InjectionID: injectionID,
		SessionID:   sessionID,
		UserID:      userID,
		TextLength:  utf8.RuneCountInString(req.Text),
		Metadata:    metadataJSON,
		Source:      "api",
	}
	created, err := h.store.CreateInjection(r.Context(), inj)
	if err != nil {
		h.logger.Error("failed to create injection audit record", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create injection record")
		return
	}

	// Enqueue — if this fails the audit record stays with delivered_at = NULL,
	// which correctly represents "attempted but not delivered".
	if err := h.injectionQueue.Enqueue(r.Context(), sessionID, sess.MachineID, data, req.DelayMs, injectionID); err != nil {
		if errors.Is(err, ErrQueueFull) {
			httputil.WriteError(w, http.StatusTooManyRequests, "injection queue full")
			return
		}
		if errors.Is(err, ErrSessionNotRunning) {
			httputil.WriteError(w, http.StatusConflict, "session is not running")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to enqueue injection")
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"injection_id": created.InjectionID,
		"queued_at":    time.Now().UTC(),
	})
}

// ListInjections handles GET /api/v1/sessions/{sessionID}/injections.
func (h *SessionHandler) ListInjections(w http.ResponseWriter, r *http.Request) {
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

	injections, err := h.store.ListInjectionsBySession(r.Context(), sessionID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list injections")
		return
	}
	if injections == nil {
		injections = []store.Injection{}
	}
	httputil.WriteJSON(w, http.StatusOK, injections)
}

// GetSessionStats handles GET /api/v1/sessions/stats
func (h *SessionHandler) GetSessionStats(w http.ResponseWriter, r *http.Request) {
	if h.getClaims != nil {
		claims := h.getClaims(r)
		if claims == nil || claims.Role != "admin" {
			httputil.WriteError(w, http.StatusForbidden, "admin access required")
			return
		}
	}

	since := time.Now().UTC().Add(-24 * time.Hour)
	if s := r.URL.Query().Get("since"); s != "" {
		parsed, err := parseTime(s)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid since: expected RFC3339")
			return
		}
		// Cap lookback to 90 days
		minSince := time.Now().UTC().Add(-90 * 24 * time.Hour)
		if parsed.Before(minSince) {
			parsed = minSince
		}
		since = parsed
	}

	total, succeeded, failed, err := h.store.GetSessionStats(since)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get session stats")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"total":     total,
		"succeeded": succeeded,
		"failed":    failed,
		"since":     since.Format(time.RFC3339),
	})
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

// parseTime tries RFC3339Nano first (handles fractional seconds from JavaScript
// Date.toISOString()), then falls back to RFC3339.
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// boolToSkipPerms converts a *bool skip_permissions field to the string
// representation used by the store ("1", "0", or "" when nil).
func boolToSkipPerms(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return "1"
	}
	return "0"
}

// marshalJSON serializes a value to a JSON string. Returns an empty string
// if the value is nil or serialization fails.
func marshalJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return ""
	}
	return string(b)
}
