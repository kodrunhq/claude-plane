package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ContentLine represents a single line of session terminal output.
type ContentLine struct {
	SessionID  string
	LineNumber int
	Content    string
}

// ContentSearchResult represents a search match with context.
type ContentSearchResult struct {
	SessionID     string `json:"session_id"`
	MachineID     string `json:"machine_id"`
	Line          string `json:"line"`
	LineNumber    int    `json:"line_number"`
	ContextBefore string `json:"context_before"`
	ContextAfter  string `json:"context_after"`
	TimestampMs   int64  `json:"timestamp_ms"`
	SessionStatus string `json:"session_status,omitempty"`
}

// InsertContentLines bulk-inserts lines into session_lines.
// The FTS5 index is updated automatically via the session_lines_ai trigger.
func (s *Store) InsertContentLines(ctx context.Context, lines []ContentLine) error {
	if len(lines) == 0 {
		return nil
	}
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO session_lines (session_id, line_number, content) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, l := range lines {
		if _, err := stmt.ExecContext(ctx, l.SessionID, l.LineNumber, l.Content); err != nil {
			return fmt.Errorf("insert line: %w", err)
		}
	}
	return tx.Commit()
}

// UpsertContentMeta creates or updates the content metadata for a session.
func (s *Store) UpsertContentMeta(ctx context.Context, sessionID string, lineCount int) error {
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO session_content_meta (session_id, line_count)
		 VALUES (?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET line_count = excluded.line_count`,
		sessionID, lineCount)
	if err != nil {
		return fmt.Errorf("upsert content meta: %w", err)
	}
	return nil
}

// sanitizeFTS5Query wraps the user query in double quotes so FTS5 treats it
// as a phrase query, preventing operator injection. Internal quotes are escaped.
func sanitizeFTS5Query(q string) string {
	escaped := strings.ReplaceAll(q, `"`, `""`)
	return `"` + escaped + `"`
}

// SearchContent performs a full-text search across session terminal output.
// When userID is non-empty, results are scoped to sessions owned by that user.
func (s *Store) SearchContent(ctx context.Context, query string, limit, offset int, userID string) ([]ContentSearchResult, error) {
	safe := sanitizeFTS5Query(query)

	baseQuery := `SELECT sl.session_id, sl.line_number, sl.content,
		        s.machine_id, s.status, s.started_at
		 FROM session_content sc
		 JOIN session_lines sl ON sc.rowid = sl.rowid
		 JOIN sessions s ON sl.session_id = s.session_id
		 WHERE session_content MATCH ?`
	args := []any{safe}

	if userID != "" {
		baseQuery += ` AND s.user_id = ?`
		args = append(args, userID)
	}

	baseQuery += ` ORDER BY bm25(session_content) LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.reader.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search content: %w", err)
	}
	defer rows.Close()

	var results []ContentSearchResult
	for rows.Next() {
		var r ContentSearchResult
		var startedAt sql.NullTime
		if err := rows.Scan(&r.SessionID, &r.LineNumber, &r.Line,
			&r.MachineID, &r.SessionStatus, &startedAt); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		if startedAt.Valid {
			r.TimestampMs = startedAt.Time.UnixMilli()
		}
		results = append(results, r)
	}
	if results == nil {
		results = []ContentSearchResult{}
	}
	return results, rows.Err()
}

// FetchContextLines retrieves lines around a match for context display.
func (s *Store) FetchContextLines(ctx context.Context, sessionID string, lineNumber, before, after int) (contextBefore, contextAfter string, err error) {
	startLine := lineNumber - before
	if startLine < 1 {
		startLine = 1
	}
	endLine := lineNumber + after

	rows, err := s.reader.QueryContext(ctx,
		`SELECT line_number, content FROM session_lines
		 WHERE session_id = ? AND line_number BETWEEN ? AND ?
		 ORDER BY line_number`,
		sessionID, startLine, endLine)
	if err != nil {
		return "", "", fmt.Errorf("fetch context: %w", err)
	}
	defer rows.Close()

	var beforeLines, afterLines []string
	for rows.Next() {
		var ln int
		var content string
		if err := rows.Scan(&ln, &content); err != nil {
			return "", "", fmt.Errorf("scan context line: %w", err)
		}
		if ln < lineNumber {
			beforeLines = append(beforeLines, content)
		} else if ln > lineNumber {
			afterLines = append(afterLines, content)
		}
	}
	return strings.Join(beforeLines, "\n"), strings.Join(afterLines, "\n"), rows.Err()
}

// DeleteSessionContent removes all content for a session (lines + meta).
func (s *Store) DeleteSessionContent(ctx context.Context, sessionID string) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM session_lines WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("delete lines: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM session_content_meta WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("delete meta: %w", err)
	}
	return tx.Commit()
}

// ListExpiredContentSessions returns sessions eligible for content cleanup.
func (s *Store) ListExpiredContentSessions(ctx context.Context, retentionDays int) ([]struct{ SessionID, MachineID string }, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT s.session_id, s.machine_id FROM sessions s
		 JOIN session_content_meta m ON s.session_id = m.session_id
		 WHERE s.status IN ('completed', 'failed', 'terminated')
		 AND s.ended_at < datetime('now', '-' || ? || ' days')`,
		retentionDays)
	if err != nil {
		return nil, fmt.Errorf("list expired: %w", err)
	}
	defer rows.Close()

	var results []struct{ SessionID, MachineID string }
	for rows.Next() {
		var r struct{ SessionID, MachineID string }
		if err := rows.Scan(&r.SessionID, &r.MachineID); err != nil {
			return nil, fmt.Errorf("scan expired: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// OptimizeFTS runs FTS5 merge optimization after bulk deletes.
func (s *Store) OptimizeFTS(ctx context.Context) error {
	_, err := s.writer.ExecContext(ctx, "INSERT INTO session_content(session_content) VALUES('optimize')")
	if err != nil {
		return fmt.Errorf("optimize FTS: %w", err)
	}
	return nil
}

// --- Server Settings ---

// GetSetting reads a server setting by key. Returns empty string if not found.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.reader.QueryRowContext(ctx,
		"SELECT value FROM server_settings WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get setting %s: %w", key, err)
	}
	return value, nil
}

// SetSetting upserts a server setting.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO server_settings (key, value, updated_at)
		 VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value)
	if err != nil {
		return fmt.Errorf("set setting %s: %w", key, err)
	}
	return nil
}

