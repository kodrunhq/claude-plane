package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Session represents a terminal session persisted in SQLite.
type Session struct {
	SessionID  string    `json:"session_id"`
	MachineID  string    `json:"machine_id"`
	UserID     string    `json:"user_id"`
	Command    string    `json:"command"`
	WorkingDir string    `json:"working_dir"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateSession inserts a new session into the sessions table.
func (s *Store) CreateSession(sess *Session) error {
	// Use NULL for empty user_id to satisfy FK constraint
	var userID interface{} = sess.UserID
	if sess.UserID == "" {
		userID = nil
	}
	_, err := s.writer.Exec(`
		INSERT INTO sessions (session_id, machine_id, user_id, command, working_dir, status, started_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		sess.SessionID, sess.MachineID, userID, sess.Command, sess.WorkingDir, sess.Status,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSession retrieves a session by ID.
func (s *Store) GetSession(id string) (*Session, error) {
	sess := &Session{}
	var userID sql.NullString
	var endedAt sql.NullTime
	err := s.reader.QueryRow(`
		SELECT session_id, machine_id, user_id, COALESCE(command, 'claude'),
		       COALESCE(working_dir, ''), status, started_at, ended_at
		FROM sessions WHERE session_id = ?`, id,
	).Scan(&sess.SessionID, &sess.MachineID, &userID, &sess.Command,
		&sess.WorkingDir, &sess.Status, &sess.CreatedAt, &endedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if userID.Valid {
		sess.UserID = userID.String
	}
	if endedAt.Valid {
		sess.UpdatedAt = endedAt.Time
	} else {
		sess.UpdatedAt = sess.CreatedAt
	}
	return sess, nil
}

// ListSessions returns all sessions ordered by creation time descending.
func (s *Store) ListSessions() ([]Session, error) {
	rows, err := s.reader.Query(`
		SELECT session_id, machine_id, user_id, COALESCE(command, 'claude'),
		       COALESCE(working_dir, ''), status, started_at, ended_at
		FROM sessions ORDER BY started_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

// ListSessionsByMachine returns sessions for a specific machine.
func (s *Store) ListSessionsByMachine(machineID string) ([]Session, error) {
	rows, err := s.reader.Query(`
		SELECT session_id, machine_id, user_id, COALESCE(command, 'claude'),
		       COALESCE(working_dir, ''), status, started_at, ended_at
		FROM sessions WHERE machine_id = ? ORDER BY started_at DESC`, machineID)
	if err != nil {
		return nil, fmt.Errorf("list sessions by machine: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

// UpdateSessionStatus updates the status for a session.
// Sets ended_at only when transitioning to a terminal state.
func (s *Store) UpdateSessionStatus(id, status string) error {
	var query string
	switch status {
	case StatusCompleted, StatusFailed, StatusTerminated:
		query = `UPDATE sessions SET status = ?, ended_at = CURRENT_TIMESTAMP WHERE session_id = ?`
	default:
		query = `UPDATE sessions SET status = ? WHERE session_id = ?`
	}
	result, err := s.writer.Exec(query, status, id)
	if err != nil {
		return fmt.Errorf("update session status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}
	return nil
}

// UpdateSessionStatusIfNotTerminal updates the session status only if it is not
// already in a terminal state (completed, failed, terminated). This prevents
// agent exit events from overwriting user-initiated terminations.
func (s *Store) UpdateSessionStatusIfNotTerminal(id, status string) error {
	query := `UPDATE sessions SET status = ?, ended_at = CURRENT_TIMESTAMP
		WHERE session_id = ? AND status NOT IN (?, ?, ?)`
	result, err := s.writer.Exec(query, status, id, StatusCompleted, StatusFailed, StatusTerminated)
	if err != nil {
		return fmt.Errorf("update session status if not terminal: %w", err)
	}
	// Zero rows affected is expected if the session is already terminal — not an error.
	_ = result
	return nil
}

// DeleteSession removes a session from the table.
func (s *Store) DeleteSession(id string) error {
	result, err := s.writer.Exec(`DELETE FROM sessions WHERE session_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}
	return nil
}

func scanSessions(rows *sql.Rows) ([]Session, error) {
	var sessions []Session
	for rows.Next() {
		var sess Session
		var userID sql.NullString
		var endedAt sql.NullTime
		if err := rows.Scan(&sess.SessionID, &sess.MachineID, &userID, &sess.Command,
			&sess.WorkingDir, &sess.Status, &sess.CreatedAt, &endedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		if userID.Valid {
			sess.UserID = userID.String
		}
		if endedAt.Valid {
			sess.UpdatedAt = endedAt.Time
		} else {
			sess.UpdatedAt = sess.CreatedAt
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}
