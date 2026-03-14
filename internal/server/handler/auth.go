package handler

import "net/http"

// UserClaims holds the minimal user identity for authorization in job/run handlers.
type UserClaims struct {
	UserID string
	Role   string
	Scopes []string
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
