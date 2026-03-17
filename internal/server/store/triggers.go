package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// JobTrigger represents a configured trigger that fires a job run when a matching
// event is published on the bus.
type JobTrigger struct {
	TriggerID string    `json:"trigger_id"`
	JobID     string    `json:"job_id"`
	EventType string    `json:"event_type"` // glob pattern, e.g. "run.completed", "trigger.*"
	Filter    string    `json:"filter"`     // optional JSON filter conditions
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateJobTrigger inserts a new job trigger and returns it with a generated ID.
func (s *Store) CreateJobTrigger(ctx context.Context, t JobTrigger) (*JobTrigger, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO job_triggers (trigger_id, job_id, event_type, filter, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, t.JobID, t.EventType, nullStringIfEmpty(t.Filter), t.Enabled, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create job trigger: %w", err)
	}

	result := JobTrigger{
		TriggerID: id,
		JobID:     t.JobID,
		EventType: t.EventType,
		Filter:    t.Filter,
		Enabled:   t.Enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return &result, nil
}

// ListJobTriggers returns all triggers for a specific job, ordered by created_at DESC.
func (s *Store) ListJobTriggers(ctx context.Context, jobID string) ([]JobTrigger, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT trigger_id, job_id, event_type, COALESCE(filter, ''), enabled, created_at, updated_at
		 FROM job_triggers WHERE job_id = ? ORDER BY created_at DESC`,
		jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("list job triggers: %w", err)
	}
	defer rows.Close()

	return scanTriggers(rows)
}

// DeleteJobTrigger removes a trigger by ID.
func (s *Store) DeleteJobTrigger(ctx context.Context, triggerID string) error {
	result, err := s.writer.ExecContext(ctx,
		`DELETE FROM job_triggers WHERE trigger_id = ?`, triggerID,
	)
	if err != nil {
		return fmt.Errorf("delete job trigger: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete job trigger rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("trigger %s: %w", triggerID, ErrNotFound)
	}
	return nil
}

// UpdateJobTrigger updates the event_type and filter of an existing trigger.
// Returns the updated trigger or ErrNotFound if it doesn't exist.
func (s *Store) UpdateJobTrigger(ctx context.Context, triggerID, eventType, filter string) (*JobTrigger, error) {
	now := time.Now().UTC()

	result, err := s.writer.ExecContext(ctx,
		`UPDATE job_triggers SET event_type = ?, filter = ?, updated_at = ? WHERE trigger_id = ?`,
		eventType, nullStringIfEmpty(filter), now, triggerID,
	)
	if err != nil {
		return nil, fmt.Errorf("update job trigger: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("update job trigger rows affected: %w", err)
	}
	if affected == 0 {
		return nil, fmt.Errorf("trigger %s: %w", triggerID, ErrNotFound)
	}

	row := s.reader.QueryRowContext(ctx,
		`SELECT trigger_id, job_id, event_type, COALESCE(filter, ''), enabled, created_at, updated_at
		 FROM job_triggers WHERE trigger_id = ?`, triggerID,
	)
	var t JobTrigger
	if err := row.Scan(&t.TriggerID, &t.JobID, &t.EventType, &t.Filter, &t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, fmt.Errorf("read updated trigger: %w", err)
	}
	return &t, nil
}

// ToggleJobTrigger flips the enabled flag of a trigger and returns the updated trigger.
func (s *Store) ToggleJobTrigger(ctx context.Context, triggerID string) (*JobTrigger, error) {
	now := time.Now().UTC()

	result, err := s.writer.ExecContext(ctx,
		`UPDATE job_triggers SET enabled = NOT enabled, updated_at = ? WHERE trigger_id = ?`,
		now, triggerID,
	)
	if err != nil {
		return nil, fmt.Errorf("toggle job trigger: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("toggle job trigger rows affected: %w", err)
	}
	if affected == 0 {
		return nil, fmt.Errorf("trigger %s: %w", triggerID, ErrNotFound)
	}

	row := s.reader.QueryRowContext(ctx,
		`SELECT trigger_id, job_id, event_type, COALESCE(filter, ''), enabled, created_at, updated_at
		 FROM job_triggers WHERE trigger_id = ?`, triggerID,
	)
	var t JobTrigger
	if err := row.Scan(&t.TriggerID, &t.JobID, &t.EventType, &t.Filter, &t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, fmt.Errorf("read toggled trigger: %w", err)
	}
	return &t, nil
}

// GetJobTrigger returns a single trigger by ID or ErrNotFound.
func (s *Store) GetJobTrigger(ctx context.Context, triggerID string) (*JobTrigger, error) {
	row := s.reader.QueryRowContext(ctx,
		`SELECT trigger_id, job_id, event_type, COALESCE(filter, ''), enabled, created_at, updated_at
		 FROM job_triggers WHERE trigger_id = ?`, triggerID,
	)
	var t JobTrigger
	if err := row.Scan(&t.TriggerID, &t.JobID, &t.EventType, &t.Filter, &t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("trigger %s: %w", triggerID, ErrNotFound)
		}
		return nil, fmt.Errorf("get job trigger: %w", err)
	}
	return &t, nil
}

// ListEnabledTriggers returns all enabled triggers across all jobs.
// Used by the trigger subscriber to match incoming events.
func (s *Store) ListEnabledTriggers(ctx context.Context) ([]JobTrigger, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT trigger_id, job_id, event_type, COALESCE(filter, ''), enabled, created_at, updated_at
		 FROM job_triggers WHERE enabled = 1 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled triggers: %w", err)
	}
	defer rows.Close()

	return scanTriggers(rows)
}

// JobTriggerWithJob includes the parent job's name for display in global lists.
type JobTriggerWithJob struct {
	JobTrigger
	JobName string `json:"job_name"`
}

// ListAllTriggers returns all triggers across all jobs, with job names.
func (s *Store) ListAllTriggers(ctx context.Context) ([]JobTriggerWithJob, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT t.trigger_id, t.job_id, t.event_type, COALESCE(t.filter, ''), t.enabled,
		        t.created_at, t.updated_at, j.name as job_name
		 FROM job_triggers t
		 JOIN jobs j ON t.job_id = j.job_id
		 ORDER BY t.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list all triggers: %w", err)
	}
	defer rows.Close()

	var triggers []JobTriggerWithJob
	for rows.Next() {
		var t JobTriggerWithJob
		if err := rows.Scan(
			&t.TriggerID, &t.JobID, &t.EventType, &t.Filter, &t.Enabled,
			&t.CreatedAt, &t.UpdatedAt, &t.JobName,
		); err != nil {
			return nil, fmt.Errorf("scan trigger: %w", err)
		}
		triggers = append(triggers, t)
	}
	return triggers, rows.Err()
}

// scanTriggers scans rows into a slice of JobTrigger.
func scanTriggers(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]JobTrigger, error) {
	var triggers []JobTrigger
	for rows.Next() {
		var t JobTrigger
		if err := rows.Scan(
			&t.TriggerID, &t.JobID, &t.EventType, &t.Filter,
			&t.Enabled, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan trigger: %w", err)
		}
		triggers = append(triggers, t)
	}
	return triggers, rows.Err()
}
