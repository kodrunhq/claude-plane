package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestStoreForProvisioning(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "provisioning_test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// makeToken returns a ProvisioningToken with the given expiry offset from now.
func makeToken(tokenVal, machineID string, expiresIn time.Duration) ProvisioningToken {
	now := time.Now().UTC()
	return ProvisioningToken{
		Token:         tokenVal,
		MachineID:     machineID,
		TargetOS:      "linux",
		TargetArch:    "amd64",
		CACertPEM:     "ca-cert-pem",
		AgentCertPEM:  "agent-cert-pem",
		AgentKeyPEM:   "agent-key-pem",
		ServerAddress: "https://example.com",
		GRPCAddress:   "grpc.example.com:443",
		CreatedBy:     "admin",
		CreatedAt:     now,
		ExpiresAt:     now.Add(expiresIn),
	}
}

// --- CreateProvisioningToken / GetProvisioningToken ---

func TestProvisioningToken_CreateAndGet(t *testing.T) {
	s := newTestStoreForProvisioning(t)
	ctx := context.Background()

	pt := makeToken("tok-create-get", "machine-01", time.Hour)

	if err := s.CreateProvisioningToken(ctx, pt); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}

	got, err := s.GetProvisioningToken(ctx, pt.Token)
	if err != nil {
		t.Fatalf("GetProvisioningToken: %v", err)
	}

	if got.Token != pt.Token {
		t.Errorf("Token = %q, want %q", got.Token, pt.Token)
	}
	if got.MachineID != pt.MachineID {
		t.Errorf("MachineID = %q, want %q", got.MachineID, pt.MachineID)
	}
	if got.TargetOS != pt.TargetOS {
		t.Errorf("TargetOS = %q, want %q", got.TargetOS, pt.TargetOS)
	}
	if got.TargetArch != pt.TargetArch {
		t.Errorf("TargetArch = %q, want %q", got.TargetArch, pt.TargetArch)
	}
	if got.CACertPEM != pt.CACertPEM {
		t.Errorf("CACertPEM = %q, want %q", got.CACertPEM, pt.CACertPEM)
	}
	if got.AgentCertPEM != pt.AgentCertPEM {
		t.Errorf("AgentCertPEM = %q, want %q", got.AgentCertPEM, pt.AgentCertPEM)
	}
	if got.AgentKeyPEM != pt.AgentKeyPEM {
		t.Errorf("AgentKeyPEM = %q, want %q", got.AgentKeyPEM, pt.AgentKeyPEM)
	}
	if got.ServerAddress != pt.ServerAddress {
		t.Errorf("ServerAddress = %q, want %q", got.ServerAddress, pt.ServerAddress)
	}
	if got.GRPCAddress != pt.GRPCAddress {
		t.Errorf("GRPCAddress = %q, want %q", got.GRPCAddress, pt.GRPCAddress)
	}
	if got.CreatedBy != pt.CreatedBy {
		t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, pt.CreatedBy)
	}
	if got.RedeemedAt != nil {
		t.Error("expected RedeemedAt = nil for fresh token")
	}
}

// --- GetProvisioningToken: not found ---

