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
func NewRunEvent(eventType, runID, jobID, status, triggerType, jobName string) Event {
	return newEvent(eventType, "orchestrator", map[string]any{
		"run_id":       runID,
		"job_id":       jobID,
		"status":       status,
		"trigger_type": triggerType,
		"job_name":     jobName,
	})
}

// NewSessionEvent constructs an event for session lifecycle changes.
// eventType should be one of the TypeSession* constants.
func NewSessionEvent(eventType, sessionID, machineID, machineName, command string) Event {
	return newEvent(eventType, "session", map[string]any{
		"session_id":   sessionID,
		"machine_id":   machineID,
		"machine_name": machineName,
		"command":      command,
	})
}

// NewMachineEvent constructs an event for agent connectivity changes.
// eventType should be one of the TypeMachine* constants.
func NewMachineEvent(eventType, machineID, displayName string) Event {
	return newEvent(eventType, "connmgr", map[string]any{
		"machine_id":   machineID,
		"display_name": displayName,
	})
}

// NewTriggerEvent constructs an event representing an external trigger.
// source identifies the trigger origin (e.g. "webhook:github", "cron").
// payload carries trigger-specific fields and is merged as-is.
func NewTriggerEvent(eventType, source string, payload map[string]any) Event {
	return newEvent(eventType, source, payload)
}

// NewTemplateEvent constructs an event for template lifecycle changes.
func NewTemplateEvent(eventType, templateID, userID, templateName string) Event {
	return newEvent(eventType, "template", map[string]any{
		"template_id":   templateID,
		"user_id":       userID,
		"template_name": templateName,
	})
}

// NewRunStepEvent constructs an event for individual run step completions or failures.
// eventType should be one of TypeJobRunStepCompleted or TypeJobRunStepFailed.
func NewRunStepEvent(eventType, runID, runStepID, stepID, status, stepName, jobName string) Event {
	return newEvent(eventType, "orchestrator", map[string]any{
		"run_id":      runID,
		"run_step_id": runStepID,
		"step_id":     stepID,
		"status":      status,
		"step_name":   stepName,
		"job_name":    jobName,
	})
}

// NewJobEvent constructs an event for job lifecycle changes.
func NewJobEvent(eventType, jobID, jobName, userID string) Event {
	return newEvent(eventType, "handler", map[string]any{
		"job_id":   jobID,
		"job_name": jobName,
		"user_id":  userID,
	})
}

// NewUserEvent constructs an event for user lifecycle changes.
func NewUserEvent(eventType, userID, email string) Event {
	return newEvent(eventType, "handler", map[string]any{
		"user_id": userID,
		"email":   email,
	})
}

// NewScheduleEvent constructs an event for schedule lifecycle changes.
func NewScheduleEvent(eventType, scheduleID, jobID, jobName, cronExpr string) Event {
	return newEvent(eventType, "handler", map[string]any{
		"schedule_id": scheduleID,
		"job_id":      jobID,
		"job_name":    jobName,
		"cron_expr":   cronExpr,
	})
}

// NewCredentialEvent constructs an event for credential lifecycle changes.
// Does NOT include the credential value — only ID and name.
func NewCredentialEvent(eventType, credentialID, credentialName, userID string) Event {
	return newEvent(eventType, "handler", map[string]any{
		"credential_id":   credentialID,
		"credential_name": credentialName,
		"user_id":         userID,
	})
}

// NewWebhookEvent constructs an event for webhook lifecycle changes.
func NewWebhookEvent(eventType, webhookID, webhookName string) Event {
	return newEvent(eventType, "handler", map[string]any{
		"webhook_id":   webhookID,
		"webhook_name": webhookName,
	})
}
