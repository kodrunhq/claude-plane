package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
	"github.com/kodrunhq/claude-plane/internal/bridge/state"
)

const (
	eventPollInterval = 5 * time.Second
	telegramAPIBase   = "https://api.telegram.org/bot"
)

// Config holds all configuration for the Telegram connector.
type Config struct {
	BotToken        string   `json:"bot_token"`
	GroupID         int64    `json:"group_id"`
	EventsTopicID   int      `json:"events_topic_id"`
	CommandsTopicID int      `json:"commands_topic_id"`
	PollTimeout     int      `json:"poll_timeout"`
	EventTypes      []string `json:"event_types"`
}

// telegramResponse is the outer envelope of every Telegram Bot API response.
type telegramResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
}

// telegramRateLimitResponse is returned by the Telegram API on HTTP 429.
type telegramRateLimitResponse struct {
	OK         bool `json:"ok"`
	Parameters struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// CheckRateLimit inspects the HTTP response for a 429 status and, if found,
// waits for the retry_after duration (or 5 s by default) before returning an
// error. For all other non-2xx statuses it returns an error immediately.
// It returns nil for 2xx responses.
func CheckRateLimit(ctx context.Context, resp *http.Response, rawBody []byte) error {
	if resp.StatusCode == http.StatusTooManyRequests {
		var rl telegramRateLimitResponse
		retryAfter := 5 // default 5 seconds
		if json.Unmarshal(rawBody, &rl) == nil && rl.Parameters.RetryAfter > 0 {
			retryAfter = rl.Parameters.RetryAfter
		}
		select {
		case <-time.After(time.Duration(retryAfter) * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
		return fmt.Errorf("telegram rate limited, waited %ds", retryAfter)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram API error: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}
	return nil
}

// Update represents a single Telegram update.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

// Message is a Telegram message.
type Message struct {
	MessageID       int64  `json:"message_id"`
	MessageThreadID int    `json:"message_thread_id"`
	Text            string `json:"text"`
	Chat            struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}

// Telegram is a bridge connector that forwards events to a Telegram group topic
// and processes slash commands from another topic.
type Telegram struct {
	connectorID string
	config      Config
	apiClient   *client.Client
	stateStore  *state.Store
	logger      *slog.Logger
	httpClient  *http.Client
	healthy     atomic.Bool
}

// New creates a new Telegram connector. The connectorID is used to namespace
// state (cursors, processed events) and is returned by Name().
func New(connectorID string, cfg Config, apiClient *client.Client, stateStore *state.Store, logger *slog.Logger) *Telegram {
	if logger == nil {
		logger = slog.Default()
	}
	t := &Telegram{
		connectorID: connectorID,
		config:      cfg,
		apiClient:   apiClient,
		stateStore:  stateStore,
		logger:      logger,
		httpClient:  &http.Client{Timeout: 60 * time.Second},
	}
	return t
}

// Name implements connector.Connector.
func (t *Telegram) Name() string { return t.connectorID }

// Healthy implements connector.Connector.
func (t *Telegram) Healthy() bool { return t.healthy.Load() }

// Start implements connector.Connector. It runs the events poller and the
// Telegram commands poller concurrently until ctx is cancelled.
func (t *Telegram) Start(ctx context.Context) error {
	t.healthy.Store(true)
	defer t.healthy.Store(false)

	t.logger.Info("telegram connector starting",
		"group_id", t.config.GroupID,
		"events_topic", t.config.EventsTopicID,
		"commands_topic", t.config.CommandsTopicID,
	)

	errCh := make(chan error, 2)

	go func() {
		if err := t.runEventsPoller(ctx); err != nil {
			errCh <- fmt.Errorf("events poller: %w", err)
		}
	}()

	go func() {
		if err := t.runCommandsPoller(ctx); err != nil {
			errCh <- fmt.Errorf("commands poller: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		t.logger.Info("telegram connector stopping")
		return nil
	case err := <-errCh:
		return err
	}
}

// runEventsPoller polls the claude-plane event feed every 5 seconds and forwards
// matching events to the Telegram Events topic.
func (t *Telegram) runEventsPoller(ctx context.Context) error {
	ticker := time.NewTicker(eventPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := t.pollAndForwardEvents(ctx); err != nil {
				t.logger.Warn("event poll error", "error", err)
				// Non-fatal: keep running.
			}
		}
	}
}

// pollAndForwardEvents fetches new events from the server and sends relevant ones to Telegram.
func (t *Telegram) pollAndForwardEvents(ctx context.Context) error {
	cursor := t.stateStore.GetCursor(t.connectorID)

	events, nextCursor, err := t.apiClient.PollEvents(ctx, cursor)
	if err != nil {
		return fmt.Errorf("poll events: %w", err)
	}

	for _, e := range events {
		if t.stateStore.IsProcessed(e.EventID) {
			continue
		}
		if !ShouldForwardEvent(t.config.EventTypes, e.Type) {
			continue
		}

		msg := FormatEvent(e)
		if sendErr := t.sendMessage(ctx, msg, t.config.EventsTopicID); sendErr != nil {
			t.logger.Warn("failed to send event to telegram",
				"event_id", e.EventID,
				"event_type", e.Type,
				"error", sendErr,
			)
			// Continue processing remaining events; do not mark as processed so
			// we retry next cycle.
			continue
		}

		if markErr := t.stateStore.MarkProcessed(e.EventID); markErr != nil {
			t.logger.Warn("failed to mark event processed", "event_id", e.EventID, "error", markErr)
		}
	}

	if nextCursor != "" && nextCursor != cursor {
		if err := t.stateStore.SetCursor(t.connectorID, nextCursor); err != nil {
			t.logger.Warn("failed to persist cursor", "cursor", nextCursor, "error", err)
		}
	}

	return nil
}

// runCommandsPoller long-polls Telegram for new updates and dispatches commands.
func (t *Telegram) runCommandsPoller(ctx context.Context) error {
	var offset int64

	for {
		if ctx.Err() != nil {
			return nil
		}

		updates, err := t.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			t.logger.Warn("getUpdates error", "error", err)
			// Back off briefly before retrying.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, upd := range updates {
			offset = upd.UpdateID + 1

			if upd.Message == nil {
				continue
			}
			// Only process messages from the commands topic.
			if upd.Message.MessageThreadID != t.config.CommandsTopicID {
				continue
			}

			text := strings.TrimSpace(upd.Message.Text)
			if !strings.HasPrefix(text, "/") {
				continue
			}

			reply := t.handleCommand(ctx, text)
			if sendErr := t.sendMessage(ctx, reply, t.config.CommandsTopicID); sendErr != nil {
				t.logger.Warn("failed to send command reply", "error", sendErr)
			}
		}
	}
}

// handleCommand parses and dispatches a command, returning the reply text.
func (t *Telegram) handleCommand(ctx context.Context, text string) string {
	cmd, err := ParseCommand(text)
	if err != nil {
		return fmt.Sprintf("❌ %s", escapeMarkdownV2(err.Error()))
	}

	switch cmd.Name {
	case "help":
		return helpText()

	case "status":
		return "✅ Bridge is running\nConnector: telegram"

	case "list", "templates":
		return t.handleTemplates(ctx)

	case "sessions":
		return t.handleSessions(ctx)

	case "machines":
		return t.handleMachines(ctx)

	case "kill":
		return t.handleKill(ctx, cmd)

	case "inject":
		return t.handleInject(ctx, cmd)

	case "start":
		return t.handleStart(ctx, cmd)

	default:
		return fmt.Sprintf("❌ Unknown command: %s", escapeMarkdownV2(cmd.Name))
	}
}

func (t *Telegram) handleTemplates(ctx context.Context) string {
	templates, err := t.apiClient.ListTemplates(ctx)
	if err != nil {
		return fmt.Sprintf("❌ Failed to list templates: %s", escapeMarkdownV2(err.Error()))
	}
	if len(templates) == 0 {
		return "No templates configured\\."
	}
	var sb strings.Builder
	sb.WriteString("*Available templates:*\n")
	for _, tmpl := range templates {
		desc := tmpl.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		machine := tmpl.MachineID
		if machine == "" {
			machine = "any"
		}
		sb.WriteString(fmt.Sprintf("• `%s` — %s \\(%s\\)\n",
			escapeMarkdownV2(tmpl.Name),
			escapeMarkdownV2(desc),
			escapeMarkdownV2(machine)))
	}
	return sb.String()
}

func (t *Telegram) handleSessions(ctx context.Context) string {
	sessions, err := t.apiClient.ListSessions(ctx)
	if err != nil {
		return fmt.Sprintf("❌ Failed to list sessions: %s", escapeMarkdownV2(err.Error()))
	}
	if len(sessions) == 0 {
		return "No active sessions\\."
	}
	var sb strings.Builder
	sb.WriteString("*Active sessions:*\n")
	for _, s := range sessions {
		sb.WriteString(fmt.Sprintf("• `%s` — %s \\(%s\\)\n",
			s.SessionID,
			escapeMarkdownV2(s.MachineID),
			escapeMarkdownV2(s.Status)))
	}
	return sb.String()
}

func (t *Telegram) handleMachines(ctx context.Context) string {
	machines, err := t.apiClient.ListMachines(ctx)
	if err != nil {
		return fmt.Sprintf("❌ Failed to list machines: %s", escapeMarkdownV2(err.Error()))
	}
	if len(machines) == 0 {
		return "No machines connected\\."
	}
	var sb strings.Builder
	sb.WriteString("*Connected machines:*\n")
	for _, m := range machines {
		name := m.DisplayName
		if name == "" {
			name = m.MachineID
		}
		sb.WriteString(fmt.Sprintf("• `%s` — %s \\(%s\\)\n",
			m.MachineID,
			escapeMarkdownV2(name),
			escapeMarkdownV2(m.Status)))
	}
	return sb.String()
}

func (t *Telegram) handleKill(ctx context.Context, cmd *Command) string {
	if len(cmd.Args) < 1 {
		return "❌ Usage: /kill <session\\_id>"
	}
	sessionID := cmd.Args[0]
	if err := t.apiClient.KillSession(ctx, sessionID); err != nil {
		return fmt.Sprintf("❌ Failed to kill session %s: %s", escapeMarkdownV2(sessionID), escapeMarkdownV2(err.Error()))
	}
	return fmt.Sprintf("✅ Session `%s` killed\\.", sessionID)
}

func (t *Telegram) handleInject(ctx context.Context, cmd *Command) string {
	if len(cmd.Args) < 2 {
		return "❌ Usage: /inject <session\\_id> <text>"
	}
	sessionID := cmd.Args[0]
	text := strings.Join(cmd.Args[1:], " ")
	req := client.InjectRequest{
		Text:   text,
		Source: "telegram",
	}
	if err := t.apiClient.InjectSession(ctx, sessionID, req); err != nil {
		return fmt.Sprintf("❌ Failed to inject into session %s: %s", escapeMarkdownV2(sessionID), escapeMarkdownV2(err.Error()))
	}
	return fmt.Sprintf("✅ Text injected into session `%s`\\.", sessionID)
}

func (t *Telegram) handleStart(ctx context.Context, cmd *Command) string {
	if len(cmd.Args) < 1 {
		return "❌ Usage: /start <template\\_name> \\[machine\\_id\\] \\[\\| VAR\\=val …\\]"
	}
	templateName := cmd.Args[0]

	// Machine ID is optional — falls back to template default.
	var machineID string
	if len(cmd.Args) >= 2 {
		machineID = cmd.Args[1]
	}

	// Resolve template name to ID.
	matched, err := t.apiClient.GetTemplateByName(ctx, templateName)
	if err != nil {
		return fmt.Sprintf("❌ Template %q not found\\.", escapeMarkdownV2(templateName))
	}

	// Fall back to template's default machine.
	if machineID == "" {
		machineID = matched.MachineID
	}
	if machineID == "" {
		return "❌ Template has no default machine\\. Usage: /start <template> <machine>"
	}

	req := client.CreateSessionRequest{
		MachineID:  machineID,
		TemplateID: matched.TemplateID,
		Variables:  cmd.Vars,
	}
	session, err := t.apiClient.CreateSession(ctx, req)
	if err != nil {
		return fmt.Sprintf("❌ Failed to create session: %s", escapeMarkdownV2(err.Error()))
	}
	return fmt.Sprintf("✅ Session started\nID: `%s`\nMachine: `%s`", session.SessionID, session.MachineID)
}

// sendMessage sends a MarkdownV2-formatted message to the given Telegram topic.
func (t *Telegram) sendMessage(ctx context.Context, text string, topicID int) error {
	apiURL := fmt.Sprintf("%s%s/sendMessage", telegramAPIBase, t.config.BotToken)

	body := map[string]interface{}{
		"chat_id":                  t.config.GroupID,
		"message_thread_id":        topicID,
		"text":                     text,
		"parse_mode":               "MarkdownV2",
		"disable_web_page_preview": true,
	}

	resp, err := t.postJSON(ctx, apiURL, body)
	if err != nil {
		return fmt.Errorf("sendMessage HTTP: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read sendMessage body: %w", err)
	}

	if err := CheckRateLimit(ctx, resp, raw); err != nil {
		return fmt.Errorf("sendMessage: %w", err)
	}

	var tgResp telegramResponse
	if err := json.Unmarshal(raw, &tgResp); err != nil {
		return fmt.Errorf("decode sendMessage response: %w", err)
	}
	if !tgResp.OK {
		return fmt.Errorf("telegram sendMessage error: %s", string(raw))
	}

	return nil
}

// getUpdates long-polls Telegram for updates starting at the given offset.
func (t *Telegram) getUpdates(ctx context.Context, offset int64) ([]Update, error) {
	apiURL := fmt.Sprintf("%s%s/getUpdates", telegramAPIBase, t.config.BotToken)

	body := map[string]interface{}{
		"offset":          offset,
		"timeout":         t.config.PollTimeout,
		"allowed_updates": []string{"message"},
	}

	resp, err := t.postJSON(ctx, apiURL, body)
	if err != nil {
		return nil, fmt.Errorf("getUpdates HTTP: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read getUpdates body: %w", err)
	}

	if err := CheckRateLimit(ctx, resp, raw); err != nil {
		return nil, fmt.Errorf("getUpdates: %w", err)
	}

	var tgResp telegramResponse
	if err := json.Unmarshal(raw, &tgResp); err != nil {
		return nil, fmt.Errorf("decode getUpdates response: %w", err)
	}
	if !tgResp.OK {
		return nil, fmt.Errorf("telegram getUpdates error: %s", string(raw))
	}

	var updates []Update
	if err := json.Unmarshal(tgResp.Result, &updates); err != nil {
		return nil, fmt.Errorf("decode updates array: %w", err)
	}

	return updates, nil
}

// postJSON performs an HTTP POST with a JSON body and returns the raw response.
func (t *Telegram) postJSON(ctx context.Context, url string, body map[string]interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	return resp, nil
}

// helpText returns the static help message shown by /help.
func helpText() string {
	return `*claude\-plane bot commands*

/start <template> \[machine\] \[| VAR\=val …\] — Start a session from a template
/list — List available templates
/templates — List available templates
/sessions — List active sessions
/machines — List connected machines
/status — Bridge status
/kill <session\_id> — Kill a session
/inject <session\_id> <text> — Inject text into a session
/help — Show this message`
}
