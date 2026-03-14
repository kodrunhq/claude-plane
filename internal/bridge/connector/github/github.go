package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
	"github.com/kodrunhq/claude-plane/internal/bridge/state"
)

const (
	defaultPollInterval  = 60 * time.Second
	pruneEveryNCycles    = 100
)

// WatchConfig represents a single repository watch.
type WatchConfig struct {
	Repo         string        `json:"repo"`          // "owner/repo"
	Template     string        `json:"template"`      // template name for session creation
	MachineID    string        `json:"machine_id"`    // optional: specific machine to create session on
	PollInterval string        `json:"poll_interval"` // e.g., "60s"
	Triggers     TriggerConfig `json:"triggers"`
}

// Config holds all configuration for the GitHub connector.
type Config struct {
	Token   string        `json:"token"`
	Watches []WatchConfig `json:"watches"`
}

// GitHub is a bridge connector that polls GitHub repositories and creates sessions
// based on configurable trigger rules.
type GitHub struct {
	connectorID string
	config      Config
	apiClient   *client.Client
	stateStore  *state.Store
	logger      *slog.Logger
	healthy     atomic.Bool
	apiBase     string       // GitHub API base URL (for testing)
	httpClient  *http.Client // shared HTTP client for GitHub API calls
}

// New creates a new GitHub connector. The connectorID is used to namespace
// state (cursors, processed events) and is returned by Name().
func New(connectorID string, cfg Config, apiClient *client.Client, stateStore *state.Store, logger *slog.Logger) *GitHub {
	if logger == nil {
		logger = slog.Default()
	}
	return &GitHub{
		connectorID: connectorID,
		config:      cfg,
		apiClient:   apiClient,
		stateStore:  stateStore,
		logger:      logger,
		apiBase:     defaultAPIBase,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// SetAPIBase overrides the GitHub API base URL. Intended for test injection.
func (g *GitHub) SetAPIBase(base string) {
	g.apiBase = base
}

// Name implements connector.Connector.
func (g *GitHub) Name() string { return g.connectorID }

// Healthy implements connector.Connector.
func (g *GitHub) Healthy() bool { return g.healthy.Load() }

// Start implements connector.Connector. It validates the GitHub token, then
// starts one goroutine per watch configuration. It blocks until ctx is
// cancelled or a fatal error occurs.
func (g *GitHub) Start(ctx context.Context) error {
	username, err := g.validateToken(ctx)
	if err != nil {
		return fmt.Errorf("github connector %s: token validation failed: %w", g.connectorID, err)
	}

	g.logger.Info("github connector authenticated",
		slog.String("connector", g.connectorID),
		slog.String("github_user", username),
		slog.Int("watches", len(g.config.Watches)),
	)

	g.healthy.Store(true)
	defer g.healthy.Store(false)

	var wg sync.WaitGroup
	for _, watch := range g.config.Watches {
		wg.Add(1)
		go func(w WatchConfig) {
			defer wg.Done()
			g.runWatch(ctx, w)
		}(watch)
	}

	wg.Wait()
	g.logger.Info("github connector stopped", slog.String("connector", g.connectorID))
	return nil
}

// runWatch polls a single repository watch until ctx is cancelled.
func (g *GitHub) runWatch(ctx context.Context, watch WatchConfig) {
	interval := parsePollInterval(watch.PollInterval)

	poller := NewRepoPoller(
		watch.Repo,
		g.config.Token,
		watch.Template,
		g.connectorID,
		watch.Triggers,
		g.stateStore,
		g.logger,
	)
	poller.SetAPIBase(g.apiBase)

	g.logger.Info("starting github watch",
		slog.String("connector", g.connectorID),
		slog.String("repo", watch.Repo),
		slog.Duration("interval", interval),
	)

	var cycle int64
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Poll immediately on first cycle before waiting for the ticker.
	g.pollOnce(ctx, poller, watch)
	cycle++

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.pollOnce(ctx, poller, watch)
			cycle++

			if cycle%pruneEveryNCycles == 0 {
				if err := g.stateStore.Prune(0); err != nil {
					g.logger.Warn("github connector: failed to prune state",
						slog.String("connector", g.connectorID),
						slog.String("repo", watch.Repo),
						slog.String("error", err.Error()),
					)
				}
			}
		}
	}
}

// pollOnce runs a single poll cycle for the poller and creates sessions for matched events.
func (g *GitHub) pollOnce(ctx context.Context, poller *RepoPoller, watch WatchConfig) {
	events, err := poller.Poll(ctx)
	if err != nil {
		g.logger.Warn("github connector: poll error",
			slog.String("connector", g.connectorID),
			slog.String("repo", watch.Repo),
			slog.String("error", err.Error()),
		)
		return
	}

	for _, event := range events {
		if ctx.Err() != nil {
			return
		}
		g.createSession(ctx, watch, event)
	}
}

// createSession calls the claude-plane API to create a session for the matched event.
func (g *GitHub) createSession(ctx context.Context, watch WatchConfig, event MatchedEvent) {
	req := client.CreateSessionRequest{
		TemplateName: event.Template,
		MachineID:    watch.MachineID,
		Variables:    event.Variables,
	}

	_, err := g.apiClient.CreateSession(ctx, req)
	if err != nil {
		g.logger.Warn("github connector: failed to create session",
			slog.String("connector", g.connectorID),
			slog.String("repo", watch.Repo),
			slog.String("event_key", event.EventKey),
			slog.String("template", event.Template),
			slog.String("error", err.Error()),
		)
		return
	}

	g.logger.Info("github connector: session created",
		slog.String("connector", g.connectorID),
		slog.String("repo", watch.Repo),
		slog.String("event_key", event.EventKey),
		slog.String("template", event.Template),
	)
}

// validateToken calls GET /user on the GitHub API to verify the token.
// Returns the authenticated username or an error.
func (g *GitHub) validateToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBase+"/user", nil)
	if err != nil {
		return "", fmt.Errorf("build /user request: %w", err)
	}
	req.Header.Set("Authorization", "token "+g.config.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET /user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("invalid GitHub token (HTTP 401)")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET /user returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode /user response: %w", err)
	}

	return payload.Login, nil
}

// parsePollInterval parses a duration string. Returns defaultPollInterval on
// empty input or parse failure.
func parsePollInterval(s string) time.Duration {
	if s == "" {
		return defaultPollInterval
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return defaultPollInterval
	}
	return d
}
