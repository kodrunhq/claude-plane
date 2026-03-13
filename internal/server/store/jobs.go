package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

// CreateStepParams holds parameters for creating a step.
type CreateStepParams struct {
	JobID          string
	Name           string
	Prompt         string
	MachineID      string
	WorkingDir     string
	Command        string
	Args           string
	TimeoutSeconds int
	SortOrder      int
	OnFailure      string
}

// UpdateStepParams holds parameters for updating a step.
type UpdateStepParams struct {
	StepID         string
	Name           string
	Prompt         string
	MachineID      string
	WorkingDir     string
	Command        string
	Args           string
	TimeoutSeconds int
	SortOrder      int
	OnFailure      string
}

// ListRunsOptions holds optional filters and pagination for ListAllRuns.
type ListRunsOptions struct {
	JobID       string
	Status      string
	TriggerType string
	Limit       int
	Offset      int
}

// RunWithJobName embeds Run and adds the human-readable job name.
type RunWithJobName struct {
	Run
	JobName string `json:"job_name"`
}

// JobStoreIface defines the interface for job-related database operations.
// Used by the orchestrator package for dependency injection and testability.
type JobStoreIface interface {
	CreateJob(ctx context.Context, name, description, userID string) (*Job, error)
	GetJob(ctx context.Context, jobID string) (*JobDetail, error)
	ListJobs(ctx context.Context) ([]Job, error)
	ListJobsByUser(ctx context.Context, userID string) ([]Job, error)
	DeleteJob(ctx context.Context, jobID string) error
	UpdateJob(ctx context.Context, jobID, name, description string) (*Job, error)
	CreateStep(ctx context.Context, p CreateStepParams) (*Step, error)
	UpdateStep(ctx context.Context, p UpdateStepParams) error
	DeleteStep(ctx context.Context, stepID string) error
	AddDependency(ctx context.Context, stepID, dependsOn string) error
	RemoveDependency(ctx context.Context, stepID, dependsOn string) error
	GetStepsWithDeps(ctx context.Context, jobID string) ([]Step, []StepDependency, error)
	CreateRun(ctx context.Context, jobID, triggerType string) (*Run, error)
	InsertRunSteps(ctx context.Context, runID string, steps []Step) error
	GetRunWithSteps(ctx context.Context, runID string) (*RunDetail, error)
	UpdateRunStepStatus(ctx context.Context, runStepID, status, sessionID string, exitCode int) error
	UpdateRunStatus(ctx context.Context, runID, status string) error
	ListRuns(ctx context.Context, jobID string) ([]Run, error)
	ListAllRuns(ctx context.Context, opts ListRunsOptions) ([]RunWithJobName, error)
}

// Compile-time check that Store implements JobStoreIface.
var _ JobStoreIface = (*Store)(nil)

