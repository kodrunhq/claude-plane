package handler

import "net/http"

// requireAdmin checks that the request comes from an admin user.
// It writes an error response and returns false if the caller is not an admin.
func requireAdmin(w http.ResponseWriter, r *http.Request, getClaims ClaimsGetter) bool {
	if getClaims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	c := getClaims(r)
	if c == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	if c.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}
