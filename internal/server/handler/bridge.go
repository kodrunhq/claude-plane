package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/httputil"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

const bridgeRestartKey = "restart_requested_at"

// BridgeStore is the persistence interface required by BridgeHandler.
type BridgeStore interface {
	CreateConnector(ctx context.Context, c *store.BridgeConnector, secretJSON []byte, encKey []byte) (*store.BridgeConnector, error)
	GetConnector(ctx context.Context, connectorID string) (*store.BridgeConnector, error)
	GetConnectorWithSecret(ctx context.Context, connectorID string, encKey []byte) (*store.BridgeConnector, []byte, error)
	ListConnectors(ctx context.Context) ([]store.BridgeConnector, error)
	UpdateConnector(ctx context.Context, connectorID string, c *store.BridgeConnector, secretJSON []byte, encKey []byte) (*store.BridgeConnector, error)
	DeleteConnector(ctx context.Context, connectorID string) error
	SetBridgeControl(ctx context.Context, key, value string) error
	GetBridgeControl(ctx context.Context, key string) (string, error)
}

// NotifStore is the persistence interface for notification channel auto-sync.
type NotifStore interface {
	CreateNotificationChannel(ctx context.Context, ch store.NotificationChannel) (*store.NotificationChannel, error)
	UpdateNotificationChannel(ctx context.Context, ch store.NotificationChannel) (*store.NotificationChannel, error)
	DeleteNotificationChannel(ctx context.Context, channelID string) error
	GetChannelByConnectorID(ctx context.Context, connectorID string) (*store.NotificationChannel, error)
}

// BridgeHandler handles REST endpoints for bridge connectors and control signals.
type BridgeHandler struct {
	store      BridgeStore
	notifStore NotifStore
	getClaims  ClaimsGetter
	encKey     []byte // AES-256 encryption key for connector secrets
}

// NewBridgeHandler creates a new BridgeHandler.
func NewBridgeHandler(s BridgeStore, getClaims ClaimsGetter, encKey []byte, notifStore NotifStore) *BridgeHandler {
	return &BridgeHandler{
		store:      s,
		notifStore: notifStore,
		getClaims:  getClaims,
		encKey:     encKey,
	}
}

// RegisterBridgeRoutes mounts all bridge routes on the given router.
func RegisterBridgeRoutes(r chi.Router, h *BridgeHandler) {
	r.Post("/api/v1/bridge/connectors", h.CreateConnector)
	r.Get("/api/v1/bridge/connectors", h.ListConnectors)
	r.Get("/api/v1/bridge/connectors/{connectorID}", h.GetConnector)
	r.Put("/api/v1/bridge/connectors/{connectorID}", h.UpdateConnector)
	r.Delete("/api/v1/bridge/connectors/{connectorID}", h.DeleteConnector)
	r.Post("/api/v1/bridge/restart", h.Restart)
	r.Get("/api/v1/bridge/status", h.Status)
}

// connectorRequest is the JSON body for create and update connector endpoints.
// Enabled and ConfigSecret use pointer types so that callers can distinguish
// between "omitted" and "explicitly set to the zero value".
type connectorRequest struct {
	ConnectorType string  `json:"connector_type"`
	Name          string  `json:"name"`
	Config        string  `json:"config"`
	ConfigSecret  *string `json:"config_secret,omitempty"`
	Enabled       *bool   `json:"enabled,omitempty"`
}

// authorizeAdmin checks that the request was made by an admin user.
// Returns false and writes a 403 response when the caller is not an admin.
func (h *BridgeHandler) authorizeAdmin(w http.ResponseWriter, r *http.Request) bool {
	c := h.getClaims(r)
	if c == nil || c.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}

