package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Injection represents a row in the injections audit table.
type Injection struct {
	InjectionID string     `json:"injection_id"`
	SessionID   string     `json:"session_id"`
	UserID      string     `json:"user_id"`
	TextLength  int        `json:"text_length"`
	Metadata    string     `json:"metadata,omitempty"`
	Source      string     `json:"source"`
	CreatedAt   time.Time  `json:"created_at"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
}

// CreateInjection inserts a new injection record and returns the hydrated struct.
func (s *Store) CreateInjection(ctx context.Context, inj *Injection) (*Injection, error) {
	id := inj.InjectionID
	if id == "" {
		id = uuid.New().String()
	}
	now := time.Now().UTC()

	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO injections (injection_id, session_id, user_id, text_length, metadata, source, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, inj.SessionID, inj.UserID, inj.TextLength,
		nullIfEmpty(inj.Metadata), inj.Source, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create injection: %w", err)
	}

	return &Injection{
		InjectionID: id,
		SessionID:   inj.SessionID,
		UserID:      inj.UserID,
		TextLength:  inj.TextLength,
		Metadata:    inj.Metadata,
		Source:      inj.Source,
		CreatedAt:   now,
	}, nil
}

// UpdateInjectionDelivered sets the delivered_at timestamp for an injection.
func (s *Store) UpdateInjectionDelivered(ctx context.Context, injectionID string, deliveredAt time.Time) error {
	result, err := s.writer.ExecContext(ctx,
		`UPDATE injections SET delivered_at = ? WHERE injection_id = ?`,
		deliveredAt.UTC(), injectionID,
	)
	if err != nil {
		return fmt.Errorf("update injection delivered: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("injection %s: %w", injectionID, ErrNotFound)
	}
	return nil
}

// UpdateInjectionFailed clears delivered_at and records the failure reason in metadata.
func (s *Store) UpdateInjectionFailed(ctx context.Context, injectionID string, reason string) error {
	_, err := s.writer.ExecContext(ctx,
		`UPDATE injections SET delivered_at = NULL, metadata = json_set(COALESCE(metadata, '{}'), '$.failure_reason', ?) WHERE injection_id = ?`,
		reason, injectionID,
	)
	return err
}

// ListInjectionsBySession returns all injections for a session ordered by created_at DESC.
func (s *Store) ListInjectionsBySession(ctx context.Context, sessionID string) ([]Injection, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT injection_id, session_id, user_id, text_length, metadata, source, created_at, delivered_at
		 FROM injections WHERE session_id = ? ORDER BY created_at DESC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list injections: %w", err)
	}
	defer rows.Close()

	var injections []Injection
	for rows.Next() {
		var inj Injection
		var metadata sql.NullString
		var deliveredAt sql.NullTime

		if err := rows.Scan(
			&inj.InjectionID, &inj.SessionID, &inj.UserID,
			&inj.TextLength, &metadata, &inj.Source,
			&inj.CreatedAt, &deliveredAt,
		); err != nil {
			return nil, fmt.Errorf("scan injection: %w", err)
		}

		if metadata.Valid {
			inj.Metadata = metadata.String
		}
		if deliveredAt.Valid {
			inj.DeliveredAt = &deliveredAt.Time
		}

		injections = append(injections, inj)
	}
	return injections, rows.Err()
}
