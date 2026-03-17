package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/notify"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// NotificationChannelStore is the persistence interface for notification channels.
type NotificationChannelStore interface {
	CreateNotificationChannel(ctx context.Context, ch store.NotificationChannel) (*store.NotificationChannel, error)
	GetNotificationChannel(ctx context.Context, channelID string) (*store.NotificationChannel, error)
	ListNotificationChannels(ctx context.Context) ([]store.NotificationChannel, error)
	UpdateNotificationChannel(ctx context.Context, ch store.NotificationChannel) (*store.NotificationChannel, error)
	DeleteNotificationChannel(ctx context.Context, channelID string) error
	GetSubscriptions(ctx context.Context, userID string) ([]store.NotificationSubscription, error)
	SetSubscriptions(ctx context.Context, userID string, subs []store.NotificationSubscription) error
}

// NotificationHandler handles REST endpoints for notification channel CRUD
// and per-user subscription management.
type NotificationHandler struct {
	store     NotificationChannelStore
	notifiers map[string]notify.Notifier
	getClaims ClaimsGetter
}

// NewNotificationHandler creates a new NotificationHandler.
func NewNotificationHandler(s NotificationChannelStore, notifiers map[string]notify.Notifier, getClaims ClaimsGetter) *NotificationHandler {
	return &NotificationHandler{
		store:     s,
		notifiers: notifiers,
		getClaims: getClaims,
	}
}

// RegisterNotificationRoutes mounts all notification-related routes on the given router.
func RegisterNotificationRoutes(r chi.Router, h *NotificationHandler) {
	r.Get("/api/v1/notification-channels", h.ListChannels)
	r.Post("/api/v1/notification-channels", h.CreateChannel)
	r.Get("/api/v1/notification-channels/{channelID}", h.GetChannel)
	r.Put("/api/v1/notification-channels/{channelID}", h.UpdateChannel)
	r.Delete("/api/v1/notification-channels/{channelID}", h.DeleteChannel)
	r.Post("/api/v1/notification-channels/{channelID}/test", h.TestChannel)
	r.Get("/api/v1/notifications/subscriptions", h.GetSubscriptions)
	r.Put("/api/v1/notifications/subscriptions", h.SetSubscriptions)
}

// --- Request types ---

type createChannelRequest struct {
	ChannelType string `json:"channel_type"`
	Name        string `json:"name"`
	Config      string `json:"config"`
}

type updateChannelRequest struct {
	Name    string `json:"name"`
	Config  string `json:"config"`
	Enabled *bool  `json:"enabled"`
}

type subscriptionRequest struct {
	Subscriptions []subscriptionEntry `json:"subscriptions"`
}

type subscriptionEntry struct {
	ChannelID string `json:"channel_id"`
	EventType string `json:"event_type"`
}

// redactConfig masks sensitive fields (password, bot_token) in a JSON channel
// config string before returning it in API responses.
func redactConfig(config string) string {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(config), &parsed); err != nil {
		return "{}"
	}
	for _, key := range []string{"password", "bot_token"} {
		if _, ok := parsed[key]; ok {
			parsed[key] = "***"
		}
	}
	redacted, _ := json.Marshal(parsed)
	return string(redacted)
}

// mergeRedactedConfig replaces sentinel "***" values in incomingConfig with the
// corresponding values from existingConfig. This lets the frontend omit secrets
// it received as "***" without accidentally clearing them.
func mergeRedactedConfig(existingConfig, incomingConfig string) string {
	var existing map[string]any
	if err := json.Unmarshal([]byte(existingConfig), &existing); err != nil {
		return incomingConfig
	}
	var incoming map[string]any
	if err := json.Unmarshal([]byte(incomingConfig), &incoming); err != nil {
		return incomingConfig
	}
	for _, key := range []string{"password", "bot_token"} {
		if val, ok := incoming[key]; ok {
			if s, isStr := val.(string); isStr && s == "***" {
				if orig, hasOrig := existing[key]; hasOrig {
					incoming[key] = orig
				}
			}
		}
	}
	merged, err := json.Marshal(incoming)
	if err != nil {
		return incomingConfig
	}
	return string(merged)
}

// requireAdmin checks that the request comes from an admin user. It writes an
// error response and returns false if the caller is not an admin.
func (h *NotificationHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.getClaims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	c := h.getClaims(r)
	if c == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	if c.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}

