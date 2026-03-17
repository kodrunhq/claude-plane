package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// Separate atomic counters per entity type to avoid ID collisions across tests.
var (
	factoryMachineCounter  atomic.Int64
	factorySessionCounter  atomic.Int64
	factoryJobCounter      atomic.Int64
	factoryUserCounter     atomic.Int64
	factoryTemplateCounter atomic.Int64
	factoryRunCounter      atomic.Int64
)

// --- Store helper ---

// mustNewStore creates a temporary SQLite-backed Store for testing.
// The database is automatically cleaned up when the test finishes.
func mustNewStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("mustNewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// --- Machine ---

// MachineOption customizes machine creation.
type MachineOption func(*machineConfig)

type machineConfig struct {
	MachineID   string
	MaxSessions int32
}

func machineDefaults(n int64) machineConfig {
	return machineConfig{
		MachineID:   fmt.Sprintf("machine-%d", n),
		MaxSessions: 5,
	}
}

// WithMachineID overrides the auto-generated machine ID.
func WithMachineID(id string) MachineOption {
	return func(c *machineConfig) { c.MachineID = id }
}

// WithMaxSessions sets the maximum concurrent sessions for the machine.
func WithMaxSessions(n int32) MachineOption {
	return func(c *machineConfig) { c.MaxSessions = n }
}

// mustCreateMachine creates a machine with a unique ID and returns the machine_id.
func mustCreateMachine(t *testing.T, s *Store, opts ...MachineOption) string {
	t.Helper()
	n := factoryMachineCounter.Add(1)
	cfg := machineDefaults(n)
	for _, o := range opts {
		o(&cfg)
	}
	if err := s.UpsertMachine(cfg.MachineID, cfg.MaxSessions); err != nil {
		t.Fatalf("mustCreateMachine: %v", err)
	}
	return cfg.MachineID
}

// --- Session ---

// SessionOption customizes session creation.
type SessionOption func(*sessionConfig)

type sessionConfig struct {
	SessionID     string
	MachineID     string
	UserID        string
	TemplateID    string
	Command       string
	WorkingDir    string
	Status        string
	Model         string
	SkipPerms     string
	EnvVars       string
	Args          string
	InitialPrompt string
}

func sessionDefaults(n int64, machineID string) sessionConfig {
	return sessionConfig{
		SessionID:  fmt.Sprintf("sess-%d", n),
		MachineID:  machineID,
		Command:    "claude",
		WorkingDir: "/tmp",
		Status:     StatusCreated,
	}
}

// WithSessionID overrides the auto-generated session ID.
func WithSessionID(id string) SessionOption {
	return func(c *sessionConfig) { c.SessionID = id }
}

// WithSessionUserID sets the user ID on the session.
func WithSessionUserID(uid string) SessionOption {
	return func(c *sessionConfig) { c.UserID = uid }
}

// WithSessionStatus sets the initial status.
func WithSessionStatus(status string) SessionOption {
	return func(c *sessionConfig) { c.Status = status }
}

// WithSessionCommand sets the command.
func WithSessionCommand(cmd string) SessionOption {
	return func(c *sessionConfig) { c.Command = cmd }
}

// WithSessionWorkingDir sets the working directory.
func WithSessionWorkingDir(dir string) SessionOption {
	return func(c *sessionConfig) { c.WorkingDir = dir }
}

// WithSessionModel sets the model.
func WithSessionModel(model string) SessionOption {
	return func(c *sessionConfig) { c.Model = model }
}

// WithSessionInitialPrompt sets the initial prompt.
func WithSessionInitialPrompt(prompt string) SessionOption {
	return func(c *sessionConfig) { c.InitialPrompt = prompt }
}

// WithSessionTemplateID sets the template ID.
func WithSessionTemplateID(id string) SessionOption {
	return func(c *sessionConfig) { c.TemplateID = id }
}

// WithSessionSkipPerms sets skip_permissions.
func WithSessionSkipPerms(v string) SessionOption {
	return func(c *sessionConfig) { c.SkipPerms = v }
}

// WithSessionEnvVars sets the env_vars JSON.
func WithSessionEnvVars(v string) SessionOption {
	return func(c *sessionConfig) { c.EnvVars = v }
}

// WithSessionArgs sets the args JSON.
func WithSessionArgs(v string) SessionOption {
	return func(c *sessionConfig) { c.Args = v }
}

// mustCreateSession creates a session with defaults and returns the Session.
func mustCreateSession(t *testing.T, s *Store, machineID string, opts ...SessionOption) *Session {
	t.Helper()
	n := factorySessionCounter.Add(1)
	cfg := sessionDefaults(n, machineID)
	for _, o := range opts {
		o(&cfg)
	}
	sess := &Session{
		SessionID:     cfg.SessionID,
		MachineID:     cfg.MachineID,
		UserID:        cfg.UserID,
		TemplateID:    cfg.TemplateID,
		Command:       cfg.Command,
		WorkingDir:    cfg.WorkingDir,
		Status:        cfg.Status,
		Model:         cfg.Model,
		SkipPerms:     cfg.SkipPerms,
		EnvVars:       cfg.EnvVars,
		Args:          cfg.Args,
		InitialPrompt: cfg.InitialPrompt,
	}
	if err := s.CreateSession(sess); err != nil {
		t.Fatalf("mustCreateSession: %v", err)
	}
	return sess
}

// --- Job ---

// JobOption customizes job creation.
type JobOption func(*jobConfig)

type jobConfig struct {
	Name              string
	Description       string
	UserID            string
	Parameters        string
	TimeoutSeconds    int
	MaxConcurrentRuns int
}

func jobDefaults(n int64) jobConfig {
	return jobConfig{
		Name: fmt.Sprintf("test-job-%d", n),
	}
}

// WithJobName overrides the auto-generated job name.
func WithJobName(name string) JobOption {
	return func(c *jobConfig) { c.Name = name }
}

// WithJobDescription sets the job description.
func WithJobDescription(desc string) JobOption {
	return func(c *jobConfig) { c.Description = desc }
}

// WithJobUserID sets the user ID on the job.
func WithJobUserID(uid string) JobOption {
	return func(c *jobConfig) { c.UserID = uid }
}

// WithJobParameters sets the parameters JSON.
func WithJobParameters(params string) JobOption {
	return func(c *jobConfig) { c.Parameters = params }
}

// WithJobTimeout sets the timeout in seconds.
func WithJobTimeout(secs int) JobOption {
	return func(c *jobConfig) { c.TimeoutSeconds = secs }
}

// WithJobMaxConcurrentRuns sets the max concurrent runs.
func WithJobMaxConcurrentRuns(n int) JobOption {
	return func(c *jobConfig) { c.MaxConcurrentRuns = n }
}

// mustCreateJob creates a job with defaults and returns the Job.
func mustCreateJob(t *testing.T, s *Store, opts ...JobOption) *Job {
	t.Helper()
	n := factoryJobCounter.Add(1)
	cfg := jobDefaults(n)
	for _, o := range opts {
		o(&cfg)
	}
	job, err := s.CreateJob(context.Background(), CreateJobParams{
		Name:              cfg.Name,
		Description:       cfg.Description,
		UserID:            cfg.UserID,
		Parameters:        cfg.Parameters,
		TimeoutSeconds:    cfg.TimeoutSeconds,
		MaxConcurrentRuns: cfg.MaxConcurrentRuns,
	})
	if err != nil {
		t.Fatalf("mustCreateJob: %v", err)
	}
	return job
}

// --- User ---

// mustCreateUser creates a user with the given email and role, returning the User.
// A dummy password hash is used for speed.
func mustCreateUser(t *testing.T, s *Store, email, role string) *User {
	t.Helper()
	n := factoryUserCounter.Add(1)
	if email == "" {
		email = fmt.Sprintf("user-%d@test.local", n)
	}
	if role == "" {
		role = "viewer"
	}
	user := &User{
		UserID:       uuid.New().String(),
		Email:        email,
		DisplayName:  fmt.Sprintf("Test User %d", n),
		PasswordHash: "$argon2id$v=19$m=65536,t=2,p=4$dGVzdHNhbHQxMjM0NTY=$dGVzdGhhc2gxMjM0NTY3ODkwMTIzNDU2Nzg5MDEy",
		Role:         role,
	}
	if err := s.CreateUser(user); err != nil {
		t.Fatalf("mustCreateUser: %v", err)
	}
	return user
}

// --- Template ---

// TemplateOption customizes template creation.
type TemplateOption func(*templateConfig)

type templateConfig struct {
	UserID         string
	Name           string
	MachineID      string
	Description    string
	Command        string
	WorkingDir     string
	InitialPrompt  string
	TerminalRows   int
	TerminalCols   int
	TimeoutSeconds int
}

func templateDefaults(n int64, userID string) templateConfig {
	return templateConfig{
		UserID:       userID,
		Name:         fmt.Sprintf("template-%d", n),
		TerminalRows: 24,
		TerminalCols: 80,
	}
}

// WithTemplateName overrides the auto-generated template name.
func WithTemplateName(name string) TemplateOption {
	return func(c *templateConfig) { c.Name = name }
}

// WithTemplateMachineID sets the machine ID on the template.
func WithTemplateMachineID(id string) TemplateOption {
	return func(c *templateConfig) { c.MachineID = id }
}

// WithTemplateDescription sets the description.
func WithTemplateDescription(desc string) TemplateOption {
	return func(c *templateConfig) { c.Description = desc }
}

// WithTemplateCommand sets the command.
func WithTemplateCommand(cmd string) TemplateOption {
	return func(c *templateConfig) { c.Command = cmd }
}

// mustCreateTemplate creates a session template with defaults and returns it.
func mustCreateTemplate(t *testing.T, s *Store, userID string, opts ...TemplateOption) *SessionTemplate {
	t.Helper()
	n := factoryTemplateCounter.Add(1)
	cfg := templateDefaults(n, userID)
	for _, o := range opts {
		o(&cfg)
	}
	tmpl, err := s.CreateTemplate(context.Background(), &SessionTemplate{
		UserID:         cfg.UserID,
		Name:           cfg.Name,
		MachineID:      cfg.MachineID,
		Description:    cfg.Description,
		Command:        cfg.Command,
		WorkingDir:     cfg.WorkingDir,
		InitialPrompt:  cfg.InitialPrompt,
		TerminalRows:   cfg.TerminalRows,
		TerminalCols:   cfg.TerminalCols,
		TimeoutSeconds: cfg.TimeoutSeconds,
	})
	if err != nil {
		t.Fatalf("mustCreateTemplate: %v", err)
	}
	return tmpl
}

// --- Run ---

// RunOption customizes run creation.
type RunOption func(*runConfig)

type runConfig struct {
	TriggerType   string
	TriggerDetail string
	Parameters    string
}

func runDefaults() runConfig {
	return runConfig{
		TriggerType: "manual",
	}
}

// WithRunTriggerType sets the trigger type.
func WithRunTriggerType(tt string) RunOption {
	return func(c *runConfig) { c.TriggerType = tt }
}

// WithRunTriggerDetail sets the trigger detail.
func WithRunTriggerDetail(td string) RunOption {
	return func(c *runConfig) { c.TriggerDetail = td }
}

// WithRunParameters sets the parameters JSON.
func WithRunParameters(params string) RunOption {
	return func(c *runConfig) { c.Parameters = params }
}

// mustCreateRun creates a run for the given job and returns it.
func mustCreateRun(t *testing.T, s *Store, jobID string, opts ...RunOption) *Run {
	t.Helper()
	factoryRunCounter.Add(1)
	cfg := runDefaults()
	for _, o := range opts {
		o(&cfg)
	}
	run, err := s.CreateRun(context.Background(), CreateRunParams{
		JobID:         jobID,
		TriggerType:   cfg.TriggerType,
		TriggerDetail: cfg.TriggerDetail,
		Parameters:    cfg.Parameters,
	})
	if err != nil {
		t.Fatalf("mustCreateRun: %v", err)
	}
	return run
}
