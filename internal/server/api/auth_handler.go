package api

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// sessionCookieName is the name of the httpOnly cookie used for JWT auth.
const sessionCookieName = "session_token"

func isSecureRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// setSessionCookie sets an httpOnly, SameSite=Strict cookie with the JWT.
func (h *Handlers) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.authSvc.TokenTTL().Seconds()),
	})
}

// clearSessionCookie removes the session cookie by setting MaxAge=-1.
func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// registerRequest is the JSON body for POST /api/v1/auth/register.
type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	InviteCode  string `json:"invite_code,omitempty"`
}

// loginRequest is the JSON body for POST /api/v1/auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register handles POST /api/v1/auth/register.
// Creates a new user account with hashed password.
// Respects the registration_mode setting: "open" allows anyone, "invite" requires
// a valid invite code, "closed" (default) rejects all self-registration.
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	switch h.registrationMode {
	case "closed":
		writeError(w, http.StatusForbidden, "registration is closed")
		return
	case "invite":
		// Parse body first, then check invite code below after decoding
	case "open":
		// Allow registration
	default:
		writeError(w, http.StatusForbidden, "registration is closed")
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Check invite code when in invite mode
	if h.registrationMode == "invite" {
		if req.InviteCode == "" || req.InviteCode != h.inviteCode {
			writeError(w, http.StatusForbidden, "invalid invite code")
			return
		}
	}

	// Validate input
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len(req.Password) > 128 {
		writeError(w, http.StatusBadRequest, "password must be at most 128 characters")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, "invalid email format")
		return
	}

	// Hash password
	hash, err := store.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user := &store.User{
		UserID:       uuid.New().String(),
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PasswordHash: hash,
		Role:         "user",
	}

	if err := h.store.CreateUser(user); err != nil {
		if store.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"user_id": user.UserID,
		"email":   user.Email,
	})
}

// Login handles POST /api/v1/auth/login.
// Authenticates a user and returns a JWT token.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Password) > 128 {
		writeError(w, http.StatusBadRequest, "password must be at most 128 characters")
		return
	}

	user, err := h.store.GetUserByEmail(req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		// Perform dummy hash to prevent timing oracle for user enumeration
		_, _ = store.HashPassword(req.Password)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	valid, err := store.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !valid {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.authSvc.IssueToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Set httpOnly cookie with the JWT (primary auth mechanism).
	// The token is also returned in the JSON response for backwards compatibility.
	h.setSessionCookie(w, r, token)

	writeJSON(w, http.StatusOK, map[string]string{
		"token":   token,
		"user_id": user.UserID,
		"email":   user.Email,
		"role":    user.Role,
	})
}

// Logout handles POST /api/v1/auth/logout.
// Revokes the current JWT token.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "missing claims")
		return
	}

	if err := h.authSvc.RevokeToken(claims.ID, claims.UserID, claims.ExpiresAt.Time); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	clearSessionCookie(w, r)

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "logged out",
	})
}
