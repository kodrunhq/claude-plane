package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// UserAdminStore is the persistence interface required by UserHandler.
type UserAdminStore interface {
	ListUsers(ctx context.Context) ([]store.User, error)
	GetUserByID(ctx context.Context, userID string) (*store.User, error)
	CreateUser(user *store.User) error
	UpdateUser(ctx context.Context, userID, displayName, role string) error
	UpdatePassword(ctx context.Context, userID string, passwordHash string) error
	UpdateDisplayName(ctx context.Context, userID, displayName string) error
	DeleteUser(ctx context.Context, userID string) error
}

// UserHandler handles REST endpoints for user management (admin only).
type UserHandler struct {
	store        UserAdminStore
	getClaims    ClaimsGetter
	publisher    event.Publisher
	tokenRevoker TokenRevoker
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(s UserAdminStore, getClaims ClaimsGetter) *UserHandler {
	return &UserHandler{store: s, getClaims: getClaims}
}

// SetTokenRevoker configures the token revoker for invalidating JWTs after password changes.
func (h *UserHandler) SetTokenRevoker(tr TokenRevoker) {
	h.tokenRevoker = tr
}

// SetPublisher configures the event publisher for user lifecycle events.
func (h *UserHandler) SetPublisher(p event.Publisher) {
	h.publisher = p
}

// publishUserEvent fires a user lifecycle event if a publisher is configured.
// Errors are logged but not propagated.
func (h *UserHandler) publishUserEvent(eventType, userID, email string) {
	if h.publisher == nil {
		return
	}
	evt := event.NewUserEvent(eventType, userID, email)
	if err := h.publisher.Publish(context.Background(), evt); err != nil {
		slog.Warn("failed to publish user event", "type", eventType, "user_id", userID, "error", err)
	}
}

// validateNewPassword checks that a new password meets minimum requirements.
func validateNewPassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("new password must be at least 8 characters")
	}
	return nil
}

// RegisterUserRoutes mounts all user-management routes on the given router.
// IMPORTANT: /users/me/* routes are registered before /{userID} routes so
// Chi does not match "me" as a userID parameter.
func RegisterUserRoutes(r chi.Router, h *UserHandler) {
	r.Post("/api/v1/users/me/password", h.ChangePassword)
	r.Put("/api/v1/users/me", h.UpdateProfile)
	r.Get("/api/v1/users", h.ListUsers)
	r.Post("/api/v1/users", h.CreateUser)
	r.Post("/api/v1/users/{userID}/reset-password", h.ResetPassword)
	r.Put("/api/v1/users/{userID}", h.UpdateUser)
	r.Delete("/api/v1/users/{userID}", h.DeleteUser)
}

// requireAdminAccess delegates to the package-level requireAdmin helper.
func (h *UserHandler) requireAdminAccess(w http.ResponseWriter, r *http.Request) bool {
	return requireAdmin(w, r, h.getClaims)
}

// ListUsers handles GET /api/v1/users.
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminAccess(w, r) {
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
	if !h.requireAdminAccess(w, r) {
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
	if len(req.DisplayName) > 255 {
		writeError(w, http.StatusBadRequest, "display_name must be at most 255 characters")
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
		h.publishUserEvent(event.TypeUserCreated, u.UserID, u.Email)
		return
	}
	writeJSON(w, http.StatusCreated, created)
	h.publishUserEvent(event.TypeUserCreated, created.UserID, created.Email)
}

// updateUserRequest is the JSON body for PUT /api/v1/users/{userID}.
type updateUserRequest struct {
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// UpdateUser handles PUT /api/v1/users/{userID}.
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminAccess(w, r) {
		return
	}

	userID := chi.URLParam(r, "userID")

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.DisplayName = strings.TrimSpace(req.DisplayName)

	if len(req.DisplayName) > 255 {
		writeError(w, http.StatusBadRequest, "display_name must be at most 255 characters")
		return
	}

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

// changePasswordRequest is the JSON body for POST /api/v1/users/me/password.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword handles POST /api/v1/users/me/password (self-service password change).
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	c := h.getClaims(r)
	if c == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CurrentPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password is required")
		return
	}
	if req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "new_password is required")
		return
	}
	if err := validateNewPassword(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.store.GetUserByID(r.Context(), c.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	ok, err := store.VerifyPassword(req.CurrentPassword, user.PasswordHash)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	hash, err := store.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.store.UpdatePassword(r.Context(), c.UserID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Revoke the current JWT so the user must re-authenticate with the new password.
	if h.tokenRevoker != nil && c.JTI != "" {
		if err := h.tokenRevoker.RevokeToken(c.JTI, c.UserID, c.ExpiresAt); err != nil {
			slog.Warn("failed to revoke token after password change", "user_id", c.UserID, "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "re_login": true})
}

// resetPasswordRequest is the JSON body for POST /api/v1/users/{userID}/reset-password.
type resetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// ResetPassword handles POST /api/v1/users/{userID}/reset-password (admin-only).
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminAccess(w, r) {
		return
	}

	userID := chi.URLParam(r, "userID")

	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "new_password is required")
		return
	}

	// Verify user exists and check admin-to-admin protection.
	targetUser, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	c := h.getClaims(r)
	if targetUser.Role == "admin" && c != nil && c.UserID != userID {
		writeError(w, http.StatusForbidden, "cannot reset another admin's password")
		return
	}

	if err := validateNewPassword(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := store.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.store.UpdatePassword(r.Context(), userID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Note: we cannot revoke the target user's existing tokens here because we
	// don't have their JTI. Their tokens will expire naturally per the configured TTL.
	slog.Warn("admin reset password — target user's existing tokens cannot be revoked",
		"admin_id", c.UserID, "target_user_id", userID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// updateProfileRequest is the JSON body for PUT /api/v1/users/me.
type updateProfileRequest struct {
	DisplayName string `json:"display_name"`
}

// UpdateProfile handles PUT /api/v1/users/me (self-service profile edit).
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	c := h.getClaims(r)
	if c == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.DisplayName = strings.TrimSpace(req.DisplayName)

	if len(req.DisplayName) > 255 {
		writeError(w, http.StatusBadRequest, "display_name must be at most 255 characters")
		return
	}

	if err := h.store.UpdateDisplayName(r.Context(), c.UserID, req.DisplayName); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := h.store.GetUserByID(r.Context(), c.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteUser handles DELETE /api/v1/users/{userID}.
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminAccess(w, r) {
		return
	}

	userID := chi.URLParam(r, "userID")

	// Prevent self-deletion
	c := h.getClaims(r)
	if c != nil && c.UserID == userID {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	// Fetch before deleting so the audit event has the email.
	existing, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
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
	h.publishUserEvent(event.TypeUserDeleted, userID, existing.Email)
	w.WriteHeader(http.StatusNoContent)
}
