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
		return fmt.Sprintf("❌ %s", err.Error())
	}

	switch cmd.Name {
	case "help":
		return helpText()

	case "status":
		return "✅ Bridge is running\nConnector: telegram"

	case "list":
		return t.handleList(ctx)

	case "machines":
		return t.handleMachines(ctx)

	case "kill":
		return t.handleKill(ctx, cmd)

	case "inject":
		return t.handleInject(ctx, cmd)

	case "start":
		return t.handleStart(ctx, cmd)

	default:
		return fmt.Sprintf("❌ Unknown command: %s", cmd.Name)
	}
}

func (t *Telegram) handleList(ctx context.Context) string {
	sessions, err := t.apiClient.ListSessions(ctx)
	if err != nil {
		return fmt.Sprintf("❌ Failed to list sessions: %s", err.Error())
	}
	if len(sessions) == 0 {
		return "No active sessions."
	}
	var sb strings.Builder
	sb.WriteString("*Active sessions:*\n")
	for _, s := range sessions {
		sb.WriteString(fmt.Sprintf("• `%s` — %s (%s)\n", s.SessionID, s.MachineID, s.Status))
	}
	return sb.String()
}

func (t *Telegram) handleMachines(ctx context.Context) string {
	machines, err := t.apiClient.ListMachines(ctx)
	if err != nil {
		return fmt.Sprintf("❌ Failed to list machines: %s", err.Error())
	}
	if len(machines) == 0 {
		return "No machines connected."
	}
	var sb strings.Builder
	sb.WriteString("*Connected machines:*\n")
	for _, m := range machines {
		name := m.DisplayName
		if name == "" {
			name = m.MachineID
		}
		sb.WriteString(fmt.Sprintf("• `%s` — %s (%s)\n", m.MachineID, name, m.Status))
	}
	return sb.String()
}

func (t *Telegram) handleKill(ctx context.Context, cmd *Command) string {
	if len(cmd.Args) < 1 {
		return "❌ Usage: /kill <session_id>"
	}
	sessionID := cmd.Args[0]
	if err := t.apiClient.KillSession(ctx, sessionID); err != nil {
		return fmt.Sprintf("❌ Failed to kill session %s: %s", sessionID, err.Error())
	}
	return fmt.Sprintf("✅ Session `%s` killed.", sessionID)
}

func (t *Telegram) handleInject(ctx context.Context, cmd *Command) string {
	if len(cmd.Args) < 2 {
		return "❌ Usage: /inject <session_id> <text>"
	}
	sessionID := cmd.Args[0]
	text := strings.Join(cmd.Args[1:], " ")
	req := client.InjectRequest{
		Text:   text,
		Source: "telegram",
	}
	if err := t.apiClient.InjectSession(ctx, sessionID, req); err != nil {
		return fmt.Sprintf("❌ Failed to inject into session %s: %s", sessionID, err.Error())
	}
	return fmt.Sprintf("✅ Text injected into session `%s`.", sessionID)
}

func (t *Telegram) handleStart(ctx context.Context, cmd *Command) string {
	if len(cmd.Args) < 2 {
		return "❌ Usage: /start <template_name> <machine_id> [| VAR=val …]"
	}
	templateName := cmd.Args[0]
	machineID := cmd.Args[1]

	// Resolve template name to ID.
	templates, err := t.apiClient.ListTemplates(ctx)
	if err != nil {
		return fmt.Sprintf("❌ Failed to list templates: %s", err.Error())
	}
	templateID := ""
	for _, tmpl := range templates {
		if tmpl.Name == templateName {
			templateID = tmpl.TemplateID
			break
		}
	}
	if templateID == "" {
		return fmt.Sprintf("❌ Template %q not found.", templateName)
	}

	req := client.CreateSessionRequest{
		MachineID:  machineID,
		TemplateID: templateID,
		Vars:       cmd.Vars,
	}
	session, err := t.apiClient.CreateSession(ctx, req)
	if err != nil {
		return fmt.Sprintf("❌ Failed to create session: %s", err.Error())
	}
	return fmt.Sprintf("✅ Session started\nID: `%s`\nMachine: `%s`", session.SessionID, session.MachineID)
}

// sendMessage sends a Markdown-formatted message to the given Telegram topic.
func (t *Telegram) sendMessage(ctx context.Context, text string, topicID int) error {
	apiURL := fmt.Sprintf("%s%s/sendMessage", telegramAPIBase, t.config.BotToken)

	body := map[string]interface{}{
		"chat_id":                  t.config.GroupID,
		"message_thread_id":        topicID,
		"text":                     text,
		"parse_mode":               "Markdown",
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
	return `*claude-plane bot commands*

/start <template> <machine> [| VAR=val …] — Start a session from a template
/list — List active sessions
/machines — List connected machines
/status — Bridge status
/kill <session_id> — Kill a session
/inject <session_id> <text> — Inject text into a session
/help — Show this message`
}
