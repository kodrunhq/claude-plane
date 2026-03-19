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

// CreateJobParams holds parameters for creating a job.
type CreateJobParams struct {
	Name              string
	Description       string
	UserID            string
	Parameters        string // JSON
	TimeoutSeconds    int
	MaxConcurrentRuns int // defaults to 1 if 0
}

// UpdateJobParams holds parameters for updating a job.
type UpdateJobParams struct {
	JobID             string
	Name              string
	Description       string
	Parameters        string
	TimeoutSeconds    int
	MaxConcurrentRuns int
}

// CreateRunParams holds parameters for creating a run.
type CreateRunParams struct {
	JobID         string
	TriggerType   string
	TriggerDetail string
	Parameters    string // JSON, resolved
}

// CreateStepParams holds parameters for creating a step.
type CreateStepParams struct {
	JobID             string
	Name              string
	Prompt            string
	MachineID         string
	WorkingDir        string
	Command           string
	Args              string
	TimeoutSeconds    int
	SortOrder         int
	OnFailure         string
	SkipPermissions   *int
	Model             string
	DelaySeconds      int
	TaskType          string
	SessionKey        string
	RunIf             string
	MaxRetries        int
	RetryDelaySeconds int
	Parameters        string
	TargetJobID       string
	JobParams         string
}

// UpdateStepParams holds parameters for updating a step.
type UpdateStepParams struct {
	StepID            string
	Name              string
	Prompt            string
	MachineID         string
	WorkingDir        string
	Command           string
	Args              string
	TimeoutSeconds    int
	SortOrder         int
	OnFailure         string
	SkipPermissions   *int
	Model             string
	DelaySeconds      int
	TaskType          string
	SessionKey        string
	RunIf             string
	MaxRetries        int
	RetryDelaySeconds int
	Parameters        string
	TargetJobID       string
	JobParams         string
}

// ListRunsOptions holds optional filters and pagination for ListAllRuns.
type ListRunsOptions struct {
	JobID       string
	Status      string
	TriggerType string
	UserID      string
	Limit       int
	Offset      int
}

// RunWithJobName embeds Run and adds the human-readable job name.
type RunWithJobName struct {
	Run
	JobName   string `json:"job_name"`
	MachineIDs string `json:"machine_ids,omitempty"`
}

// JobWithStats extends Job with computed stats for list views.
type JobWithStats struct {
	Job
	StepCount     int    `json:"step_count"`
	LastRunStatus string `json:"last_run_status,omitempty"`
	TriggerType   string `json:"trigger_type"`
	MachineIDs    string `json:"machine_ids,omitempty"`
}

// JobStoreIface defines the interface for job-related database operations.
// Used by the orchestrator package for dependency injection and testability.
type JobStoreIface interface {
	CreateJob(ctx context.Context, p CreateJobParams) (*Job, error)
	GetJob(ctx context.Context, jobID string) (*JobDetail, error)
	ListJobs(ctx context.Context) ([]Job, error)
	ListJobsByUser(ctx context.Context, userID string) ([]Job, error)
	ListJobsWithStats(ctx context.Context, userID string) ([]JobWithStats, error)
	DeleteJob(ctx context.Context, jobID string) error
	UpdateJob(ctx context.Context, p UpdateJobParams) (*Job, error)
	CreateStep(ctx context.Context, p CreateStepParams) (*Step, error)
	UpdateStep(ctx context.Context, p UpdateStepParams) error
	DeleteStep(ctx context.Context, stepID string) error
	AddDependency(ctx context.Context, stepID, dependsOn string) error
	RemoveDependency(ctx context.Context, stepID, dependsOn string) error
	CloneJob(ctx context.Context, jobID string, newName string) (*JobDetail, error)
	GetStepsWithDeps(ctx context.Context, jobID string) ([]Step, []StepDependency, error)
	CreateRun(ctx context.Context, p CreateRunParams) (*Run, error)
	InsertRunSteps(ctx context.Context, runID string, steps []Step) error
	GetRunWithSteps(ctx context.Context, runID string) (*RunDetail, error)
	UpdateRunStepStatus(ctx context.Context, runStepID, status, sessionID string, exitCode int) error
	UpdateRunStatus(ctx context.Context, runID, status string) error
	ListRuns(ctx context.Context, jobID string) ([]Run, error)
	ListAllRuns(ctx context.Context, opts ListRunsOptions) ([]RunWithJobName, error)
	UpdateRunParameters(ctx context.Context, runID, parametersJSON string) error
	UpdateRunStepAttempt(ctx context.Context, runStepID string, attempt int) error
	SetTaskValue(ctx context.Context, runStepID, key, value string) error
	GetTaskValues(ctx context.Context, runStepID string) ([]TaskValue, error)
	GetTaskValuesForSteps(ctx context.Context, runStepIDs []string) ([]TaskValue, error)
	DeleteTaskValuesForStep(ctx context.Context, runStepID string) error
	CountRunsForJobUpTo(ctx context.Context, jobID string, upTo time.Time) (int, error)
}

