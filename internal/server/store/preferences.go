package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// UserPreferences holds a user's preference blob.
type UserPreferences struct {
	UserID      string    `json:"user_id"`
	Preferences string    `json:"preferences"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GetUserPreferences returns the preferences for a user.
// Returns empty defaults if no preferences row exists.
func (s *Store) GetUserPreferences(ctx context.Context, userID string) (*UserPreferences, error) {
	row := s.reader.QueryRowContext(ctx,
		`SELECT user_id, preferences, updated_at FROM user_preferences WHERE user_id = ?`, userID)

	var p UserPreferences
	err := row.Scan(&p.UserID, &p.Preferences, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return &UserPreferences{UserID: userID, Preferences: "{}", UpdatedAt: time.Now()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user preferences: %w", err)
	}
	return &p, nil
}

// UpsertUserPreferences inserts or updates the preferences JSON for a user.
func (s *Store) UpsertUserPreferences(ctx context.Context, userID, preferences string) error {
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO user_preferences (user_id, preferences, updated_at)
		 VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET preferences = excluded.preferences, updated_at = CURRENT_TIMESTAMP`,
		userID, preferences)
	if err != nil {
		return fmt.Errorf("upsert user preferences: %w", err)
	}
	return nil
}
