package handler

import (
	"net/http"
	"time"
)

// UserClaims holds the minimal user identity for authorization in job/run handlers.
type UserClaims struct {
	UserID    string
	Role      string
	Scopes    []string
	JTI       string    // JWT ID for token revocation
	ExpiresAt time.Time // Token expiry for revocation records
}

// TokenRevoker revokes JWT tokens (e.g. after password change).
type TokenRevoker interface {
	RevokeToken(jti, userID string, expiresAt time.Time) error
}

// HasScope reports whether the claims include the given scope.
func (c *UserClaims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// ClaimsGetter extracts user claims from a request context.
type ClaimsGetter func(r *http.Request) *UserClaims
