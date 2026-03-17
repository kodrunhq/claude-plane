package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrMachineNotFound is returned when a machine lookup finds no matching row.
var ErrMachineNotFound = errors.New("machine not found")

// Machine represents a row in the machines table.
type Machine struct {
	MachineID    string
	DisplayName  string
	Status       string
	MaxSessions  int32
	LastHealth   *string
	LastSeenAt   *time.Time
	CertExpires  *time.Time
	CreatedAt    time.Time
}

// UpsertMachine inserts a new machine or updates max_sessions if it already exists.
// New machines start with status "disconnected".
func (s *Store) UpsertMachine(machineID string, maxSessions int32) error {
	_, err := s.writer.Exec(`
		INSERT INTO machines (machine_id, max_sessions)
		VALUES (?, ?)
		ON CONFLICT(machine_id) DO UPDATE SET max_sessions = excluded.max_sessions
	`, machineID, maxSessions)
	if err != nil {
		return fmt.Errorf("upsert machine %q: %w", machineID, err)
	}
	return nil
}

// UpdateMachineStatus updates the status and last_seen_at for a machine.
func (s *Store) UpdateMachineStatus(machineID, status string, lastSeenAt time.Time) error {
	res, err := s.writer.Exec(`
		UPDATE machines SET status = ?, last_seen_at = ? WHERE machine_id = ?
	`, status, lastSeenAt, machineID)
	if err != nil {
		return fmt.Errorf("update machine status %q: %w", machineID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("machine %q not found", machineID)
	}
	return nil
}

// UpdateMachineDisplayName sets the display_name for the given machine.
// Returns ErrMachineNotFound if no matching row exists.
func (s *Store) UpdateMachineDisplayName(machineID, displayName string) error {
	result, err := s.writer.Exec(
		`UPDATE machines SET display_name = ? WHERE machine_id = ?`,
		displayName, machineID,
	)
	if err != nil {
		return fmt.Errorf("update machine display name: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update machine display name rows: %w", err)
	}
	if rows == 0 {
		return ErrMachineNotFound
	}
	return nil
}

// SoftDeleteMachine sets the deleted_at timestamp for the given machine.
// Returns ErrMachineNotFound if no matching row exists.
func (s *Store) SoftDeleteMachine(machineID string) error {
	result, err := s.writer.Exec(
		`UPDATE machines SET deleted_at = CURRENT_TIMESTAMP WHERE machine_id = ? AND deleted_at IS NULL`,
		machineID,
	)
	if err != nil {
		return fmt.Errorf("soft delete machine %q: %w", machineID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("soft delete machine rows: %w", err)
	}
	if rows == 0 {
		return ErrMachineNotFound
	}
	return nil
}

// ListMachines returns all non-deleted machines ordered by machine_id.
func (s *Store) ListMachines() ([]Machine, error) {
	rows, err := s.reader.Query(`
		SELECT machine_id, display_name, status, max_sessions,
		       last_health, last_seen_at, cert_expires_at, created_at
		FROM machines WHERE deleted_at IS NULL ORDER BY machine_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	defer rows.Close()

	var machines []Machine
	for rows.Next() {
		var m Machine
		var displayName, lastHealth sql.NullString
		var lastSeenAt, certExpires sql.NullTime
		if err := rows.Scan(
			&m.MachineID, &displayName, &m.Status, &m.MaxSessions,
			&lastHealth, &lastSeenAt, &certExpires, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan machine row: %w", err)
		}
		if displayName.Valid {
			m.DisplayName = displayName.String
		}
		if lastHealth.Valid {
			m.LastHealth = &lastHealth.String
		}
		if lastSeenAt.Valid {
			m.LastSeenAt = &lastSeenAt.Time
		}
		if certExpires.Valid {
			m.CertExpires = &certExpires.Time
		}
		machines = append(machines, m)
	}
	return machines, rows.Err()
}

// GetMachine returns a single non-deleted machine by ID, or an error if not found.
func (s *Store) GetMachine(machineID string) (*Machine, error) {
	var m Machine
	var displayName, lastHealth sql.NullString
	var lastSeenAt, certExpires sql.NullTime
	err := s.reader.QueryRow(`
		SELECT machine_id, display_name, status, max_sessions,
		       last_health, last_seen_at, cert_expires_at, created_at
		FROM machines WHERE machine_id = ? AND deleted_at IS NULL
	`, machineID).Scan(
		&m.MachineID, &displayName, &m.Status, &m.MaxSessions,
		&lastHealth, &lastSeenAt, &certExpires, &m.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMachineNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get machine %q: %w", machineID, err)
	}
	if displayName.Valid {
		m.DisplayName = displayName.String
	}
	if lastHealth.Valid {
		m.LastHealth = &lastHealth.String
	}
	if lastSeenAt.Valid {
		m.LastSeenAt = &lastSeenAt.Time
	}
	if certExpires.Valid {
		m.CertExpires = &certExpires.Time
	}
	return &m, nil
}

// GetMachineDisplayName returns the display name of a machine, or "" if not found.
func (s *Store) GetMachineDisplayName(machineID string) string {
	m, err := s.GetMachine(machineID)
	if err != nil || m == nil {
		return ""
	}
	return m.DisplayName
}
