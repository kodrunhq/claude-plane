package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
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
		if errors.Is(err, provision.ErrInvalidMachineID) || errors.Is(err, provision.ErrUnsupportedPlatform) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		slog.Error("create provision failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, result)
}

// ListTokens handles GET /api/v1/provision/tokens (admin-only, JWT-protected).
func (h *ProvisionHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)
	if claims == nil || claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	tokens, err := h.store.ListProvisioningTokens(r.Context())
	if err != nil {
		slog.Error("list provisioning tokens failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Return an empty array instead of null when there are no tokens.
	if tokens == nil {
		tokens = []store.ProvisioningTokenSummary{}
	}

	httputil.WriteJSON(w, http.StatusOK, tokens)
}

// RevokeToken handles DELETE /api/v1/provision/tokens/{tokenID} (admin-only, JWT-protected).
func (h *ProvisionHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)
	if claims == nil || claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	tokenID := chi.URLParam(r, "tokenID")
	if tokenID == "" {
		writeError(w, http.StatusBadRequest, "token ID is required")
		return
	}

	if err := h.store.RevokeProvisioningToken(r.Context(), tokenID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		slog.Error("revoke provisioning token failed", "error", err, "token_id", tokenID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
		if errors.Is(err, store.ErrTokenAlreadyRedeemed) {
			writeError(w, http.StatusGone, "token already used")
			return
		}
		if errors.Is(err, store.ErrTokenExpired) {
			writeError(w, http.StatusGone, "token expired")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to redeem token")
		return
	}

	script, err := provision.RenderInstallScript(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to render script")
		return
	}

	w.Header().Set("Content-Type", "text/x-shellscript")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

type joinRequest struct {
	Code string `json:"code"`
}

type joinResponse struct {
	MachineID    string `json:"machine_id"`
	GRPCAddress  string `json:"grpc_address"`
	CACertPEM    string `json:"ca_cert_pem"`
	AgentCertPEM string `json:"agent_cert_pem"`
	AgentKeyPEM  string `json:"agent_key_pem"`
}

// JoinByCode handles POST /api/v1/provision/join (public, no JWT).
// Redeems a provisioning token by its 6-character short code.
func (h *ProvisionHandler) JoinByCode(w http.ResponseWriter, r *http.Request) {
	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !provision.ValidateShortCode(req.Code) {
		writeError(w, http.StatusBadRequest, "invalid code format: must be 6 characters from ABCDEFGHJKMNPQRSTUVWXYZ23456789")
		return
	}

	token, err := h.store.GetProvisioningTokenByCode(r.Context(), req.Code)
	if err != nil {
		// All error cases (not found, expired, redeemed) return 404
		// to avoid leaking whether a code was ever valid.
		writeError(w, http.StatusNotFound, "invalid or expired code")
		return
	}

	if err := h.store.RedeemProvisioningToken(r.Context(), token.Token); err != nil {
		writeError(w, http.StatusNotFound, "invalid or expired code")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, joinResponse{
		MachineID:    token.MachineID,
		GRPCAddress:  token.GRPCAddress,
		CACertPEM:    token.CACertPEM,
		AgentCertPEM: token.AgentCertPEM,
		AgentKeyPEM:  token.AgentKeyPEM,
	})
}

// RegisterProvisionRoutes registers the JWT-protected provisioning routes.
func RegisterProvisionRoutes(r chi.Router, h *ProvisionHandler) {
	r.Post("/api/v1/provision/agent", h.CreateProvision)
	r.Get("/api/v1/provision/tokens", h.ListTokens)
	r.Delete("/api/v1/provision/tokens/{tokenID}", h.RevokeToken)
}

// RegisterProvisionPublicRoutes registers the public (token-authenticated) provisioning routes.
func RegisterProvisionPublicRoutes(r chi.Router, h *ProvisionHandler) {
	r.Get("/api/v1/provision/{token}/script", h.ServeScript)
	r.Post("/api/v1/provision/join", h.JoinByCode)
}
