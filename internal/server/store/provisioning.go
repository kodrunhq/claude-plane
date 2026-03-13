package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ProvisioningToken holds a short-lived token used to provision a new agent
// machine. Once redeemed, the token cannot be used again.
type ProvisioningToken struct {
	Token         string     `json:"token"`
	MachineID     string     `json:"machine_id"`
	TargetOS      string     `json:"target_os"`
	TargetArch    string     `json:"target_arch"`
	CACertPEM     string     `json:"ca_cert_pem"`
	AgentCertPEM  string     `json:"agent_cert_pem"`
	AgentKeyPEM   string     `json:"agent_key_pem"`
	ServerAddress string     `json:"server_address"`
	GRPCAddress   string     `json:"grpc_address"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	RedeemedAt    *time.Time `json:"redeemed_at,omitempty"`
}

// ErrTokenExpired is returned when a provisioning token has passed its expiry time.
var ErrTokenExpired = errors.New("token expired")

// ErrTokenAlreadyRedeemed is returned when a provisioning token has already been used.
var ErrTokenAlreadyRedeemed = errors.New("token already redeemed")

// CreateProvisioningToken inserts a new provisioning token into the database.
// The caller is responsible for generating a cryptographically secure token value.
func (s *Store) CreateProvisioningToken(ctx context.Context, t ProvisioningToken) error {
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO provisioning_tokens
		 (token, machine_id, target_os, target_arch, ca_cert_pem, agent_cert_pem,
		  agent_key_pem, server_address, grpc_address, created_by, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Token, t.MachineID, t.TargetOS, t.TargetArch, t.CACertPEM, t.AgentCertPEM,
		t.AgentKeyPEM, t.ServerAddress, t.GRPCAddress, t.CreatedBy,
		t.CreatedAt.UTC(), t.ExpiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("create provisioning token: %w", err)
	}
	return nil
}

// GetProvisioningToken retrieves a provisioning token by its value.
// Returns ErrNotFound if the token does not exist.
// Returns ErrTokenExpired if the token has passed its expiry time.
// Returns ErrTokenAlreadyRedeemed if the token has already been used.
func (s *Store) GetProvisioningToken(ctx context.Context, token string) (*ProvisioningToken, error) {
	var pt ProvisioningToken
	var redeemedAt sql.NullTime

	err := s.reader.QueryRowContext(ctx,
		`SELECT token, machine_id, target_os, target_arch, ca_cert_pem, agent_cert_pem,
		        agent_key_pem, server_address, grpc_address, created_by, created_at,
		        expires_at, redeemed_at
		 FROM provisioning_tokens WHERE token = ?`,
		token,
	).Scan(
		&pt.Token, &pt.MachineID, &pt.TargetOS, &pt.TargetArch, &pt.CACertPEM, &pt.AgentCertPEM,
		&pt.AgentKeyPEM, &pt.ServerAddress, &pt.GRPCAddress, &pt.CreatedBy, &pt.CreatedAt,
		&pt.ExpiresAt, &redeemedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("provisioning token: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get provisioning token: %w", err)
	}

	if redeemedAt.Valid {
		pt.RedeemedAt = &redeemedAt.Time
		return nil, fmt.Errorf("get provisioning token: %w", ErrTokenAlreadyRedeemed)
	}

	if time.Now().UTC().After(pt.ExpiresAt.UTC()) {
		return nil, fmt.Errorf("get provisioning token: %w", ErrTokenExpired)
	}

	return &pt, nil
}

// RedeemProvisioningToken marks a token as redeemed by setting redeemed_at to the
// current time. Returns ErrNotFound if the token does not exist, ErrTokenExpired
// if it has expired, and ErrTokenAlreadyRedeemed if it was already used.
// The operation is atomic — a single UPDATE with predicates avoids TOCTOU races.
func (s *Store) RedeemProvisioningToken(ctx context.Context, token string) error {
	now := time.Now().UTC()
	result, err := s.writer.ExecContext(ctx,
		`UPDATE provisioning_tokens
		 SET redeemed_at = ?, ca_cert_pem = '', agent_cert_pem = '', agent_key_pem = ''
		 WHERE token = ? AND redeemed_at IS NULL AND expires_at >= ?`,
		now, token, now,
	)
	if err != nil {
		return fmt.Errorf("redeem provisioning token: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 1 {
		return nil
	}

	// No rows updated — determine why by checking the token's current state.
	var exists bool
	var redeemedAt sql.NullTime
	var expiresAt time.Time
	err = s.reader.QueryRowContext(ctx,
		`SELECT 1, redeemed_at, expires_at FROM provisioning_tokens WHERE token = ?`,
		token,
	).Scan(&exists, &redeemedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("redeem provisioning token: %w", ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("redeem provisioning token: %w", err)
	}
	if redeemedAt.Valid {
		return fmt.Errorf("redeem provisioning token: %w", ErrTokenAlreadyRedeemed)
	}
	if now.After(expiresAt.UTC()) {
		return fmt.Errorf("redeem provisioning token: %w", ErrTokenExpired)
	}
	// Shouldn't reach here, but cover it defensively.
	return fmt.Errorf("redeem provisioning token: %w", ErrTokenAlreadyRedeemed)
}

// CleanExpiredProvisioningTokens deletes all provisioning tokens whose expiry time
// is in the past. Returns the number of rows deleted.
func (s *Store) CleanExpiredProvisioningTokens(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	result, err := s.writer.ExecContext(ctx,
		`DELETE FROM provisioning_tokens WHERE expires_at < ?`,
		now,
	)
	if err != nil {
		return 0, fmt.Errorf("clean expired provisioning tokens: %w", err)
	}

	deleted, _ := result.RowsAffected()
	return deleted, nil
}
