package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// APIKeyHandler handles REST endpoints for API key CRUD.
type APIKeyHandler struct {
	store      *store.Store
	signingKey []byte
	getClaims  ClaimsGetter
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(s *store.Store, signingKey []byte, getClaims ClaimsGetter) *APIKeyHandler {
	return &APIKeyHandler{
		store:      s,
		signingKey: signingKey,
		getClaims:  getClaims,
	}
}

// RegisterAPIKeyRoutes mounts all API key routes on the given router.
func RegisterAPIKeyRoutes(r chi.Router, h *APIKeyHandler) {
	r.Post("/api/v1/api-keys", h.Create)
	r.Get("/api/v1/api-keys", h.List)
	r.Delete("/api/v1/api-keys/{keyID}", h.Delete)
}

// claims returns the current user's claims, or nil if no getter is configured.
func (h *APIKeyHandler) claims(r *http.Request) *UserClaims {
	if h.getClaims == nil {
		return nil
	}
	return h.getClaims(r)
}

// createAPIKeyRequest is the JSON body for POST /api/v1/api-keys.
type createAPIKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// createAPIKeyResponse is the response body for POST /api/v1/api-keys.
// The plaintext key is included only in this one-time response.
type createAPIKeyResponse struct {
	Key       string     `json:"key"`
	KeyID     string     `json:"key_id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Create handles POST /api/v1/api-keys.
func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	c := h.claims(r)
	userID := ""
	if c != nil {
		userID = c.UserID
	}

	plaintextKey, keyID, err := h.store.CreateAPIKey(r.Context(), userID, req.Name, req.Scopes, req.ExpiresAt, h.signingKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Fetch the created key to get its CreatedAt timestamp.
	created, err := h.store.GetAPIKeyByID(r.Context(), keyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := createAPIKeyResponse{
		Key:       plaintextKey,
		KeyID:     keyID,
		Name:      req.Name,
		Scopes:    req.Scopes,
		ExpiresAt: req.ExpiresAt,
		CreatedAt: created.CreatedAt,
	}
	writeJSON(w, http.StatusCreated, resp)
}

// List handles GET /api/v1/api-keys.
// Admin users see all keys; regular users see only their own.
func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)

	var (
		keys []store.APIKey
		err  error
	)

	if c != nil && c.Role == "admin" {
		keys, err = h.store.ListAllAPIKeys(r.Context())
	} else {
		userID := ""
		if c != nil {
			userID = c.UserID
		}
		keys, err = h.store.ListAPIKeys(r.Context(), userID)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, keys)
}

// Delete handles DELETE /api/v1/api-keys/{keyID}.
// Owner or admin can delete. Non-owner non-admin gets 404.
func (h *APIKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	keyID := chi.URLParam(r, "keyID")

	existing, err := h.store.GetAPIKeyByID(r.Context(), keyID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "api key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	c := h.claims(r)
	isOwner := c != nil && c.UserID == existing.UserID
	isAdmin := c != nil && c.Role == "admin"

	if !isOwner && !isAdmin {
		// Return 404 to avoid leaking that the key exists.
		writeError(w, http.StatusNotFound, "api key not found")
		return
	}

	if err := h.store.DeleteAPIKey(r.Context(), keyID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "api key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
