package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kodrunhq/claude-plane/internal/server/httputil"
	"github.com/kodrunhq/claude-plane/internal/server/provision"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// ProvisionHandler handles REST endpoints for agent provisioning.
type ProvisionHandler struct {
	service   *provision.Service
	store     *store.Store
	getClaims ClaimsGetter
}

// NewProvisionHandler creates a new ProvisionHandler.
func NewProvisionHandler(svc *provision.Service, s *store.Store, getClaims ClaimsGetter) *ProvisionHandler {
	return &ProvisionHandler{service: svc, store: s, getClaims: getClaims}
}

type createProvisionRequest struct {
	MachineID string `json:"machine_id"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// CreateProvision handles POST /api/v1/provision/agent (admin-only, JWT-protected).
func (h *ProvisionHandler) CreateProvision(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)
	if claims == nil || claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	var req createProvisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MachineID == "" {
		writeError(w, http.StatusBadRequest, "machine_id is required")
		return
	}

	result, err := h.service.CreateAgentProvision(r.Context(), req.MachineID, req.OS, req.Arch, claims.UserID, 1*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, result)
}

// ServeScript handles GET /api/v1/provision/{token}/script (token-authenticated, no JWT).
func (h *ProvisionHandler) ServeScript(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "token")

	token, err := h.store.GetProvisioningToken(r.Context(), tokenID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		if errors.Is(err, store.ErrTokenExpired) {
			writeError(w, http.StatusGone, "token expired")
			return
		}
		if errors.Is(err, store.ErrTokenAlreadyRedeemed) {
			writeError(w, http.StatusGone, "token already used")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.store.RedeemProvisioningToken(r.Context(), tokenID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to redeem token")
		return
	}

	script, err := provision.RenderInstallScript(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to render script")
		return
	}

	w.Header().Set("Content-Type", "text/x-shellscript")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

// RegisterProvisionRoutes registers the JWT-protected provisioning route.
func RegisterProvisionRoutes(r chi.Router, h *ProvisionHandler) {
	r.Post("/api/v1/provision/agent", h.CreateProvision)
}

// RegisterProvisionPublicRoutes registers the public (token-authenticated) provisioning routes.
func RegisterProvisionPublicRoutes(r chi.Router, h *ProvisionHandler) {
	r.Get("/api/v1/provision/{token}/script", h.ServeScript)
}
