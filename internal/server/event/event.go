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
	TypeSessionStarted = "session.started"
	TypeSessionExited  = "session.exited"

	// Machine connectivity events.
	TypeMachineConnected    = "machine.connected"
	TypeMachineDisconnected = "machine.disconnected"

	// Trigger events.
	TypeTriggerCron         = "trigger.cron"
	TypeTriggerWebhook      = "trigger.webhook"
	TypeTriggerJobCompleted = "trigger.job_completed"
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
