package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/claudeplane/claude-plane/internal/server/store"
	"github.com/golang-jwt/jwt/v5"
)

// testBlocklist is a minimal Blocklist for testing.
type testBlocklist struct {
	revoked map[string]bool
}

func newTestBlocklist() *testBlocklist {
	return &testBlocklist{revoked: make(map[string]bool)}
}

func (b *testBlocklist) IsRevoked(jti string) bool {
	return b.revoked[jti]
}

func (b *testBlocklist) Add(jti, userID string, expiresAt time.Time) error {
	b.revoked[jti] = true
	return nil
}

func testUser() *store.User {
	return &store.User{
		UserID:      "user-123",
		Email:       "admin@test.com",
		DisplayName: "Test Admin",
		Role:        "admin",
	}
}

func TestIssueToken(t *testing.T) {
	svc := NewService([]byte("test-secret-key-32bytes-long!!!!"), 15*time.Minute, newTestBlocklist())
	user := testUser()

	tokenStr, err := svc.IssueToken(user)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	if tokenStr == "" {
		t.Fatal("IssueToken returned empty string")
	}

	// Parse and verify claims structure
	claims, err := svc.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if claims.UserID != user.UserID {
		t.Errorf("UserID = %q, want %q", claims.UserID, user.UserID)
	}
	if claims.Email != user.Email {
		t.Errorf("Email = %q, want %q", claims.Email, user.Email)
	}
	if claims.Role != user.Role {
		t.Errorf("Role = %q, want %q", claims.Role, user.Role)
	}
	if claims.Issuer != "claude-plane" {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, "claude-plane")
	}
	if claims.Subject != user.UserID {
		t.Errorf("Subject = %q, want %q", claims.Subject, user.UserID)
	}
	if claims.ID == "" {
		t.Error("JTI (ID) should not be empty")
	}
}

func TestValidateToken(t *testing.T) {
	svc := NewService([]byte("test-secret-key-32bytes-long!!!!"), 15*time.Minute, newTestBlocklist())

	tokenStr, err := svc.IssueToken(testUser())
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	claims, err := svc.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-123")
	}
}

func TestValidateExpired(t *testing.T) {
	svc := NewService([]byte("test-secret-key-32bytes-long!!!!"), 1*time.Millisecond, newTestBlocklist())

	tokenStr, err := svc.IssueToken(testUser())
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	_, err = svc.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateWrongAlgorithm(t *testing.T) {
	svc := NewService([]byte("test-secret-key-32bytes-long!!!!"), 15*time.Minute, newTestBlocklist())

	// Create a token with alg:none attack
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]interface{}{
		"sub":   "user-123",
		"uid":   "user-123",
		"email": "admin@test.com",
		"role":  "admin",
		"iss":   "claude-plane",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
		"jti":   "fake-jti",
	})
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)
	noneToken := header + "." + payloadEnc + "."

	_, err := svc.ValidateToken(noneToken)
	if err == nil {
		t.Fatal("expected error for alg:none token")
	}

	// Also test with a different HMAC algorithm (HS384)
	hs384Token := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{
		"sub":   "user-123",
		"uid":   "user-123",
		"email": "admin@test.com",
		"role":  "admin",
		"iss":   "claude-plane",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
		"jti":   "fake-jti",
	})
	hs384Str, err := hs384Token.SignedString([]byte("test-secret-key-32bytes-long!!!!"))
	if err != nil {
		t.Fatalf("sign HS384 token: %v", err)
	}

	_, err = svc.ValidateToken(hs384Str)
	if err == nil {
		t.Fatal("expected error for HS384 token (only HS256 allowed)")
	}
}

func TestValidateRevoked(t *testing.T) {
	bl := newTestBlocklist()
	svc := NewService([]byte("test-secret-key-32bytes-long!!!!"), 15*time.Minute, bl)

	tokenStr, err := svc.IssueToken(testUser())
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	// Parse to get JTI
	claims, err := svc.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	// Revoke the token via the blocklist
	bl.revoked[claims.ID] = true

	// Validate should now fail
	_, err = svc.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for revoked token")
	}
}

func TestIssueTokenUniqueness(t *testing.T) {
	svc := NewService([]byte("test-secret-key-32bytes-long!!!!"), 15*time.Minute, newTestBlocklist())
	user := testUser()

	token1, err := svc.IssueToken(user)
	if err != nil {
		t.Fatalf("IssueToken 1: %v", err)
	}

	token2, err := svc.IssueToken(user)
	if err != nil {
		t.Fatalf("IssueToken 2: %v", err)
	}

	if token1 == token2 {
		t.Error("two issued tokens should not be identical")
	}

	// Parse both and check JTIs differ
	claims1, _ := svc.ValidateToken(token1)
	claims2, _ := svc.ValidateToken(token2)

	if claims1.ID == claims2.ID {
		t.Error("two tokens should have different JTIs")
	}
}

func TestValidateWrongSignature(t *testing.T) {
	svc := NewService([]byte("test-secret-key-32bytes-long!!!!"), 15*time.Minute, newTestBlocklist())

	tokenStr, err := svc.IssueToken(testUser())
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	// Tamper with the signature
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		t.Fatal("expected 3 parts in JWT")
	}
	tampered := parts[0] + "." + parts[1] + ".invalidsignature"

	_, err = svc.ValidateToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}