// --- Handlers ---

// ListChannels handles GET /api/v1/notification-channels.
func (h *NotificationHandler) ListChannels(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	channels, err := h.store.ListNotificationChannels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if channels == nil {
		channels = []store.NotificationChannel{}
	}
	redacted := make([]store.NotificationChannel, len(channels))
	for i, ch := range channels {
		ch.Config = redactConfig(ch.Config)
		redacted[i] = ch
	}
	writeJSON(w, http.StatusOK, redacted)
}

// CreateChannel handles POST /api/v1/notification-channels.
func (h *NotificationHandler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	claims := h.getClaims(r)

	var req createChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.ChannelType != "email" && req.ChannelType != "telegram" {
		writeError(w, http.StatusBadRequest, "channel_type must be 'email' or 'telegram'")
		return
	}
	if req.Config == "" {
		writeError(w, http.StatusBadRequest, "config is required")
		return
	}

	// Validate config is valid JSON
	if !json.Valid([]byte(req.Config)) {
		writeError(w, http.StatusBadRequest, "config must be valid JSON")
		return
	}

	ch := store.NotificationChannel{
		ChannelType: req.ChannelType,
		Name:        req.Name,
		Config:      req.Config,
		Enabled:     true,
		CreatedBy:   claims.UserID,
	}

	created, err := h.store.CreateNotificationChannel(r.Context(), ch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	created.Config = redactConfig(created.Config)
	writeJSON(w, http.StatusCreated, created)
}

// GetChannel handles GET /api/v1/notification-channels/{channelID}.
func (h *NotificationHandler) GetChannel(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "channelID")

	ch, err := h.store.GetNotificationChannel(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	ch.Config = redactConfig(ch.Config)
	writeJSON(w, http.StatusOK, ch)
}

// UpdateChannel handles PUT /api/v1/notification-channels/{channelID}.
func (h *NotificationHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "channelID")

	existing, err := h.store.GetNotificationChannel(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req updateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Config == "" {
		writeError(w, http.StatusBadRequest, "config is required")
		return
	}
	if !json.Valid([]byte(req.Config)) {
		writeError(w, http.StatusBadRequest, "config must be valid JSON")
		return
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// If the frontend sends "***" for any sensitive field, preserve the
	// existing value from the store rather than overwriting with the sentinel.
	config := req.Config
	config = mergeRedactedConfig(existing.Config, config)

	updated, err := h.store.UpdateNotificationChannel(r.Context(), store.NotificationChannel{
		ChannelID: id,
		Name:      req.Name,
		Config:    config,
		Enabled:   enabled,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	updated.Config = redactConfig(updated.Config)
	writeJSON(w, http.StatusOK, updated)
}

// DeleteChannel handles DELETE /api/v1/notification-channels/{channelID}.
func (h *NotificationHandler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "channelID")

	if err := h.store.DeleteNotificationChannel(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestChannel handles POST /api/v1/notification-channels/{channelID}/test.
func (h *NotificationHandler) TestChannel(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "channelID")

	ch, err := h.store.GetNotificationChannel(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	notifier, ok := h.notifiers[ch.ChannelType]
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported channel type: "+ch.ChannelType)
		return
	}

	subject := "claude-plane Test Notification"
	body := "This is a test notification from claude-plane to verify your channel configuration."

	if err := notifier.Send(r.Context(), ch.Config, subject, body); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
	})
}

// GetSubscriptions handles GET /api/v1/notifications/subscriptions.
func (h *NotificationHandler) GetSubscriptions(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	subs, err := h.store.GetSubscriptions(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if subs == nil {
		subs = []store.NotificationSubscription{}
	}
	writeJSON(w, http.StatusOK, subs)
}

// SetSubscriptions handles PUT /api/v1/notifications/subscriptions.
func (h *NotificationHandler) SetSubscriptions(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req subscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	subs := make([]store.NotificationSubscription, len(req.Subscriptions))
	for i, s := range req.Subscriptions {
		if s.ChannelID == "" || s.EventType == "" {
			writeError(w, http.StatusBadRequest, "channel_id and event_type are required for each subscription")
			return
		}
		subs[i] = store.NotificationSubscription{
			UserID:    claims.UserID,
			ChannelID: s.ChannelID,
			EventType: s.EventType,
		}
	}

	if err := h.store.SetSubscriptions(r.Context(), claims.UserID, subs); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
