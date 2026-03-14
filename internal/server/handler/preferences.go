package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// PreferencesStore is the persistence interface required by PreferencesHandler.
type PreferencesStore interface {
	GetUserPreferences(ctx context.Context, userID string) (*store.UserPreferences, error)
	UpsertUserPreferences(ctx context.Context, userID, preferences string) error
}

// PreferencesHandler handles REST endpoints for user preferences.
type PreferencesHandler struct {
	store     PreferencesStore
	getClaims ClaimsGetter
}

// NewPreferencesHandler creates a new PreferencesHandler.
func NewPreferencesHandler(s PreferencesStore, getClaims ClaimsGetter) *PreferencesHandler {
	return &PreferencesHandler{store: s, getClaims: getClaims}
}

// RegisterPreferencesRoutes mounts all preferences routes on the given router.
func RegisterPreferencesRoutes(r chi.Router, h *PreferencesHandler) {
	r.Get("/api/v1/users/me/preferences", h.Get)
	r.Put("/api/v1/users/me/preferences", h.Put)
	r.Patch("/api/v1/users/me/preferences", h.Patch)
}

// Get handles GET /api/v1/users/me/preferences.
func (h *PreferencesHandler) Get(w http.ResponseWriter, r *http.Request) {
	c := h.getClaims(r)
	if c == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	prefs, err := h.store.GetUserPreferences(r.Context(), c.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, json.RawMessage(prefs.Preferences))
}

// Put handles PUT /api/v1/users/me/preferences.
func (h *PreferencesHandler) Put(w http.ResponseWriter, r *http.Request) {
	c := h.getClaims(r)
	if c == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validatePreferences(raw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.UpsertUserPreferences(r.Context(), c.UserID, string(raw)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, json.RawMessage(raw))
}

// Patch handles PATCH /api/v1/users/me/preferences.
// Performs a shallow merge at top-level keys.
func (h *PreferencesHandler) Patch(w http.ResponseWriter, r *http.Request) {
	c := h.getClaims(r)
	if c == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var patch json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Decode patch into top-level map.
	var patchMap map[string]json.RawMessage
	if err := json.Unmarshal(patch, &patchMap); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be a JSON object")
		return
	}

	// Fetch existing preferences.
	existing, err := h.store.GetUserPreferences(r.Context(), c.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Decode existing into top-level map.
	var baseMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(existing.Preferences), &baseMap); err != nil {
		baseMap = make(map[string]json.RawMessage)
	}

	// Shallow merge: patch keys overwrite base keys.
	for k, v := range patchMap {
		baseMap[k] = v
	}

	merged, err := json.Marshal(baseMap)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := validatePreferences(merged); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.UpsertUserPreferences(r.Context(), c.UserID, string(merged)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, json.RawMessage(merged))
}

// preferencesPayload is used for validation only.
type preferencesPayload struct {
	MachineOverrides       map[string]machineOverride `json:"machine_overrides"`
	MaxConcurrentSessions  *int                       `json:"max_concurrent_sessions"`
	DefaultSessionTimeout  *int                       `json:"default_session_timeout"`
	DefaultStepTimeout     *int                       `json:"default_step_timeout"`
	DefaultStepDelay       *int                       `json:"default_step_delay"`
	Theme                  *string                    `json:"theme"`
}

// machineOverride holds per-machine preference overrides.
type machineOverride struct {
	Model string `json:"model"`
}

// validatePreferences checks that known fields satisfy constraints.
func validatePreferences(raw json.RawMessage) error {
	var p preferencesPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("invalid preferences JSON: %w", err)
	}

	// Validate machine_overrides model values.
	for machineID, override := range p.MachineOverrides {
		if override.Model != "" && override.Model != "opus" && override.Model != "sonnet" && override.Model != "haiku" {
			return fmt.Errorf("machine_overrides[%s].model must be one of: opus, sonnet, haiku", machineID)
		}
	}

	// Validate max_concurrent_sessions.
	if p.MaxConcurrentSessions != nil && *p.MaxConcurrentSessions < 0 {
		return fmt.Errorf("max_concurrent_sessions must be >= 0")
	}

	// Validate default_session_timeout.
	if p.DefaultSessionTimeout != nil && *p.DefaultSessionTimeout < 0 {
		return fmt.Errorf("default_session_timeout must be >= 0")
	}

	// Validate default_step_timeout.
	if p.DefaultStepTimeout != nil && *p.DefaultStepTimeout < 0 {
		return fmt.Errorf("default_step_timeout must be >= 0")
	}

	// Validate default_step_delay.
	if p.DefaultStepDelay != nil && (*p.DefaultStepDelay < 0 || *p.DefaultStepDelay > 86400) {
		return fmt.Errorf("default_step_delay must be between 0 and 86400")
	}

	// Validate theme.
	if p.Theme != nil && *p.Theme != "" && *p.Theme != "light" && *p.Theme != "dark" && *p.Theme != "system" {
		return fmt.Errorf("theme must be one of: light, dark, system")
	}

	return nil
}
