package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// WebhookCRUDStore is the persistence interface required by WebhookHandler.
type WebhookCRUDStore interface {
	CreateWebhook(ctx context.Context, w store.Webhook) (*store.Webhook, error)
	GetWebhook(ctx context.Context, id string) (*store.Webhook, error)
	ListWebhooks(ctx context.Context) ([]store.Webhook, error)
	UpdateWebhook(ctx context.Context, w store.Webhook) (*store.Webhook, error)
	DeleteWebhook(ctx context.Context, id string) error
	ListDeliveries(ctx context.Context, webhookID string) ([]store.WebhookDelivery, error)
}

// WebhookHandler handles REST endpoints for webhook CRUD and delivery queries.
type WebhookHandler struct {
	store WebhookCRUDStore
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(store WebhookCRUDStore) *WebhookHandler {
	return &WebhookHandler{store: store}
}

// RegisterWebhookRoutes mounts all webhook-related routes on the given router.
func RegisterWebhookRoutes(r chi.Router, h *WebhookHandler) {
	r.Get("/api/v1/webhooks", h.ListWebhooks)
	r.Post("/api/v1/webhooks", h.CreateWebhook)
	r.Get("/api/v1/webhooks/{webhookID}", h.GetWebhook)
	r.Put("/api/v1/webhooks/{webhookID}", h.UpdateWebhook)
	r.Delete("/api/v1/webhooks/{webhookID}", h.DeleteWebhook)
	r.Get("/api/v1/webhooks/{webhookID}/deliveries", h.ListDeliveries)
}

// createWebhookRequest is the JSON body for POST /api/v1/webhooks.
type createWebhookRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

// updateWebhookRequest is the JSON body for PUT /api/v1/webhooks/{id}.
// Secret is a *string: nil (or absent) preserves existing, "" clears, non-empty replaces.
type updateWebhookRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  *string  `json:"secret"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

// ListWebhooks handles GET /api/v1/webhooks.
func (h *WebhookHandler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	webhooks, err := h.store.ListWebhooks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if webhooks == nil {
		webhooks = []store.Webhook{}
	}
	writeJSON(w, http.StatusOK, webhooks)
}

// CreateWebhook handles POST /api/v1/webhooks.
func (h *WebhookHandler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	var req createWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	if parsed, err := url.Parse(req.URL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "url must be a valid http or https URL")
		return
	}
	if len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "events is required")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	webhook := store.Webhook{
		Name:    req.Name,
		URL:     req.URL,
		Secret:  []byte(req.Secret),
		Events:  req.Events,
		Enabled: enabled,
	}

	created, err := h.store.CreateWebhook(r.Context(), webhook)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// GetWebhook handles GET /api/v1/webhooks/{webhookID}.
func (h *WebhookHandler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "webhookID")

	webhook, err := h.store.GetWebhook(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, webhook)
}

// UpdateWebhook handles PUT /api/v1/webhooks/{webhookID}.
func (h *WebhookHandler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "webhookID")

	// Verify the webhook exists before decoding body.
	existing, err := h.store.GetWebhook(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req updateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	if parsed, err := url.Parse(req.URL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "url must be a valid http or https URL")
		return
	}
	if len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "events is required")
		return
	}

	// Secret: nil = preserve existing, "" = clear, non-empty = replace.
	secret := existing.Secret
	if req.Secret != nil {
		secret = []byte(*req.Secret)
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	updated, err := h.store.UpdateWebhook(r.Context(), store.Webhook{
		WebhookID: id,
		Name:      req.Name,
		URL:       req.URL,
		Secret:    secret,
		Events:    req.Events,
		Enabled:   enabled,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteWebhook handles DELETE /api/v1/webhooks/{webhookID}.
func (h *WebhookHandler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "webhookID")

	if err := h.store.DeleteWebhook(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListDeliveries handles GET /api/v1/webhooks/{webhookID}/deliveries.
func (h *WebhookHandler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "webhookID")

	// Verify the webhook exists first so we return 404 rather than an empty list.
	if _, err := h.store.GetWebhook(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	deliveries, err := h.store.ListDeliveries(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if deliveries == nil {
		deliveries = []store.WebhookDelivery{}
	}
	writeJSON(w, http.StatusOK, deliveries)
}