// GetAllSettings returns all server settings as a map.
func (s *Store) GetAllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.reader.QueryContext(ctx, "SELECT key, value FROM server_settings")
	if err != nil {
		return nil, fmt.Errorf("get all settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		settings[k] = v
	}
	return settings, rows.Err()
}

// --- Pending Cleanups ---

// AddPendingCleanup records a cleanup for offline agents.
func (s *Store) AddPendingCleanup(ctx context.Context, sessionID, machineID string) error {
	id := uuid.New().String()
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO pending_cleanups (cleanup_id, session_id, machine_id) VALUES (?, ?, ?)`,
		id, sessionID, machineID)
	if err != nil {
		return fmt.Errorf("add pending cleanup: %w", err)
	}
	return nil
}

// ListPendingCleanups returns pending cleanups for a machine.
func (s *Store) ListPendingCleanups(ctx context.Context, machineID string) ([]string, error) {
	rows, err := s.reader.QueryContext(ctx,
		"SELECT session_id FROM pending_cleanups WHERE machine_id = ?", machineID)
	if err != nil {
		return nil, fmt.Errorf("list pending cleanups: %w", err)
	}
	defer rows.Close()

	var sessionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan cleanup: %w", err)
		}
		sessionIDs = append(sessionIDs, id)
	}
	return sessionIDs, rows.Err()
}

// DeletePendingCleanups removes pending cleanups for a machine.
func (s *Store) DeletePendingCleanups(ctx context.Context, machineID string) error {
	_, err := s.writer.ExecContext(ctx,
		"DELETE FROM pending_cleanups WHERE machine_id = ?", machineID)
	if err != nil {
		return fmt.Errorf("delete pending cleanups: %w", err)
	}
	return nil
}
