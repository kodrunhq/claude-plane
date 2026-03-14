// Package auth provides JWT authentication services for the claude-plane server.
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// SessionCookieName is the name of the httpOnly cookie used for JWT auth.
// Shared across api and session packages to prevent accidental drift.
const SessionCookieName = "session_token"

// Claims represents the JWT claims issued by the claude-plane server.
type Claims struct {
	jwt.RegisteredClaims
	UserID string   `json:"uid"`
	Email  string   `json:"email"`
	Role   string   `json:"role"`
	Scopes []string `json:"scopes,omitempty"`
}

// HasScope reports whether the claims include the given scope.
func (c *Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// TokenRevoker defines the interface for checking and adding token revocations.
// This decouples the JWT service from the concrete Blocklist implementation.
type TokenRevoker interface {
	IsRevoked(jti string) bool
	Add(jti, userID string, expiresAt time.Time) error
}

// Service provides JWT token issuance, validation, and revocation.
type Service struct {
	signingKey []byte
	tokenTTL   time.Duration
	blocklist  TokenRevoker
}

// NewService creates a new JWT authentication service.
func NewService(signingKey []byte, tokenTTL time.Duration, blocklist TokenRevoker) *Service {
	return &Service{
		signingKey: signingKey,
		tokenTTL:   tokenTTL,
		blocklist:  blocklist,
	}
}

// TokenTTL returns the configured token time-to-live duration.
func (s *Service) TokenTTL() time.Duration {
	return s.tokenTTL
}

// IssueToken creates a signed HS256 JWT for the given user.
func (s *Service) IssueToken(user *store.User) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			Subject:   user.UserID,
			Issuer:    "claude-plane",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
		},
		UserID: user.UserID,
		Email:  user.Email,
		Role:   user.Role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(s.signingKey)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return tokenStr, nil
}

// ValidateToken parses and validates a JWT token string.
// It enforces HS256-only signing, checks issuer, expiry, and blocklist.
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		return s.signingKey, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer("claude-plane"),
	)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	if s.blocklist != nil && s.blocklist.IsRevoked(claims.ID) {
		return nil, fmt.Errorf("token revoked")
	}

	return claims, nil
}

// RevokeToken adds a token to the blocklist by its JTI.
func (s *Service) RevokeToken(jti, userID string, expiresAt time.Time) error {
	if s.blocklist == nil {
		return fmt.Errorf("no blocklist configured")
	}
	return s.blocklist.Add(jti, userID, expiresAt)
}
