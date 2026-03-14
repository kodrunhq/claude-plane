// Package handler provides REST API handlers for the template system.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// varPattern matches template variable placeholders like ${VAR_NAME}.
var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// validVarName matches uppercase identifiers: starts with A-Z, then A-Z0-9_.
var validVarName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// TemplateHandler handles REST endpoints for session template CRUD.
type TemplateHandler struct {
	store     store.TemplateStoreIface
	getClaims ClaimsGetter
	publisher event.Publisher
}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler(s store.TemplateStoreIface, getClaims ClaimsGetter) *TemplateHandler {
	return &TemplateHandler{store: s, getClaims: getClaims}
}

// SetPublisher configures the event publisher for template lifecycle events.
func (h *TemplateHandler) SetPublisher(p event.Publisher) {
	h.publisher = p
}

// claims returns the current user's claims, or nil if no getter is configured.
func (h *TemplateHandler) claims(r *http.Request) *UserClaims {
	if h.getClaims == nil {
		return nil
	}
	return h.getClaims(r)
}

// authorizeTemplate checks whether the current user can access the given template.
func (h *TemplateHandler) authorizeTemplate(w http.ResponseWriter, r *http.Request, tmpl *store.SessionTemplate) bool {
	c := h.claims(r)
	if c == nil || c.Role == "admin" || c.UserID == tmpl.UserID {
		return true
	}
	writeError(w, http.StatusNotFound, "template not found")
	return false
}

// publish fires an event if a publisher is configured. Errors are logged but not propagated.
func (h *TemplateHandler) publish(eventType, templateID, userID string) {
	if h.publisher == nil {
		return
	}
	evt := event.NewTemplateEvent(eventType, templateID, userID)
	if err := h.publisher.Publish(context.Background(), evt); err != nil {
		slog.Warn("failed to publish template event", "type", eventType, "template_id", templateID, "error", err)
	}
}

// RegisterTemplateRoutes mounts all template-related routes on the given router.
func RegisterTemplateRoutes(r chi.Router, h *TemplateHandler) {
	r.Post("/api/v1/templates", h.Create)
	r.Get("/api/v1/templates", h.List)
	r.Get("/api/v1/templates/{templateID}", h.Get)
	r.Put("/api/v1/templates/{templateID}", h.Update)
	r.Delete("/api/v1/templates/{templateID}", h.Delete)
	r.Post("/api/v1/templates/{templateID}/clone", h.Clone)
}

// createTemplateRequest is the JSON body for POST /api/v1/templates.
type createTemplateRequest struct {
	Name           string            `json:"name"`
	MachineID      string            `json:"machine_id,omitempty"`
	Description    string            `json:"description,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	EnvVars        map[string]string `json:"env_vars,omitempty"`
	InitialPrompt  string            `json:"initial_prompt,omitempty"`
	TerminalRows   int               `json:"terminal_rows,omitempty"`
	TerminalCols   int               `json:"terminal_cols,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
}

// Create handles POST /api/v1/templates.
func (h *TemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TerminalRows <= 0 {
		req.TerminalRows = 24
	}
	if req.TerminalCols <= 0 {
		req.TerminalCols = 80
	}

	if msg := validatePromptVars(req.InitialPrompt); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	userID := ""
	if c := h.claims(r); c != nil {
		userID = c.UserID
	}

	// Check for duplicate name for the same user.
	existing, err := h.store.GetTemplateByName(r.Context(), userID, req.Name)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "template name already exists")
		return
	}

	tmpl := &store.SessionTemplate{
		UserID:         userID,
		Name:           req.Name,
		MachineID:      req.MachineID,
		Description:    req.Description,
		Command:        req.Command,
		Args:           req.Args,
		WorkingDir:     req.WorkingDir,
		EnvVars:        req.EnvVars,
		InitialPrompt:  req.InitialPrompt,
		TerminalRows:   req.TerminalRows,
		TerminalCols:   req.TerminalCols,
		Tags:           req.Tags,
		TimeoutSeconds: req.TimeoutSeconds,
	}

	created, err := h.store.CreateTemplate(r.Context(), tmpl)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeError(w, http.StatusConflict, "template name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.publish(event.TypeTemplateCreated, created.TemplateID, userID)
	writeJSON(w, http.StatusCreated, created)
}