// Compile-time check that Store implements JobStoreIface.
var _ JobStoreIface = (*Store)(nil)

// Job represents a reusable job definition.
type Job struct {
	JobID             string    `json:"job_id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	UserID            string    `json:"user_id"`
	Parameters        string    `json:"parameters,omitempty"`
	TimeoutSeconds    int       `json:"timeout_seconds"`
	MaxConcurrentRuns int       `json:"max_concurrent_runs"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Step represents a step within a job.
type Step struct {
	StepID            string `json:"step_id"`
	JobID             string `json:"job_id"`
	Name              string `json:"name"`
	Prompt            string `json:"prompt"`
	MachineID         string `json:"machine_id"`
	WorkingDir        string `json:"working_dir"`
	Command           string `json:"command"`
	Args              string `json:"args"`
	TimeoutSeconds    int    `json:"timeout_seconds"`
	SortOrder         int    `json:"sort_order"`
	OnFailure         string `json:"on_failure"`
	SkipPermissions   *int   `json:"skip_permissions"`
	Model             string `json:"model"`
	DelaySeconds      int    `json:"delay_seconds"`
	TaskType          string `json:"task_type"`
	SessionKey        string `json:"session_key,omitempty"`
	RunIf             string `json:"run_if"`
	MaxRetries        int    `json:"max_retries"`
	RetryDelaySeconds int    `json:"retry_delay_seconds"`
	Parameters        string `json:"parameters,omitempty"`
	TargetJobID       string `json:"target_job_id,omitempty"`
	JobParams         string `json:"job_params,omitempty"`
}

// StepDependency represents a dependency edge in the step DAG.
type StepDependency struct {
	StepID    string `json:"step_id"`
	DependsOn string `json:"depends_on"`
}

// Run represents a specific execution of a job.
type Run struct {
	RunID         string     `json:"run_id"`
	JobID         string     `json:"job_id"`
	Status        string     `json:"status"`
	TriggerType   string     `json:"trigger_type"`
	TriggerDetail string     `json:"trigger_detail,omitempty"`
	Parameters    string     `json:"parameters,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// RunStep represents an instance of a step within a specific run.
type RunStep struct {
	RunStepID                    string     `json:"run_step_id"`
	RunID                        string     `json:"run_id"`
	StepID                       string     `json:"step_id"`
	Status                       string     `json:"status"`
	SessionID                    string     `json:"session_id,omitempty"`
	MachineID                    string     `json:"machine_id,omitempty"`
	ExitCode                     *int       `json:"exit_code,omitempty"`
	StartedAt                    *time.Time `json:"started_at,omitempty"`
	CompletedAt                  *time.Time `json:"completed_at,omitempty"`
	PromptSnapshot               string     `json:"prompt_snapshot"`
	MachineIDSnapshot            string     `json:"machine_id_snapshot"`
	WorkingDirSnapshot           string     `json:"working_dir_snapshot"`
	CommandSnapshot              string     `json:"command_snapshot"`
	ArgsSnapshot                 string     `json:"args_snapshot"`
	SkipPermissionsSnapshot      *int       `json:"skip_permissions_snapshot"`
	ModelSnapshot                string     `json:"model_snapshot"`
	DelaySecondsSnapshot         int        `json:"delay_seconds_snapshot"`
	OnFailureSnapshot            string     `json:"on_failure_snapshot"`
	TimeoutSecondsSnapshot       int        `json:"timeout_seconds_snapshot"`
	TaskTypeSnapshot             string     `json:"task_type_snapshot"`
	SessionKeySnapshot           string     `json:"session_key_snapshot,omitempty"`
	RunIfSnapshot                string     `json:"run_if_snapshot"`
	MaxRetriesSnapshot           int        `json:"max_retries_snapshot"`
	RetryDelaySecondsSnapshot    int        `json:"retry_delay_seconds_snapshot"`
	Attempt                      int        `json:"attempt"`
	ParametersSnapshot           string     `json:"parameters_snapshot,omitempty"`
	TargetJobIDSnapshot          string     `json:"target_job_id_snapshot,omitempty"`
	JobParamsSnapshot            string     `json:"job_params_snapshot,omitempty"`
	OnFailure                    string     `json:"on_failure,omitempty"`
	ErrorMessage                 string     `json:"error_message,omitempty"`
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
func (s *Store) CreateJob(ctx context.Context, p CreateJobParams) (*Job, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	maxConcurrent := p.MaxConcurrentRuns
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO jobs (job_id, name, description, user_id, parameters, timeout_seconds, max_concurrent_runs, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, p.Name, nullIfEmpty(p.Description), nullIfEmpty(p.UserID),
		nullIfEmpty(p.Parameters), p.TimeoutSeconds, maxConcurrent, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	return &Job{
		JobID:             id,
		Name:              p.Name,
		Description:       p.Description,
		UserID:            p.UserID,
		Parameters:        p.Parameters,
		TimeoutSeconds:    p.TimeoutSeconds,
		MaxConcurrentRuns: maxConcurrent,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// GetJob retrieves a job by ID with its steps and dependency edges.
func (s *Store) GetJob(ctx context.Context, jobID string) (*JobDetail, error) {
	var job Job
	var desc, userID, params sql.NullString
	err := s.reader.QueryRowContext(ctx,
		`SELECT job_id, name, description, user_id, COALESCE(parameters, ''),
		        COALESCE(timeout_seconds, 0), COALESCE(max_concurrent_runs, 1),
		        created_at, updated_at
		 FROM jobs WHERE job_id = ?`, jobID,
	).Scan(&job.JobID, &job.Name, &desc, &userID, &params,
		&job.TimeoutSeconds, &job.MaxConcurrentRuns,
		&job.CreatedAt, &job.UpdatedAt)
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
	if params.Valid {
		job.Parameters = params.String
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
		`SELECT job_id, name, description, user_id, COALESCE(parameters, ''),
		        COALESCE(timeout_seconds, 0), COALESCE(max_concurrent_runs, 1),
		        created_at, updated_at
		 FROM jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var desc, userID, params sql.NullString
		if err := rows.Scan(&j.JobID, &j.Name, &desc, &userID, &params,
			&j.TimeoutSeconds, &j.MaxConcurrentRuns,
			&j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		if desc.Valid {
			j.Description = desc.String
		}
		if userID.Valid {
			j.UserID = userID.String
		}
		if params.Valid {
			j.Parameters = params.String
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// ListJobsByUser returns jobs owned by a specific user, ordered by created_at DESC.
func (s *Store) ListJobsByUser(ctx context.Context, userID string) ([]Job, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT job_id, name, description, user_id, COALESCE(parameters, ''),
		        COALESCE(timeout_seconds, 0), COALESCE(max_concurrent_runs, 1),
		        created_at, updated_at
		 FROM jobs WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list jobs by user: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var desc, uid, params sql.NullString
		if err := rows.Scan(&j.JobID, &j.Name, &desc, &uid, &params,
			&j.TimeoutSeconds, &j.MaxConcurrentRuns,
			&j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		if desc.Valid {
			j.Description = desc.String
		}
		if uid.Valid {
			j.UserID = uid.String
		}
		if params.Valid {
			j.Parameters = params.String
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

// UpdateJob updates a job's fields and returns the updated job.
func (s *Store) UpdateJob(ctx context.Context, p UpdateJobParams) (*Job, error) {
	maxConcurrent := p.MaxConcurrentRuns
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	result, err := s.writer.ExecContext(ctx,
		`UPDATE jobs SET name = ?, description = ?, parameters = ?, timeout_seconds = ?,
		 max_concurrent_runs = ?, updated_at = CURRENT_TIMESTAMP WHERE job_id = ?`,
		p.Name, nullIfEmpty(p.Description), nullIfEmpty(p.Parameters),
		p.TimeoutSeconds, maxConcurrent, p.JobID,
	)
	if err != nil {
		return nil, fmt.Errorf("update job: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, fmt.Errorf("job %s: %w", p.JobID, ErrNotFound)
	}

	var job Job
	var desc, userID, params sql.NullString
	err = s.reader.QueryRowContext(ctx,
		`SELECT job_id, name, description, user_id, COALESCE(parameters, ''),
		        COALESCE(timeout_seconds, 0), COALESCE(max_concurrent_runs, 1),
		        created_at, updated_at
		 FROM jobs WHERE job_id = ?`, p.JobID,
	).Scan(&job.JobID, &job.Name, &desc, &userID, &params,
		&job.TimeoutSeconds, &job.MaxConcurrentRuns,
		&job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("read updated job: %w", err)
	}
	if desc.Valid {
		job.Description = desc.String
	}
	if userID.Valid {
		job.UserID = userID.String
	}
	if params.Valid {
		job.Parameters = params.String
	}
	return &job, nil
}

// CreateStep inserts a step for a job and returns it.
func (s *Store) CreateStep(ctx context.Context, p CreateStepParams) (*Step, error) {
	// Default task_type and run_if
	if p.TaskType == "" {
		p.TaskType = "claude_session"
	}
	if p.RunIf == "" {
		p.RunIf = "all_success"
	}

	// Validate task_type
	if p.TaskType != "claude_session" && p.TaskType != "shell" && p.TaskType != "run_job" {
		return nil, fmt.Errorf("invalid task_type %q: must be claude_session, shell, or run_job", p.TaskType)
	}
	// Validate run_if
	if p.RunIf != "all_success" && p.RunIf != "all_done" {
		return nil, fmt.Errorf("invalid run_if %q: must be all_success or all_done", p.RunIf)
	}
	// Validate retries
	if p.MaxRetries < 0 {
		return nil, fmt.Errorf("max_retries must be >= 0, got %d", p.MaxRetries)
	}
	if p.RetryDelaySeconds < 0 {
		return nil, fmt.Errorf("retry_delay_seconds must be >= 0, got %d", p.RetryDelaySeconds)
	}
	// Shell tasks have no prompt
	if p.TaskType == "shell" {
		p.Prompt = ""
	}
	// run_job tasks have no prompt or command
	if p.TaskType == "run_job" {
		p.Prompt = ""
		p.Command = ""
	}

	id := uuid.New().String()
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO steps (step_id, job_id, name, prompt, machine_id, working_dir, command, args,
		 timeout_seconds, sort_order, on_failure, skip_permissions, model, delay_seconds,
		 task_type, session_key, run_if, max_retries, retry_delay_seconds, parameters,
		 target_job_id, job_params)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, p.JobID, p.Name, p.Prompt, nullIfEmpty(p.MachineID), p.WorkingDir, p.Command, p.Args,
		p.TimeoutSeconds, p.SortOrder, p.OnFailure,
		p.SkipPermissions, p.Model, p.DelaySeconds,
		p.TaskType, nullIfEmpty(p.SessionKey), p.RunIf, p.MaxRetries, p.RetryDelaySeconds,
		nullIfEmpty(p.Parameters), nullIfEmpty(p.TargetJobID), nullIfEmpty(p.JobParams),
	)
	if err != nil {
		return nil, fmt.Errorf("create step: %w", err)
	}
	return &Step{
		StepID:            id,
		JobID:             p.JobID,
		Name:              p.Name,
		Prompt:            p.Prompt,
		MachineID:         p.MachineID,
		WorkingDir:        p.WorkingDir,
		Command:           p.Command,
		Args:              p.Args,
		TimeoutSeconds:    p.TimeoutSeconds,
		SortOrder:         p.SortOrder,
		OnFailure:         p.OnFailure,
		SkipPermissions:   p.SkipPermissions,
		Model:             p.Model,
		DelaySeconds:      p.DelaySeconds,
		TaskType:          p.TaskType,
		SessionKey:        p.SessionKey,
		RunIf:             p.RunIf,
		MaxRetries:        p.MaxRetries,
		RetryDelaySeconds: p.RetryDelaySeconds,
		Parameters:        p.Parameters,
		TargetJobID:       p.TargetJobID,
		JobParams:         p.JobParams,
	}, nil
}

// UpdateStep modifies step fields.
func (s *Store) UpdateStep(ctx context.Context, p UpdateStepParams) error {
	// Default task_type and run_if
	if p.TaskType == "" {
		p.TaskType = "claude_session"
	}
	if p.RunIf == "" {
		p.RunIf = "all_success"
	}

	// Validate task_type
	if p.TaskType != "claude_session" && p.TaskType != "shell" && p.TaskType != "run_job" {
		return fmt.Errorf("invalid task_type %q: must be claude_session, shell, or run_job", p.TaskType)
	}
	// Validate run_if
	if p.RunIf != "all_success" && p.RunIf != "all_done" {
		return fmt.Errorf("invalid run_if %q: must be all_success or all_done", p.RunIf)
	}
	if p.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be >= 0, got %d", p.MaxRetries)
	}
	if p.RetryDelaySeconds < 0 {
		return fmt.Errorf("retry_delay_seconds must be >= 0, got %d", p.RetryDelaySeconds)
	}
	// Shell tasks have no prompt
	if p.TaskType == "shell" {
		p.Prompt = ""
	}
	// run_job tasks have no prompt or command
	if p.TaskType == "run_job" {
		p.Prompt = ""
		p.Command = ""
	}

	result, err := s.writer.ExecContext(ctx,
		`UPDATE steps SET name = ?, prompt = ?, machine_id = ?, working_dir = ?, command = ?, args = ?,
		 timeout_seconds = ?, sort_order = ?, on_failure = ?, skip_permissions = ?, model = ?, delay_seconds = ?,
		 task_type = ?, session_key = ?, run_if = ?, max_retries = ?, retry_delay_seconds = ?, parameters = ?,
		 target_job_id = ?, job_params = ?
		 WHERE step_id = ?`,
		p.Name, p.Prompt, nullIfEmpty(p.MachineID), p.WorkingDir, p.Command, p.Args,
		p.TimeoutSeconds, p.SortOrder, p.OnFailure,
		p.SkipPermissions, p.Model, p.DelaySeconds,
		p.TaskType, nullIfEmpty(p.SessionKey), p.RunIf, p.MaxRetries, p.RetryDelaySeconds,
		nullIfEmpty(p.Parameters), nullIfEmpty(p.TargetJobID), nullIfEmpty(p.JobParams), p.StepID,
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
		        sort_order, COALESCE(on_failure, 'fail_run'),
		        skip_permissions, COALESCE(model, ''), COALESCE(delay_seconds, 0),
		        COALESCE(task_type, 'claude_session'), COALESCE(session_key, ''),
		        COALESCE(run_if, 'all_success'), COALESCE(max_retries, 0),
		        COALESCE(retry_delay_seconds, 30), COALESCE(parameters, ''),
		        COALESCE(target_job_id, ''), COALESCE(job_params, '')
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
			&st.WorkingDir, &st.Command, &st.Args, &st.TimeoutSeconds, &st.SortOrder, &st.OnFailure,
			&st.SkipPermissions, &st.Model, &st.DelaySeconds,
			&st.TaskType, &st.SessionKey, &st.RunIf, &st.MaxRetries,
			&st.RetryDelaySeconds, &st.Parameters,
			&st.TargetJobID, &st.JobParams); err != nil {
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
func (s *Store) CreateRun(ctx context.Context, p CreateRunParams) (*Run, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO runs (run_id, job_id, status, trigger_type, trigger_detail, parameters, created_at)
		 VALUES (?, ?, 'pending', ?, ?, ?, ?)`,
		id, p.JobID, p.TriggerType, nullIfEmpty(p.TriggerDetail),
		nullIfEmpty(p.Parameters), now,
	)
	if err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}
	return &Run{
		RunID:         id,
		JobID:         p.JobID,
		Status:        StatusPending,
		TriggerType:   p.TriggerType,
		TriggerDetail: p.TriggerDetail,
		Parameters:    p.Parameters,
		CreatedAt:     now,
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
			 prompt_snapshot, machine_id_snapshot, working_dir_snapshot, command_snapshot, args_snapshot,
			 skip_permissions_snapshot, model_snapshot, delay_seconds_snapshot, on_failure_snapshot, timeout_seconds_snapshot,
			 task_type_snapshot, session_key_snapshot, run_if_snapshot, max_retries_snapshot,
			 retry_delay_seconds_snapshot, parameters_snapshot,
			 target_job_id_snapshot, job_params_snapshot)
			 VALUES (?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, runID, st.StepID, nullIfEmpty(st.MachineID),
			st.Prompt, st.MachineID, st.WorkingDir, st.Command, st.Args,
			st.SkipPermissions, st.Model, st.DelaySeconds, st.OnFailure, st.TimeoutSeconds,
			st.TaskType, nullIfEmpty(st.SessionKey), st.RunIf, st.MaxRetries,
			st.RetryDelaySeconds, nullIfEmpty(st.Parameters),
			nullIfEmpty(st.TargetJobID), nullIfEmpty(st.JobParams),
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
	var startedAt, endedAt sql.NullTime
	var triggerDetail, params sql.NullString
	err := s.reader.QueryRowContext(ctx,
		`SELECT run_id, job_id, status, trigger_type, COALESCE(trigger_detail, ''),
		        COALESCE(parameters, ''), started_at, ended_at, created_at
		 FROM runs WHERE run_id = ?`, runID,
	).Scan(&run.RunID, &run.JobID, &run.Status, &run.TriggerType, &triggerDetail,
		&params, &startedAt, &endedAt, &run.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run %s: %w", runID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	if triggerDetail.Valid {
		run.TriggerDetail = triggerDetail.String
	}
	if params.Valid {
		run.Parameters = params.String
	}
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if endedAt.Valid {
		run.CompletedAt = &endedAt.Time
	}

	rows, err := s.reader.QueryContext(ctx,
		`SELECT run_step_id, run_id, step_id, status, COALESCE(session_id, ''),
		        COALESCE(machine_id, ''), exit_code, started_at, ended_at,
		        COALESCE(prompt_snapshot, ''), COALESCE(machine_id_snapshot, ''),
		        COALESCE(working_dir_snapshot, ''), COALESCE(command_snapshot, ''),
		        COALESCE(args_snapshot, ''),
		        skip_permissions_snapshot, COALESCE(model_snapshot, ''),
		        COALESCE(delay_seconds_snapshot, 0), COALESCE(on_failure_snapshot, 'fail_run'),
		        COALESCE(timeout_seconds_snapshot, 0),
		        COALESCE(task_type_snapshot, 'claude_session'), COALESCE(session_key_snapshot, ''),
		        COALESCE(run_if_snapshot, 'all_success'), COALESCE(max_retries_snapshot, 0),
		        COALESCE(retry_delay_seconds_snapshot, 30), COALESCE(attempt, 1),
		        COALESCE(parameters_snapshot, ''),
		        COALESCE(target_job_id_snapshot, ''), COALESCE(job_params_snapshot, ''),
		        COALESCE(error_message, '')
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
			&rs.CommandSnapshot, &rs.ArgsSnapshot,
			&rs.SkipPermissionsSnapshot, &rs.ModelSnapshot,
			&rs.DelaySecondsSnapshot, &rs.OnFailureSnapshot,
			&rs.TimeoutSecondsSnapshot,
			&rs.TaskTypeSnapshot, &rs.SessionKeySnapshot,
			&rs.RunIfSnapshot, &rs.MaxRetriesSnapshot,
			&rs.RetryDelaySecondsSnapshot, &rs.Attempt,
			&rs.ParametersSnapshot,
			&rs.TargetJobIDSnapshot, &rs.JobParamsSnapshot,
			&rs.ErrorMessage); err != nil {
			return nil, fmt.Errorf("scan run step: %w", err)
		}
		rs.OnFailure = rs.OnFailureSnapshot
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
		if sessionID != "" {
			query = `UPDATE run_steps SET status = ?, session_id = ?, exit_code = ?, ended_at = CURRENT_TIMESTAMP WHERE run_step_id = ?`
			args = []interface{}{status, sessionID, exitCode, runStepID}
		} else {
			// Preserve existing session_id when not provided.
			query = `UPDATE run_steps SET status = ?, exit_code = ?, ended_at = CURRENT_TIMESTAMP WHERE run_step_id = ?`
			args = []interface{}{status, exitCode, runStepID}
		}
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

// UpdateRunStepErrorMessage sets the error_message on a run step.
func (s *Store) UpdateRunStepErrorMessage(ctx context.Context, runStepID, message string) error {
	result, err := s.writer.ExecContext(ctx,
		`UPDATE run_steps SET error_message = ? WHERE run_step_id = ?`,
		message, runStepID,
	)
	if err != nil {
		return fmt.Errorf("update run step error message: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("run step %s: %w", runStepID, ErrNotFound)
	}
	return nil
}

// ListRuns returns runs for a job ordered by created_at DESC.
func (s *Store) ListRuns(ctx context.Context, jobID string) ([]Run, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT run_id, job_id, status, trigger_type, COALESCE(trigger_detail, ''),
		        COALESCE(parameters, ''), started_at, ended_at, created_at
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
		var triggerDetail, params sql.NullString
		if err := rows.Scan(&r.RunID, &r.JobID, &r.Status, &r.TriggerType, &triggerDetail,
			&params, &startedAt, &completedAt, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		if triggerDetail.Valid {
			r.TriggerDetail = triggerDetail.String
		}
		if params.Valid {
			r.Parameters = params.String
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

// CountRunsForJobUpTo returns the number of runs for a job created at or before
// the given timestamp. This provides a stable ordinal for run numbering that
// does not change as new runs are added.
func (s *Store) CountRunsForJobUpTo(ctx context.Context, jobID string, upTo time.Time) (int, error) {
	var count int
	err := s.reader.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE job_id = ? AND created_at <= ?`,
		jobID, upTo,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count runs for job up to %v: %w", upTo, err)
	}
	return count, nil
}

// ListAllRuns returns runs across all jobs with optional filtering and pagination.
// Results are ordered by created_at DESC. Defaults to limit 50 when Limit is 0.
func (s *Store) ListAllRuns(ctx context.Context, opts ListRunsOptions) ([]RunWithJobName, error) {
	const defaultLimit = 50

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	query := `SELECT r.run_id, r.job_id, r.status, r.trigger_type, COALESCE(r.trigger_detail, ''), r.started_at, r.ended_at, r.created_at, j.name,
	                 COALESCE(
	                   (SELECT GROUP_CONCAT(DISTINCT rs.machine_id) FROM run_steps rs WHERE rs.run_id = r.run_id AND rs.machine_id IS NOT NULL AND rs.machine_id != ''),
	                   ''
	                 ) AS machine_id
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
	if opts.UserID != "" {
		query += ` AND j.user_id = ?`
		args = append(args, opts.UserID)
	}

	query += ` ORDER BY r.created_at DESC, r.run_id DESC LIMIT ? OFFSET ?`
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
		var triggerDetail, machineID sql.NullString
		if err := rows.Scan(
			&rj.RunID, &rj.JobID, &rj.Status, &rj.TriggerType, &triggerDetail,
			&startedAt, &endedAt, &rj.CreatedAt, &rj.JobName, &machineID,
		); err != nil {
			return nil, fmt.Errorf("scan run with job name: %w", err)
		}
		if triggerDetail.Valid {
			rj.TriggerDetail = triggerDetail.String
		}
		if startedAt.Valid {
			rj.StartedAt = &startedAt.Time
		}
		if endedAt.Valid {
			rj.CompletedAt = &endedAt.Time
		}
		if machineID.Valid {
			rj.MachineIDs = machineID.String
		}
		results = append(results, rj)
	}
	return results, rows.Err()
}

// ListJobsWithStats returns jobs with step_count and last_run_status.
// If userID is empty, returns all jobs (admin view).
func (s *Store) ListJobsWithStats(ctx context.Context, userID string) ([]JobWithStats, error) {
	query := `SELECT j.job_id, j.name, COALESCE(j.description, ''), COALESCE(j.user_id, ''),
	                 COALESCE(j.parameters, ''), COALESCE(j.timeout_seconds, 0), COALESCE(j.max_concurrent_runs, 1),
	                 j.created_at, j.updated_at,
	                 (SELECT COUNT(*) FROM steps s WHERE s.job_id = j.job_id) AS step_count,
	                 COALESCE(
	                   (SELECT r.status FROM runs r WHERE r.job_id = j.job_id ORDER BY r.created_at DESC LIMIT 1),
	                   ''
	                 ) AS last_run_status,
	                 CASE
	                   WHEN EXISTS (SELECT 1 FROM cron_schedules cs WHERE cs.job_id = j.job_id)
	                     AND EXISTS (SELECT 1 FROM job_triggers jt WHERE jt.job_id = j.job_id)
	                   THEN 'mixed'
	                   WHEN EXISTS (SELECT 1 FROM cron_schedules cs WHERE cs.job_id = j.job_id)
	                   THEN 'cron'
	                   WHEN EXISTS (SELECT 1 FROM job_triggers jt WHERE jt.job_id = j.job_id)
	                   THEN 'event'
	                   ELSE 'manual'
	                 END AS trigger_type,
	                 COALESCE(
	                   (SELECT GROUP_CONCAT(DISTINCT s.machine_id) FROM steps s WHERE s.job_id = j.job_id AND s.machine_id IS NOT NULL AND s.machine_id != ''),
	                   ''
	                 ) AS machine_ids
	          FROM jobs j`

	var args []interface{}
	if userID != "" {
		query += ` WHERE j.user_id = ?`
		args = append(args, userID)
	}
	query += ` ORDER BY j.created_at DESC`

	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs with stats: %w", err)
	}
	defer rows.Close()

	var jobs []JobWithStats
	for rows.Next() {
		var j JobWithStats
		var desc, uid, params, lastStatus, triggerType, machineIDs sql.NullString
		if err := rows.Scan(&j.JobID, &j.Name, &desc, &uid, &params,
			&j.TimeoutSeconds, &j.MaxConcurrentRuns,
			&j.CreatedAt, &j.UpdatedAt, &j.StepCount, &lastStatus, &triggerType, &machineIDs); err != nil {
			return nil, fmt.Errorf("scan job with stats: %w", err)
		}
		if desc.Valid {
			j.Description = desc.String
		}
		if uid.Valid {
			j.UserID = uid.String
		}
		if params.Valid {
			j.Parameters = params.String
		}
		if lastStatus.Valid {
			j.LastRunStatus = lastStatus.String
		}
		if triggerType.Valid {
			j.TriggerType = triggerType.String
		}
		if machineIDs.Valid {
			j.MachineIDs = machineIDs.String
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateRunParameters updates the parameters JSON on a run.
func (s *Store) UpdateRunParameters(ctx context.Context, runID, parametersJSON string) error {
	res, err := s.writer.ExecContext(ctx,
		"UPDATE runs SET parameters = ? WHERE run_id = ?", parametersJSON, runID)
	if err != nil {
		return fmt.Errorf("update run parameters: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("run %s: %w", runID, ErrNotFound)
	}
	return nil
}

// UpdateRunStepAttempt updates the attempt counter on a run step.
func (s *Store) UpdateRunStepAttempt(ctx context.Context, runStepID string, attempt int) error {
	_, err := s.writer.ExecContext(ctx,
		"UPDATE run_steps SET attempt = ? WHERE run_step_id = ?", attempt, runStepID)
	if err != nil {
		return fmt.Errorf("update run step attempt: %w", err)
	}
	return nil
}

// CloneJob duplicates a job (with all its steps and dependencies) under a new
// name. If newName is empty the clone is named "{original} (copy)". The entire
// operation runs inside a single transaction so partial state is never visible.
func (s *Store) CloneJob(ctx context.Context, jobID string, newName string) (*JobDetail, error) {
	// Read source job, steps, and dependencies (uses reader — outside tx).
	src, err := s.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if newName == "" {
		newName = src.Job.Name + " (copy)"
	}

	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("clone job begin tx: %w", err)
	}
	defer tx.Rollback()

	// Create the new job.
	newJobID := uuid.New().String()
	now := time.Now().UTC()
	maxConcurrent := src.Job.MaxConcurrentRuns
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO jobs (job_id, name, description, user_id, parameters, timeout_seconds, max_concurrent_runs, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newJobID, newName, nullIfEmpty(src.Job.Description), nullIfEmpty(src.Job.UserID),
		nullIfEmpty(src.Job.Parameters), src.Job.TimeoutSeconds, maxConcurrent, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("clone job insert: %w", err)
	}

	// Map old step IDs to new step IDs for dependency remapping.
	stepIDMap := make(map[string]string, len(src.Steps))
	newSteps := make([]Step, 0, len(src.Steps))

	for _, st := range src.Steps {
		newStepID := uuid.New().String()
		stepIDMap[st.StepID] = newStepID

		_, err = tx.ExecContext(ctx,
			`INSERT INTO steps (step_id, job_id, name, prompt, machine_id, working_dir, command, args,
			 timeout_seconds, sort_order, on_failure, skip_permissions, model, delay_seconds,
			 task_type, session_key, run_if, max_retries, retry_delay_seconds, parameters,
			 target_job_id, job_params)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newStepID, newJobID, st.Name, st.Prompt, nullIfEmpty(st.MachineID), st.WorkingDir,
			st.Command, st.Args, st.TimeoutSeconds, st.SortOrder, st.OnFailure,
			st.SkipPermissions, st.Model, st.DelaySeconds,
			st.TaskType, nullIfEmpty(st.SessionKey), st.RunIf, st.MaxRetries, st.RetryDelaySeconds,
			nullIfEmpty(st.Parameters), nullIfEmpty(st.TargetJobID), nullIfEmpty(st.JobParams),
		)
		if err != nil {
			return nil, fmt.Errorf("clone step %s: %w", st.StepID, err)
		}

		newSteps = append(newSteps, Step{
			StepID: newStepID, JobID: newJobID, Name: st.Name, Prompt: st.Prompt,
			MachineID: st.MachineID, WorkingDir: st.WorkingDir, Command: st.Command,
			Args: st.Args, TimeoutSeconds: st.TimeoutSeconds, SortOrder: st.SortOrder,
			OnFailure: st.OnFailure, SkipPermissions: st.SkipPermissions, Model: st.Model,
			DelaySeconds: st.DelaySeconds, TaskType: st.TaskType, SessionKey: st.SessionKey,
			RunIf: st.RunIf, MaxRetries: st.MaxRetries, RetryDelaySeconds: st.RetryDelaySeconds,
			Parameters: st.Parameters, TargetJobID: st.TargetJobID, JobParams: st.JobParams,
		})
	}

	// Remap and insert dependencies.
	newDeps := make([]StepDependency, 0, len(src.Dependencies))
	for _, dep := range src.Dependencies {
		newStepID, ok1 := stepIDMap[dep.StepID]
		newDepsOn, ok2 := stepIDMap[dep.DependsOn]
		if !ok1 || !ok2 {
			continue // skip orphaned dependency edges
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO step_dependencies (step_id, depends_on) VALUES (?, ?)`,
			newStepID, newDepsOn,
		)
		if err != nil {
			return nil, fmt.Errorf("clone dependency %s->%s: %w", dep.StepID, dep.DependsOn, err)
		}
		newDeps = append(newDeps, StepDependency{StepID: newStepID, DependsOn: newDepsOn})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("clone job commit: %w", err)
	}

	return &JobDetail{
		Job: Job{
			JobID:             newJobID,
			Name:              newName,
			Description:       src.Job.Description,
			UserID:            src.Job.UserID,
			Parameters:        src.Job.Parameters,
			TimeoutSeconds:    src.Job.TimeoutSeconds,
			MaxConcurrentRuns: maxConcurrent,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		Steps:        newSteps,
		Dependencies: newDeps,
	}, nil
}

// nullIfEmpty returns nil for empty strings to satisfy SQL NULL constraints.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
