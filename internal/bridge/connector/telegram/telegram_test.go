package telegram_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
	"github.com/kodrunhq/claude-plane/internal/bridge/connector/telegram"
)

// --- FormatEvent tests ---

func makeEvent(eventType string, payload map[string]interface{}) client.Event {
	return client.Event{
		EventID:   "evt-1",
		Type:      eventType,
		Timestamp: time.Now(),
		Source:    "test",
		Payload:   payload,
	}
}

func TestFormatEvent_SessionStarted(t *testing.T) {
	e := makeEvent("session.started", map[string]interface{}{
		"machine_id": "m1",
		"session_id": "s1",
	})
	got := telegram.FormatEvent(e)
	want := "🟢 *Session started*\nMachine: `m1`\nSession: `s1`"
	if got != want {
		t.Errorf("FormatEvent(session.started)\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatEvent_SessionExited(t *testing.T) {
	e := makeEvent("session.exited", map[string]interface{}{
		"machine_id": "m2",
		"session_id": "s2",
	})
	got := telegram.FormatEvent(e)
	want := "⚪ *Session exited*\nMachine: `m2`\nSession: `s2`"
	if got != want {
		t.Errorf("FormatEvent(session.exited)\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatEvent_SessionTerminated(t *testing.T) {
	e := makeEvent("session.terminated", map[string]interface{}{
		"machine_id": "m3",
		"session_id": "s3",
	})
	got := telegram.FormatEvent(e)
	want := "🔴 *Session terminated*\nMachine: `m3`\nSession: `s3`"
	if got != want {
		t.Errorf("FormatEvent(session.terminated)\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatEvent_RunCreated(t *testing.T) {
	e := makeEvent("run.created", map[string]interface{}{
		"job_id":       "j1",
		"run_id":       "r1",
		"trigger_type": "manual",
	})
	got := telegram.FormatEvent(e)
	want := "📋 *Run created*\nJob: `j1`\nRun: `r1`\nTrigger: manual"
	if got != want {
		t.Errorf("FormatEvent(run.created)\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatEvent_RunCompleted(t *testing.T) {
	e := makeEvent("run.completed", map[string]interface{}{
		"run_id": "r2",
	})
	got := telegram.FormatEvent(e)
	// run.completed now includes a Job line (empty when no job_id/job_name provided)
	want := "✅ *Run completed*\nJob: ``\nRun: `r2`"
	if got != want {
		t.Errorf("FormatEvent(run.completed)\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatEvent_RunFailed(t *testing.T) {
	e := makeEvent("run.failed", map[string]interface{}{
		"run_id": "r3",
	})
	got := telegram.FormatEvent(e)
	// run.failed now includes a Job line (empty when no job_id/job_name provided)
	want := "❌ *Run failed*\nJob: ``\nRun: `r3`"
	if got != want {
		t.Errorf("FormatEvent(run.failed)\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatEvent_MachineConnected(t *testing.T) {
	e := makeEvent("machine.connected", map[string]interface{}{
		"machine_id": "mach-1",
	})
	got := telegram.FormatEvent(e)
	// Hyphens inside backtick spans are escaped for MarkdownV2
	want := "🖥️ *Machine connected*\nMachine: `mach\\-1`"
	if got != want {
		t.Errorf("FormatEvent(machine.connected)\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatEvent_MachineDisconnected(t *testing.T) {
	e := makeEvent("machine.disconnected", map[string]interface{}{
		"machine_id": "mach-2",
	})
	got := telegram.FormatEvent(e)
	// Hyphens inside backtick spans are escaped for MarkdownV2
	want := "⚠️ *Machine disconnected*\nMachine: `mach\\-2`"
	if got != want {
		t.Errorf("FormatEvent(machine.disconnected)\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatEvent_Default(t *testing.T) {
	e := makeEvent("custom.event", map[string]interface{}{
		"key": "value",
	})
	got := telegram.FormatEvent(e)
	// Default must start with the escaped event type header (MarkdownV2)
	prefix := "📢 *custom\\.event*\n"
	if len(got) < len(prefix) || got[:len(prefix)] != prefix {
		t.Errorf("FormatEvent(default) missing prefix\ngot: %q", got)
	}
}

// --- ParseCommand tests ---

func TestParseCommand_Start(t *testing.T) {
	cmd, err := telegram.ParseCommand("/start my-template worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name != "start" {
		t.Errorf("Name = %q, want %q", cmd.Name, "start")
	}
	if len(cmd.Args) != 2 || cmd.Args[0] != "my-template" || cmd.Args[1] != "worker-1" {
		t.Errorf("Args = %v, want [my-template worker-1]", cmd.Args)
	}
}

func TestParseCommand_List(t *testing.T) {
	cmd, err := telegram.ParseCommand("/list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name != "list" {
		t.Errorf("Name = %q, want %q", cmd.Name, "list")
	}
	if len(cmd.Args) != 0 {
		t.Errorf("Args = %v, want []", cmd.Args)
	}
}

func TestParseCommand_Status(t *testing.T) {
	cmd, err := telegram.ParseCommand("/status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name != "status" {
		t.Errorf("Name = %q, want %q", cmd.Name, "status")
	}
}

func TestParseCommand_Kill(t *testing.T) {
	cmd, err := telegram.ParseCommand("/kill session-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name != "kill" {
		t.Errorf("Name = %q, want %q", cmd.Name, "kill")
	}
	if len(cmd.Args) != 1 || cmd.Args[0] != "session-abc" {
		t.Errorf("Args = %v, want [session-abc]", cmd.Args)
	}
}

func TestParseCommand_Inject(t *testing.T) {
	cmd, err := telegram.ParseCommand("/inject session-abc hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name != "inject" {
		t.Errorf("Name = %q, want %q", cmd.Name, "inject")
	}
	if len(cmd.Args) < 2 {
		t.Fatalf("Args = %v, want at least 2", cmd.Args)
	}
	if cmd.Args[0] != "session-abc" {
		t.Errorf("Args[0] = %q, want session-abc", cmd.Args[0])
	}
}

func TestParseCommand_Machines(t *testing.T) {
	cmd, err := telegram.ParseCommand("/machines")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name != "machines" {
		t.Errorf("Name = %q, want %q", cmd.Name, "machines")
	}
}

func TestParseCommand_Help(t *testing.T) {
	cmd, err := telegram.ParseCommand("/help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name != "help" {
		t.Errorf("Name = %q, want %q", cmd.Name, "help")
	}
}

func TestParseCommand_WithVars(t *testing.T) {
	cmd, err := telegram.ParseCommand("/start my-template worker-1 | FOO=bar BAZ=qux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name != "start" {
		t.Errorf("Name = %q, want %q", cmd.Name, "start")
	}
	if len(cmd.Args) != 2 {
		t.Errorf("Args = %v, want [my-template worker-1]", cmd.Args)
	}
	if cmd.Vars["FOO"] != "bar" {
		t.Errorf("Vars[FOO] = %q, want %q", cmd.Vars["FOO"], "bar")
	}
	if cmd.Vars["BAZ"] != "qux" {
		t.Errorf("Vars[BAZ] = %q, want %q", cmd.Vars["BAZ"], "qux")
	}
}

func TestParseCommand_WithVars_SingleVar(t *testing.T) {
	cmd, err := telegram.ParseCommand("/start tmpl m1 | MYVAR=hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Vars["MYVAR"] != "hello" {
		t.Errorf("Vars[MYVAR] = %q, want %q", cmd.Vars["MYVAR"], "hello")
	}
}

// --- Invalid command tests ---

func TestParseCommand_Invalid_NoSlash(t *testing.T) {
	_, err := telegram.ParseCommand("list")
	if err == nil {
		t.Error("expected error for command without slash")
	}
}

func TestParseCommand_Invalid_UnknownCommand(t *testing.T) {
	_, err := telegram.ParseCommand("/unknown")
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestParseCommand_Invalid_EmptyText(t *testing.T) {
	_, err := telegram.ParseCommand("")
	if err == nil {
		t.Error("expected error for empty text")
	}
}

func TestParseCommand_Invalid_MalformedVar(t *testing.T) {
	_, err := telegram.ParseCommand("/start tmpl m1 | NOEQUALSIGN")
	if err == nil {
		t.Error("expected error for malformed variable (no = sign)")
	}
}

// --- MatchEventType tests ---

func TestMatchEventType_Wildcard(t *testing.T) {
	if !telegram.MatchEventType("*", "session.started") {
		t.Error("* should match any event type")
	}
}

func TestMatchEventType_PrefixWildcard(t *testing.T) {
	if !telegram.MatchEventType("session.*", "session.started") {
		t.Error("session.* should match session.started")
	}
	if telegram.MatchEventType("session.*", "run.started") {
		t.Error("session.* should not match run.started")
	}
}

func TestMatchEventType_Exact(t *testing.T) {
	if !telegram.MatchEventType("run.completed", "run.completed") {
		t.Error("exact match should work")
	}
	if telegram.MatchEventType("run.completed", "run.failed") {
		t.Error("exact match should not match different event type")
	}
}

func TestMatchEventType_EmptyPatterns_AllowsAll(t *testing.T) {
	// When no patterns, all events pass — tested via ShouldForwardEvent
	if !telegram.ShouldForwardEvent(nil, "anything") {
		t.Error("nil patterns should allow all events")
	}
	if !telegram.ShouldForwardEvent([]string{}, "anything") {
		t.Error("empty patterns should allow all events")
	}
}

func TestShouldForwardEvent_WithPatterns(t *testing.T) {
	patterns := []string{"session.*", "machine.connected"}
	if !telegram.ShouldForwardEvent(patterns, "session.started") {
		t.Error("session.started should match session.*")
	}
	if !telegram.ShouldForwardEvent(patterns, "machine.connected") {
		t.Error("machine.connected should match exact pattern")
	}
	if telegram.ShouldForwardEvent(patterns, "run.completed") {
		t.Error("run.completed should not match any pattern")
	}
}

// --- CheckRateLimit tests ---

func makeResp(statusCode int, body string) (*http.Response, []byte) {
	raw := []byte(body)
	resp := &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	return resp, raw
}

func TestCheckRateLimit_429WithRetryAfter(t *testing.T) {
	body := `{"ok":false,"parameters":{"retry_after":1}}`
	resp, raw := makeResp(http.StatusTooManyRequests, body)

	ctx := context.Background()
	start := time.Now()
	err := telegram.CheckRateLimit(ctx, resp, raw)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error for 429, got nil")
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("expected wait of ~1s, waited only %v", elapsed)
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("error message should mention rate limited, got: %v", err)
	}
}

func TestCheckRateLimit_429ContextCancelled(t *testing.T) {
	body := `{"ok":false,"parameters":{"retry_after":30}}`
	resp, raw := makeResp(http.StatusTooManyRequests, body)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel the context immediately.
	cancel()

	err := telegram.CheckRateLimit(ctx, resp, raw)

	if err == nil {
		t.Fatal("expected error after context cancellation, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestCheckRateLimit_200(t *testing.T) {
	resp, raw := makeResp(http.StatusOK, `{"ok":true,"result":[]}`)

	err := telegram.CheckRateLimit(context.Background(), resp, raw)
	if err != nil {
		t.Errorf("expected nil for 200, got: %v", err)
	}
}

func TestCheckRateLimit_500(t *testing.T) {
	resp, raw := makeResp(http.StatusInternalServerError, `internal server error`)

	err := telegram.CheckRateLimit(context.Background(), resp, raw)
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error message should contain status code 500, got: %v", err)
	}
}