// Job represents a reusable job definition.
type Job struct {
	JobID       string    `json:"job_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	UserID      string    `json:"user_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Step represents a step within a job.
type Step struct {
	StepID         string `json:"step_id"`
	JobID          string `json:"job_id"`
	Name           string `json:"name"`
	Prompt         string `json:"prompt"`
	MachineID      string `json:"machine_id"`
	WorkingDir     string `json:"working_dir"`
	Command        string `json:"command"`
	Args           string `json:"args"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	SortOrder      int    `json:"sort_order"`
	OnFailure      string `json:"on_failure"`
}

// StepDependency represents a dependency edge in the step DAG.
type StepDependency struct {
	StepID    string `json:"step_id"`
	DependsOn string `json:"depends_on"`
}

// Run represents a specific execution of a job.
type Run struct {
	RunID       string     `json:"run_id"`
	JobID       string     `json:"job_id"`
	Status      string     `json:"status"`
	TriggerType string     `json:"trigger_type"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// RunStep represents an instance of a step within a specific run.
type RunStep struct {
	RunStepID          string     `json:"run_step_id"`
	RunID              string     `json:"run_id"`
	StepID             string     `json:"step_id"`
	Status             string     `json:"status"`
	SessionID          string     `json:"session_id,omitempty"`
	MachineID          string     `json:"machine_id,omitempty"`
	ExitCode           *int       `json:"exit_code,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	PromptSnapshot     string     `json:"prompt_snapshot"`
	MachineIDSnapshot  string     `json:"machine_id_snapshot"`
	WorkingDirSnapshot string     `json:"working_dir_snapshot"`
	CommandSnapshot    string     `json:"command_snapshot"`
	ArgsSnapshot       string     `json:"args_snapshot"`
	OnFailure          string     `json:"on_failure,omitempty"`
}

// JobDetail is a job with its steps and dependency edges.
type JobDetail struct {
	Job          Job              `json:"job"`
	Steps        []Step           `json:"steps"`
	Dependencies []StepDependency `json:"dependencies"`
}

// RunDetail is a run with all its run steps.
type RunDetail struct {
	Run      Run       `json:"run"`
	RunSteps []RunStep `json:"run_steps"`
}

// CreateJob inserts a new job and returns it.
func (s *Store) CreateJob(ctx context.Context, name, description, userID string) (*Job, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO jobs (job_id, name, description, user_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, name, description, nullIfEmpty(userID), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	return &Job{
		JobID:       id,
		Name:        name,
		Description: description,
		UserID:      userID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetJob retrieves a job by ID with its steps and dependency edges.
func (s *Store) GetJob(ctx context.Context, jobID string) (*JobDetail, error) {
	var job Job
	var desc sql.NullString
	var userID sql.NullString
	err := s.reader.QueryRowContext(ctx,
		`SELECT job_id, name, description, user_id, created_at, updated_at FROM jobs WHERE job_id = ?`, jobID,
	).Scan(&job.JobID, &job.Name, &desc, &userID, &job.CreatedAt, &job.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job %s: %w", jobID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	if desc.Valid {
		job.Description = desc.String
	}
	if userID.Valid {
		job.UserID = userID.String
	}

	steps, deps, err := s.GetStepsWithDeps(ctx, jobID)
	if err != nil {
		return nil, err
	}

	return &JobDetail{
		Job:          job,
		Steps:        steps,
		Dependencies: deps,
	}, nil
}

// ListJobs returns all jobs ordered by created_at DESC.
func (s *Store) ListJobs(ctx context.Context) ([]Job, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT job_id, name, description, user_id, created_at, updated_at FROM jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var desc, userID sql.NullString
		if err := rows.Scan(&j.JobID, &j.Name, &desc, &userID, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		if desc.Valid {
			j.Description = desc.String
		}
		if userID.Valid {
			j.UserID = userID.String
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// ListJobsByUser returns jobs owned by a specific user, ordered by created_at DESC.
func (s *Store) ListJobsByUser(ctx context.Context, userID string) ([]Job, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT job_id, name, description, user_id, created_at, updated_at FROM jobs WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list jobs by user: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var desc, uid sql.NullString
		if err := rows.Scan(&j.JobID, &j.Name, &desc, &uid, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		if desc.Valid {
			j.Description = desc.String
		}
		if uid.Valid {
			j.UserID = uid.String
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// DeleteJob removes a job and cascades to steps, dependencies, runs, run_steps.
// All deletes are wrapped in a transaction to prevent partial cleanup.
func (s *Store) DeleteJob(ctx context.Context, jobID string) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete run_steps via runs
	_, err = tx.ExecContext(ctx,
		`DELETE FROM run_steps WHERE run_id IN (SELECT run_id FROM runs WHERE job_id = ?)`, jobID)
	if err != nil {
		return fmt.Errorf("delete run_steps: %w", err)
	}
	// Delete runs
	_, err = tx.ExecContext(ctx, `DELETE FROM runs WHERE job_id = ?`, jobID)
	if err != nil {
		return fmt.Errorf("delete runs: %w", err)
	}
	// Delete job (cascades to steps and step_dependencies via ON DELETE CASCADE)
	result, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE job_id = ?`, jobID)
	if err != nil {
		return fmt.Errorf("delete job: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("job %s: %w", jobID, ErrNotFound)
	}
	return tx.Commit()
}

// UpdateJob updates a job's name and description and returns the updated job.
func (s *Store) UpdateJob(ctx context.Context, jobID, name, description string) (*Job, error) {
	result, err := s.writer.ExecContext(ctx,
		`UPDATE jobs SET name = ?, description = ?, updated_at = CURRENT_TIMESTAMP WHERE job_id = ?`,
		name, nullIfEmpty(description), jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("update job: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, fmt.Errorf("job %s: %w", jobID, ErrNotFound)
	}

	var job Job
	var desc sql.NullString
	var userID sql.NullString
	err = s.reader.QueryRowContext(ctx,
		`SELECT job_id, name, description, user_id, created_at, updated_at FROM jobs WHERE job_id = ?`, jobID,
	).Scan(&job.JobID, &job.Name, &desc, &userID, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("read updated job: %w", err)
	}
	if desc.Valid {
		job.Description = desc.String
	}
	if userID.Valid {
		job.UserID = userID.String
	}
	return &job, nil
}

// CreateStep inserts a step for a job and returns it.
func (s *Store) CreateStep(ctx context.Context, p CreateStepParams) (*Step, error) {
	id := uuid.New().String()
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO steps (step_id, job_id, name, prompt, machine_id, working_dir, command, args, timeout_seconds, sort_order, on_failure)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, p.JobID, p.Name, p.Prompt, nullIfEmpty(p.MachineID), p.WorkingDir, p.Command, p.Args, p.TimeoutSeconds, p.SortOrder, p.OnFailure,
	)
	if err != nil {
		return nil, fmt.Errorf("create step: %w", err)
	}
	return &Step{
		StepID:         id,
		JobID:          p.JobID,
		Name:           p.Name,
		Prompt:         p.Prompt,
		MachineID:      p.MachineID,
		WorkingDir:     p.WorkingDir,
		Command:        p.Command,
		Args:           p.Args,
		TimeoutSeconds: p.TimeoutSeconds,
		SortOrder:      p.SortOrder,
		OnFailure:      p.OnFailure,
	}, nil
}

// UpdateStep modifies step fields.
func (s *Store) UpdateStep(ctx context.Context, p UpdateStepParams) error {
	result, err := s.writer.ExecContext(ctx,
		`UPDATE steps SET name = ?, prompt = ?, machine_id = ?, working_dir = ?, command = ?, args = ?,
		 timeout_seconds = ?, sort_order = ?, on_failure = ? WHERE step_id = ?`,
		p.Name, p.Prompt, nullIfEmpty(p.MachineID), p.WorkingDir, p.Command, p.Args, p.TimeoutSeconds, p.SortOrder, p.OnFailure, p.StepID,
	)
	if err != nil {
		return fmt.Errorf("update step: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("step %s: %w", p.StepID, ErrNotFound)
	}
	return nil
}

// DeleteStep removes a step and its dependency edges (CASCADE).
func (s *Store) DeleteStep(ctx context.Context, stepID string) error {
	result, err := s.writer.ExecContext(ctx, `DELETE FROM steps WHERE step_id = ?`, stepID)
	if err != nil {
		return fmt.Errorf("delete step: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("step %s: %w", stepID, ErrNotFound)
	}
	return nil
}

// AddDependency inserts a dependency edge. Rejects self-references.
func (s *Store) AddDependency(ctx context.Context, stepID, dependsOn string) error {
	if stepID == dependsOn {
		return fmt.Errorf("self-reference dependency not allowed: %s", stepID)
	}
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO step_dependencies (step_id, depends_on) VALUES (?, ?)`,
		stepID, dependsOn,
	)
	if err != nil {
		return fmt.Errorf("add dependency: %w", err)
	}
	return nil
}

// RemoveDependency deletes a dependency edge.
func (s *Store) RemoveDependency(ctx context.Context, stepID, dependsOn string) error {
	result, err := s.writer.ExecContext(ctx,
		`DELETE FROM step_dependencies WHERE step_id = ? AND depends_on = ?`,
		stepID, dependsOn,
	)
	if err != nil {
		return fmt.Errorf("remove dependency: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("dependency %s -> %s: %w", stepID, dependsOn, ErrNotFound)
	}
	return nil
}

// GetStepsWithDeps returns all steps for a job with their dependency edges.
func (s *Store) GetStepsWithDeps(ctx context.Context, jobID string) ([]Step, []StepDependency, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT step_id, job_id, name, prompt, COALESCE(machine_id, ''), COALESCE(working_dir, ''),
		        COALESCE(command, 'claude'), COALESCE(args, ''), COALESCE(timeout_seconds, 0),
		        sort_order, COALESCE(on_failure, 'fail_run')
		 FROM steps WHERE job_id = ? ORDER BY sort_order`, jobID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("get steps: %w", err)
	}
	defer rows.Close()

	var steps []Step
	for rows.Next() {
		var st Step
		if err := rows.Scan(&st.StepID, &st.JobID, &st.Name, &st.Prompt, &st.MachineID,
			&st.WorkingDir, &st.Command, &st.Args, &st.TimeoutSeconds, &st.SortOrder, &st.OnFailure); err != nil {
			return nil, nil, fmt.Errorf("scan step: %w", err)
		}
		steps = append(steps, st)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	depRows, err := s.reader.QueryContext(ctx,
		`SELECT sd.step_id, sd.depends_on FROM step_dependencies sd
		 JOIN steps s ON sd.step_id = s.step_id WHERE s.job_id = ?`, jobID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("get dependencies: %w", err)
	}
	defer depRows.Close()

	var deps []StepDependency
	for depRows.Next() {
		var d StepDependency
		if err := depRows.Scan(&d.StepID, &d.DependsOn); err != nil {
			return nil, nil, fmt.Errorf("scan dependency: %w", err)
		}
		deps = append(deps, d)
	}
	return steps, deps, depRows.Err()
}

// CreateRun inserts a run row for a job.
func (s *Store) CreateRun(ctx context.Context, jobID, triggerType string) (*Run, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO runs (run_id, job_id, status, trigger_type, created_at) VALUES (?, ?, 'pending', ?, ?)`,
		id, jobID, triggerType, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}
	return &Run{
		RunID:       id,
		JobID:       jobID,
		Status:      StatusPending,
		TriggerType: triggerType,
		CreatedAt:   now,
	}, nil
}

// InsertRunSteps bulk-inserts run_step rows with snapshot fields copied from steps.
// All inserts are wrapped in a transaction to prevent partial state on failure.
func (s *Store) InsertRunSteps(ctx context.Context, runID string, steps []Step) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, st := range steps {
		id := uuid.New().String()
		_, err := tx.ExecContext(ctx,
			`INSERT INTO run_steps (run_step_id, run_id, step_id, status, machine_id,
			 prompt_snapshot, machine_id_snapshot, working_dir_snapshot, command_snapshot, args_snapshot)
			 VALUES (?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?)`,
			id, runID, st.StepID, nullIfEmpty(st.MachineID),
			st.Prompt, st.MachineID, st.WorkingDir, st.Command, st.Args,
		)
		if err != nil {
			return fmt.Errorf("insert run step for %s: %w", st.StepID, err)
		}
	}
	return tx.Commit()
}

// GetRunWithSteps retrieves a run with all its run steps.
func (s *Store) GetRunWithSteps(ctx context.Context, runID string) (*RunDetail, error) {
	var run Run
	var startedAt, completedAt sql.NullTime
	err := s.reader.QueryRowContext(ctx,
		`SELECT run_id, job_id, status, trigger_type, started_at, ended_at, created_at
		 FROM runs WHERE run_id = ?`, runID,
	).Scan(&run.RunID, &run.JobID, &run.Status, &run.TriggerType, &startedAt, &completedAt, &run.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run %s: %w", runID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}

	rows, err := s.reader.QueryContext(ctx,
		`SELECT run_step_id, run_id, step_id, status, COALESCE(session_id, ''),
		        COALESCE(machine_id, ''), exit_code, started_at, ended_at,
		        COALESCE(prompt_snapshot, ''), COALESCE(machine_id_snapshot, ''),
		        COALESCE(working_dir_snapshot, ''), COALESCE(command_snapshot, ''),
		        COALESCE(args_snapshot, '')
		 FROM run_steps WHERE run_id = ?`, runID,
	)
	if err != nil {
		return nil, fmt.Errorf("get run steps: %w", err)
	}
	defer rows.Close()

	var runSteps []RunStep
	for rows.Next() {
		var rs RunStep
		var exitCode sql.NullInt64
		var rsStarted, rsCompleted sql.NullTime
		if err := rows.Scan(&rs.RunStepID, &rs.RunID, &rs.StepID, &rs.Status,
			&rs.SessionID, &rs.MachineID, &exitCode, &rsStarted, &rsCompleted,
			&rs.PromptSnapshot, &rs.MachineIDSnapshot, &rs.WorkingDirSnapshot,
			&rs.CommandSnapshot, &rs.ArgsSnapshot); err != nil {
			return nil, fmt.Errorf("scan run step: %w", err)
		}
		if exitCode.Valid {
			ec := int(exitCode.Int64)
			rs.ExitCode = &ec
		}
		if rsStarted.Valid {
			rs.StartedAt = &rsStarted.Time
		}
		if rsCompleted.Valid {
			rs.CompletedAt = &rsCompleted.Time
		}
		runSteps = append(runSteps, rs)
	}

	return &RunDetail{Run: run, RunSteps: runSteps}, rows.Err()
}

// UpdateRunStepStatus updates a run_step's status, session_id, exit_code, and timestamps.
func (s *Store) UpdateRunStepStatus(ctx context.Context, runStepID, status, sessionID string, exitCode int) error {
	var query string
	var args []interface{}

	switch status {
	case StatusRunning:
		query = `UPDATE run_steps SET status = ?, session_id = ?, started_at = CURRENT_TIMESTAMP WHERE run_step_id = ?`
		args = []interface{}{status, nullIfEmpty(sessionID), runStepID}
	case StatusCompleted, StatusFailed:
		query = `UPDATE run_steps SET status = ?, session_id = ?, exit_code = ?, ended_at = CURRENT_TIMESTAMP WHERE run_step_id = ?`
		args = []interface{}{status, nullIfEmpty(sessionID), exitCode, runStepID}
	default:
		query = `UPDATE run_steps SET status = ? WHERE run_step_id = ?`
		args = []interface{}{status, runStepID}
	}

	result, err := s.writer.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update run step status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("run step %s: %w", runStepID, ErrNotFound)
	}
	return nil
}

// UpdateRunStatus updates a run's overall status and timestamps.
func (s *Store) UpdateRunStatus(ctx context.Context, runID, status string) error {
	var query string
	switch status {
	case StatusRunning:
		query = `UPDATE runs SET status = ?, started_at = CURRENT_TIMESTAMP WHERE run_id = ?`
	case StatusCompleted, StatusFailed, StatusCancelled:
		query = `UPDATE runs SET status = ?, ended_at = CURRENT_TIMESTAMP WHERE run_id = ?`
	default:
		query = `UPDATE runs SET status = ? WHERE run_id = ?`
	}

	result, err := s.writer.ExecContext(ctx, query, status, runID)
	if err != nil {
		return fmt.Errorf("update run status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("run %s: %w", runID, ErrNotFound)
	}
	return nil
}

// ListRuns returns runs for a job ordered by created_at DESC.
func (s *Store) ListRuns(ctx context.Context, jobID string) ([]Run, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT run_id, job_id, status, trigger_type, started_at, ended_at, created_at
		 FROM runs WHERE job_id = ? ORDER BY created_at DESC`, jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var r Run
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&r.RunID, &r.JobID, &r.Status, &r.TriggerType, &startedAt, &completedAt, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		if startedAt.Valid {
			r.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			r.CompletedAt = &completedAt.Time
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// ListAllRuns returns runs across all jobs with optional filtering and pagination.
// Results are ordered by created_at DESC. Defaults to limit 50 when Limit is 0.
func (s *Store) ListAllRuns(ctx context.Context, opts ListRunsOptions) ([]RunWithJobName, error) {
	const defaultLimit = 50

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	query := `SELECT r.run_id, r.job_id, r.status, r.trigger_type, r.started_at, r.ended_at, r.created_at, j.name
	           FROM runs r
	           JOIN jobs j ON r.job_id = j.job_id
	           WHERE 1=1`

	args := make([]interface{}, 0, 5)

	if opts.JobID != "" {
		query += ` AND r.job_id = ?`
		args = append(args, opts.JobID)
	}
	if opts.Status != "" {
		query += ` AND r.status = ?`
		args = append(args, opts.Status)
	}
	if opts.TriggerType != "" {
		query += ` AND r.trigger_type = ?`
		args = append(args, opts.TriggerType)
	}

	query += ` ORDER BY r.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, opts.Offset)

	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list all runs: %w", err)
	}
	defer rows.Close()

	results := make([]RunWithJobName, 0)
	for rows.Next() {
		var rj RunWithJobName
		var startedAt, endedAt sql.NullTime
		if err := rows.Scan(
			&rj.RunID, &rj.JobID, &rj.Status, &rj.TriggerType,
			&startedAt, &endedAt, &rj.CreatedAt, &rj.JobName,
		); err != nil {
			return nil, fmt.Errorf("scan run with job name: %w", err)
		}
		if startedAt.Valid {
			rj.StartedAt = &startedAt.Time
		}
		if endedAt.Valid {
			rj.CompletedAt = &endedAt.Time
		}
		results = append(results, rj)
	}
	return results, rows.Err()
}

// nullIfEmpty returns nil for empty strings to satisfy SQL NULL constraints.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
