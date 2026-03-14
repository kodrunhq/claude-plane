package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/event"
)

// EventFilter holds optional filters and pagination for ListEvents.
type EventFilter struct {
	// TypePattern supports glob-like matching: "run.*" matches any event type
	// starting with "run.". Use "*" for all events, or an exact type for exact match.
	TypePattern string
	Since       time.Time
	Limit       int
	Offset      int
}

const defaultEventLimit = 50

// InsertEvent persists an event to the events table.
// Payload is serialized as JSON.
func (s *Store) InsertEvent(ctx context.Context, e event.Event) error {
	payloadJSON, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}

	_, err = s.writer.ExecContext(ctx,
		`INSERT INTO events (event_id, event_type, timestamp, source, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.EventID, e.Type, e.Timestamp.UTC(), e.Source, string(payloadJSON), time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

// ListEvents returns events matching the filter, ordered by timestamp DESC.
// Defaults to limit 50 when filter.Limit is 0.
func (s *Store) ListEvents(ctx context.Context, filter EventFilter) ([]event.Event, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultEventLimit
	}

	query := `SELECT event_id, event_type, timestamp, source, payload FROM events WHERE 1=1`
	args := make([]interface{}, 0, 4)

	if filter.TypePattern != "" && filter.TypePattern != "*" {
		if strings.Contains(filter.TypePattern, "*") {
			query += ` AND event_type LIKE ? ESCAPE '\'`
			args = append(args, typePatternToSQL(filter.TypePattern))
		} else {
			query += ` AND event_type = ?`
			args = append(args, filter.TypePattern)
		}
	}

	if !filter.Since.IsZero() {
		query += ` AND timestamp >= ?`
		args = append(args, filter.Since.UTC())
	}

	query += ` ORDER BY timestamp DESC LIMIT ? OFFSET ?`
	args = append(args, limit, filter.Offset)

	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []event.Event
	for rows.Next() {
		var e event.Event
		var payloadStr string
		if err := rows.Scan(&e.EventID, &e.Type, &e.Timestamp, &e.Source, &payloadStr); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if err := json.Unmarshal([]byte(payloadStr), &e.Payload); err != nil {
			return nil, fmt.Errorf("unmarshal event payload: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetEventByID retrieves a single event by its primary key.
func (s *Store) GetEventByID(ctx context.Context, eventID string) (*event.Event, error) {
	var e event.Event
	var payloadStr string

	err := s.reader.QueryRowContext(ctx,
		`SELECT event_id, event_type, timestamp, source, payload FROM events WHERE event_id = ?`, eventID,
	).Scan(&e.EventID, &e.Type, &e.Timestamp, &e.Source, &payloadStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event %s: %w", eventID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get event by id: %w", err)
	}

	if err := json.Unmarshal([]byte(payloadStr), &e.Payload); err != nil {
		return nil, fmt.Errorf("unmarshal event payload: %w", err)
	}
	return &e, nil
}

// PurgeEvents deletes events older than before and returns the number deleted.
func (s *Store) PurgeEvents(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.writer.ExecContext(ctx,
		`DELETE FROM events WHERE timestamp < ?`, before.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("purge events: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}

const defaultFeedLimit = 100

// ListEventsAfter returns up to limit events after the given cursor point,
// ordered ASC (oldest first). The cursor is compound (afterTimestamp, afterEventID)
// to handle timestamp ties. If afterTimestamp is zero and afterEventID is empty,
// returns the most recent limit events ordered ASC. Default limit: 100.
func (s *Store) ListEventsAfter(ctx context.Context, afterTimestamp time.Time, afterEventID string, limit int) ([]event.Event, error) {
	if limit <= 0 {
		limit = defaultFeedLimit
	}

	var (
		rows *sql.Rows
		err  error
	)

	if afterTimestamp.IsZero() && afterEventID == "" {
		// No cursor: return the most recent limit events, then re-order ASC.
		rows, err = s.reader.QueryContext(ctx,
			`SELECT event_id, event_type, timestamp, source, payload FROM (
				SELECT event_id, event_type, timestamp, source, payload FROM events
				ORDER BY timestamp DESC, event_id DESC
				LIMIT ?
			) ORDER BY timestamp ASC, event_id ASC`,
			limit,
		)
	} else {
		rows, err = s.reader.QueryContext(ctx,
			`SELECT event_id, event_type, timestamp, source, payload FROM events
			WHERE (timestamp > ? OR (timestamp = ? AND event_id > ?))
			ORDER BY timestamp ASC, event_id ASC
			LIMIT ?`,
			afterTimestamp.UTC(), afterTimestamp.UTC(), afterEventID, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list events after: %w", err)
	}
	defer rows.Close()

	var events []event.Event
	for rows.Next() {
		var e event.Event
		var payloadStr string
		if err := rows.Scan(&e.EventID, &e.Type, &e.Timestamp, &e.Source, &payloadStr); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if err := json.Unmarshal([]byte(payloadStr), &e.Payload); err != nil {
			return nil, fmt.Errorf("unmarshal event payload: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// escapeForLIKE escapes SQL LIKE special characters in s so that they are
// treated as literals. The caller must add ESCAPE '\' to the LIKE clause.
func escapeForLIKE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// typePatternToSQL converts a glob-style type pattern to a SQL LIKE pattern.
// "run.*" → "run.%", "*" → "%" (though * alone is handled separately).
// Literal %, _, and \ in the pattern are escaped before conversion so they
// cannot be exploited as LIKE wildcards. Use ESCAPE '\' in the query.
func typePatternToSQL(pattern string) string {
	if pattern == "*" {
		return "%"
	}
	// Escape SQL LIKE metacharacters first, then convert glob wildcards.
	// escapeForLIKE escapes \, %, _ — but not *, so the subsequent
	// ReplaceAll only replaces our intentional glob wildcards.
	escaped := escapeForLIKE(pattern)
	return strings.ReplaceAll(escaped, "*", "%")
}
