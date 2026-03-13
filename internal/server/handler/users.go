package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// UserAdminStore is the persistence interface required by UserHandler.
type UserAdminStore interface {
	ListUsers(ctx context.Context) ([]store.User, error)
	GetUserByID(ctx context.Context, userID string) (*store.User, error)
	CreateUser(user *store.User) error
	UpdateUser(ctx context.Context, userID, displayName, role string) error
	DeleteUser(ctx context.Context, userID string) error
}

// UserHandler handles REST endpoints for user management (admin only).
type UserHandler struct {
	store     UserAdminStore
	getClaims ClaimsGetter
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(s UserAdminStore, getClaims ClaimsGetter) *UserHandler {
	return &UserHandler{store: s, getClaims: getClaims}
}

// RegisterUserRoutes mounts all user-management routes on the given router.
func RegisterUserRoutes(r chi.Router, h *UserHandler) {
	r.Get("/api/v1/users", h.ListUsers)
	r.Post("/api/v1/users", h.CreateUser)
	r.Put("/api/v1/users/{userID}", h.UpdateUser)
	r.Delete("/api/v1/users/{userID}", h.DeleteUser)
}

// requireAdmin checks that the requesting user has the admin role.
// Returns false and writes a 403 response if not.
func (h *UserHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	c := h.getClaims(r)
	if c == nil || c.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}

// ListUsers handles GET /api/v1/users.
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if users == nil {
		users = []store.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

// createUserRequest is the JSON body for POST /api/v1/users.
type createUserRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// CreateUser handles POST /api/v1/users.
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.DisplayName = strings.TrimSpace(req.DisplayName)

	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}
	if req.Role != "admin" && req.Role != "user" {
		writeError(w, http.StatusBadRequest, "role must be 'admin' or 'user'")
		return
	}

	hash, err := store.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	u := &store.User{
		UserID:       uuid.New().String(),
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PasswordHash: hash,
		Role:         req.Role,
	}

	if err := h.store.CreateUser(u); err != nil {
		if store.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a user with that email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Re-fetch to get DB-populated timestamps
	created, err := h.store.GetUserByID(r.Context(), u.UserID)
	if err != nil {
		// Still return the partial record rather than failing entirely
		writeJSON(w, http.StatusCreated, u)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// updateUserRequest is the JSON body for PUT /api/v1/users/{userID}.
type updateUserRequest struct {
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// UpdateUser handles PUT /api/v1/users/{userID}.
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	userID := chi.URLParam(r, "userID")

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.DisplayName = strings.TrimSpace(req.DisplayName)

	if req.Role != "" && req.Role != "admin" && req.Role != "user" {
		writeError(w, http.StatusBadRequest, "role must be 'admin' or 'user'")
		return
	}

	// Fetch existing user to fill defaults for missing fields
	existing, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	displayName := existing.DisplayName
	if req.DisplayName != "" {
		displayName = req.DisplayName
	}
	role := existing.Role
	if req.Role != "" {
		role = req.Role
	}

	if err := h.store.UpdateUser(r.Context(), userID, displayName, role); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteUser handles DELETE /api/v1/users/{userID}.
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	userID := chi.URLParam(r, "userID")

	// Prevent self-deletion
	c := h.getClaims(r)
	if c != nil && c.UserID == userID {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	if err := h.store.DeleteUser(r.Context(), userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
