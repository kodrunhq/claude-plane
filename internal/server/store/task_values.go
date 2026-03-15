package store

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// TaskValue represents a key-value pair stored by a run step for inter-step data passing.
type TaskValue struct {
	RunStepID string `json:"run_step_id"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	CreatedAt string `json:"created_at"`
}

const (
	maxTaskValueSize     = 32 * 1024
	maxTaskValuesPerStep = 20
	maxTaskValueKeyLen   = 64
)

var taskValueKeyPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

// SetTaskValue upserts a task value for a run step. Validates key format/length,
// value size, and per-step count limit.
func (s *Store) SetTaskValue(ctx context.Context, runStepID, key, value string) error {
	// Validate key
	if key == "" {
		return fmt.Errorf("task value key must not be empty")
	}
	if len(key) > maxTaskValueKeyLen {
		return fmt.Errorf("task value key exceeds max length of %d", maxTaskValueKeyLen)
	}
	if !taskValueKeyPattern.MatchString(key) {
		return fmt.Errorf("task value key %q is invalid: must match %s", key, taskValueKeyPattern.String())
	}

	// Validate value size
	if len(value) > maxTaskValueSize {
		return fmt.Errorf("task value exceeds max size of %d bytes", maxTaskValueSize)
	}

	// Check count limit (only for new keys, not upserts)
	var existingCount int
	err := s.reader.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM run_step_values WHERE run_step_id = ?", runStepID,
	).Scan(&existingCount)
	if err != nil {
		return fmt.Errorf("count task values: %w", err)
	}

	// Check if this key already exists (upsert doesn't count against limit)
	var exists int
	err = s.reader.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM run_step_values WHERE run_step_id = ? AND key = ?",
		runStepID, key,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check existing task value: %w", err)
	}

	if exists == 0 && existingCount >= maxTaskValuesPerStep {
		return fmt.Errorf("run step %s has reached the maximum of %d task values", runStepID, maxTaskValuesPerStep)
	}

	// Upsert
	_, err = s.writer.ExecContext(ctx,
		`INSERT INTO run_step_values (run_step_id, key, value, created_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(run_step_id, key) DO UPDATE SET value = excluded.value, created_at = excluded.created_at`,
		runStepID, key, value,
	)
	if err != nil {
		return fmt.Errorf("set task value: %w", err)
	}
	return nil
}

// GetTaskValues returns all task values for a run step.
func (s *Store) GetTaskValues(ctx context.Context, runStepID string) ([]TaskValue, error) {
	rows, err := s.reader.QueryContext(ctx,
		"SELECT run_step_id, key, value, created_at FROM run_step_values WHERE run_step_id = ? ORDER BY key",
		runStepID,
	)
	if err != nil {
		return nil, fmt.Errorf("get task values: %w", err)
	}
	defer rows.Close()

	var values []TaskValue
	for rows.Next() {
		var v TaskValue
		if err := rows.Scan(&v.RunStepID, &v.Key, &v.Value, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task value: %w", err)
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

// GetTaskValuesForSteps returns all task values for multiple run steps (bulk fetch).
func (s *Store) GetTaskValuesForSteps(ctx context.Context, runStepIDs []string) ([]TaskValue, error) {
	if len(runStepIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(runStepIDs))
	args := make([]interface{}, len(runStepIDs))
	for i, id := range runStepIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		"SELECT run_step_id, key, value, created_at FROM run_step_values WHERE run_step_id IN (%s) ORDER BY run_step_id, key",
		strings.Join(placeholders, ","),
	)

	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get task values for steps: %w", err)
	}
	defer rows.Close()

	var values []TaskValue
	for rows.Next() {
		var v TaskValue
		if err := rows.Scan(&v.RunStepID, &v.Key, &v.Value, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task value: %w", err)
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

// DeleteTaskValuesForStep removes all task values for a run step.
func (s *Store) DeleteTaskValuesForStep(ctx context.Context, runStepID string) error {
	_, err := s.writer.ExecContext(ctx,
		"DELETE FROM run_step_values WHERE run_step_id = ?", runStepID,
	)
	if err != nil {
		return fmt.Errorf("delete task values for step: %w", err)
	}
	return nil
}
