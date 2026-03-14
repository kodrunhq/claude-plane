package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/httputil"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// userClaimsKey is the context key used to store JWT claims on the request context.
const userClaimsKey contextKey = "user_claims"

// apiKeyPrefix is the expected prefix for API key tokens.
const apiKeyPrefix = "cpk_"

// APIKeyStore defines the store operations needed for API key authentication.
type APIKeyStore interface {
	ValidateAPIKey(ctx context.Context, plaintextKey string, signingKey []byte) (*store.APIKey, error)
	UpdateAPIKeyLastUsed(ctx context.Context, keyID string) error
}

// UserStore defines the store operations needed to look up users during API key auth.
type UserStore interface {
	GetUserByID(ctx context.Context, userID string) (*store.User, error)
}

// APIKeyAuth bundles the dependencies required for API key validation.
// All fields must be non-nil when the struct is passed to JWTAuthMiddleware.
type APIKeyAuth struct {
	Store      APIKeyStore
	UserStore  UserStore
	SigningKey []byte
}

// JWTAuthMiddleware returns an HTTP middleware that validates tokens.
// It checks for the token in this order:
//  1. httpOnly cookie named "session_token"
//  2. Authorization: Bearer <token> header (backwards compatibility)
//
// When apiKeyAuth is provided and the resolved token starts with "cpk_",
// the token is validated as an API key via the store instead of as a JWT.
// On success, claims are injected into the request context under userClaimsKey.
//
// The variadic apiKeyAuth parameter preserves backwards compatibility — existing
// callers that only pass authSvc continue to work without modification.
func JWTAuthMiddleware(authSvc *auth.Service, apiKeyAuth ...*APIKeyAuth) func(http.Handler) http.Handler {
	var aka *APIKeyAuth
	if len(apiKeyAuth) > 0 {
		aka = apiKeyAuth[0]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var token string

			// 1. Try cookie first
			if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
				token = cookie.Value
			}

			// 2. Fall back to Authorization header
			if token == "" {
				header := r.Header.Get("Authorization")
				if header == "" {
					writeError(w, http.StatusUnauthorized, "missing authorization")
					return
				}
				parts := strings.SplitN(header, " ", 2)
				if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
					writeError(w, http.StatusUnauthorized, "invalid authorization format")
					return
				}
				token = parts[1]
			}

			// 3. Route to API key validation when token has the cpk_ prefix.
			if strings.HasPrefix(token, apiKeyPrefix) {
				if aka == nil {
					writeError(w, http.StatusUnauthorized, "api key authentication not configured")
					return
				}
				claims := validateAPIKeyToken(w, r, token, aka)
				if claims == nil {
					return
				}
				ctx := context.WithValue(r.Context(), userClaimsKey, claims)
				ctx = httputil.SetAPIKeyAuth(ctx)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 4. Standard JWT path.
			claims, err := authSvc.ValidateToken(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), userClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// validateAPIKeyToken validates a cpk_ prefixed token against the store and
// returns the resulting auth.Claims, or writes a 401 and returns nil on failure.
func validateAPIKeyToken(w http.ResponseWriter, r *http.Request, token string, aka *APIKeyAuth) *auth.Claims {
	apiKey, err := aka.Store.ValidateAPIKey(r.Context(), token, aka.SigningKey)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired api key")
		return nil
	}

	user, err := aka.UserStore.GetUserByID(r.Context(), apiKey.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "api key user not found")
		return nil
	}

	// Fire-and-forget last-used timestamp update — never blocks the request.
	// Bounded context prevents goroutine leaks if the DB is slow/unavailable.
	go func(keyID string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := aka.Store.UpdateAPIKeyLastUsed(ctx, keyID); err != nil {
			slog.Error("update api key last_used_at", "key_id", keyID, "error", err)
		}
	}(apiKey.KeyID)

	return &auth.Claims{
		UserID: user.UserID,
		Email:  user.Email,
		Role:   user.Role,
		Scopes: apiKey.Scopes,
	}
}

// SetClaims returns a new context with the given claims attached.
// This allows external packages to inject claims for testing or
// middleware chaining without accessing the unexported context key.
func SetClaims(ctx context.Context, claims *auth.Claims) context.Context {
	return context.WithValue(ctx, userClaimsKey, claims)
}

// GetClaims extracts JWT claims from the request context.
// Returns nil if no claims are present.
func GetClaims(r *http.Request) *auth.Claims {
	claims, _ := r.Context().Value(userClaimsKey).(*auth.Claims)
	return claims
}

