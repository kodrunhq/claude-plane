package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Session represents a terminal session persisted in SQLite.
type Session struct {
	SessionID      string    `json:"session_id"`
	MachineID      string    `json:"machine_id"`
	UserID         string    `json:"user_id"`
	TemplateID     string    `json:"template_id,omitempty"`
	Command        string    `json:"command"`
	WorkingDir     string    `json:"working_dir"`
	Status         string    `json:"status"`
	Model          string    `json:"model,omitempty"`
	SkipPerms      string    `json:"skip_permissions,omitempty"`
	EnvVars        string    `json:"env_vars,omitempty"`
	Args           string    `json:"args,omitempty"`
	InitialPrompt  string    `json:"initial_prompt,omitempty"`
	// CreatedAt corresponds to the database column `started_at`.
	CreatedAt      time.Time `json:"created_at"`
	// UpdatedAt corresponds to the database column `ended_at` (or CreatedAt if not ended).
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateSession inserts a new session into the sessions table.
func (s *Store) CreateSession(sess *Session) error {
	// Use NULL for empty user_id to satisfy FK constraint
	var userID interface{} = sess.UserID
	if sess.UserID == "" {
		userID = nil
	}
	_, err := s.writer.Exec(`
		INSERT INTO sessions (session_id, machine_id, user_id, template_id, command, working_dir, status,
		                      model, skip_permissions, env_vars, args, initial_prompt, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		sess.SessionID, sess.MachineID, userID, nullIfEmpty(sess.TemplateID),
		sess.Command, sess.WorkingDir, sess.Status,
		sess.Model, sess.SkipPerms, sess.EnvVars, sess.Args, sess.InitialPrompt,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSession retrieves a session by ID.
func (s *Store) GetSession(id string) (*Session, error) {
	sess := &Session{}
	var userID, templateID sql.NullString
	// endedAt temporarily holds the database `ended_at` column, which is then
	// mapped to Session.UpdatedAt (or left equal to CreatedAt if NULL).
	var endedAt sql.NullTime
	err := s.reader.QueryRow(`
		SELECT session_id, machine_id, user_id, COALESCE(template_id, ''),
		       COALESCE(command, 'claude'), COALESCE(working_dir, ''),
		       status, COALESCE(model, ''), COALESCE(skip_permissions, ''),
		       COALESCE(env_vars, ''), COALESCE(args, ''), COALESCE(initial_prompt, ''),
		       started_at, ended_at
		FROM sessions WHERE session_id = ?`, id,
	// Note: started_at -> sess.CreatedAt, ended_at -> endedAt (-> sess.UpdatedAt).
	).Scan(&sess.SessionID, &sess.MachineID, &userID, &templateID,
		&sess.Command, &sess.WorkingDir, &sess.Status,
		&sess.Model, &sess.SkipPerms, &sess.EnvVars, &sess.Args, &sess.InitialPrompt,
		&sess.CreatedAt, &endedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if userID.Valid {
		sess.UserID = userID.String
	}
	if templateID.Valid {
		sess.TemplateID = templateID.String
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
		SELECT session_id, machine_id, user_id, COALESCE(template_id, ''),
		       COALESCE(command, 'claude'), COALESCE(working_dir, ''),
		       status, COALESCE(model, ''), COALESCE(skip_permissions, ''),
		       COALESCE(env_vars, ''), COALESCE(args, ''), COALESCE(initial_prompt, ''),
		       started_at, ended_at
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
		SELECT session_id, machine_id, user_id, COALESCE(template_id, ''),
		       COALESCE(command, 'claude'), COALESCE(working_dir, ''),
		       status, COALESCE(model, ''), COALESCE(skip_permissions, ''),
		       COALESCE(env_vars, ''), COALESCE(args, ''), COALESCE(initial_prompt, ''),
		       started_at, ended_at
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
		var userID, templateID sql.NullString
		var endedAt sql.NullTime
		if err := rows.Scan(&sess.SessionID, &sess.MachineID, &userID, &templateID,
			&sess.Command, &sess.WorkingDir, &sess.Status,
			&sess.Model, &sess.SkipPerms, &sess.EnvVars, &sess.Args, &sess.InitialPrompt,
			&sess.CreatedAt, &endedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		if userID.Valid {
			sess.UserID = userID.String
		}
		if templateID.Valid {
			sess.TemplateID = templateID.String
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
