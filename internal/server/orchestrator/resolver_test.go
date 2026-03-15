package orchestrator

import (
	"testing"
)

func TestResolveReferences_ShorthandParams(t *testing.T) {
	params := map[string]string{"ENV": "production", "VERSION": "1.2.3"}
	result := ResolveReferences("Deploy ${ENV} version ${VERSION}", params, JobMeta{}, nil, nil)
	expected := "Deploy production version 1.2.3"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestResolveReferences_JobParameters(t *testing.T) {
	params := map[string]string{"BRANCH": "main"}
	result := ResolveReferences("Build ${BRANCH}", params, JobMeta{}, nil, nil)
	expected := "Build main"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestResolveReferences_JobMetadata(t *testing.T) {
	meta := JobMeta{
		Name:        "deploy-job",
		RunID:       "run-abc-123",
		RunNumber:   42,
		StartTime:   "2026-01-15T10:00:00Z",
		TriggerType: "manual",
	}
	tests := []struct {
		template string
		expected string
	}{
		{"Job: {{job.name}}", "Job: deploy-job"},
		{"Run: {{job.run_id}}", "Run: run-abc-123"},
		{"#{{job.run_number}}", "#42"},
		{"Started: {{job.start_time}}", "Started: 2026-01-15T10:00:00Z"},
		{"Via: {{job.trigger_type}}", "Via: manual"},
	}
	for _, tt := range tests {
		result := ResolveReferences(tt.template, nil, meta, nil, nil)
		if result != tt.expected {
			t.Errorf("template %q: got %q, want %q", tt.template, result, tt.expected)
		}
	}
}

func TestResolveReferences_StepValues(t *testing.T) {
	stepValues := map[string]map[string]string{
		"build": {"artifact_url": "s3://bucket/build.tar.gz", "sha": "abc123"},
	}
	result := ResolveReferences(
		"Deploy {{steps.build.values.artifact_url}} ({{steps.build.values.sha}})",
		nil, JobMeta{}, stepValues, nil,
	)
	expected := "Deploy s3://bucket/build.tar.gz (abc123)"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestResolveReferences_StepStatus(t *testing.T) {
	stepResults := map[string]StepResult{
		"validate": {Status: "completed", ExitCode: 0},
	}
	result := ResolveReferences("Status: {{steps.validate.status}}", nil, JobMeta{}, nil, stepResults)
	expected := "Status: completed"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestResolveReferences_StepExitCode(t *testing.T) {
	stepResults := map[string]StepResult{
		"test": {Status: "failed", ExitCode: 1},
	}
	result := ResolveReferences("Exit: {{steps.test.exit_code}}", nil, JobMeta{}, nil, stepResults)
	expected := "Exit: 1"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestResolveReferences_UnresolvedPassthrough(t *testing.T) {
	// Unresolved references should be left as-is
	result := ResolveReferences("${MISSING} and {{unknown.ref}}", nil, JobMeta{}, nil, nil)
	expected := "${MISSING} and {{unknown.ref}}"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestResolveReferences_MultipleInOneLine(t *testing.T) {
	params := map[string]string{"A": "1", "B": "2"}
	meta := JobMeta{Name: "test-job", RunID: "r-1"}
	result := ResolveReferences("${A}-${B}-{{job.name}}-{{job.run_id}}", params, meta, nil, nil)
	expected := "1-2-test-job-r-1"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestResolveReferences_EmptyParams(t *testing.T) {
	result := ResolveReferences("Hello ${NAME}", nil, JobMeta{}, nil, nil)
	expected := "Hello ${NAME}"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestResolveReferences_EmptyTemplate(t *testing.T) {
	result := ResolveReferences("", map[string]string{"X": "Y"}, JobMeta{}, nil, nil)
	if result != "" {
		t.Errorf("got %q, want empty string", result)
	}
}

func TestResolveParameters_MergesDefaults(t *testing.T) {
	defaults := `{"ENV":"staging","VERSION":"1.0"}`
	overrides := map[string]string{"ENV": "production"}
	result := resolveParameters(defaults, overrides)

	if result["ENV"] != "production" {
		t.Errorf("ENV = %q, want %q", result["ENV"], "production")
	}
	if result["VERSION"] != "1.0" {
		t.Errorf("VERSION = %q, want %q", result["VERSION"], "1.0")
	}
}

func TestResolveParameters_EmptyDefaults(t *testing.T) {
	// When job defines no parameters, all overrides are dropped.
	result := resolveParameters("", map[string]string{"KEY": "val"})
	if _, exists := result["KEY"]; exists {
		t.Errorf("KEY should be dropped when job has no defaults, got %q", result["KEY"])
	}
}

func TestResolveParameters_InvalidJSON(t *testing.T) {
	// Invalid JSON means no defaults parsed, so overrides are dropped.
	result := resolveParameters("{invalid", map[string]string{"K": "V"})
	if _, exists := result["K"]; exists {
		t.Errorf("K should be dropped when defaults JSON is invalid, got %q", result["K"])
	}
}

func TestResolveParameters_UnknownKeysDropped(t *testing.T) {
	defaults := `{"ENV":"staging"}`
	overrides := map[string]string{"ENV": "production", "UNKNOWN": "injected"}
	result := resolveParameters(defaults, overrides)

	if result["ENV"] != "production" {
		t.Errorf("ENV = %q, want %q", result["ENV"], "production")
	}
	if _, exists := result["UNKNOWN"]; exists {
		t.Errorf("UNKNOWN should be dropped, got %q", result["UNKNOWN"])
	}
}

func TestResolveParameters_NilOverrides(t *testing.T) {
	result := resolveParameters(`{"A":"1"}`, nil)
	if result["A"] != "1" {
		t.Errorf("A = %q, want %q", result["A"], "1")
	}
}
