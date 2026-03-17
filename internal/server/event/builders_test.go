package event

import "testing"

func TestNewRunEvent_IncludesJobName(t *testing.T) {
	e := NewRunEvent(TypeRunCreated, "run-1", "job-1", "pending", "manual", "deploy-prod")
	if e.Payload["job_name"] != "deploy-prod" {
		t.Errorf("expected job_name 'deploy-prod', got %v", e.Payload["job_name"])
	}
	if e.Payload["job_id"] != "job-1" {
		t.Errorf("expected job_id 'job-1', got %v", e.Payload["job_id"])
	}
}

func TestNewSessionEvent_IncludesNames(t *testing.T) {
	e := NewSessionEvent(TypeSessionStarted, "sess-1", "machine-1", "worker-alpha", "/bin/bash")
	if e.Payload["machine_name"] != "worker-alpha" {
		t.Errorf("expected machine_name 'worker-alpha', got %v", e.Payload["machine_name"])
	}
	if e.Payload["command"] != "/bin/bash" {
		t.Errorf("expected command '/bin/bash', got %v", e.Payload["command"])
	}
	if e.Source != "session" {
		t.Errorf("expected source 'session', got %q", e.Source)
	}
	if e.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestNewMachineEvent_IncludesDisplayName(t *testing.T) {
	e := NewMachineEvent(TypeMachineConnected, "machine-1", "Worker Alpha")
	if e.Payload["display_name"] != "Worker Alpha" {
		t.Errorf("expected display_name 'Worker Alpha', got %v", e.Payload["display_name"])
	}
	if e.Source != "connmgr" {
		t.Errorf("expected source 'connmgr', got %q", e.Source)
	}
	if e.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestNewTemplateEvent_IncludesTemplateName(t *testing.T) {
	e := NewTemplateEvent(TypeTemplateCreated, "tmpl-1", "user-1", "My Template")
	if e.Payload["template_name"] != "My Template" {
		t.Errorf("expected template_name 'My Template', got %v", e.Payload["template_name"])
	}
	if e.Source != "template" {
		t.Errorf("expected source 'template', got %q", e.Source)
	}
	if e.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestNewRunStepEvent_IncludesNames(t *testing.T) {
	e := NewRunStepEvent(TypeJobRunStepCompleted, "run-1", "rs-1", "step-1", "completed", "Build Step", "deploy-prod")
	if e.Payload["step_name"] != "Build Step" {
		t.Errorf("expected step_name 'Build Step', got %v", e.Payload["step_name"])
	}
	if e.Payload["job_name"] != "deploy-prod" {
		t.Errorf("expected job_name 'deploy-prod', got %v", e.Payload["job_name"])
	}
	if e.Source != "orchestrator" {
		t.Errorf("expected source 'orchestrator', got %q", e.Source)
	}
	if e.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestNewJobEvent(t *testing.T) {
	e := NewJobEvent(TypeJobCreated, "job-1", "deploy-prod", "user-1")
	if e.Type != TypeJobCreated {
		t.Errorf("expected type %q, got %q", TypeJobCreated, e.Type)
	}
	if e.Payload["job_id"] != "job-1" {
		t.Errorf("expected job_id 'job-1', got %v", e.Payload["job_id"])
	}
	if e.Payload["job_name"] != "deploy-prod" {
		t.Errorf("expected job_name 'deploy-prod', got %v", e.Payload["job_name"])
	}
	if e.Payload["user_id"] != "user-1" {
		t.Errorf("expected user_id 'user-1', got %v", e.Payload["user_id"])
	}
	if e.Source != "handler" {
		t.Errorf("expected source 'handler', got %q", e.Source)
	}
	if e.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestNewUserEvent(t *testing.T) {
	e := NewUserEvent(TypeUserCreated, "user-1", "admin@test.com")
	if e.Payload["user_id"] != "user-1" {
		t.Errorf("expected user_id 'user-1', got %v", e.Payload["user_id"])
	}
	if e.Payload["email"] != "admin@test.com" {
		t.Errorf("expected email 'admin@test.com', got %v", e.Payload["email"])
	}
	if e.Source != "handler" {
		t.Errorf("expected source 'handler', got %q", e.Source)
	}
	if e.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestNewScheduleEvent(t *testing.T) {
	e := NewScheduleEvent(TypeScheduleCreated, "sched-1", "job-1", "deploy-prod", "0 9 * * *")
	if e.Payload["schedule_id"] != "sched-1" {
		t.Errorf("expected schedule_id 'sched-1', got %v", e.Payload["schedule_id"])
	}
	if e.Payload["job_name"] != "deploy-prod" {
		t.Errorf("expected job_name 'deploy-prod', got %v", e.Payload["job_name"])
	}
	if e.Source != "handler" {
		t.Errorf("expected source 'handler', got %q", e.Source)
	}
	if e.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestNewCredentialEvent(t *testing.T) {
	e := NewCredentialEvent(TypeCredentialCreated, "cred-1", "MY_SECRET", "user-1")
	if e.Payload["credential_id"] != "cred-1" {
		t.Errorf("expected credential_id 'cred-1', got %v", e.Payload["credential_id"])
	}
	if e.Payload["credential_name"] != "MY_SECRET" {
		t.Errorf("expected credential_name 'MY_SECRET', got %v", e.Payload["credential_name"])
	}
	if e.Source != "handler" {
		t.Errorf("expected source 'handler', got %q", e.Source)
	}
	if e.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestNewWebhookEvent(t *testing.T) {
	e := NewWebhookEvent(TypeWebhookCreated, "wh-1", "My Hook")
	if e.Payload["webhook_id"] != "wh-1" {
		t.Errorf("expected webhook_id 'wh-1', got %v", e.Payload["webhook_id"])
	}
	if e.Payload["webhook_name"] != "My Hook" {
		t.Errorf("expected webhook_name 'My Hook', got %v", e.Payload["webhook_name"])
	}
	if e.Source != "handler" {
		t.Errorf("expected source 'handler', got %q", e.Source)
	}
	if e.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}
