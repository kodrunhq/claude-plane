package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// CredentialStore is the persistence interface required by CredentialHandler.
type CredentialStore interface {
	CreateCredential(ctx context.Context, userID, name string, value []byte, encryptionKey []byte) (*store.Credential, error)
	ListCredentialsByUser(ctx context.Context, userID string) ([]store.Credential, error)
	GetCredential(ctx context.Context, credentialID string) (*store.Credential, error)
	DeleteCredential(ctx context.Context, credentialID string) error
}

// CredentialHandler handles REST endpoints for the credentials vault.
type CredentialHandler struct {
	store         CredentialStore
	getClaims     ClaimsGetter
	encryptionKey []byte
}

// NewCredentialHandler creates a new CredentialHandler.
func NewCredentialHandler(s CredentialStore, getClaims ClaimsGetter, encryptionKey []byte) *CredentialHandler {
	return &CredentialHandler{
		store:         s,
		getClaims:     getClaims,
		encryptionKey: encryptionKey,
	}
}

// RegisterCredentialRoutes mounts all credential routes on the given router.
func RegisterCredentialRoutes(r chi.Router, h *CredentialHandler) {
	r.Get("/api/v1/credentials", h.ListCredentials)
	r.Post("/api/v1/credentials", h.CreateCredential)
	r.Delete("/api/v1/credentials/{credentialID}", h.DeleteCredential)
}

// createCredentialRequest is the JSON body for POST /api/v1/credentials.
type createCredentialRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// credentialResponse is a safe view of a Credential — value fields are never included.
type credentialResponse struct {
	CredentialID string `json:"credential_id"`
	UserID       string `json:"user_id"`
	Name         string `json:"name"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func toCredentialResponse(c store.Credential) credentialResponse {
	return credentialResponse{
		CredentialID: c.CredentialID,
		UserID:       c.UserID,
		Name:         c.Name,
		CreatedAt:    c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    c.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// ListCredentials handles GET /api/v1/credentials.
// Returns only the current user's credentials (from JWT claims).
func (h *CredentialHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	c := h.getClaims(r)
	if c == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	credentials, err := h.store.ListCredentialsByUser(r.Context(), c.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]credentialResponse, len(credentials))
	for i, cred := range credentials {
		resp[i] = toCredentialResponse(cred)
	}
	writeJSON(w, http.StatusOK, resp)
}

// CreateCredential handles POST /api/v1/credentials.
func (h *CredentialHandler) CreateCredential(w http.ResponseWriter, r *http.Request) {
	c := h.getClaims(r)
	if c == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Value = strings.TrimSpace(req.Value)

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}

	created, err := h.store.CreateCredential(r.Context(), c.UserID, req.Name, []byte(req.Value), h.encryptionKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toCredentialResponse(*created))
}

// DeleteCredential handles DELETE /api/v1/credentials/{credentialID}.
// Only the owning user may delete their own credentials.
func (h *CredentialHandler) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	c := h.getClaims(r)
	if c == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	credentialID := chi.URLParam(r, "credentialID")

	// Verify ownership before deleting.
	existing, err := h.store.GetCredential(r.Context(), credentialID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "credential not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if existing.UserID != c.UserID && c.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := h.store.DeleteCredential(r.Context(), credentialID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "credential not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
