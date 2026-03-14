package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SessionTemplate represents a reusable session configuration.
type SessionTemplate struct {
	TemplateID     string            `json:"template_id"`
	UserID         string            `json:"user_id"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	EnvVars        map[string]string `json:"env_vars,omitempty"`
	InitialPrompt  string            `json:"initial_prompt,omitempty"`
	TerminalRows   int               `json:"terminal_rows"`
	TerminalCols   int               `json:"terminal_cols"`
	Tags           []string          `json:"tags,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	DeletedAt      *time.Time        `json:"deleted_at,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// ListTemplateOptions holds optional filters for listing templates.
type ListTemplateOptions struct {
	Tag  string
	Name string
}

// TemplateStoreIface defines the interface for template-related database operations.
type TemplateStoreIface interface {
	CreateTemplate(ctx context.Context, t *SessionTemplate) (*SessionTemplate, error)
	GetTemplate(ctx context.Context, templateID string) (*SessionTemplate, error)
	GetTemplateByName(ctx context.Context, userID, name string) (*SessionTemplate, error)
	ListTemplates(ctx context.Context, userID string, opts ListTemplateOptions) ([]SessionTemplate, error)
	UpdateTemplate(ctx context.Context, templateID string, t *SessionTemplate) (*SessionTemplate, error)
	DeleteTemplate(ctx context.Context, templateID string) error
	CloneTemplate(ctx context.Context, templateID string) (*SessionTemplate, error)
}

// Compile-time check that Store implements TemplateStoreIface.
var _ TemplateStoreIface = (*Store)(nil)

// CreateTemplate inserts a new session template and returns the hydrated struct.
func (s *Store) CreateTemplate(ctx context.Context, t *SessionTemplate) (*SessionTemplate, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	argsJSON, err := marshalJSONField(t.Args)
	if err != nil {
		return nil, fmt.Errorf("marshal args: %w", err)
	}
	tagsJSON, err := marshalJSONField(t.Tags)
	if err != nil {
		return nil, fmt.Errorf("marshal tags: %w", err)
	}
	envJSON, err := marshalJSONField(t.EnvVars)
	if err != nil {
		return nil, fmt.Errorf("marshal env_vars: %w", err)
	}

	_, err = s.writer.ExecContext(ctx,
		`INSERT INTO session_templates
		 (template_id, user_id, name, description, command, args, working_dir,
		  env_vars, initial_prompt, terminal_rows, terminal_cols, tags,
		  timeout_seconds, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, t.UserID, t.Name, nullIfEmpty(t.Description), nullIfEmpty(t.Command),
		argsJSON, nullIfEmpty(t.WorkingDir), envJSON, nullIfEmpty(t.InitialPrompt),
		t.TerminalRows, t.TerminalCols, tagsJSON, t.TimeoutSeconds, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}

	return &SessionTemplate{
		TemplateID:     id,
		UserID:         t.UserID,
		Name:           t.Name,
		Description:    t.Description,
		Command:        t.Command,
		Args:           t.Args,
		WorkingDir:     t.WorkingDir,
		EnvVars:        t.EnvVars,
		InitialPrompt:  t.InitialPrompt,
		TerminalRows:   t.TerminalRows,
		TerminalCols:   t.TerminalCols,
		Tags:           t.Tags,
		TimeoutSeconds: t.TimeoutSeconds,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// GetTemplate retrieves a template by ID, excluding soft-deleted templates.
func (s *Store) GetTemplate(ctx context.Context, templateID string) (*SessionTemplate, error) {
	return s.scanTemplate(s.reader.QueryRowContext(ctx,
		`SELECT template_id, user_id, name, description, command, args, working_dir,
		        env_vars, initial_prompt, terminal_rows, terminal_cols, tags,
		        timeout_seconds, deleted_at, created_at, updated_at
		 FROM session_templates WHERE template_id = ? AND deleted_at IS NULL`, templateID,
	), templateID)
}

// GetTemplateByName retrieves a template by user and name, excluding soft-deleted.
func (s *Store) GetTemplateByName(ctx context.Context, userID, name string) (*SessionTemplate, error) {
	return s.scanTemplate(s.reader.QueryRowContext(ctx,
		`SELECT template_id, user_id, name, description, command, args, working_dir,
		        env_vars, initial_prompt, terminal_rows, terminal_cols, tags,
		        timeout_seconds, deleted_at, created_at, updated_at
		 FROM session_templates WHERE user_id = ? AND name = ? AND deleted_at IS NULL`, userID, name,
	), fmt.Sprintf("user=%s name=%s", userID, name))
}

// ListTemplates returns templates for a user with optional filters.
// If userID is empty, returns all templates (admin mode).
func (s *Store) ListTemplates(ctx context.Context, userID string, opts ListTemplateOptions) ([]SessionTemplate, error) {
	query := `SELECT template_id, user_id, name, description, command, args, working_dir,
	                 env_vars, initial_prompt, terminal_rows, terminal_cols, tags,
	                 timeout_seconds, deleted_at, created_at, updated_at
	          FROM session_templates WHERE deleted_at IS NULL`

	args := make([]interface{}, 0, 3)

	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	if opts.Name != "" {
		query += ` AND name = ?`
		args = append(args, opts.Name)
	}
	if opts.Tag != "" {
		query += ` AND tags LIKE ?`
		args = append(args, "%\""+opts.Tag+"\"%")
	}

	query += ` ORDER BY created_at DESC`

	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	templates := make([]SessionTemplate, 0)
	for rows.Next() {
		tmpl, err := scanTemplateRow(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, *tmpl)
	}
	return templates, rows.Err()
}

// UpdateTemplate updates all fields of a template, excluding soft-deleted ones.
func (s *Store) UpdateTemplate(ctx context.Context, templateID string, t *SessionTemplate) (*SessionTemplate, error) {
	argsJSON, err := marshalJSONField(t.Args)
	if err != nil {
		return nil, fmt.Errorf("marshal args: %w", err)
	}
	tagsJSON, err := marshalJSONField(t.Tags)
	if err != nil {
		return nil, fmt.Errorf("marshal tags: %w", err)
	}
	envJSON, err := marshalJSONField(t.EnvVars)
	if err != nil {
		return nil, fmt.Errorf("marshal env_vars: %w", err)
	}

	now := time.Now().UTC()
	result, err := s.writer.ExecContext(ctx,
		`UPDATE session_templates
		 SET name = ?, description = ?, command = ?, args = ?, working_dir = ?,
		     env_vars = ?, initial_prompt = ?, terminal_rows = ?, terminal_cols = ?,
		     tags = ?, timeout_seconds = ?, updated_at = ?
		 WHERE template_id = ? AND deleted_at IS NULL`,
		t.Name, nullIfEmpty(t.Description), nullIfEmpty(t.Command), argsJSON,
		nullIfEmpty(t.WorkingDir), envJSON, nullIfEmpty(t.InitialPrompt),
		t.TerminalRows, t.TerminalCols, tagsJSON, t.TimeoutSeconds, now,
		templateID,
	)
	if err != nil {
		return nil, fmt.Errorf("update template: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, fmt.Errorf("template %s: %w", templateID, ErrNotFound)
	}

	return s.GetTemplate(ctx, templateID)
}

// DeleteTemplate performs a soft delete by setting deleted_at.
func (s *Store) DeleteTemplate(ctx context.Context, templateID string) error {
	result, err := s.writer.ExecContext(ctx,
		`UPDATE session_templates SET deleted_at = CURRENT_TIMESTAMP
		 WHERE template_id = ? AND deleted_at IS NULL`, templateID,
	)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("template %s: %w", templateID, ErrNotFound)
	}
	return nil
}

// CloneTemplate duplicates a template with a new ID and a "-copy" name suffix.
// If the copy name already exists, appends "-copy-2" through "-copy-10".
func (s *Store) CloneTemplate(ctx context.Context, templateID string) (*SessionTemplate, error) {
	original, err := s.GetTemplate(ctx, templateID)
	if err != nil {
		return nil, err
	}

	baseName := original.Name + "-copy"
	cloneName := baseName

	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			cloneName = fmt.Sprintf("%s-%d", baseName, i+1)
		}
		_, err := s.GetTemplateByName(ctx, original.UserID, cloneName)
		if err != nil {
			if isNotFound(err) {
				break
			}
			return nil, fmt.Errorf("check clone name: %w", err)
		}
		if i == maxRetries-1 {
			return nil, fmt.Errorf("could not find unique clone name after %d attempts", maxRetries)
		}
	}

	clone := &SessionTemplate{
		UserID:         original.UserID,
		Name:           cloneName,
		Description:    original.Description,
		Command:        original.Command,
		Args:           original.Args,
		WorkingDir:     original.WorkingDir,
		EnvVars:        original.EnvVars,
		InitialPrompt:  original.InitialPrompt,
		TerminalRows:   original.TerminalRows,
		TerminalCols:   original.TerminalCols,
		Tags:           original.Tags,
		TimeoutSeconds: original.TimeoutSeconds,
	}

	return s.CreateTemplate(ctx, clone)
}

// isNotFound returns true if the error wraps ErrNotFound.
func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// scanTemplate scans a single row into a SessionTemplate.
func (s *Store) scanTemplate(row *sql.Row, identifier string) (*SessionTemplate, error) {
	var tmpl SessionTemplate
	var desc, cmd, argsJSON, workDir, envJSON, prompt, tagsJSON sql.NullString
	var deletedAt sql.NullTime

	err := row.Scan(
		&tmpl.TemplateID, &tmpl.UserID, &tmpl.Name, &desc, &cmd,
		&argsJSON, &workDir, &envJSON, &prompt,
		&tmpl.TerminalRows, &tmpl.TerminalCols, &tagsJSON,
		&tmpl.TimeoutSeconds, &deletedAt, &tmpl.CreatedAt, &tmpl.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("template %s: %w", identifier, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get template: %w", err)
	}

	if desc.Valid {
		tmpl.Description = desc.String
	}
	if cmd.Valid {
		tmpl.Command = cmd.String
	}
	if workDir.Valid {
		tmpl.WorkingDir = workDir.String
	}
	if prompt.Valid {
		tmpl.InitialPrompt = prompt.String
	}
	if deletedAt.Valid {
		tmpl.DeletedAt = &deletedAt.Time
	}

	if argsJSON.Valid && argsJSON.String != "" {
		if err := json.Unmarshal([]byte(argsJSON.String), &tmpl.Args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
	}
	if tagsJSON.Valid && tagsJSON.String != "" {
		if err := json.Unmarshal([]byte(tagsJSON.String), &tmpl.Tags); err != nil {
			return nil, fmt.Errorf("unmarshal tags: %w", err)
		}
	}
	if envJSON.Valid && envJSON.String != "" {
		if err := json.Unmarshal([]byte(envJSON.String), &tmpl.EnvVars); err != nil {
			return nil, fmt.Errorf("unmarshal env_vars: %w", err)
		}
	}

	return &tmpl, nil
}

// scanTemplateRow scans a row from sql.Rows into a SessionTemplate.
func scanTemplateRow(rows *sql.Rows) (*SessionTemplate, error) {
	var tmpl SessionTemplate
	var desc, cmd, argsJSON, workDir, envJSON, prompt, tagsJSON sql.NullString
	var deletedAt sql.NullTime

	err := rows.Scan(
		&tmpl.TemplateID, &tmpl.UserID, &tmpl.Name, &desc, &cmd,
		&argsJSON, &workDir, &envJSON, &prompt,
		&tmpl.TerminalRows, &tmpl.TerminalCols, &tagsJSON,
		&tmpl.TimeoutSeconds, &deletedAt, &tmpl.CreatedAt, &tmpl.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan template: %w", err)
	}

	if desc.Valid {
		tmpl.Description = desc.String
	}
	if cmd.Valid {
		tmpl.Command = cmd.String
	}
	if workDir.Valid {
		tmpl.WorkingDir = workDir.String
	}
	if prompt.Valid {
		tmpl.InitialPrompt = prompt.String
	}
	if deletedAt.Valid {
		tmpl.DeletedAt = &deletedAt.Time
	}

	if argsJSON.Valid && argsJSON.String != "" {
		if err := json.Unmarshal([]byte(argsJSON.String), &tmpl.Args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
	}
	if tagsJSON.Valid && tagsJSON.String != "" {
		if err := json.Unmarshal([]byte(tagsJSON.String), &tmpl.Tags); err != nil {
			return nil, fmt.Errorf("unmarshal tags: %w", err)
		}
	}
	if envJSON.Valid && envJSON.String != "" {
		if err := json.Unmarshal([]byte(envJSON.String), &tmpl.EnvVars); err != nil {
			return nil, fmt.Errorf("unmarshal env_vars: %w", err)
		}
	}

	return &tmpl, nil
}

// marshalJSONField marshals a value to JSON, returning nil for nil/empty values.
func marshalJSONField(v interface{}) (interface{}, error) {
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	s := string(data)
	if s == "null" || s == "[]" || s == "{}" {
		return nil, nil
	}
	return s, nil
}
