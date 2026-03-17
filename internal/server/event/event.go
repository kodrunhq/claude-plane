//go:generate go run ../../../cmd/generate-event-types/main.go

// Package event provides the in-process event bus for claude-plane server.
// Components publish typed events; other components subscribe using glob-style
// patterns to react without direct coupling.
package event

import (
	"time"
)

// Event type constants grouped by domain.
const (
	// Run lifecycle events.
	TypeRunCreated   = "run.created"
	TypeRunStarted   = "run.started"
	TypeRunCompleted = "run.completed"
	TypeRunFailed    = "run.failed"
	TypeRunCancelled = "run.cancelled"

	// Session lifecycle events.
	TypeSessionStarted    = "session.started"
	TypeSessionExited     = "session.exited"
	TypeSessionTerminated = "session.terminated"

	// Machine connectivity events.
	TypeMachineConnected    = "machine.connected"
	TypeMachineDisconnected = "machine.disconnected"

	// Trigger events.
	TypeTriggerCron         = "trigger.cron"
	TypeTriggerWebhook      = "trigger.webhook"
	TypeTriggerJobCompleted = "trigger.job_completed"

	// Template lifecycle events.
	TypeTemplateCreated = "template.created"
	TypeTemplateUpdated = "template.updated"
	TypeTemplateDeleted = "template.deleted"

	// Run step events.
	TypeJobRunStepCompleted = "run.step.completed"
	TypeJobRunStepFailed    = "run.step.failed"

	// Job lifecycle events.
	TypeJobCreated = "job.created"
	TypeJobUpdated = "job.updated"
	TypeJobDeleted = "job.deleted"

	// User lifecycle events.
	TypeUserCreated = "user.created"
	TypeUserDeleted = "user.deleted"

	// Schedule lifecycle events.
	TypeScheduleCreated = "schedule.created"
	TypeSchedulePaused  = "schedule.paused"
	TypeScheduleResumed = "schedule.resumed"
	TypeScheduleDeleted = "schedule.deleted"

	// Credential lifecycle events.
	TypeCredentialCreated = "credential.created"
	TypeCredentialDeleted = "credential.deleted"

	// Webhook lifecycle events.
	TypeWebhookCreated = "webhook.created"
	TypeWebhookDeleted = "webhook.deleted"
	TypeWebhookTest    = "webhook.test"
)

// Event is the envelope for all bus messages.
// Payload carries event-specific data; consumers should document expected keys.
type Event struct {
	EventID   string         `json:"event_id"`   // UUID assigned at creation
	Type      string         `json:"event_type"` // e.g. "run.completed"
	Timestamp time.Time      `json:"timestamp"`
	Source    string         `json:"source"`  // e.g. "orchestrator", "connmgr"
	Payload   map[string]any `json:"payload"` // event-specific key/value pairs
}
