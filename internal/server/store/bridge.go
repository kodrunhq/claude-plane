package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BridgeConnector represents a row in the bridge_connectors table.
// Sensitive fields (like bot tokens) are stored encrypted in config_secret + config_nonce.
// The Config field holds non-sensitive JSON configuration.
type BridgeConnector struct {
	ConnectorID   string    `json:"connector_id"`
	ConnectorType string    `json:"connector_type"`
	Name          string    `json:"name"`
	Enabled       bool      `json:"enabled"`
	Config        string    `json:"config"`      // JSON, non-sensitive fields
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CreateConnector inserts a new bridge connector. The secretJSON bytes are
// encrypted with encKey (AES-256-GCM) and stored alongside the non-sensitive
// Config. When secretJSON is nil, config_secret and config_nonce are stored as NULL.
// Returns the newly created connector record.
func (s *Store) CreateConnector(ctx context.Context, c *BridgeConnector, secretJSON []byte, encKey []byte) (*BridgeConnector, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	var configSecret, configNonce []byte
	if secretJSON != nil {
		var err error
		configSecret, configNonce, err = Encrypt(secretJSON, encKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt connector secret: %w", err)
		}
	}

	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO bridge_connectors
		 (connector_id, connector_type, name, enabled, config, config_secret, config_nonce, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, c.ConnectorType, c.Name, boolToInt(c.Enabled),
		c.Config, configSecret, configNonce, c.CreatedBy, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert bridge connector: %w", err)
	}

	return &BridgeConnector{
		ConnectorID:   id,
		ConnectorType: c.ConnectorType,
		Name:          c.Name,
		Enabled:       c.Enabled,
		Config:        c.Config,
		CreatedBy:     c.CreatedBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// GetConnector retrieves a bridge connector by ID (without decrypting the secret).
// Returns ErrNotFound when no matching connector exists.
func (s *Store) GetConnector(ctx context.Context, connectorID string) (*BridgeConnector, error) {
	var (
		c       BridgeConnector
		enabled int
	)
	err := s.reader.QueryRowContext(ctx,
		`SELECT connector_id, connector_type, name, enabled, config, created_by, created_at, updated_at
		 FROM bridge_connectors WHERE connector_id = ?`,
		connectorID,
	).Scan(
		&c.ConnectorID, &c.ConnectorType, &c.Name, &enabled,
		&c.Config, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("connector %s: %w", connectorID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get connector: %w", err)
	}
	c.Enabled = enabled != 0
	return &c, nil
}

// GetConnectorWithSecret retrieves a bridge connector by ID and decrypts the
// stored secret using encKey. The returned secretJSON will be nil when no
// secret was stored. Returns ErrNotFound when no matching connector exists.
func (s *Store) GetConnectorWithSecret(ctx context.Context, connectorID string, encKey []byte) (*BridgeConnector, []byte, error) {
	var (
		c            BridgeConnector
		enabled      int
		configSecret []byte
		configNonce  []byte
	)
	err := s.reader.QueryRowContext(ctx,
		`SELECT connector_id, connector_type, name, enabled, config, config_secret, config_nonce, created_by, created_at, updated_at
		 FROM bridge_connectors WHERE connector_id = ?`,
		connectorID,
	).Scan(
		&c.ConnectorID, &c.ConnectorType, &c.Name, &enabled,
		&c.Config, &configSecret, &configNonce, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil, fmt.Errorf("connector %s: %w", connectorID, ErrNotFound)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get connector with secret: %w", err)
	}
	c.Enabled = enabled != 0

	if len(configSecret) == 0 {
		return &c, nil, nil
	}

	secretJSON, err := Decrypt(configSecret, configNonce, encKey)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt connector secret: %w", err)
	}

	return &c, secretJSON, nil
}

// ListConnectors returns all bridge connectors ordered by created_at ascending.
func (s *Store) ListConnectors(ctx context.Context) ([]BridgeConnector, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT connector_id, connector_type, name, enabled, config, created_by, created_at, updated_at
		 FROM bridge_connectors ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list connectors: %w", err)
	}
	defer rows.Close()

	connectors := make([]BridgeConnector, 0)
	for rows.Next() {
		var (
			c       BridgeConnector
			enabled int
		)
		if err := rows.Scan(
			&c.ConnectorID, &c.ConnectorType, &c.Name, &enabled,
			&c.Config, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan connector: %w", err)
		}
		c.Enabled = enabled != 0
		connectors = append(connectors, c)
	}
	return connectors, rows.Err()
}

// UpdateConnector updates the mutable fields of a bridge connector and
// re-encrypts the secret when secretJSON is non-nil. When secretJSON is nil,
// the existing config_secret and config_nonce are preserved. Returns the
// updated record re-read from the database. Returns ErrNotFound when no
// matching connector exists.
func (s *Store) UpdateConnector(ctx context.Context, connectorID string, c *BridgeConnector, secretJSON []byte, encKey []byte) (*BridgeConnector, error) {
	now := time.Now().UTC()

	var result sql.Result
	var err error

	if secretJSON != nil {
		var configSecret, configNonce []byte
		if len(secretJSON) > 0 {
			configSecret, configNonce, err = Encrypt(secretJSON, encKey)
			if err != nil {
				return nil, fmt.Errorf("encrypt connector secret: %w", err)
			}
		}
		result, err = s.writer.ExecContext(ctx,
			`UPDATE bridge_connectors
			 SET connector_type = ?, name = ?, enabled = ?, config = ?,
			     config_secret = ?, config_nonce = ?, updated_at = ?
			 WHERE connector_id = ?`,
			c.ConnectorType, c.Name, boolToInt(c.Enabled), c.Config,
			configSecret, configNonce, now, connectorID,
		)
	} else {
		result, err = s.writer.ExecContext(ctx,
			`UPDATE bridge_connectors
			 SET connector_type = ?, name = ?, enabled = ?, config = ?, updated_at = ?
			 WHERE connector_id = ?`,
			c.ConnectorType, c.Name, boolToInt(c.Enabled), c.Config,
			now, connectorID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("update connector: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("update connector rows affected: %w", err)
	}
	if affected == 0 {
		return nil, fmt.Errorf("connector %s: %w", connectorID, ErrNotFound)
	}

	// Re-read the full row from the writer connection to return accurate data.
	var updated BridgeConnector
	var enabledInt int
	err = s.writer.QueryRowContext(ctx,
		`SELECT connector_id, connector_type, name, enabled, config, created_by, created_at, updated_at
		 FROM bridge_connectors WHERE connector_id = ?`, connectorID,
	).Scan(&updated.ConnectorID, &updated.ConnectorType, &updated.Name, &enabledInt,
		&updated.Config, &updated.CreatedBy, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("read updated connector: %w", err)
	}
	updated.Enabled = enabledInt != 0
	return &updated, nil
}

// DeleteConnector removes a bridge connector by ID.
// Returns ErrNotFound when no matching connector exists.
func (s *Store) DeleteConnector(ctx context.Context, connectorID string) error {
	result, err := s.writer.ExecContext(ctx,
		`DELETE FROM bridge_connectors WHERE connector_id = ?`, connectorID,
	)
	if err != nil {
		return fmt.Errorf("delete connector: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete connector rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("connector %s: %w", connectorID, ErrNotFound)
	}
	return nil
}

// SetBridgeControl upserts a key-value pair in the bridge_control table.
// This is used for control signals such as restart requests.
func (s *Store) SetBridgeControl(ctx context.Context, key, value string) error {
	_, err := s.writer.ExecContext(ctx,
		`INSERT OR REPLACE INTO bridge_control (key, value, updated_at) VALUES (?, ?, ?)`,
		key, value, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("set bridge control %q: %w", key, err)
	}
	return nil
}

// GetBridgeControl retrieves a control value by key.
// Returns ErrNotFound when the key does not exist.
func (s *Store) GetBridgeControl(ctx context.Context, key string) (string, error) {
	var value string
	err := s.reader.QueryRowContext(ctx,
		`SELECT value FROM bridge_control WHERE key = ?`, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("bridge control %q: %w", key, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("get bridge control: %w", err)
	}
	return value, nil
}

// boolToInt converts a bool to its SQLite integer representation (0 or 1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
