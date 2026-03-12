package handler

import "net/http"

// UserClaims holds the minimal user identity for authorization in job/run handlers.
type UserClaims struct {
	UserID string
	Role   string
}

// ClaimsGetter extracts user claims from a request context.
type ClaimsGetter func(r *http.Request) *UserClaims
