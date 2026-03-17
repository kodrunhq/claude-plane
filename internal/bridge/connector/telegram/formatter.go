// Package telegram implements a bridge connector for Telegram groups with topic support.
package telegram

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
)

var mdv2Replacer = strings.NewReplacer(
	"_", "\\_",
	"*", "\\*",
	"[", "\\[",
	"]", "\\]",
	"(", "\\(",
	")", "\\)",
	"~", "\\~",
	"`", "\\`",
	">", "\\>",
	"#", "\\#",
	"+", "\\+",
	"-", "\\-",
	"=", "\\=",
	"|", "\\|",
	"{", "\\{",
	"}", "\\}",
	".", "\\.",
	"!", "\\!",
)

func escapeMarkdownV2(s string) string {
	return mdv2Replacer.Replace(s)
}

// FormatEvent converts a claude-plane event into a Telegram MarkdownV2 message string.
func FormatEvent(e client.Event) string {
	str := func(key string) string {
		v, _ := e.Payload[key]
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}

	// nameOrID returns the value of nameKey if non-empty, otherwise falls back to idKey.
	nameOrID := func(nameKey, idKey string) string {
		if n := str(nameKey); n != "" {
			return n
		}
		return str(idKey)
	}

	switch e.Type {
	// ---- session events ----
	case "session.started":
		machine := nameOrID("machine_name", "machine_id")
		msg := fmt.Sprintf("🟢 *Session started*\nMachine: `%s`\nSession: `%s`",
			escapeMarkdownV2(machine), escapeMarkdownV2(str("session_id")))
		if cmd := str("command"); cmd != "" {
			msg += fmt.Sprintf("\nCommand: `%s`", escapeMarkdownV2(cmd))
		}
		return msg
	case "session.exited":
		machine := nameOrID("machine_name", "machine_id")
		msg := fmt.Sprintf("⚪ *Session exited*\nMachine: `%s`\nSession: `%s`",
			escapeMarkdownV2(machine), escapeMarkdownV2(str("session_id")))
		if cmd := str("command"); cmd != "" {
			msg += fmt.Sprintf("\nCommand: `%s`", escapeMarkdownV2(cmd))
		}
		return msg
	case "session.terminated":
		machine := nameOrID("machine_name", "machine_id")
		msg := fmt.Sprintf("🔴 *Session terminated*\nMachine: `%s`\nSession: `%s`",
			escapeMarkdownV2(machine), escapeMarkdownV2(str("session_id")))
		if cmd := str("command"); cmd != "" {
			msg += fmt.Sprintf("\nCommand: `%s`", escapeMarkdownV2(cmd))
		}
		return msg

	// ---- run events ----
	case "run.created":
		return fmt.Sprintf(
			"📋 *Run created*\nJob: `%s`\nRun: `%s`\nTrigger: %s",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(str("run_id")),
			escapeMarkdownV2(str("trigger_type")),
		)
	case "run.started":
		return fmt.Sprintf(
			"▶️ *Run started*\nJob: `%s`\nRun: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(str("run_id")),
		)
	case "run.completed":
		return fmt.Sprintf(
			"✅ *Run completed*\nJob: `%s`\nRun: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(str("run_id")),
		)
	case "run.failed":
		return fmt.Sprintf(
			"❌ *Run failed*\nJob: `%s`\nRun: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(str("run_id")),
		)
	case "run.cancelled":
		return fmt.Sprintf(
			"🚫 *Run cancelled*\nJob: `%s`\nRun: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(str("run_id")),
		)

	// ---- run step events ----
	case "run.step.completed":
		return fmt.Sprintf(
			"✅ *Step completed*\nJob: `%s`\nStep: `%s`\nRun: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(nameOrID("step_name", "step_id")),
			escapeMarkdownV2(str("run_id")),
		)
	case "run.step.failed":
		return fmt.Sprintf(
			"❌ *Step failed*\nJob: `%s`\nStep: `%s`\nRun: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(nameOrID("step_name", "step_id")),
			escapeMarkdownV2(str("run_id")),
		)

	// ---- machine events ----
	case "machine.connected":
		return fmt.Sprintf(
			"🖥️ *Machine connected*\nMachine: `%s`",
			escapeMarkdownV2(nameOrID("display_name", "machine_id")),
		)
	case "machine.disconnected":
		return fmt.Sprintf(
			"⚠️ *Machine disconnected*\nMachine: `%s`",
			escapeMarkdownV2(nameOrID("display_name", "machine_id")),
		)

	// ---- template events ----
	case "template.created":
		return fmt.Sprintf(
			"📄 *Template created*\nTemplate: `%s`",
			escapeMarkdownV2(nameOrID("template_name", "template_id")),
		)
	case "template.updated":
		return fmt.Sprintf(
			"📄 *Template updated*\nTemplate: `%s`",
			escapeMarkdownV2(nameOrID("template_name", "template_id")),
		)
	case "template.deleted":
		return fmt.Sprintf(
			"🗑️ *Template deleted*\nTemplate: `%s`",
			escapeMarkdownV2(nameOrID("template_name", "template_id")),
		)

	// ---- job events ----
	case "job.created":
		return fmt.Sprintf(
			"📋 *Job created*\nJob: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
		)
	case "job.updated":
		return fmt.Sprintf(
			"📋 *Job updated*\nJob: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
		)
	case "job.deleted":
		return fmt.Sprintf(
			"🗑️ *Job deleted*\nJob: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
		)

	// ---- user events ----
	case "user.created":
		return fmt.Sprintf(
			"👤 *User created*\nUser: `%s`",
			escapeMarkdownV2(nameOrID("email", "user_id")),
		)
	case "user.deleted":
		return fmt.Sprintf(
			"👤 *User deleted*\nUser: `%s`",
			escapeMarkdownV2(nameOrID("email", "user_id")),
		)

	// ---- schedule events ----
	case "schedule.created":
		return fmt.Sprintf(
			"🕐 *Schedule created*\nJob: `%s`\nCron: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(str("cron_expr")),
		)
	case "schedule.paused":
		return fmt.Sprintf(
			"⏸️ *Schedule paused*\nJob: `%s`\nCron: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(str("cron_expr")),
		)
	case "schedule.resumed":
		return fmt.Sprintf(
			"▶️ *Schedule resumed*\nJob: `%s`\nCron: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(str("cron_expr")),
		)
	case "schedule.deleted":
		return fmt.Sprintf(
			"🗑️ *Schedule deleted*\nJob: `%s`\nCron: `%s`",
			escapeMarkdownV2(nameOrID("job_name", "job_id")),
			escapeMarkdownV2(str("cron_expr")),
		)

	// ---- credential events ----
	case "credential.created":
		return fmt.Sprintf(
			"🔑 *Credential created*\nCredential: `%s`",
			escapeMarkdownV2(nameOrID("credential_name", "credential_id")),
		)
	case "credential.deleted":
		return fmt.Sprintf(
			"🔑 *Credential deleted*\nCredential: `%s`",
			escapeMarkdownV2(nameOrID("credential_name", "credential_id")),
		)

	// ---- webhook events ----
	case "webhook.created":
		return fmt.Sprintf(
			"🔗 *Webhook created*\nWebhook: `%s`",
			escapeMarkdownV2(nameOrID("webhook_name", "webhook_id")),
		)
	case "webhook.deleted":
		return fmt.Sprintf(
			"🔗 *Webhook deleted*\nWebhook: `%s`",
			escapeMarkdownV2(nameOrID("webhook_name", "webhook_id")),
		)

	// ---- trigger events (JSON dump) ----
	case "trigger.cron", "trigger.webhook", "trigger.job_completed":
		payload, _ := json.Marshal(e.Payload)
		return fmt.Sprintf("📢 *%s*\n%s", escapeMarkdownV2(e.Type), escapeMarkdownV2(string(payload)))

	default:
		payload, _ := json.Marshal(e.Payload)
		return fmt.Sprintf("📢 *%s*\n%s", escapeMarkdownV2(e.Type), escapeMarkdownV2(string(payload)))
	}
}

// MatchEventType reports whether eventType matches the given pattern.
// Patterns:
//   - "*" matches everything.
//   - "prefix.*" matches any event whose type starts with "prefix.".
//   - Otherwise an exact string comparison is used.
func MatchEventType(pattern, eventType string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(eventType, prefix)
	}
	return pattern == eventType
}

// ShouldForwardEvent reports whether an event of the given type should be forwarded
// given the configured event type patterns. If patterns is nil or empty, all events
// are forwarded.
func ShouldForwardEvent(patterns []string, eventType string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if MatchEventType(p, eventType) {
			return true
		}
	}
	return false
}
