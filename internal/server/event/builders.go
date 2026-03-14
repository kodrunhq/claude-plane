package event

import (
	"time"

	"github.com/google/uuid"
)

// newEvent is the shared constructor that stamps EventID and Timestamp.
func newEvent(eventType, source string, payload map[string]any) Event {
	if payload == nil {
		payload = make(map[string]any)
	}
	return Event{
		EventID:   uuid.New().String(),
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Source:    source,
		Payload:   payload,
	}
}

// NewRunEvent constructs an event for run lifecycle transitions.
// eventType should be one of the TypeRun* constants.
func NewRunEvent(eventType, runID, jobID, status, triggerType string) Event {
	return newEvent(eventType, "orchestrator", map[string]any{
		"run_id":       runID,
		"job_id":       jobID,
		"status":       status,
		"trigger_type": triggerType,
	})
}

// NewSessionEvent constructs an event for session lifecycle changes.
// eventType should be one of the TypeSession* constants.
func NewSessionEvent(eventType, sessionID, machineID string) Event {
	return newEvent(eventType, "session", map[string]any{
		"session_id": sessionID,
		"machine_id": machineID,
	})
}

// NewMachineEvent constructs an event for agent connectivity changes.
// eventType should be one of the TypeMachine* constants.
func NewMachineEvent(eventType, machineID string) Event {
	return newEvent(eventType, "connmgr", map[string]any{
		"machine_id": machineID,
	})
}

// NewTriggerEvent constructs an event representing an external trigger.
// source identifies the trigger origin (e.g. "webhook:github", "cron").
// payload carries trigger-specific fields and is merged as-is.
func NewTriggerEvent(eventType, source string, payload map[string]any) Event {
	return newEvent(eventType, source, payload)
}

// NewTemplateEvent constructs an event for template lifecycle changes.
func NewTemplateEvent(eventType, templateID, userID string) Event {
	return newEvent(eventType, "template", map[string]any{
		"template_id": templateID,
		"user_id":     userID,
	})
}
