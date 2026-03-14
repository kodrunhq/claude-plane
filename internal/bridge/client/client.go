// Package client provides a REST API client for the claude-plane server.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is a typed HTTP client for the claude-plane REST API.
// It authenticates using an API key sent as a Bearer token.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a new Client targeting the given base URL with the provided API key.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// --- Request / Response Types ---

// CreateSessionRequest is the payload for creating a new session.
type CreateSessionRequest struct {
	MachineID     string            `json:"machine_id"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	InitialPrompt string            `json:"initial_prompt,omitempty"`
	TemplateID    string            `json:"template_id,omitempty"`
	TemplateName  string            `json:"template_name,omitempty"`
	Vars          map[string]string `json:"vars,omitempty"`
}

// InjectRequest is the payload for injecting text into a running session.
type InjectRequest struct {
	Text    string `json:"text"`
	Source  string `json:"source"`
	DelayMS int    `json:"delay_ms,omitempty"`
}

// Session represents a claude-plane session resource.
type Session struct {
	SessionID string `json:"session_id"`
	MachineID string `json:"machine_id"`
	Status    string `json:"status"`
	Command   string `json:"command"`
}

// Template represents a session template resource.
type Template struct {
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
}

// Machine represents a connected agent machine.
type Machine struct {
	MachineID   string `json:"machine_id"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

// Event represents a single event emitted by the server.
type Event struct {
	EventID   string                 `json:"event_id"`
	Type      string                 `json:"event_type"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`
	Payload   map[string]interface{} `json:"payload"`
}

// ConnectorConfig represents a bridge connector configuration record.
// The server decrypts ConfigSecret before returning it to API-key-authenticated callers.
type ConnectorConfig struct {
	ConnectorID   string `json:"connector_id"`
	ConnectorType string `json:"connector_type"`
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	Config        string `json:"config"`
	ConfigSecret  string `json:"config_secret,omitempty"`
}

// EventFeedResponse is the envelope returned by the event feed endpoint.
type EventFeedResponse struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor"`
}

// BridgeStatusResponse is the envelope returned by the bridge status endpoint.
type BridgeStatusResponse struct {
	RestartRequestedAt *string `json:"restart_requested_at"`
}

// --- Session Methods ---

// CreateSession creates a new session on the server.
func (c *Client) CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error) {
	var session Session
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/sessions", req, &session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &session, nil
}

// ListSessions returns all sessions known to the server.
func (c *Client) ListSessions(ctx context.Context) ([]Session, error) {
	var sessions []Session
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/sessions", nil, &sessions); err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return sessions, nil
}

// GetSession returns a single session by ID.
func (c *Client) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	var session Session
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/sessions/"+sessionID, nil, &session); err != nil {
		return nil, fmt.Errorf("get session %s: %w", sessionID, err)
	}
	return &session, nil
}

// KillSession terminates a running session.
func (c *Client) KillSession(ctx context.Context, sessionID string) error {
	if err := c.doJSON(ctx, http.MethodDelete, "/api/v1/sessions/"+sessionID, nil, nil); err != nil {
		return fmt.Errorf("kill session %s: %w", sessionID, err)
	}
	return nil
}

// InjectSession sends text into a running session.
func (c *Client) InjectSession(ctx context.Context, sessionID string, req InjectRequest) error {
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/sessions/"+sessionID+"/inject", req, nil); err != nil {
		return fmt.Errorf("inject session %s: %w", sessionID, err)
	}
	return nil
}

// --- Template Methods ---

// ListTemplates returns all session templates.
func (c *Client) ListTemplates(ctx context.Context) ([]Template, error) {
	var templates []Template
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/templates", nil, &templates); err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	return templates, nil
}

// --- Machine Methods ---

// ListMachines returns all registered agent machines.
func (c *Client) ListMachines(ctx context.Context) ([]Machine, error) {
	var machines []Machine
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/machines", nil, &machines); err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	return machines, nil
}

// --- Event Methods ---

// PollEvents fetches events from the server feed after the given cursor.
// Returns (events, nextCursor, error). Pass an empty cursor to start from the beginning.
func (c *Client) PollEvents(ctx context.Context, afterCursor string) ([]Event, string, error) {
	endpoint := "/api/v1/events/feed"
	if afterCursor != "" {
		endpoint += "?after=" + url.QueryEscape(afterCursor)
	}

	var feed EventFeedResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &feed); err != nil {
		return nil, "", fmt.Errorf("poll events: %w", err)
	}

	return feed.Events, feed.NextCursor, nil
}

// --- Bridge Config Methods ---

// GetConnectorConfigs returns the connector configurations from the server.
// The server decrypts secrets before returning them to API-key-authenticated callers.
func (c *Client) GetConnectorConfigs(ctx context.Context) ([]ConnectorConfig, error) {
	var configs []ConnectorConfig
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/bridge/connectors", nil, &configs); err != nil {
		return nil, fmt.Errorf("get connector configs: %w", err)
	}
	return configs, nil
}

// CheckRestartSignal returns true if the server has requested a bridge restart
// after the given bootTime. Returns false when no restart has been requested.
func (c *Client) CheckRestartSignal(ctx context.Context, bootTime time.Time) (bool, error) {
	var status BridgeStatusResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/bridge/status", nil, &status); err != nil {
		return false, fmt.Errorf("check restart signal: %w", err)
	}

	if status.RestartRequestedAt == nil {
		return false, nil
	}

	requestedAt, err := time.Parse(time.RFC3339, *status.RestartRequestedAt)
	if err != nil {
		return false, fmt.Errorf("parse restart_requested_at %q: %w", *status.RestartRequestedAt, err)
	}

	return requestedAt.After(bootTime), nil
}

// --- HTTP Helpers ---

// APIError represents a non-2xx response from the server.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Body)
}

// doJSON performs an HTTP request, optionally serialising reqBody as JSON,
// and deserialises a successful response into respDest (may be nil).
func (c *Client) doJSON(ctx context.Context, method, path string, reqBody, respDest interface{}) error {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(rawBody),
		}
	}

	if respDest != nil && len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, respDest); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}