// List handles GET /api/v1/templates.
func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)

	userID := ""
	if c != nil && c.Role != "admin" {
		userID = c.UserID
	}

	opts := store.ListTemplateOptions{
		Tag:  r.URL.Query().Get("tag"),
		Name: r.URL.Query().Get("name"),
	}

	templates, err := h.store.ListTemplates(r.Context(), userID, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if templates == nil {
		templates = []store.SessionTemplate{}
	}
	writeJSON(w, http.StatusOK, templates)
}

// Get handles GET /api/v1/templates/{templateID}.
func (h *TemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "templateID")
	tmpl, err := h.store.GetTemplate(r.Context(), templateID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !h.authorizeTemplate(w, r, tmpl) {
		return
	}
	writeJSON(w, http.StatusOK, tmpl)
}

// updateTemplateRequest is the JSON body for PUT /api/v1/templates/{templateID}.
type updateTemplateRequest struct {
	Name           string            `json:"name"`
	MachineID      string            `json:"machine_id,omitempty"`
	Description    string            `json:"description,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	EnvVars        map[string]string `json:"env_vars,omitempty"`
	InitialPrompt  string            `json:"initial_prompt,omitempty"`
	TerminalRows   int               `json:"terminal_rows,omitempty"`
	TerminalCols   int               `json:"terminal_cols,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
}

// Update handles PUT /api/v1/templates/{templateID}.
func (h *TemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "templateID")
	existing, err := h.store.GetTemplate(r.Context(), templateID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !h.authorizeTemplate(w, r, existing) {
		return
	}

	var req updateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TerminalRows <= 0 {
		req.TerminalRows = 24
	}
	if req.TerminalCols <= 0 {
		req.TerminalCols = 80
	}

	if msg := validatePromptVars(req.InitialPrompt); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	updated, err := h.store.UpdateTemplate(r.Context(), templateID, &store.SessionTemplate{
		Name:           req.Name,
		MachineID:      req.MachineID,
		Description:    req.Description,
		Command:        req.Command,
		Args:           req.Args,
		WorkingDir:     req.WorkingDir,
		EnvVars:        req.EnvVars,
		InitialPrompt:  req.InitialPrompt,
		TerminalRows:   req.TerminalRows,
		TerminalCols:   req.TerminalCols,
		Tags:           req.Tags,
		TimeoutSeconds: req.TimeoutSeconds,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeError(w, http.StatusConflict, "template name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.publish(event.TypeTemplateUpdated, templateID, existing.UserID)
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/v1/templates/{templateID}.
func (h *TemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "templateID")
	existing, err := h.store.GetTemplate(r.Context(), templateID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !h.authorizeTemplate(w, r, existing) {
		return
	}

	if err := h.store.DeleteTemplate(r.Context(), templateID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.publish(event.TypeTemplateDeleted, templateID, existing.UserID)
	w.WriteHeader(http.StatusNoContent)
}

// Clone handles POST /api/v1/templates/{templateID}/clone.
func (h *TemplateHandler) Clone(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "templateID")
	existing, err := h.store.GetTemplate(r.Context(), templateID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !h.authorizeTemplate(w, r, existing) {
		return
	}

	cloned, err := h.store.CloneTemplate(r.Context(), templateID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.publish(event.TypeTemplateCreated, cloned.TemplateID, existing.UserID)
	writeJSON(w, http.StatusCreated, cloned)
}

// validatePromptVars checks that all ${VAR} placeholders in a prompt use valid names.
func validatePromptVars(prompt string) string {
	if prompt == "" {
		return ""
	}
	matches := varPattern.FindAllStringSubmatch(prompt, -1)
	for _, m := range matches {
		if !validVarName.MatchString(m[1]) {
			return "invalid variable name: " + m[1] + " (must match [A-Z][A-Z0-9_]*)"
		}
	}
	return ""
}
