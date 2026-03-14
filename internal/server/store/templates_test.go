package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestStoreForTemplates(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// createTestUser inserts a user and returns the user_id.
func createTestUser(t *testing.T, s *Store, email string) string {
	t.Helper()
	id := "user-" + email
	hash, err := HashPassword("password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	err = s.CreateUser(&User{
		UserID:       id,
		Email:        email,
		DisplayName:  "Test User",
		PasswordHash: hash,
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return id
}

func TestTemplate_CreateAndGet(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userID := createTestUser(t, s, "alice@test.com")

	tmpl := &SessionTemplate{
		UserID:         userID,
		Name:           "my-template",
		Description:    "A test template",
		Command:        "claude",
		Args:           []string{"--model", "opus"},
		WorkingDir:     "/home/alice",
		EnvVars:        map[string]string{"FOO": "bar", "BAZ": "qux"},
		InitialPrompt:  "Hello world",
		TerminalRows:   30,
		TerminalCols:   120,
		Tags:           []string{"dev", "test"},
		TimeoutSeconds: 300,
	}

	created, err := s.CreateTemplate(ctx, tmpl)
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if created.TemplateID == "" {
		t.Fatal("expected non-empty TemplateID")
	}
	if created.Name != "my-template" {
		t.Errorf("Name = %q, want %q", created.Name, "my-template")
	}
	if created.Description != "A test template" {
		t.Errorf("Description = %q, want %q", created.Description, "A test template")
	}
	if len(created.Args) != 2 || created.Args[0] != "--model" {
		t.Errorf("Args = %v, want [--model opus]", created.Args)
	}
	if len(created.EnvVars) != 2 || created.EnvVars["FOO"] != "bar" {
		t.Errorf("EnvVars = %v, want {FOO:bar BAZ:qux}", created.EnvVars)
	}
	if len(created.Tags) != 2 || created.Tags[0] != "dev" {
		t.Errorf("Tags = %v, want [dev test]", created.Tags)
	}
	if created.TerminalRows != 30 {
		t.Errorf("TerminalRows = %d, want 30", created.TerminalRows)
	}
	if created.TerminalCols != 120 {
		t.Errorf("TerminalCols = %d, want 120", created.TerminalCols)
	}
	if created.TimeoutSeconds != 300 {
		t.Errorf("TimeoutSeconds = %d, want 300", created.TimeoutSeconds)
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if created.DeletedAt != nil {
		t.Error("DeletedAt should be nil")
	}

	// Get by ID
	got, err := s.GetTemplate(ctx, created.TemplateID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if got.TemplateID != created.TemplateID {
		t.Errorf("GetTemplate ID = %q, want %q", got.TemplateID, created.TemplateID)
	}
	if got.Name != "my-template" {
		t.Errorf("GetTemplate Name = %q, want %q", got.Name, "my-template")
	}
	if got.Command != "claude" {
		t.Errorf("GetTemplate Command = %q, want %q", got.Command, "claude")
	}
	if len(got.Args) != 2 {
		t.Errorf("GetTemplate Args = %v, want 2 items", got.Args)
	}
	if len(got.EnvVars) != 2 {
		t.Errorf("GetTemplate EnvVars = %v, want 2 items", got.EnvVars)
	}
	if len(got.Tags) != 2 {
		t.Errorf("GetTemplate Tags = %v, want 2 items", got.Tags)
	}
	if got.InitialPrompt != "Hello world" {
		t.Errorf("GetTemplate InitialPrompt = %q, want %q", got.InitialPrompt, "Hello world")
	}
}

func TestTemplate_CreateDuplicateNameSameUser(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userID := createTestUser(t, s, "bob@test.com")

	tmpl := &SessionTemplate{UserID: userID, Name: "dup-template", TerminalRows: 24, TerminalCols: 80}
	_, err := s.CreateTemplate(ctx, tmpl)
	if err != nil {
		t.Fatalf("CreateTemplate first: %v", err)
	}

	_, err = s.CreateTemplate(ctx, tmpl)
	if err == nil {
		t.Fatal("expected error for duplicate name+user, got nil")
	}
	if !IsUniqueViolation(err) {
		t.Errorf("expected unique violation, got: %v", err)
	}
}

func TestTemplate_CreateDuplicateNameDifferentUsers(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userA := createTestUser(t, s, "userA@test.com")
	userB := createTestUser(t, s, "userB@test.com")

	tmpl := &SessionTemplate{Name: "shared-name", TerminalRows: 24, TerminalCols: 80}

	tmplA := *tmpl
	tmplA.UserID = userA
	_, err := s.CreateTemplate(ctx, &tmplA)
	if err != nil {
		t.Fatalf("CreateTemplate userA: %v", err)
	}

	tmplB := *tmpl
	tmplB.UserID = userB
	_, err = s.CreateTemplate(ctx, &tmplB)
	if err != nil {
		t.Fatalf("CreateTemplate userB: %v", err)
	}
}

func TestTemplate_GetNonExistent(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()

	_, err := s.GetTemplate(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestTemplate_GetByName(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userID := createTestUser(t, s, "carol@test.com")

	tmpl := &SessionTemplate{
		UserID: userID, Name: "find-me", Command: "test-cmd",
		TerminalRows: 24, TerminalCols: 80,
	}
	created, err := s.CreateTemplate(ctx, tmpl)
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	got, err := s.GetTemplateByName(ctx, userID, "find-me")
	if err != nil {
		t.Fatalf("GetTemplateByName: %v", err)
	}
	if got.TemplateID != created.TemplateID {
		t.Errorf("ID = %q, want %q", got.TemplateID, created.TemplateID)
	}

	// Non-existent name
	_, err = s.GetTemplateByName(ctx, userID, "no-such-template")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing name, got: %v", err)
	}
}

func TestTemplate_GetByNameSoftDeleted(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userID := createTestUser(t, s, "dave@test.com")

	tmpl := &SessionTemplate{
		UserID: userID, Name: "will-delete",
		TerminalRows: 24, TerminalCols: 80,
	}
	created, err := s.CreateTemplate(ctx, tmpl)
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	err = s.DeleteTemplate(ctx, created.TemplateID)
	if err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}

	_, err = s.GetTemplateByName(ctx, userID, "will-delete")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for soft-deleted template, got: %v", err)
	}
}

func TestTemplate_List(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userA := createTestUser(t, s, "listA@test.com")
	userB := createTestUser(t, s, "listB@test.com")

	_, err := s.CreateTemplate(ctx, &SessionTemplate{
		UserID: userA, Name: "tmpl-1", Tags: []string{"backend"},
		TerminalRows: 24, TerminalCols: 80,
	})
	if err != nil {
		t.Fatalf("CreateTemplate 1: %v", err)
	}
	_, err = s.CreateTemplate(ctx, &SessionTemplate{
		UserID: userA, Name: "tmpl-2", Tags: []string{"frontend", "dev"},
		TerminalRows: 24, TerminalCols: 80,
	})
	if err != nil {
		t.Fatalf("CreateTemplate 2: %v", err)
	}
	_, err = s.CreateTemplate(ctx, &SessionTemplate{
		UserID: userB, Name: "tmpl-3", Tags: []string{"backend"},
		TerminalRows: 24, TerminalCols: 80,
	})
	if err != nil {
		t.Fatalf("CreateTemplate 3: %v", err)
	}

	// List for userA
	list, err := s.ListTemplates(ctx, userA, ListTemplateOptions{})
	if err != nil {
		t.Fatalf("ListTemplates userA: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("userA templates = %d, want 2", len(list))
	}

	// List all (admin mode)
	all, err := s.ListTemplates(ctx, "", ListTemplateOptions{})
	if err != nil {
		t.Fatalf("ListTemplates all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("all templates = %d, want 3", len(all))
	}

	// Filter by tag
	tagged, err := s.ListTemplates(ctx, "", ListTemplateOptions{Tag: "backend"})
	if err != nil {
		t.Fatalf("ListTemplates tag=backend: %v", err)
	}
	if len(tagged) != 2 {
		t.Errorf("backend-tagged templates = %d, want 2", len(tagged))
	}

	// Filter by name
	named, err := s.ListTemplates(ctx, userA, ListTemplateOptions{Name: "tmpl-1"})
	if err != nil {
		t.Fatalf("ListTemplates name=tmpl-1: %v", err)
	}
	if len(named) != 1 {
		t.Errorf("name-filtered templates = %d, want 1", len(named))
	}
}

func TestTemplate_Update(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userID := createTestUser(t, s, "eve@test.com")

	created, err := s.CreateTemplate(ctx, &SessionTemplate{
		UserID: userID, Name: "update-me", Description: "old desc",
		TerminalRows: 24, TerminalCols: 80, TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	// Small pause to ensure updated_at differs
	time.Sleep(10 * time.Millisecond)

	updated, err := s.UpdateTemplate(ctx, created.TemplateID, &SessionTemplate{
		Name:           "update-me",
		Description:    "new desc",
		Command:        "new-cmd",
		Tags:           []string{"updated"},
		TerminalRows:   40,
		TerminalCols:   160,
		TimeoutSeconds: 120,
	})
	if err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}
	if updated.Description != "new desc" {
		t.Errorf("Description = %q, want %q", updated.Description, "new desc")
	}
	if updated.Command != "new-cmd" {
		t.Errorf("Command = %q, want %q", updated.Command, "new-cmd")
	}
	if updated.TerminalRows != 40 {
		t.Errorf("TerminalRows = %d, want 40", updated.TerminalRows)
	}
	if updated.TimeoutSeconds != 120 {
		t.Errorf("TimeoutSeconds = %d, want 120", updated.TimeoutSeconds)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Error("UpdatedAt should be after original CreatedAt")
	}
}

func TestTemplate_UpdateSoftDeleted(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userID := createTestUser(t, s, "frank@test.com")

	created, err := s.CreateTemplate(ctx, &SessionTemplate{
		UserID: userID, Name: "del-then-update",
		TerminalRows: 24, TerminalCols: 80,
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if err := s.DeleteTemplate(ctx, created.TemplateID); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}

	_, err = s.UpdateTemplate(ctx, created.TemplateID, &SessionTemplate{
		Name: "new-name", TerminalRows: 24, TerminalCols: 80,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for soft-deleted update, got: %v", err)
	}
}

func TestTemplate_Delete(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userID := createTestUser(t, s, "grace@test.com")

	created, err := s.CreateTemplate(ctx, &SessionTemplate{
		UserID: userID, Name: "to-delete",
		TerminalRows: 24, TerminalCols: 80,
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	err = s.DeleteTemplate(ctx, created.TemplateID)
	if err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}

	// Get should return ErrNotFound
	_, err = s.GetTemplate(ctx, created.TemplateID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}

	// List should exclude deleted
	list, err := s.ListTemplates(ctx, userID, ListTemplateOptions{})
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListTemplates count = %d, want 0", len(list))
	}

	// Delete again should return ErrNotFound
	err = s.DeleteTemplate(ctx, created.TemplateID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for double delete, got: %v", err)
	}
}

func TestTemplate_DeleteNonExistent(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()

	err := s.DeleteTemplate(ctx, "nonexistent-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestTemplate_Clone(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userID := createTestUser(t, s, "heidi@test.com")

	original, err := s.CreateTemplate(ctx, &SessionTemplate{
		UserID: userID, Name: "original", Description: "the original",
		Command: "claude", Args: []string{"--verbose"},
		Tags: []string{"prod"}, TerminalRows: 30, TerminalCols: 100,
		TimeoutSeconds: 600,
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	clone, err := s.CloneTemplate(ctx, original.TemplateID)
	if err != nil {
		t.Fatalf("CloneTemplate: %v", err)
	}
	if clone.TemplateID == original.TemplateID {
		t.Error("clone should have a different ID")
	}
	if clone.Name != "original-copy" {
		t.Errorf("clone Name = %q, want %q", clone.Name, "original-copy")
	}
	if clone.Description != "the original" {
		t.Errorf("clone Description = %q, want %q", clone.Description, "the original")
	}
	if clone.Command != "claude" {
		t.Errorf("clone Command = %q, want %q", clone.Command, "claude")
	}
	if len(clone.Args) != 1 || clone.Args[0] != "--verbose" {
		t.Errorf("clone Args = %v, want [--verbose]", clone.Args)
	}
	if clone.TerminalRows != 30 {
		t.Errorf("clone TerminalRows = %d, want 30", clone.TerminalRows)
	}
	if clone.TimeoutSeconds != 600 {
		t.Errorf("clone TimeoutSeconds = %d, want 600", clone.TimeoutSeconds)
	}
}

func TestTemplate_CloneCopyExists(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userID := createTestUser(t, s, "ivan@test.com")

	original, err := s.CreateTemplate(ctx, &SessionTemplate{
		UserID: userID, Name: "base", TerminalRows: 24, TerminalCols: 80,
	})
	if err != nil {
		t.Fatalf("CreateTemplate original: %v", err)
	}

	// Create "base-copy" manually
	_, err = s.CreateTemplate(ctx, &SessionTemplate{
		UserID: userID, Name: "base-copy", TerminalRows: 24, TerminalCols: 80,
	})
	if err != nil {
		t.Fatalf("CreateTemplate base-copy: %v", err)
	}

	clone, err := s.CloneTemplate(ctx, original.TemplateID)
	if err != nil {
		t.Fatalf("CloneTemplate: %v", err)
	}
	if clone.Name != "base-copy-2" {
		t.Errorf("clone Name = %q, want %q", clone.Name, "base-copy-2")
	}
}

func TestTemplate_CloneNonExistent(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()

	_, err := s.CloneTemplate(ctx, "nonexistent-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestTemplate_NilSlicesAndMaps(t *testing.T) {
	s := newTestStoreForTemplates(t)
	ctx := context.Background()
	userID := createTestUser(t, s, "judy@test.com")

	created, err := s.CreateTemplate(ctx, &SessionTemplate{
		UserID: userID, Name: "minimal",
		TerminalRows: 24, TerminalCols: 80,
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	got, err := s.GetTemplate(ctx, created.TemplateID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if got.Args != nil {
		t.Errorf("Args = %v, want nil", got.Args)
	}
	if got.EnvVars != nil {
		t.Errorf("EnvVars = %v, want nil", got.EnvVars)
	}
	if got.Tags != nil {
		t.Errorf("Tags = %v, want nil", got.Tags)
	}
}
