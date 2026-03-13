package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Credential represents an encrypted credential stored in the database.
// The Value is never exposed via JSON; only the metadata is returned to callers.
type Credential struct {
	CredentialID   string    `json:"credential_id"`
	UserID         string    `json:"user_id"`
	Name           string    `json:"name"`
	EncryptedValue []byte    `json:"-"`
	Nonce          []byte    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateCredential encrypts value with encryptionKey and stores the credential.
// Returns the new credential record (without the plaintext value).
func (s *Store) CreateCredential(ctx context.Context, userID, name string, value []byte, encryptionKey []byte) (*Credential, error) {
	encrypted, nonce, err := Encrypt(value, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt credential: %w", err)
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	_, err = s.writer.ExecContext(ctx,
		`INSERT INTO credentials (credential_id, user_id, name, encrypted_value, nonce, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, userID, name, encrypted, nonce, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}

	return &Credential{
		CredentialID:   id,
		UserID:         userID,
		Name:           name,
		EncryptedValue: encrypted,
		Nonce:          nonce,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// ListCredentialsByUser returns all credentials for a given user, ordered by name.
// The encrypted values and nonces are populated but callers should not expose them.
func (s *Store) ListCredentialsByUser(ctx context.Context, userID string) ([]Credential, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT credential_id, user_id, name, encrypted_value, nonce, created_at, updated_at
		 FROM credentials WHERE user_id = ? ORDER BY name ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer rows.Close()

	var credentials []Credential
	for rows.Next() {
		var c Credential
		if err := rows.Scan(
			&c.CredentialID, &c.UserID, &c.Name,
			&c.EncryptedValue, &c.Nonce,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		credentials = append(credentials, c)
	}
	return credentials, rows.Err()
}

// GetCredential retrieves a single credential by ID, including its encrypted value.
func (s *Store) GetCredential(ctx context.Context, credentialID string) (*Credential, error) {
	var c Credential
	err := s.reader.QueryRowContext(ctx,
		`SELECT credential_id, user_id, name, encrypted_value, nonce, created_at, updated_at
		 FROM credentials WHERE credential_id = ?`,
		credentialID,
	).Scan(
		&c.CredentialID, &c.UserID, &c.Name,
		&c.EncryptedValue, &c.Nonce,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("credential %s: %w", credentialID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get credential: %w", err)
	}
	return &c, nil
}

// DeleteCredential removes a credential by ID.
func (s *Store) DeleteCredential(ctx context.Context, credentialID string) error {
	result, err := s.writer.ExecContext(ctx,
		`DELETE FROM credentials WHERE credential_id = ?`, credentialID,
	)
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("credential %s: %w", credentialID, ErrNotFound)
	}
	return nil
}
