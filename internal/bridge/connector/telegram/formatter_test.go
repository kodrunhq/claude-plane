package telegram

import (
	"strings"
	"testing"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
)

func TestEscapeMarkdownV2(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"hello.world", "hello\\.world"},
		{"test-123", "test\\-123"},
		{"a_b*c[d](e)~f>g#h+i=j|k{l}m!n", "a\\_b\\*c\\[d\\]\\(e\\)\\~f\\>g\\#h\\+i\\=j\\|k\\{l\\}m\\!n"},
	}
	for _, tt := range tests {
		got := escapeMarkdownV2(tt.input)
		if got != tt.want {
			t.Errorf("escapeMarkdownV2(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// makeEvent is a helper to construct a client.Event with a string payload.
func makeEvent(eventType string, payload map[string]interface{}) client.Event {
	return client.Event{
		Type:    eventType,
		Payload: payload,
	}
}

// --- session events ---

func TestFormatEvent_SessionStarted_WithMachineName(t *testing.T) {
	e := makeEvent("session.started", map[string]interface{}{
		"machine_name": "worker-prod",
		"machine_id":   "m-123",
		"session_id":   "s-456",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "worker\\-prod") {
		t.Errorf("expected machine_name in output, got: %s", got)
	}
	if strings.Contains(got, "m\\-123") {
		t.Errorf("should not fall back to machine_id when machine_name is present, got: %s", got)
	}
	if !strings.Contains(got, "Session started") {
		t.Errorf("expected 'Session started' in output, got: %s", got)
	}
}

func TestFormatEvent_SessionStarted_FallbackToMachineID(t *testing.T) {
	e := makeEvent("session.started", map[string]interface{}{
		"machine_id": "m-123",
		"session_id": "s-456",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "m\\-123") {
		t.Errorf("expected machine_id fallback in output, got: %s", got)
	}
}

func TestFormatEvent_SessionStarted_WithCommand(t *testing.T) {
	e := makeEvent("session.started", map[string]interface{}{
		"machine_name": "worker-1",
		"session_id":   "s-789",
		"command":      "claude --dangerously-skip-permissions",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Command") {
		t.Errorf("expected Command line when command present, got: %s", got)
	}
}

func TestFormatEvent_SessionStarted_NoCommand(t *testing.T) {
	e := makeEvent("session.started", map[string]interface{}{
		"machine_name": "worker-1",
		"session_id":   "s-789",
	})
	got := FormatEvent(e)
	if strings.Contains(got, "Command") {
		t.Errorf("expected no Command line when command absent, got: %s", got)
	}
}

func TestFormatEvent_SessionExited(t *testing.T) {
	e := makeEvent("session.exited", map[string]interface{}{
		"machine_name": "dev-box",
		"session_id":   "s-001",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Session exited") {
		t.Errorf("expected 'Session exited', got: %s", got)
	}
	if !strings.Contains(got, "dev\\-box") {
		t.Errorf("expected machine name, got: %s", got)
	}
}

func TestFormatEvent_SessionTerminated(t *testing.T) {
	e := makeEvent("session.terminated", map[string]interface{}{
		"machine_id": "m-999",
		"session_id": "s-002",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Session terminated") {
		t.Errorf("expected 'Session terminated', got: %s", got)
	}
}

// --- run events ---

func TestFormatEvent_RunCreated_WithJobName(t *testing.T) {
	e := makeEvent("run.created", map[string]interface{}{
		"job_name":     "nightly-build",
		"job_id":       "j-123",
		"run_id":       "r-456",
		"trigger_type": "cron",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "nightly\\-build") {
		t.Errorf("expected job_name in output, got: %s", got)
	}
	if strings.Contains(got, "j\\-123") {
		t.Errorf("should not use job_id when job_name present, got: %s", got)
	}
	if !strings.Contains(got, "Run created") {
		t.Errorf("expected 'Run created', got: %s", got)
	}
}

func TestFormatEvent_RunCreated_FallbackToJobID(t *testing.T) {
	e := makeEvent("run.created", map[string]interface{}{
		"job_id":       "j-123",
		"run_id":       "r-456",
		"trigger_type": "manual",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "j\\-123") {
		t.Errorf("expected job_id fallback in output, got: %s", got)
	}
}

func TestFormatEvent_RunStarted(t *testing.T) {
	e := makeEvent("run.started", map[string]interface{}{
		"job_name": "deploy-prod",
		"run_id":   "r-789",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Run started") {
		t.Errorf("expected 'Run started', got: %s", got)
	}
	if !strings.Contains(got, "deploy\\-prod") {
		t.Errorf("expected job_name, got: %s", got)
	}
}

func TestFormatEvent_RunCompleted_WithJobName(t *testing.T) {
	e := makeEvent("run.completed", map[string]interface{}{
		"job_name": "test-suite",
		"run_id":   "r-100",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Run completed") {
		t.Errorf("expected 'Run completed', got: %s", got)
	}
	if !strings.Contains(got, "test\\-suite") {
		t.Errorf("expected job_name, got: %s", got)
	}
}

func TestFormatEvent_RunFailed_WithJobName(t *testing.T) {
	e := makeEvent("run.failed", map[string]interface{}{
		"job_name": "test-suite",
		"run_id":   "r-101",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Run failed") {
		t.Errorf("expected 'Run failed', got: %s", got)
	}
}

func TestFormatEvent_RunCancelled(t *testing.T) {
	e := makeEvent("run.cancelled", map[string]interface{}{
		"job_name": "long-job",
		"run_id":   "r-202",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Run cancelled") {
		t.Errorf("expected 'Run cancelled', got: %s", got)
	}
	if !strings.Contains(got, "long\\-job") {
		t.Errorf("expected job_name, got: %s", got)
	}
}

// --- run step events ---

func TestFormatEvent_RunStepCompleted(t *testing.T) {
	e := makeEvent("run.step.completed", map[string]interface{}{
		"job_name":  "build",
		"step_name": "compile-assets",
		"step_id":   "step-1",
		"run_id":    "r-300",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Step completed") {
		t.Errorf("expected 'Step completed', got: %s", got)
	}
	if !strings.Contains(got, "compile\\-assets") {
		t.Errorf("expected step_name, got: %s", got)
	}
	if strings.Contains(got, "step\\-1") {
		t.Errorf("should not use step_id when step_name present, got: %s", got)
	}
}

func TestFormatEvent_RunStepCompleted_FallbackToStepID(t *testing.T) {
	e := makeEvent("run.step.completed", map[string]interface{}{
		"job_id":  "j-1",
		"step_id": "step-42",
		"run_id":  "r-300",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "step\\-42") {
		t.Errorf("expected step_id fallback, got: %s", got)
	}
}

func TestFormatEvent_RunStepFailed(t *testing.T) {
	e := makeEvent("run.step.failed", map[string]interface{}{
		"job_name":  "deploy",
		"step_name": "push-image",
		"run_id":    "r-301",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Step failed") {
		t.Errorf("expected 'Step failed', got: %s", got)
	}
}

// --- machine events ---

func TestFormatEvent_MachineConnected_WithDisplayName(t *testing.T) {
	e := makeEvent("machine.connected", map[string]interface{}{
		"display_name": "prod-worker-1",
		"machine_id":   "m-777",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Machine connected") {
		t.Errorf("expected 'Machine connected', got: %s", got)
	}
	if !strings.Contains(got, "prod\\-worker\\-1") {
		t.Errorf("expected display_name, got: %s", got)
	}
	if strings.Contains(got, "m\\-777") {
		t.Errorf("should not use machine_id when display_name present, got: %s", got)
	}
}

func TestFormatEvent_MachineDisconnected_FallbackToMachineID(t *testing.T) {
	e := makeEvent("machine.disconnected", map[string]interface{}{
		"machine_id": "m-888",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Machine disconnected") {
		t.Errorf("expected 'Machine disconnected', got: %s", got)
	}
	if !strings.Contains(got, "m\\-888") {
		t.Errorf("expected machine_id fallback, got: %s", got)
	}
}

// --- template events ---

func TestFormatEvent_TemplateCreated(t *testing.T) {
	e := makeEvent("template.created", map[string]interface{}{
		"template_name": "default-claude",
		"template_id":   "t-1",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Template created") {
		t.Errorf("expected 'Template created', got: %s", got)
	}
	if !strings.Contains(got, "default\\-claude") {
		t.Errorf("expected template_name, got: %s", got)
	}
}

func TestFormatEvent_TemplateUpdated(t *testing.T) {
	e := makeEvent("template.updated", map[string]interface{}{
		"template_id": "t-2",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Template updated") {
		t.Errorf("expected 'Template updated', got: %s", got)
	}
	if !strings.Contains(got, "t\\-2") {
		t.Errorf("expected template_id fallback, got: %s", got)
	}
}

func TestFormatEvent_TemplateDeleted(t *testing.T) {
	e := makeEvent("template.deleted", map[string]interface{}{
		"template_name": "old-template",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Template deleted") {
		t.Errorf("expected 'Template deleted', got: %s", got)
	}
}

// --- job events ---

func TestFormatEvent_JobCreated(t *testing.T) {
	e := makeEvent("job.created", map[string]interface{}{
		"job_name": "my-pipeline",
		"job_id":   "j-500",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Job created") {
		t.Errorf("expected 'Job created', got: %s", got)
	}
	if !strings.Contains(got, "my\\-pipeline") {
		t.Errorf("expected job_name, got: %s", got)
	}
}

func TestFormatEvent_JobCreated_FallbackToJobID(t *testing.T) {
	e := makeEvent("job.created", map[string]interface{}{
		"job_id": "j-501",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "j\\-501") {
		t.Errorf("expected job_id fallback, got: %s", got)
	}
}

func TestFormatEvent_JobUpdated(t *testing.T) {
	e := makeEvent("job.updated", map[string]interface{}{
		"job_name": "pipeline-v2",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Job updated") {
		t.Errorf("expected 'Job updated', got: %s", got)
	}
}

func TestFormatEvent_JobDeleted(t *testing.T) {
	e := makeEvent("job.deleted", map[string]interface{}{
		"job_name": "old-pipeline",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Job deleted") {
		t.Errorf("expected 'Job deleted', got: %s", got)
	}
}

// --- user events ---

func TestFormatEvent_UserCreated_WithEmail(t *testing.T) {
	e := makeEvent("user.created", map[string]interface{}{
		"email":   "alice@example.com",
		"user_id": "u-1",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "User created") {
		t.Errorf("expected 'User created', got: %s", got)
	}
	// email contains @ and . which are escaped
	if !strings.Contains(got, "alice@example") {
		t.Errorf("expected email in output, got: %s", got)
	}
	if strings.Contains(got, "u\\-1") {
		t.Errorf("should not use user_id when email present, got: %s", got)
	}
}

func TestFormatEvent_UserCreated_FallbackToUserID(t *testing.T) {
	e := makeEvent("user.created", map[string]interface{}{
		"user_id": "u-2",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "u\\-2") {
		t.Errorf("expected user_id fallback, got: %s", got)
	}
}

func TestFormatEvent_UserDeleted(t *testing.T) {
	e := makeEvent("user.deleted", map[string]interface{}{
		"email": "bob@example.com",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "User deleted") {
		t.Errorf("expected 'User deleted', got: %s", got)
	}
}

// --- schedule events ---

func TestFormatEvent_ScheduleCreated(t *testing.T) {
	e := makeEvent("schedule.created", map[string]interface{}{
		"job_name":  "nightly",
		"cron_expr": "0 2 * * *",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Schedule created") {
		t.Errorf("expected 'Schedule created', got: %s", got)
	}
	if !strings.Contains(got, "nightly") {
		t.Errorf("expected job_name, got: %s", got)
	}
	if !strings.Contains(got, "0 2") {
		t.Errorf("expected cron_expr in output, got: %s", got)
	}
}

func TestFormatEvent_SchedulePaused(t *testing.T) {
	e := makeEvent("schedule.paused", map[string]interface{}{
		"job_name":  "hourly-sync",
		"job_id":    "j-10",
		"cron_expr": "0 * * * *",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Schedule paused") {
		t.Errorf("expected 'Schedule paused', got: %s", got)
	}
	if !strings.Contains(got, "hourly\\-sync") {
		t.Errorf("expected job_name, got: %s", got)
	}
}

func TestFormatEvent_ScheduleResumed(t *testing.T) {
	e := makeEvent("schedule.resumed", map[string]interface{}{
		"job_id":    "j-11",
		"cron_expr": "30 6 * * 1",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Schedule resumed") {
		t.Errorf("expected 'Schedule resumed', got: %s", got)
	}
	if !strings.Contains(got, "j\\-11") {
		t.Errorf("expected job_id fallback, got: %s", got)
	}
}

func TestFormatEvent_ScheduleCreated_FallbackToJobID(t *testing.T) {
	e := makeEvent("schedule.created", map[string]interface{}{
		"schedule_id": "s-1",
		"job_id":      "j-1",
		"cron_expr":   "0 9 * * *",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "j\\-1") {
		t.Errorf("expected job_id fallback, got: %s", got)
	}
}

func TestFormatEvent_ScheduleDeleted(t *testing.T) {
	e := makeEvent("schedule.deleted", map[string]interface{}{
		"job_name":  "old-job",
		"cron_expr": "*/5 * * * *",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Schedule deleted") {
		t.Errorf("expected 'Schedule deleted', got: %s", got)
	}
}

// --- credential events ---

func TestFormatEvent_CredentialCreated(t *testing.T) {
	e := makeEvent("credential.created", map[string]interface{}{
		"credential_name": "github-token",
		"credential_id":   "cred-1",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Credential created") {
		t.Errorf("expected 'Credential created', got: %s", got)
	}
	if !strings.Contains(got, "github\\-token") {
		t.Errorf("expected credential_name, got: %s", got)
	}
}

func TestFormatEvent_CredentialCreated_FallbackToCredentialID(t *testing.T) {
	e := makeEvent("credential.created", map[string]interface{}{
		"credential_id": "cred-99",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "cred\\-99") {
		t.Errorf("expected credential_id fallback, got: %s", got)
	}
}

func TestFormatEvent_CredentialDeleted(t *testing.T) {
	e := makeEvent("credential.deleted", map[string]interface{}{
		"credential_name": "old-secret",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Credential deleted") {
		t.Errorf("expected 'Credential deleted', got: %s", got)
	}
}

// --- webhook events ---

func TestFormatEvent_WebhookCreated(t *testing.T) {
	e := makeEvent("webhook.created", map[string]interface{}{
		"webhook_name": "gh-push",
		"webhook_id":   "wh-1",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Webhook created") {
		t.Errorf("expected 'Webhook created', got: %s", got)
	}
	if !strings.Contains(got, "gh\\-push") {
		t.Errorf("expected webhook_name, got: %s", got)
	}
}

func TestFormatEvent_WebhookCreated_FallbackToWebhookID(t *testing.T) {
	e := makeEvent("webhook.created", map[string]interface{}{
		"webhook_id": "wh-2",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "wh\\-2") {
		t.Errorf("expected webhook_id fallback, got: %s", got)
	}
}

func TestFormatEvent_WebhookDeleted(t *testing.T) {
	e := makeEvent("webhook.deleted", map[string]interface{}{
		"webhook_name": "old-hook",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "Webhook deleted") {
		t.Errorf("expected 'Webhook deleted', got: %s", got)
	}
}

// --- trigger events (JSON dump) ---

func TestFormatEvent_TriggerCron(t *testing.T) {
	e := makeEvent("trigger.cron", map[string]interface{}{
		"job_id": "j-1",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "trigger\\.cron") {
		t.Errorf("expected escaped event type in output, got: %s", got)
	}
}

func TestFormatEvent_TriggerWebhook(t *testing.T) {
	e := makeEvent("trigger.webhook", map[string]interface{}{
		"webhook_id": "wh-1",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "trigger\\.webhook") {
		t.Errorf("expected escaped event type in output, got: %s", got)
	}
}

func TestFormatEvent_TriggerJobCompleted(t *testing.T) {
	e := makeEvent("trigger.job_completed", map[string]interface{}{
		"job_id": "j-2",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "trigger\\.job") {
		t.Errorf("expected escaped event type in output, got: %s", got)
	}
}

// --- default fallback ---

func TestFormatEvent_Default_UnknownType(t *testing.T) {
	e := makeEvent("some.unknown.event", map[string]interface{}{
		"foo": "bar",
	})
	got := FormatEvent(e)
	if !strings.Contains(got, "some\\.unknown\\.event") {
		t.Errorf("expected escaped event type in default output, got: %s", got)
	}
}

// --- MatchEventType and ShouldForwardEvent ---

func TestMatchEventType(t *testing.T) {
	tests := []struct {
		pattern, eventType string
		want               bool
	}{
		{"*", "any.event", true},
		{"run.*", "run.created", true},
		{"run.*", "run.completed", true},
		{"run.*", "session.started", false},
		{"session.started", "session.started", true},
		{"session.started", "session.exited", false},
	}
	for _, tt := range tests {
		got := MatchEventType(tt.pattern, tt.eventType)
		if got != tt.want {
			t.Errorf("MatchEventType(%q, %q) = %v, want %v", tt.pattern, tt.eventType, got, tt.want)
		}
	}
}

func TestShouldForwardEvent(t *testing.T) {
	tests := []struct {
		name      string
		patterns  []string
		eventType string
		want      bool
	}{
		{"nil patterns forwards all", nil, "run.created", true},
		{"empty patterns forwards all", []string{}, "run.created", true},
		{"wildcard forwards all", []string{"*"}, "session.started", true},
		{"prefix match", []string{"run.*"}, "run.failed", true},
		{"prefix no match", []string{"run.*"}, "session.started", false},
		{"exact match", []string{"job.created"}, "job.created", true},
		{"exact no match", []string{"job.created"}, "job.deleted", false},
		{"multiple patterns", []string{"run.*", "session.started"}, "session.started", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldForwardEvent(tt.patterns, tt.eventType)
			if got != tt.want {
				t.Errorf("ShouldForwardEvent(%v, %q) = %v, want %v", tt.patterns, tt.eventType, got, tt.want)
			}
		})
	}
}
