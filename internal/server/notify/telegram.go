package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// TelegramConfig holds the configuration for a Telegram notification channel.
type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
	TopicID  int    `json:"topic_id,omitempty"`
}

// TelegramNotifier sends notifications via the Telegram Bot API.
type TelegramNotifier struct {
	client  *http.Client
	baseURL string // empty uses default Telegram API
}

// NewTelegramNotifier creates a new TelegramNotifier. If client is nil,
// http.DefaultClient is used.
func NewTelegramNotifier(client *http.Client) *TelegramNotifier {
	if client == nil {
		client = http.DefaultClient
	}
	return &TelegramNotifier{client: client}
}

// newTelegramNotifierWithBaseURL creates a TelegramNotifier with a custom
// base URL (useful for testing).
func newTelegramNotifierWithBaseURL(client *http.Client, baseURL string) *TelegramNotifier {
	n := NewTelegramNotifier(client)
	n.baseURL = baseURL
	return n
}

// Type returns "telegram".
func (*TelegramNotifier) Type() string { return "telegram" }

// Send sends a message via the Telegram Bot API.
func (n *TelegramNotifier) Send(ctx context.Context, channelConfig string, subject, body string) error {
	var cfg TelegramConfig
	if err := json.Unmarshal([]byte(channelConfig), &cfg); err != nil {
		return fmt.Errorf("parse telegram config: %w", err)
	}

	if cfg.BotToken == "" {
		return fmt.Errorf("telegram bot_token is required")
	}
	if cfg.ChatID == "" {
		return fmt.Errorf("telegram chat_id is required")
	}

	text := fmt.Sprintf("<b>%s</b>\n%s", subject, body)

	payload := map[string]any{
		"chat_id":    cfg.ChatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if cfg.TopicID > 0 {
		payload["message_thread_id"] = cfg.TopicID
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	base := n.baseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	url := fmt.Sprintf("%s/bot%s/sendMessage", base, cfg.BotToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		// Sanitize error to avoid leaking the bot token from the URL
		// embedded in net/http error messages.
		return fmt.Errorf("telegram send failed: connection error")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram API returned %d", resp.StatusCode)
	}
	return nil
}
