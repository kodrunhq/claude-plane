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
	SkipPermissions       *bool                      `json:"skip_permissions"`
	DefaultSessionTimeout *int                       `json:"default_session_timeout"`
	DefaultStepTimeout    *int                       `json:"default_step_timeout"`
	DefaultStepDelay      *int                       `json:"default_step_delay"`
	SessionStaleTimeout   *int                        `json:"session_stale_timeout"`
	DefaultEnvVars        map[string]string           `json:"default_env_vars"`
	Notifications         *notificationPrefs          `json:"notifications"`
	UI                    *uiPrefs                    `json:"ui"`
	MachineOverrides      map[string]machineOverride  `json:"machine_overrides"`
}

// notificationPrefs holds notification preferences.
type notificationPrefs struct {
	Events []string `json:"events"`
}

// uiPrefs holds UI preferences.
type uiPrefs struct {
	Theme              string   `json:"theme"`
	TerminalFontSize   *int     `json:"terminal_font_size"`
	AutoAttachSession  bool     `json:"auto_attach_session"`
	CommandCenterCards []string `json:"command_center_cards"`
}

// machineOverride holds per-machine preference overrides.
type machineOverride struct {
	WorkingDir            string            `json:"working_dir"`
	Model                 string            `json:"model"`
	EnvVars               map[string]string `json:"env_vars"`
	MaxConcurrentSessions int               `json:"max_concurrent_sessions"`
}

// validatePreferences checks that known fields satisfy constraints.
func validatePreferences(raw json.RawMessage) error {
	var p preferencesPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("invalid preferences JSON: %w", err)
	}

	// Validate machine_overrides.
	for machineID, override := range p.MachineOverrides {
		if override.Model != "" && override.Model != "opus" && override.Model != "sonnet" && override.Model != "haiku" && override.Model != "opusplan" {
			return fmt.Errorf("machine_overrides[%s].model must be one of: opus, sonnet, haiku, opusplan", machineID)
		}
		if override.MaxConcurrentSessions < 0 {
			return fmt.Errorf("machine_overrides[%s].max_concurrent_sessions must be >= 0", machineID)
		}
	}

	// Validate default_session_timeout.
	if p.DefaultSessionTimeout != nil && *p.DefaultSessionTimeout < 0 {
		return fmt.Errorf("default_session_timeout must be >= 0")
	}

	// Validate default_step_timeout.
	if p.DefaultStepTimeout != nil && *p.DefaultStepTimeout < 0 {
		return fmt.Errorf("default_step_timeout must be >= 0")
	}

	// Validate session_stale_timeout.
	if p.SessionStaleTimeout != nil && *p.SessionStaleTimeout < 0 {
		return fmt.Errorf("session_stale_timeout must be >= 0")
	}

	// Validate default_step_delay.
	if p.DefaultStepDelay != nil && (*p.DefaultStepDelay < 0 || *p.DefaultStepDelay > 86400) {
		return fmt.Errorf("default_step_delay must be between 0 and 86400")
	}

	// Validate UI preferences.
	if p.UI != nil {
		validThemes := map[string]bool{"": true, "light": true, "dark": true, "system": true}
		if !validThemes[p.UI.Theme] {
			return fmt.Errorf("ui.theme must be one of: light, dark, system")
		}
		if p.UI.TerminalFontSize != nil && (*p.UI.TerminalFontSize < 8 || *p.UI.TerminalFontSize > 72) {
			return fmt.Errorf("ui.terminal_font_size must be between 8 and 72")
		}
	}

	return nil
}