// connectorResponse is a safe view of a BridgeConnector without encrypted fields.
// When ConfigSecret is populated (API key auth path), the decrypted JSON is included.
type connectorResponse struct {
	ConnectorID   string    `json:"connector_id"`
	ConnectorType string    `json:"connector_type"`
	Name          string    `json:"name"`
	Enabled       bool      `json:"enabled"`
	Config        string    `json:"config"`
	ConfigSecret  string    `json:"config_secret,omitempty"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func toConnectorResponse(c *store.BridgeConnector, secretJSON []byte) connectorResponse {
	resp := connectorResponse{
		ConnectorID:   c.ConnectorID,
		ConnectorType: c.ConnectorType,
		Name:          c.Name,
		Enabled:       c.Enabled,
		Config:        c.Config,
		CreatedBy:     c.CreatedBy,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
	if len(secretJSON) > 0 {
		resp.ConfigSecret = string(secretJSON)
	}
	return resp
}

// buildTelegramChannelConfig parses a connector's Config JSON to extract
// group_id and events_topic_id, then builds the notification channel config.
func buildTelegramChannelConfig(connectorConfig string) (string, error) {
	var publicCfg struct {
		GroupID       int64 `json:"group_id"`
		EventsTopicID int   `json:"events_topic_id"`
	}
	if connectorConfig != "" {
		if err := json.Unmarshal([]byte(connectorConfig), &publicCfg); err != nil {
			return "", err
		}
	}

	chCfg := map[string]any{
		"chat_id": strconv.FormatInt(publicCfg.GroupID, 10),
	}
	if publicCfg.EventsTopicID > 0 {
		chCfg["topic_id"] = publicCfg.EventsTopicID
	}

	out, err := json.Marshal(chCfg)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// syncNotifChannelCreate auto-creates a linked notification channel for a
// Telegram connector. Errors are logged but do not fail the connector creation.
func (h *BridgeHandler) syncNotifChannelCreate(ctx context.Context, connector *store.BridgeConnector, userID string) {
	if h.notifStore == nil || connector.ConnectorType != "telegram" {
		return
	}

	chConfig, err := buildTelegramChannelConfig(connector.Config)
	if err != nil {
		slog.Error("auto-sync: build telegram channel config", "error", err, "connector_id", connector.ConnectorID)
		return
	}

	ch := store.NotificationChannel{
		ChannelType: "telegram",
		Name:        connector.Name,
		Config:      chConfig,
		Enabled:     true,
		ConnectorID: &connector.ConnectorID,
		CreatedBy:   userID,
	}

	if _, err := h.notifStore.CreateNotificationChannel(ctx, ch); err != nil {
		slog.Error("auto-sync: create notification channel for connector", "error", err, "connector_id", connector.ConnectorID)
	}
}

// syncNotifChannelUpdate syncs a linked notification channel when a Telegram
// connector is updated. Errors are logged but do not fail the update.
func (h *BridgeHandler) syncNotifChannelUpdate(ctx context.Context, connectorID, connectorType, name, config string) {
	if h.notifStore == nil || connectorType != "telegram" {
		return
	}

	existing, err := h.notifStore.GetChannelByConnectorID(ctx, connectorID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Error("auto-sync: lookup channel for connector", "error", err, "connector_id", connectorID)
		}
		return
	}

	chConfig, err := buildTelegramChannelConfig(config)
	if err != nil {
		slog.Error("auto-sync: build telegram channel config on update", "error", err, "connector_id", connectorID)
		return
	}

	updated := store.NotificationChannel{
		ChannelID:   existing.ChannelID,
		ChannelType: existing.ChannelType,
		Name:        name,
		Config:      chConfig,
		Enabled:     existing.Enabled,
		ConnectorID: existing.ConnectorID,
		CreatedBy:   existing.CreatedBy,
	}

	if _, err := h.notifStore.UpdateNotificationChannel(ctx, updated); err != nil {
		slog.Error("auto-sync: update notification channel for connector", "error", err, "connector_id", connectorID)
	}
}

// syncNotifChannelDelete removes the linked notification channel when a
// connector is deleted. Errors are logged but do not fail the deletion.
func (h *BridgeHandler) syncNotifChannelDelete(ctx context.Context, connectorID string) {
	if h.notifStore == nil {
		return
	}

	existing, err := h.notifStore.GetChannelByConnectorID(ctx, connectorID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Error("auto-sync: lookup channel for connector on delete", "error", err, "connector_id", connectorID)
		}
		return
	}

	if err := h.notifStore.DeleteNotificationChannel(ctx, existing.ChannelID); err != nil {
		slog.Error("auto-sync: delete notification channel for connector", "error", err, "connector_id", connectorID)
	}
}

// CreateConnector handles POST /api/v1/bridge/connectors.
func (h *BridgeHandler) CreateConnector(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}
	c := h.getClaims(r)

	var req connectorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.ConnectorType = strings.TrimSpace(req.ConnectorType)
	req.Name = strings.TrimSpace(req.Name)

	if req.ConnectorType == "" {
		writeError(w, http.StatusBadRequest, "connector_type is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	var secretJSON []byte
	if req.ConfigSecret != nil && *req.ConfigSecret != "" {
		secretJSON = []byte(*req.ConfigSecret)
	}

	connector := &store.BridgeConnector{
		ConnectorType: req.ConnectorType,
		Name:          req.Name,
		Enabled:       enabled,
		Config:        req.Config,
		CreatedBy:     c.UserID,
	}

	created, err := h.store.CreateConnector(r.Context(), connector, secretJSON, h.encKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.syncNotifChannelCreate(r.Context(), created, c.UserID)

	writeJSON(w, http.StatusCreated, toConnectorResponse(created, nil))
}

// ListConnectors handles GET /api/v1/bridge/connectors.
// API key auth (cpk_ prefix) receives decrypted secrets; JWT auth does not.
func (h *BridgeHandler) ListConnectors(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}

	connectors, err := h.store.ListConnectors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	c := h.getClaims(r)
	withSecrets := httputil.IsAPIKeyAuth(r) && c != nil && c.HasScope("connectors:read_secret")

	resp := make([]connectorResponse, 0, len(connectors))
	for i := range connectors {
		c := &connectors[i]
		if withSecrets {
			full, secretJSON, err := h.store.GetConnectorWithSecret(r.Context(), c.ConnectorID, h.encKey)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			resp = append(resp, toConnectorResponse(full, secretJSON))
		} else {
			resp = append(resp, toConnectorResponse(c, nil))
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetConnector handles GET /api/v1/bridge/connectors/{connectorID}.
// API key auth receives decrypted secrets; JWT auth does not.
func (h *BridgeHandler) GetConnector(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}

	connectorID := chi.URLParam(r, "connectorID")

	c := h.getClaims(r)
	if httputil.IsAPIKeyAuth(r) && c != nil && c.HasScope("connectors:read_secret") {
		full, secretJSON, err := h.store.GetConnectorWithSecret(r.Context(), connectorID, h.encKey)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "connector not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, toConnectorResponse(full, secretJSON))
		return
	}

	connector, err := h.store.GetConnector(r.Context(), connectorID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "connector not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toConnectorResponse(connector, nil))
}

// UpdateConnector handles PUT /api/v1/bridge/connectors/{connectorID}.
func (h *BridgeHandler) UpdateConnector(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}

	connectorID := chi.URLParam(r, "connectorID")

	var req connectorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.ConnectorType = strings.TrimSpace(req.ConnectorType)
	req.Name = strings.TrimSpace(req.Name)

	if req.ConnectorType == "" {
		writeError(w, http.StatusBadRequest, "connector_type is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Fetch existing connector to preserve fields that were not sent.
	existing, err := h.store.GetConnector(r.Context(), connectorID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "connector not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// When ConfigSecret is nil (omitted), secretJSON stays nil and the store
	// preserves the existing secret. An empty string explicitly clears it.
	var secretJSON []byte
	if req.ConfigSecret != nil {
		if *req.ConfigSecret != "" {
			secretJSON = []byte(*req.ConfigSecret)
		} else {
			// Explicitly clear the secret (empty byte slice signals "set to NULL").
			secretJSON = []byte{}
		}
	}

	connector := &store.BridgeConnector{
		ConnectorType: req.ConnectorType,
		Name:          req.Name,
		Enabled:       enabled,
		Config:        req.Config,
	}

	updated, err := h.store.UpdateConnector(r.Context(), connectorID, connector, secretJSON, h.encKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "connector not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.syncNotifChannelUpdate(r.Context(), connectorID, updated.ConnectorType, updated.Name, updated.Config)

	writeJSON(w, http.StatusOK, toConnectorResponse(updated, nil))
}

// DeleteConnector handles DELETE /api/v1/bridge/connectors/{connectorID}.
func (h *BridgeHandler) DeleteConnector(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}

	connectorID := chi.URLParam(r, "connectorID")

	// Delete the linked notification channel first (before the connector row
	// disappears and the connector_id FK would be orphaned).
	h.syncNotifChannelDelete(r.Context(), connectorID)

	if err := h.store.DeleteConnector(r.Context(), connectorID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "connector not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Restart handles POST /api/v1/bridge/restart.
// Sets a control signal requesting the bridge binary to restart.
func (h *BridgeHandler) Restart(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}

	val := time.Now().UTC().Format(time.RFC3339)
	if err := h.store.SetBridgeControl(r.Context(), bridgeRestartKey, val); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "restart signal sent"})
}

// Status handles GET /api/v1/bridge/status.
// Returns restart_requested_at from bridge control, or null if never set.
func (h *BridgeHandler) Status(w http.ResponseWriter, r *http.Request) {
	val, err := h.store.GetBridgeControl(r.Context(), bridgeRestartKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"restart_requested_at": nil})
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"restart_requested_at": val})
}