func TestProvisioningToken_Get_NotFound(t *testing.T) {
	s := newTestStoreForProvisioning(t)
	ctx := context.Background()

	_, err := s.GetProvisioningToken(ctx, "nonexistent-token")
	if err == nil {
		t.Fatal("expected error for nonexistent token")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- GetProvisioningToken: expired ---

func TestProvisioningToken_Get_Expired(t *testing.T) {
	s := newTestStoreForProvisioning(t)
	ctx := context.Background()

	// Insert a token that expired 1 second ago.
	pt := makeToken("tok-expired", "machine-exp", -time.Second)
	if err := s.CreateProvisioningToken(ctx, pt); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}

	_, err := s.GetProvisioningToken(ctx, pt.Token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

// --- GetProvisioningToken: already redeemed ---

func TestProvisioningToken_Get_AlreadyRedeemed(t *testing.T) {
	s := newTestStoreForProvisioning(t)
	ctx := context.Background()

	pt := makeToken("tok-redeemed", "machine-red", time.Hour)
	if err := s.CreateProvisioningToken(ctx, pt); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}
	if err := s.RedeemProvisioningToken(ctx, pt.Token); err != nil {
		t.Fatalf("RedeemProvisioningToken: %v", err)
	}

	_, err := s.GetProvisioningToken(ctx, pt.Token)
	if err == nil {
		t.Fatal("expected error for redeemed token")
	}
	if !errors.Is(err, ErrTokenAlreadyRedeemed) {
		t.Errorf("expected ErrTokenAlreadyRedeemed, got %v", err)
	}
}

// --- RedeemProvisioningToken ---

func TestProvisioningToken_Redeem(t *testing.T) {
	s := newTestStoreForProvisioning(t)
	ctx := context.Background()

	pt := makeToken("tok-redeem-ok", "machine-rdm", time.Hour)
	if err := s.CreateProvisioningToken(ctx, pt); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}

	if err := s.RedeemProvisioningToken(ctx, pt.Token); err != nil {
		t.Fatalf("RedeemProvisioningToken: %v", err)
	}

	// Confirm redeemed_at is set by querying the DB directly.
	var redeemedAt *time.Time
	row := s.reader.QueryRowContext(ctx,
		`SELECT redeemed_at FROM provisioning_tokens WHERE token = ?`, pt.Token,
	)
	var nullTime interface{}
	if err := row.Scan(&nullTime); err != nil {
		t.Fatalf("scan redeemed_at: %v", err)
	}
	if nullTime == nil {
		t.Fatal("expected redeemed_at to be set after redeeming")
	}
	_ = redeemedAt
}

// --- RedeemProvisioningToken: already redeemed ---

func TestProvisioningToken_Redeem_AlreadyRedeemed(t *testing.T) {
	s := newTestStoreForProvisioning(t)
	ctx := context.Background()

	pt := makeToken("tok-redeem-twice", "machine-rdt", time.Hour)
	if err := s.CreateProvisioningToken(ctx, pt); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}
	if err := s.RedeemProvisioningToken(ctx, pt.Token); err != nil {
		t.Fatalf("first RedeemProvisioningToken: %v", err)
	}

	err := s.RedeemProvisioningToken(ctx, pt.Token)
	if err == nil {
		t.Fatal("expected error when redeeming an already-redeemed token")
	}
	if !errors.Is(err, ErrTokenAlreadyRedeemed) {
		t.Errorf("expected ErrTokenAlreadyRedeemed, got %v", err)
	}
}

// --- RedeemProvisioningToken: not found ---

func TestProvisioningToken_Redeem_NotFound(t *testing.T) {
	s := newTestStoreForProvisioning(t)
	ctx := context.Background()

	err := s.RedeemProvisioningToken(ctx, "nonexistent-token")
	if err == nil {
		t.Fatal("expected error for nonexistent token")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- CleanExpiredProvisioningTokens ---

func TestProvisioningToken_CleanExpired(t *testing.T) {
	s := newTestStoreForProvisioning(t)
	ctx := context.Background()

	// Two expired, one valid.
	expired1 := makeToken("tok-exp-1", "machine-e1", -time.Second)
	expired2 := makeToken("tok-exp-2", "machine-e2", -time.Second)
	valid := makeToken("tok-valid", "machine-v1", time.Hour)

	for _, pt := range []ProvisioningToken{expired1, expired2, valid} {
		if err := s.CreateProvisioningToken(ctx, pt); err != nil {
			t.Fatalf("CreateProvisioningToken(%s): %v", pt.Token, err)
		}
	}

	deleted, err := s.CleanExpiredProvisioningTokens(ctx)
	if err != nil {
		t.Fatalf("CleanExpiredProvisioningTokens: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	// Valid token must still exist.
	got, err := s.GetProvisioningToken(ctx, valid.Token)
	if err != nil {
		t.Fatalf("GetProvisioningToken for valid token after clean: %v", err)
	}
	if got.Token != valid.Token {
		t.Errorf("Token = %q, want %q", got.Token, valid.Token)
	}

	// Expired tokens must be gone.
	for _, expiredToken := range []string{expired1.Token, expired2.Token} {
		_, err := s.GetProvisioningToken(ctx, expiredToken)
		if err == nil {
			t.Errorf("expected error for deleted token %s", expiredToken)
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for deleted token %s, got %v", expiredToken, err)
		}
	}
}

func TestProvisioningToken_CleanExpired_LeavesValid(t *testing.T) {
	s := newTestStoreForProvisioning(t)
	ctx := context.Background()

	pt1 := makeToken("tok-keep-1", "machine-k1", time.Hour)
	pt2 := makeToken("tok-keep-2", "machine-k2", 2*time.Hour)
	for _, pt := range []ProvisioningToken{pt1, pt2} {
		if err := s.CreateProvisioningToken(ctx, pt); err != nil {
			t.Fatalf("CreateProvisioningToken: %v", err)
		}
	}

	deleted, err := s.CleanExpiredProvisioningTokens(ctx)
	if err != nil {
		t.Fatalf("CleanExpiredProvisioningTokens: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deletions when no tokens are expired, got %d", deleted)
	}
}
