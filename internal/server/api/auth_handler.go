package api

import "net/http"

// Register handles POST /api/v1/auth/register.
// Creates a new user account with hashed password.
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented")
}

// Login handles POST /api/v1/auth/login.
// Authenticates a user and returns a JWT token.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented")
}

// Logout handles POST /api/v1/auth/logout.
// Revokes the current JWT token.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented")
}
