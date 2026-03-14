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

	switch e.Type {
	case "session.started":
		return fmt.Sprintf(
			"🟢 *Session started*\nMachine: `%s`\nSession: `%s`",
			str("machine_id"), str("session_id"),
		)
	case "session.exited":
		return fmt.Sprintf(
			"⚪ *Session exited*\nMachine: `%s`\nSession: `%s`",
			str("machine_id"), str("session_id"),
		)
	case "session.terminated":
		return fmt.Sprintf(
			"🔴 *Session terminated*\nMachine: `%s`\nSession: `%s`",
			str("machine_id"), str("session_id"),
		)
	case "run.created":
		return fmt.Sprintf(
			"📋 *Run created*\nJob: `%s`\nRun: `%s`\nTrigger: %s",
			str("job_id"), str("run_id"), escapeMarkdownV2(str("trigger_type")),
		)
	case "run.completed":
		return fmt.Sprintf(
			"✅ *Run completed*\nRun: `%s`",
			str("run_id"),
		)
	case "run.failed":
		return fmt.Sprintf(
			"❌ *Run failed*\nRun: `%s`",
			str("run_id"),
		)
	case "machine.connected":
		return fmt.Sprintf(
			"🖥️ *Machine connected*\nMachine: `%s`",
			str("machine_id"),
		)
	case "machine.disconnected":
		return fmt.Sprintf(
			"⚠️ *Machine disconnected*\nMachine: `%s`",
			str("machine_id"),
		)
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
