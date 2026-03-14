package httputil

import (
	"context"
	"net/http"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// apiKeyAuthKey flags that the request was authenticated via API key (cpk_ prefix).
const apiKeyAuthKey contextKey = "api_key_auth"

// SetAPIKeyAuth returns a new context flagged as API-key-authenticated.
// Called by JWTAuthMiddleware when API key validation succeeds.
func SetAPIKeyAuth(ctx context.Context) context.Context {
	return context.WithValue(ctx, apiKeyAuthKey, true)
}

// IsAPIKeyAuth reports whether the request was authenticated via API key (cpk_ prefix).
// This is more reliable than re-inspecting the Authorization header, which could be
// spoofed when cookie-based JWT auth takes precedence.
func IsAPIKeyAuth(r *http.Request) bool {
	v, _ := r.Context().Value(apiKeyAuthKey).(bool)
	return v
}
