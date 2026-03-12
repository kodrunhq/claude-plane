package store

import (
	"fmt"
	"time"
)

// RevokedToken represents a row in the revoked_tokens table.
type RevokedToken struct {
	JTI       string
	UserID    string
	RevokedAt time.Time
	ExpiresAt time.Time
}

// RevokeToken inserts a revocation record into the revoked_tokens table.
func (s *Store) RevokeToken(jti, userID string, expiresAt time.Time) error {
	_, err := s.writer.Exec(
		`INSERT INTO revoked_tokens (jti, user_id, expires_at) VALUES (?, ?, ?)`,
		jti, userID, expiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

// LoadActiveRevocations returns all revocation records whose expiry is in the future.
func (s *Store) LoadActiveRevocations() ([]RevokedToken, error) {
	rows, err := s.reader.Query(
		`SELECT jti, user_id, revoked_at, expires_at FROM revoked_tokens WHERE expires_at > ?`,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("load active revocations: %w", err)
	}
	defer rows.Close()

	var tokens []RevokedToken
	for rows.Next() {
		var t RevokedToken
		if err := rows.Scan(&t.JTI, &t.UserID, &t.RevokedAt, &t.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan revocation: %w", err)
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate revocations: %w", err)
	}
	return tokens, nil
}

// CleanExpired deletes revocation records that have expired before the given time.
func (s *Store) CleanExpired(now time.Time) error {
	_, err := s.writer.Exec(
		`DELETE FROM revoked_tokens WHERE expires_at <= ?`,
		now.UTC(),
	)
	if err != nil {
		return fmt.Errorf("clean expired revocations: %w", err)
	}
	return nil
}
