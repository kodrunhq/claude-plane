package store

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// APIKey represents a row in the api_keys table.
// The plaintext key is never stored; only the HMAC-SHA256 hash is persisted.
type APIKey struct {
	KeyID      string     `json:"key_id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

const apiKeyPrefix = "cpk"

// CreateAPIKey generates a new API key for the given user. It returns the
// plaintext key (shown once), the key ID, and any error.
//
// Key format: cpk_{8-char-hex-keyid}_{32-byte-random-base64url}
//
// Only the HMAC-SHA256 of the full plaintext key is stored in the database.
func (s *Store) CreateAPIKey(ctx context.Context, userID, name string, scopes []string, signingKey []byte) (plaintextKey string, keyID string, err error) {
	keyID = uuid.New().String()
	keyIDPrefix := strings.ReplaceAll(keyID, "-", "")[:8]

	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	randomPart := base64.RawURLEncoding.EncodeToString(randomBytes)

	plaintextKey = fmt.Sprintf("%s_%s_%s", apiKeyPrefix, keyIDPrefix, randomPart)

	mac := hmac.New(sha256.New, signingKey)
	mac.Write([]byte(plaintextKey))
	keyHMAC := hex.EncodeToString(mac.Sum(nil))

	scopesJSON, err := marshalJSONField(scopes)
	if err != nil {
		return "", "", fmt.Errorf("marshal scopes: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.writer.ExecContext(ctx,
		`INSERT INTO api_keys (key_id, key_hmac, user_id, name, scopes, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		keyID, keyHMAC, userID, name, scopesJSON, now,
	)
	if err != nil {
		return "", "", fmt.Errorf("insert api key: %w", err)
	}

	return plaintextKey, keyID, nil
}

// GetAPIKeyByID retrieves an API key by its ID.
// Returns ErrNotFound if no matching key exists.
func (s *Store) GetAPIKeyByID(ctx context.Context, keyID string) (*APIKey, error) {
	row := s.reader.QueryRowContext(ctx,
		`SELECT key_id, user_id, name, scopes, expires_at, last_used_at, created_at
		 FROM api_keys WHERE key_id = ?`, keyID,
	)
	return scanAPIKey(row, keyID)
}

// ValidateAPIKey validates a plaintext API key against its stored HMAC.
// It extracts the key ID from the plaintext, fetches the stored HMAC, recomputes
// the HMAC, and compares them in constant time. Returns the APIKey on success.
// Returns an error if the key format is invalid, the key is not found, the HMAC
// does not match, or the key has expired.
func (s *Store) ValidateAPIKey(ctx context.Context, plaintextKey string, signingKey []byte) (*APIKey, error) {
	parts := strings.SplitN(plaintextKey, "_", 3)
	if len(parts) != 3 || parts[0] != apiKeyPrefix || len(parts[1]) != 8 {
		return nil, fmt.Errorf("invalid api key format")
	}
	keyIDPrefix := parts[1]

	// Look up all keys whose key_id starts with the 8-char prefix.
	// In practice this should return exactly one row.
	rows, err := s.reader.QueryContext(ctx,
		`SELECT key_id, key_hmac, user_id, name, scopes, expires_at, last_used_at, created_at
		 FROM api_keys WHERE key_id LIKE ?`,
		keyIDPrefix+"%",
	)
	if err != nil {
		return nil, fmt.Errorf("query api keys: %w", err)
	}
	defer rows.Close()

	mac := hmac.New(sha256.New, signingKey)
	mac.Write([]byte(plaintextKey))
	computedHMAC := mac.Sum(nil)

	for rows.Next() {
		var (
			keyID      string
			keyHMACHex string
			userID     string
			apiName    string
			scopesJSON sql.NullString
			expiresAt  sql.NullTime
			lastUsedAt sql.NullTime
			createdAt  time.Time
		)

		if err := rows.Scan(&keyID, &keyHMACHex, &userID, &apiName, &scopesJSON, &expiresAt, &lastUsedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("scan api key row: %w", err)
		}

		storedHMAC, err := hex.DecodeString(keyHMACHex)
		if err != nil {
			return nil, fmt.Errorf("decode stored hmac: %w", err)
		}

		if !hmac.Equal(computedHMAC, storedHMAC) {
			continue
		}

		// HMAC matched — check expiry.
		if expiresAt.Valid && time.Now().UTC().After(expiresAt.Time) {
			return nil, fmt.Errorf("api key has expired")
		}

		key := &APIKey{
			KeyID:     keyID,
			UserID:    userID,
			Name:      apiName,
			CreatedAt: createdAt,
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			key.ExpiresAt = &t
		}
		if lastUsedAt.Valid {
			t := lastUsedAt.Time
			key.LastUsedAt = &t
		}
		if scopesJSON.Valid && scopesJSON.String != "" {
			if err := json.Unmarshal([]byte(scopesJSON.String), &key.Scopes); err != nil {
				return nil, fmt.Errorf("unmarshal scopes: %w", err)
			}
		}

		return key, nil
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return nil, fmt.Errorf("api key not found or invalid: %w", ErrNotFound)
}

// ListAPIKeys returns all API keys for the given user, ordered by creation time descending.
func (s *Store) ListAPIKeys(ctx context.Context, userID string) ([]APIKey, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT key_id, user_id, name, scopes, expires_at, last_used_at, created_at
		 FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]APIKey, 0)
	for rows.Next() {
		key, err := scanAPIKeyRow(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *key)
	}
	return keys, rows.Err()
}

