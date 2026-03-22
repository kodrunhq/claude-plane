package notify

import (
	"encoding/json"
	"fmt"
	"html"

	"github.com/kodrunhq/claude-plane/internal/server/event"
)

// EventRenderer converts an event into a subject and body for notification delivery.
type EventRenderer func(e event.Event) (subject string, body string)

// TelegramEventRenderer returns an EventRenderer that produces HTML-formatted
// messages suitable for Telegram's "HTML" parse mode.
func TelegramEventRenderer(e event.Event) (subject string, body string) {
	str := func(key string) string {
		v, ok := e.Payload[key]
		if !ok || v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}

	nameOrID := func(nameKey, idKey string) string {
		if n := str(nameKey); n != "" {
			return n
		}
		return str(idKey)
	}

	optLine := func(label, value string) string {
		if value == "" {
			return ""
		}
		return fmt.Sprintf("\n%s: <code>%s</code>", label, html.EscapeString(value))
	}

	switch e.Type {
	// ---- session events ----
	case event.TypeSessionStarted:
		machine := nameOrID("machine_name", "machine_id")
		msg := fmt.Sprintf("🟢 <b>Session started</b>\nMachine: <code>%s</code>\nSession: <code>%s</code>",
			html.EscapeString(machine), html.EscapeString(str("session_id")))
		msg += optLine("Name", str("session_name"))
		msg += optLine("Command", str("command"))
		subject = "Session started"
		body = msg

	case event.TypeSessionExited:
		machine := nameOrID("machine_name", "machine_id")
		msg := fmt.Sprintf("⚪ <b>Session exited</b>\nMachine: <code>%s</code>\nSession: <code>%s</code>",
			html.EscapeString(machine), html.EscapeString(str("session_id")))
		msg += optLine("Command", str("command"))
		subject = "Session exited"
		body = msg

	case event.TypeSessionTerminated:
		machine := nameOrID("machine_name", "machine_id")
		msg := fmt.Sprintf("🔴 <b>Session terminated</b>\nMachine: <code>%s</code>\nSession: <code>%s</code>",
			html.EscapeString(machine), html.EscapeString(str("session_id")))
		msg += optLine("Command", str("command"))
		subject = "Session terminated"
		body = msg

	case event.TypeSessionDispatchFailed:
		subject = "Session dispatch failed"
		body = fmt.Sprintf("⚠️ <b>Session dispatch failed</b>\nSession: <code>%s</code>%s",
			html.EscapeString(str("session_id")),
			optLine("Reason", str("reason")))

	case event.TypeSessionWaitingForInput:
		subject = "Session waiting for input"
		body = fmt.Sprintf("⏳ <b>Session waiting for input</b>\nSession: <code>%s</code>%s",
			html.EscapeString(str("session_id")),
			optLine("Machine", nameOrID("machine_name", "machine_id")))

	case event.TypeSessionResumed:
		subject = "Session resumed"
		body = fmt.Sprintf("▶️ <b>Session resumed</b>\nSession: <code>%s</code>%s",
			html.EscapeString(str("session_id")),
			optLine("Machine", nameOrID("machine_name", "machine_id")))

	// ---- run events ----
	case event.TypeRunCreated:
		job := nameOrID("job_name", "job_id")
		msg := "📋 <b>Run created</b>"
		msg += optLine("Job", job)
		msg += fmt.Sprintf("\nRun: <code>%s</code>", html.EscapeString(str("run_id")))
		if t := str("trigger_type"); t != "" {
			msg += fmt.Sprintf("\nTrigger: %s", html.EscapeString(t))
		}
		subject = "Run created"
		body = msg

	case event.TypeRunStarted:
		subject = "Run started"
		body = fmt.Sprintf("▶️ <b>Run started</b>%s\nRun: <code>%s</code>",
			optLine("Job", nameOrID("job_name", "job_id")),
			html.EscapeString(str("run_id")))

	case event.TypeRunCompleted:
		subject = "Run completed"
		body = fmt.Sprintf("✅ <b>Run completed</b>%s\nRun: <code>%s</code>",
			optLine("Job", nameOrID("job_name", "job_id")),
			html.EscapeString(str("run_id")))

	case event.TypeRunFailed:
		subject = "Run failed"
		body = fmt.Sprintf("❌ <b>Run failed</b>%s\nRun: <code>%s</code>",
			optLine("Job", nameOrID("job_name", "job_id")),
			html.EscapeString(str("run_id")))

	case event.TypeRunCancelled:
		subject = "Run cancelled"
		body = fmt.Sprintf("🚫 <b>Run cancelled</b>%s\nRun: <code>%s</code>",
			optLine("Job", nameOrID("job_name", "job_id")),
			html.EscapeString(str("run_id")))

	// ---- run step events ----
	case event.TypeJobRunStepStarted:
		subject = "Step started"
		body = fmt.Sprintf("▶️ <b>Step started</b>%s%s\nRun: <code>%s</code>",
			optLine("Job", nameOrID("job_name", "job_id")),
			optLine("Step", nameOrID("step_name", "step_id")),
			html.EscapeString(str("run_id")))

	case event.TypeJobRunStepCompleted:
		subject = "Step completed"
		body = fmt.Sprintf("✅ <b>Step completed</b>%s%s\nRun: <code>%s</code>",
			optLine("Job", nameOrID("job_name", "job_id")),
			optLine("Step", nameOrID("step_name", "step_id")),
			html.EscapeString(str("run_id")))

	case event.TypeJobRunStepFailed:
		subject = "Step failed"
		body = fmt.Sprintf("❌ <b>Step failed</b>%s%s\nRun: <code>%s</code>",
			optLine("Job", nameOrID("job_name", "job_id")),
			optLine("Step", nameOrID("step_name", "step_id")),
			html.EscapeString(str("run_id")))

	// ---- machine events ----
	case event.TypeMachineConnected:
		subject = "Machine connected"
		body = fmt.Sprintf("🖥️ <b>Machine connected</b>\nMachine: <code>%s</code>",
			html.EscapeString(nameOrID("display_name", "machine_id")))

	case event.TypeMachineDisconnected:
		subject = "Machine disconnected"
		body = fmt.Sprintf("⚠️ <b>Machine disconnected</b>\nMachine: <code>%s</code>",
			html.EscapeString(nameOrID("display_name", "machine_id")))

	case event.TypeMachineStale:
		subject = "Machine stale"
		body = fmt.Sprintf("💀 <b>Machine stale</b>\nMachine: <code>%s</code>",
			html.EscapeString(nameOrID("display_name", "machine_id")))

	// ---- template events ----
	case event.TypeTemplateCreated:
		subject = "Template created"
		body = fmt.Sprintf("📄 <b>Template created</b>\nTemplate: <code>%s</code>",
			html.EscapeString(nameOrID("template_name", "template_id")))

	case event.TypeTemplateUpdated:
		subject = "Template updated"
		body = fmt.Sprintf("📄 <b>Template updated</b>\nTemplate: <code>%s</code>",
			html.EscapeString(nameOrID("template_name", "template_id")))

	case event.TypeTemplateDeleted:
		subject = "Template deleted"
		body = fmt.Sprintf("🗑️ <b>Template deleted</b>\nTemplate: <code>%s</code>",
			html.EscapeString(nameOrID("template_name", "template_id")))

	// ---- job events ----
	case event.TypeJobCreated:
		subject = "Job created"
		body = fmt.Sprintf("📋 <b>Job created</b>\nJob: <code>%s</code>",
			html.EscapeString(nameOrID("job_name", "job_id")))

	case event.TypeJobUpdated:
		subject = "Job updated"
		body = fmt.Sprintf("📋 <b>Job updated</b>\nJob: <code>%s</code>",
			html.EscapeString(nameOrID("job_name", "job_id")))

	case event.TypeJobDeleted:
		subject = "Job deleted"
		body = fmt.Sprintf("🗑️ <b>Job deleted</b>\nJob: <code>%s</code>",
			html.EscapeString(nameOrID("job_name", "job_id")))

	// ---- user events ----
	case event.TypeUserCreated:
		subject = "User created"
		body = fmt.Sprintf("👤 <b>User created</b>\nUser: <code>%s</code>",
			html.EscapeString(nameOrID("email", "user_id")))

	case event.TypeUserDeleted:
		subject = "User deleted"
		body = fmt.Sprintf("👤 <b>User deleted</b>\nUser: <code>%s</code>",
			html.EscapeString(nameOrID("email", "user_id")))

	// ---- schedule events ----
	case event.TypeScheduleCreated:
		subject = "Schedule created"
		body = fmt.Sprintf("🕐 <b>Schedule created</b>%s%s",
			optLine("Job", nameOrID("job_name", "job_id")),
			optLine("Cron", str("cron_expr")))

	case event.TypeSchedulePaused:
		subject = "Schedule paused"
		body = fmt.Sprintf("⏸️ <b>Schedule paused</b>%s%s",
			optLine("Job", nameOrID("job_name", "job_id")),
			optLine("Cron", str("cron_expr")))

	case event.TypeScheduleResumed:
		subject = "Schedule resumed"
		body = fmt.Sprintf("▶️ <b>Schedule resumed</b>%s%s",
			optLine("Job", nameOrID("job_name", "job_id")),
			optLine("Cron", str("cron_expr")))

	case event.TypeScheduleDeleted:
		subject = "Schedule deleted"
		body = fmt.Sprintf("🗑️ <b>Schedule deleted</b>%s%s",
			optLine("Job", nameOrID("job_name", "job_id")),
			optLine("Cron", str("cron_expr")))

	// ---- credential events ----
	case event.TypeCredentialCreated:
		subject = "Credential created"
		body = fmt.Sprintf("🔑 <b>Credential created</b>\nCredential: <code>%s</code>",
			html.EscapeString(nameOrID("credential_name", "credential_id")))

	case event.TypeCredentialDeleted:
		subject = "Credential deleted"
		body = fmt.Sprintf("🔑 <b>Credential deleted</b>\nCredential: <code>%s</code>",
			html.EscapeString(nameOrID("credential_name", "credential_id")))

	// ---- webhook events ----
	case event.TypeWebhookCreated:
		subject = "Webhook created"
		body = fmt.Sprintf("🔗 <b>Webhook created</b>\nWebhook: <code>%s</code>",
			html.EscapeString(nameOrID("webhook_name", "webhook_id")))

	case event.TypeWebhookUpdated:
		subject = "Webhook updated"
		body = fmt.Sprintf("🔗 <b>Webhook updated</b>\nWebhook: <code>%s</code>",
			html.EscapeString(nameOrID("webhook_name", "webhook_id")))

	case event.TypeWebhookDeleted:
		subject = "Webhook deleted"
		body = fmt.Sprintf("🔗 <b>Webhook deleted</b>\nWebhook: <code>%s</code>",
			html.EscapeString(nameOrID("webhook_name", "webhook_id")))

	case event.TypeWebhookTest:
		subject = "Webhook test"
		body = fmt.Sprintf("🔗 <b>Webhook test</b>\nWebhook: <code>%s</code>",
			html.EscapeString(nameOrID("webhook_name", "webhook_id")))

	// ---- trigger events ----
	case event.TypeTriggerCron, event.TypeTriggerWebhook, event.TypeTriggerJobCompleted:
		subject = e.Type
		payload, _ := json.Marshal(e.Payload)
		body = fmt.Sprintf("📢 <b>%s</b>\n%s", html.EscapeString(e.Type), html.EscapeString(string(payload)))

	// ---- server events ----
	case event.TypeServerShutdown:
		subject = "Server shutdown"
		body = "🛑 <b>Server shutdown</b>"

	// ---- bridge events ----
	case event.TypeBridgeStarted:
		subject = "Bridge started"
		body = "🌉 <b>Bridge started</b>"

	case event.TypeBridgeStopped:
		subject = "Bridge stopped"
		body = "🌉 <b>Bridge stopped</b>"

	case event.TypeBridgeConnectorStarted:
		subject = "Bridge connector started"
		body = fmt.Sprintf("🔌 <b>Bridge connector started</b>%s",
			optLine("Connector", str("connector_name")))

	case event.TypeBridgeConnectorError:
		subject = "Bridge connector error"
		body = fmt.Sprintf("🔌 <b>Bridge connector error</b>%s%s",
			optLine("Connector", str("connector_name")),
			optLine("Error", str("error")))

	case event.TypeBridgeConnectorCommand:
		subject = "Bridge connector command"
		body = fmt.Sprintf("🔌 <b>Bridge connector command</b>%s%s",
			optLine("Connector", str("connector_name")),
			optLine("Command", str("command")))

	default:
		subject = e.Type
		payload, _ := json.Marshal(e.Payload)
		body = fmt.Sprintf("📢 <b>%s</b>\n%s", html.EscapeString(e.Type), html.EscapeString(string(payload)))
	}

	return subject, body
}
