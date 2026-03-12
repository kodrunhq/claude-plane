package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/claudeplane/claude-plane/internal/server/auth"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// UserClaimsKey is the context key used to store JWT claims on the request context.
const UserClaimsKey contextKey = "user_claims"

// JWTAuthMiddleware returns an HTTP middleware that validates Bearer tokens
// using the provided auth service. On success, the parsed claims are injected
// into the request context under UserClaimsKey.
func JWTAuthMiddleware(authSvc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				writeError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeError(w, http.StatusUnauthorized, "invalid authorization format")
				return
			}

			claims, err := authSvc.ValidateToken(parts[1])
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetClaims extracts JWT claims from the request context.
// Returns nil if no claims are present.
func GetClaims(r *http.Request) *auth.Claims {
	claims, _ := r.Context().Value(UserClaimsKey).(*auth.Claims)
	return claims
}