// DeleteAPIKey removes an API key by its ID.
// Returns ErrNotFound if no matching key exists.
func (s *Store) DeleteAPIKey(ctx context.Context, keyID string) error {
	result, err := s.writer.ExecContext(ctx,
		`DELETE FROM api_keys WHERE key_id = ?`, keyID,
	)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("api key %s: %w", keyID, ErrNotFound)
	}
	return nil
}

// UpdateAPIKeyLastUsed sets the last_used_at timestamp for a key to the current UTC time.
func (s *Store) UpdateAPIKeyLastUsed(ctx context.Context, keyID string) error {
	_, err := s.writer.ExecContext(ctx,
		`UPDATE api_keys SET last_used_at = ? WHERE key_id = ?`,
		time.Now().UTC(), keyID,
	)
	if err != nil {
		return fmt.Errorf("update api key last used: %w", err)
	}
	return nil
}

// scanAPIKey scans a single *sql.Row into an APIKey, using the identifier in error messages.
func scanAPIKey(row *sql.Row, identifier string) (*APIKey, error) {
	var (
		key        APIKey
		scopesJSON sql.NullString
		expiresAt  sql.NullTime
		lastUsedAt sql.NullTime
	)

	err := row.Scan(
		&key.KeyID, &key.UserID, &key.Name,
		&scopesJSON, &expiresAt, &lastUsedAt, &key.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("api key %s: %w", identifier, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("scan api key: %w", err)
	}

	if expiresAt.Valid {
		t := expiresAt.Time
		key.ExpiresAt = &t
	}
	if lastUsedAt.Valid {
		t := lastUsedAt.Time
		key.LastUsedAt = &t
	}
	if scopesJSON.Valid && scopesJSON.String != "" {
		if err := json.Unmarshal([]byte(scopesJSON.String), &key.Scopes); err != nil {
			return nil, fmt.Errorf("unmarshal scopes: %w", err)
		}
	}

	return &key, nil
}

// scanAPIKeyRow scans a row from *sql.Rows into an APIKey.
func scanAPIKeyRow(rows *sql.Rows) (*APIKey, error) {
	var (
		key        APIKey
		scopesJSON sql.NullString
		expiresAt  sql.NullTime
		lastUsedAt sql.NullTime
	)

	err := rows.Scan(
		&key.KeyID, &key.UserID, &key.Name,
		&scopesJSON, &expiresAt, &lastUsedAt, &key.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan api key row: %w", err)
	}

	if expiresAt.Valid {
		t := expiresAt.Time
		key.ExpiresAt = &t
	}
	if lastUsedAt.Valid {
		t := lastUsedAt.Time
		key.LastUsedAt = &t
	}
	if scopesJSON.Valid && scopesJSON.String != "" {
		if err := json.Unmarshal([]byte(scopesJSON.String), &key.Scopes); err != nil {
			return nil, fmt.Errorf("unmarshal scopes: %w", err)
		}
	}

	return &key, nil
}
