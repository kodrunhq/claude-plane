package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

func newTestStoreForShortCode(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "shortcode_test.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func makeToken(machineID, shortCode string) store.ProvisioningToken {
	now := time.Now().UTC()
	return store.ProvisioningToken{
		Token:         "tok-" + machineID,
		MachineID:     machineID,
		ShortCode:     shortCode,
		TargetOS:      "linux",
		TargetArch:    "amd64",
		CACertPEM:     "ca-pem",
		AgentCertPEM:  "cert-pem",
		AgentKeyPEM:   "key-pem",
		ServerAddress: "http://localhost:8080",
		GRPCAddress:   "localhost:9090",
		CreatedBy:     "test-user",
		CreatedAt:     now,
		ExpiresAt:     now.Add(1 * time.Hour),
	}
}

func TestCreateProvisioningToken_WithShortCode(t *testing.T) {
	s := newTestStoreForShortCode(t)
	ctx := context.Background()

	tok := makeToken("worker-01", "A3X9K2")
	if err := s.CreateProvisioningToken(ctx, tok); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}

	got, err := s.GetProvisioningTokenByCode(ctx, "A3X9K2")
	if err != nil {
		t.Fatalf("GetProvisioningTokenByCode: %v", err)
	}
	if got.MachineID != "worker-01" {
		t.Errorf("MachineID = %q, want %q", got.MachineID, "worker-01")
	}
	if got.ShortCode != "A3X9K2" {
		t.Errorf("ShortCode = %q, want %q", got.ShortCode, "A3X9K2")
	}
}

func TestGetProvisioningTokenByCode_NotFound(t *testing.T) {
	s := newTestStoreForShortCode(t)
	ctx := context.Background()

	_, err := s.GetProvisioningTokenByCode(ctx, "ZZZZZZ")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetProvisioningTokenByCode_Expired(t *testing.T) {
	s := newTestStoreForShortCode(t)
	ctx := context.Background()

	tok := makeToken("worker-02", "B4Y8J3")
	tok.ExpiresAt = time.Now().UTC().Add(-1 * time.Hour) // already expired
	if err := s.CreateProvisioningToken(ctx, tok); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}

	_, err := s.GetProvisioningTokenByCode(ctx, "B4Y8J3")
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestGetProvisioningTokenByCode_Redeemed(t *testing.T) {
	s := newTestStoreForShortCode(t)
	ctx := context.Background()

	tok := makeToken("worker-03", "C5Z7H4")
	if err := s.CreateProvisioningToken(ctx, tok); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}
	if err := s.RedeemProvisioningToken(ctx, tok.Token); err != nil {
		t.Fatalf("RedeemProvisioningToken: %v", err)
	}

	_, err := s.GetProvisioningTokenByCode(ctx, "C5Z7H4")
	if err == nil {
		t.Fatal("expected error for redeemed token, got nil")
	}
}

func TestListProvisioningTokens_IncludesShortCode(t *testing.T) {
	s := newTestStoreForShortCode(t)
	ctx := context.Background()

	tok := makeToken("worker-04", "D6W8F5")
	if err := s.CreateProvisioningToken(ctx, tok); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}

	tokens, err := s.ListProvisioningTokens(ctx)
	if err != nil {
		t.Fatalf("ListProvisioningTokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].ShortCode != "D6W8F5" {
		t.Errorf("ShortCode = %q, want %q", tokens[0].ShortCode, "D6W8F5")
	}
}
