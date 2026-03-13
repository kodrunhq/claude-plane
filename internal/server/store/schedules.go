package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CronSchedule represents a cron-based schedule that triggers a job run.
type CronSchedule struct {
	ScheduleID      string     `json:"schedule_id"`
	JobID           string     `json:"job_id"`
	CronExpr        string     `json:"cron_expr"`
	Timezone        string     `json:"timezone"`
	Enabled         bool       `json:"enabled"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// CreateScheduleParams holds parameters for creating a cron schedule.
type CreateScheduleParams struct {
	JobID    string
	CronExpr string
	Timezone string
}

// UpdateScheduleParams holds parameters for updating a cron schedule expression and timezone.
type UpdateScheduleParams struct {
	ScheduleID string
	CronExpr   string
	Timezone   string
}

// CreateSchedule inserts a new cron schedule and returns it with a generated ID.
// Timezone defaults to "UTC" if empty. The schedule is enabled by default.
// Cron expression validation is the caller's responsibility.
func (s *Store) CreateSchedule(ctx context.Context, p CreateScheduleParams) (*CronSchedule, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	tz := p.Timezone
	if tz == "" {
		tz = "UTC"
	}

	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO cron_schedules (schedule_id, job_id, cron_expr, timezone, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, ?, ?)`,
		id, p.JobID, p.CronExpr, tz, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create schedule: %w", err)
	}

	return &CronSchedule{
		ScheduleID: id,
		JobID:      p.JobID,
		CronExpr:   p.CronExpr,
		Timezone:   tz,
		Enabled:    true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// GetSchedule retrieves a cron schedule by its ID.
// Returns ErrNotFound if no schedule exists with that ID.
func (s *Store) GetSchedule(ctx context.Context, scheduleID string) (*CronSchedule, error) {
	var sc CronSchedule
	var nextRunAt, lastTriggeredAt sql.NullTime

	err := s.reader.QueryRowContext(ctx,
		`SELECT schedule_id, job_id, cron_expr, timezone, enabled,
		        next_run_at, last_triggered_at, created_at, updated_at
		 FROM cron_schedules WHERE schedule_id = ?`,
		scheduleID,
	).Scan(
		&sc.ScheduleID, &sc.JobID, &sc.CronExpr, &sc.Timezone, &sc.Enabled,
		&nextRunAt, &lastTriggeredAt, &sc.CreatedAt, &sc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("schedule %s: %w", scheduleID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}

	if nextRunAt.Valid {
		sc.NextRunAt = &nextRunAt.Time
	}
	if lastTriggeredAt.Valid {
		sc.LastTriggeredAt = &lastTriggeredAt.Time
	}

	return &sc, nil
}

// ListSchedulesByJob returns all cron schedules for a job, ordered by created_at DESC.
func (s *Store) ListSchedulesByJob(ctx context.Context, jobID string) ([]CronSchedule, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT schedule_id, job_id, cron_expr, timezone, enabled,
		        next_run_at, last_triggered_at, created_at, updated_at
		 FROM cron_schedules WHERE job_id = ? ORDER BY created_at DESC`,
		jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("list schedules by job: %w", err)
	}
	defer rows.Close()

	return scanSchedules(rows)
}

// ListEnabledSchedules returns all enabled cron schedules across all jobs,
// ordered by next_run_at ASC (nulls last). Used by the scheduler to find
// schedules due to fire.
func (s *Store) ListEnabledSchedules(ctx context.Context) ([]CronSchedule, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT schedule_id, job_id, cron_expr, timezone, enabled,
		        next_run_at, last_triggered_at, created_at, updated_at
		 FROM cron_schedules WHERE enabled = 1 ORDER BY next_run_at IS NULL, next_run_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled schedules: %w", err)
	}
	defer rows.Close()

	return scanSchedules(rows)
}

// UpdateSchedule updates the cron expression and timezone of an existing schedule.
// Clears next_run_at so the scheduler recomputes it on reload.
// Returns ErrNotFound if no schedule exists with that ID.
func (s *Store) UpdateSchedule(ctx context.Context, p UpdateScheduleParams) (*CronSchedule, error) {
	now := time.Now().UTC()

	result, err := s.writer.ExecContext(ctx,
		`UPDATE cron_schedules SET cron_expr = ?, timezone = ?, next_run_at = NULL, updated_at = ?
		 WHERE schedule_id = ?`,
		p.CronExpr, p.Timezone, now, p.ScheduleID,
	)
	if err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, fmt.Errorf("schedule %s: %w", p.ScheduleID, ErrNotFound)
	}

	return s.GetSchedule(ctx, p.ScheduleID)
}

// SetScheduleEnabled enables or disables a cron schedule.
// When disabling, next_run_at is set to NULL.
// Returns ErrNotFound if no schedule exists with that ID.
func (s *Store) SetScheduleEnabled(ctx context.Context, scheduleID string, enabled bool) error {
	now := time.Now().UTC()

	var result sql.Result
	var err error

	if enabled {
		result, err = s.writer.ExecContext(ctx,
			`UPDATE cron_schedules SET enabled = 1, updated_at = ? WHERE schedule_id = ?`,
			now, scheduleID,
		)
	} else {
		result, err = s.writer.ExecContext(ctx,
			`UPDATE cron_schedules SET enabled = 0, next_run_at = NULL, updated_at = ? WHERE schedule_id = ?`,
			now, scheduleID,
		)
	}
	if err != nil {
		return fmt.Errorf("set schedule enabled: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("schedule %s: %w", scheduleID, ErrNotFound)
	}
	return nil
}

// UpdateScheduleTimestamps updates last_triggered_at and next_run_at for a schedule.
// This is a lightweight update called by the scheduler after triggering a run.
// Returns ErrNotFound if no schedule exists with that ID.
func (s *Store) UpdateScheduleTimestamps(ctx context.Context, scheduleID string, lastTriggered, nextRun time.Time) error {
	now := time.Now().UTC()

	result, err := s.writer.ExecContext(ctx,
		`UPDATE cron_schedules SET last_triggered_at = ?, next_run_at = ?, updated_at = ?
		 WHERE schedule_id = ?`,
		lastTriggered.UTC(), nextRun.UTC(), now, scheduleID,
	)
	if err != nil {
		return fmt.Errorf("update schedule timestamps: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("schedule %s: %w", scheduleID, ErrNotFound)
	}
	return nil
}

// DeleteSchedule removes a cron schedule by ID.
// Returns ErrNotFound if no schedule exists with that ID.
func (s *Store) DeleteSchedule(ctx context.Context, scheduleID string) error {
	result, err := s.writer.ExecContext(ctx,
		`DELETE FROM cron_schedules WHERE schedule_id = ?`,
		scheduleID,
	)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("schedule %s: %w", scheduleID, ErrNotFound)
	}
	return nil
}

// scanSchedules scans rows into a slice of CronSchedule.
func scanSchedules(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]CronSchedule, error) {
	var schedules []CronSchedule
	for rows.Next() {
		var sc CronSchedule
		var nextRunAt, lastTriggeredAt sql.NullTime

		if err := rows.Scan(
			&sc.ScheduleID, &sc.JobID, &sc.CronExpr, &sc.Timezone, &sc.Enabled,
			&nextRunAt, &lastTriggeredAt, &sc.CreatedAt, &sc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}

		if nextRunAt.Valid {
			sc.NextRunAt = &nextRunAt.Time
		}
		if lastTriggeredAt.Valid {
			sc.LastTriggeredAt = &lastTriggeredAt.Time
		}

		schedules = append(schedules, sc)
	}
	return schedules, rows.Err()
}
