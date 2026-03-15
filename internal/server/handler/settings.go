package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// validSettingKeys is the allowlist of accepted setting keys.
var validSettingKeys = map[string]bool{"retention_days": true}

// validRetentionDays are the allowed values for the retention_days setting.
// 0 means unlimited.
var validRetentionDays = map[int]bool{7: true, 30: true, 90: true, 365: true, 0: true}

// SettingsHandler handles REST endpoints for server settings.
type SettingsHandler struct {
	store     *store.Store
	getClaims ClaimsGetter
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(s *store.Store, getClaims ClaimsGetter) *SettingsHandler {
	return &SettingsHandler{store: s, getClaims: getClaims}
}

// RegisterSettingsRoutes mounts settings routes.
func RegisterSettingsRoutes(r chi.Router, h *SettingsHandler) {
	r.Get("/api/v1/settings", h.GetSettings)
	r.Put("/api/v1/settings", h.UpdateSettings)
}

// GetSettings returns all server settings.
func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetAllSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// UpdateSettings upserts server settings. Admin-only.
func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)
	if claims == nil || claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Reject unknown setting keys
	for k := range body {
		if !validSettingKeys[k] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown setting key: %s", k))
			return
		}
	}

	// Validate known settings
	if val, ok := body["retention_days"]; ok {
		days, err := strconv.Atoi(val)
		if err != nil || !validRetentionDays[days] {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("retention_days must be one of: 7, 30, 90, 365, 0 (unlimited)"))
			return
		}
	}

	for k, v := range body {
		if err := h.store.SetSetting(r.Context(), k, v); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update settings")
			return
		}
	}

	// Return updated settings
	settings, err := h.store.GetAllSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}
