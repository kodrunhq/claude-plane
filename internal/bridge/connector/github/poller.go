// Package github provides a GitHub repository poller that checks configured
// trigger events (PR opened, check run completed, issue labeled) and returns
// matched events ready for session creation.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kodrunhq/claude-plane/internal/bridge/state"
)

const (
	defaultAPIBase         = "https://api.github.com"
	rateLimitWarningThresh = 100
	prPerPage              = 30
	checkRunPerPage        = 30
	issuePerPage           = 30
)

// PRTrigger configures filtering for pull_request.opened events.
type PRTrigger struct {
	Filters Filters `json:"filters"`
}

// CheckTrigger configures filtering for check_run.completed events.
type CheckTrigger struct {
	Filters Filters `json:"filters"`
}

// IssueTrigger configures filtering for issues.labeled events.
type IssueTrigger struct {
	Filters Filters `json:"filters"`
}

// TriggerConfig holds which GitHub event types to poll and their filters.
type TriggerConfig struct {
	PullRequestOpened *PRTrigger    `json:"pull_request_opened"`
	CheckRunCompleted *CheckTrigger `json:"check_run_completed"`
	IssueLabeled      *IssueTrigger `json:"issue_labeled"`
}

// MatchedEvent is a successfully matched event ready for session creation.
type MatchedEvent struct {
	Template  string            // Template name for session creation
	Variables map[string]string // Extracted template variables
	EventKey  string            // Unique key for deduplication (e.g., "pr:owner/repo:123")
}

// RepoPoller polls a single GitHub repository for configured trigger events.
type RepoPoller struct {
	repo       string        // "owner/repo"
	token      string
	template   string // template name for session creation
	triggers   TriggerConfig
	state      *state.Store
	connID     string // connector ID for state namespacing
	httpClient *http.Client
	logger     *slog.Logger
	apiBase    string // GitHub API base URL (for testing, default "https://api.github.com")
}

// NewRepoPoller creates a new RepoPoller. The apiBase defaults to
// "https://api.github.com" and can be overridden via SetAPIBase for tests.
func NewRepoPoller(
	repo, token, template, connID string,
	triggers TriggerConfig,
	stateStore *state.Store,
	logger *slog.Logger,
) *RepoPoller {
	return &RepoPoller{
		repo:       repo,
		token:      token,
		template:   template,
		connID:     connID,
		triggers:   triggers,
		state:      stateStore,
		httpClient: &http.Client{},
		logger:     logger,
		apiBase:    defaultAPIBase,
	}
}

// SetAPIBase overrides the GitHub API base URL. Intended for test injection.
func (p *RepoPoller) SetAPIBase(base string) {
	p.apiBase = base
}

// Poll checks all enabled triggers for the repo. Returns matched events.
func (p *RepoPoller) Poll(ctx context.Context) ([]MatchedEvent, error) {
	var results []MatchedEvent

	if p.triggers.PullRequestOpened != nil {
		prs, rateLow, err := p.fetchOpenPRs(ctx)
		if err != nil {
			return results, fmt.Errorf("fetch PRs for %s: %w", p.repo, err)
		}

		prResults, err := p.processPRs(ctx, prs, p.triggers.PullRequestOpened)
		if err != nil {
			return results, fmt.Errorf("process PRs for %s: %w", p.repo, err)
		}
		results = append(results, prResults...)

		if rateLow {
			p.logger.Warn("GitHub rate limit nearly exhausted, backing off",
				slog.String("repo", p.repo),
				slog.String("trigger", "pull_request_opened"),
			)
			return results, nil
		}

		// Process check runs using the same open PRs
		if p.triggers.CheckRunCompleted != nil {
			crResults, rateLowCR, err := p.processCheckRuns(ctx, prs, p.triggers.CheckRunCompleted)
			if err != nil {
				return results, fmt.Errorf("process check runs for %s: %w", p.repo, err)
			}
			results = append(results, crResults...)
			if rateLowCR {
				p.logger.Warn("GitHub rate limit nearly exhausted, backing off",
					slog.String("repo", p.repo),
					slog.String("trigger", "check_run_completed"),
				)
				return results, nil
			}
		}
	} else if p.triggers.CheckRunCompleted != nil {
		// No PR trigger but check runs requested — fetch PRs just for check runs
		prs, rateLow, err := p.fetchOpenPRs(ctx)
		if err != nil {
			return results, fmt.Errorf("fetch PRs for check runs %s: %w", p.repo, err)
		}

		crResults, rateLowCR, err := p.processCheckRuns(ctx, prs, p.triggers.CheckRunCompleted)
		if err != nil {
			return results, fmt.Errorf("process check runs for %s: %w", p.repo, err)
		}
		results = append(results, crResults...)

		if rateLow || rateLowCR {
			p.logger.Warn("GitHub rate limit nearly exhausted, backing off",
				slog.String("repo", p.repo),
				slog.String("trigger", "check_run_completed"),
			)
			return results, nil
		}
	}

	if p.triggers.IssueLabeled != nil {
		issueResults, rateLow, err := p.processIssues(ctx, p.triggers.IssueLabeled)
		if err != nil {
			return results, fmt.Errorf("process issues for %s: %w", p.repo, err)
		}
		results = append(results, issueResults...)
		if rateLow {
			p.logger.Warn("GitHub rate limit nearly exhausted, backing off",
				slog.String("repo", p.repo),
				slog.String("trigger", "issue_labeled"),
			)
		}
	}

	return results, nil
}

// fetchOpenPRs fetches open PRs from the GitHub API.
// Returns the list of PRs, a boolean indicating whether the rate limit is low,
// and any error.
func (p *RepoPoller) fetchOpenPRs(ctx context.Context) ([]PRData, bool, error) {
	url := fmt.Sprintf(
		"%s/repos/%s/pulls?state=open&sort=updated&direction=desc&per_page=%d",
		p.apiBase, p.repo, prPerPage,
	)
	req, err := p.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, false, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	rateLow := p.checkRateLimit(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, rateLow, fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}

	var prs []PRData
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return nil, rateLow, fmt.Errorf("decode PRs: %w", err)
	}
	return prs, rateLow, nil
}

// processPRs evaluates each PR against the trigger filters and returns matched events.
// It manages the cursor high-water mark and deduplication via the state store.
func (p *RepoPoller) processPRs(ctx context.Context, prs []PRData, trigger *PRTrigger) ([]MatchedEvent, error) {
	cursorKey := fmt.Sprintf("%s:pr:%s", p.connID, p.repo)
	cursor := p.state.GetCursor(cursorKey)

	var results []MatchedEvent
	var latestUpdatedAt string

	for _, pr := range prs {
		// Track high-water mark by comparing all seen timestamps
		if isAfter(pr.UpdatedAt, latestUpdatedAt) {
			latestUpdatedAt = pr.UpdatedAt
		}

		// Skip PRs not updated after the cursor
		if cursor != "" && !isAfter(pr.UpdatedAt, cursor) {
			continue
		}

		eventKey := fmt.Sprintf("pr:%s:%d", p.repo, pr.Number)
		if p.state.IsProcessed(eventKey) {
			continue
		}

		// Build event data for filter evaluation
		labelNames := make([]string, 0, len(pr.Labels))
		for _, l := range pr.Labels {
			labelNames = append(labelNames, l.Name)
		}
		eventData := EventData{
			BaseBranch: pr.Base.Ref,
			Labels:     labelNames,
			Author:     pr.User.Login,
		}

		// Fetch changed files if paths filter is set
		if len(trigger.Filters.Paths) > 0 {
			files, err := p.fetchPRFiles(ctx, pr.Number)
			if err != nil {
				p.logger.Warn("failed to fetch PR files",
					slog.String("repo", p.repo),
					slog.Int("pr", pr.Number),
					slog.String("error", err.Error()),
				)
			}
			eventData.ChangedFiles = files
		}

		if !trigger.Filters.Match(eventData) {
			continue
		}

		variables := ExtractPRVariables(pr, p.repo)
		results = append(results, MatchedEvent{
			Template:  p.template,
			Variables: variables,
			EventKey:  eventKey,
		})

		if err := p.state.MarkProcessed(eventKey); err != nil {
			p.logger.Warn("failed to mark PR processed",
				slog.String("event_key", eventKey),
				slog.String("error", err.Error()),
			)
		}
	}

	// Advance cursor to the most recent updated_at seen
	if latestUpdatedAt != "" && isAfter(latestUpdatedAt, cursor) {
		if err := p.state.SetCursor(cursorKey, latestUpdatedAt); err != nil {
			p.logger.Warn("failed to set PR cursor",
				slog.String("cursor_key", cursorKey),
				slog.String("error", err.Error()),
			)
		}
	}

	return results, nil
}

// fetchPRFiles fetches the list of changed filenames for a single PR.
func (p *RepoPoller) fetchPRFiles(ctx context.Context, prNumber int) ([]string, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d/files", p.apiBase, p.repo, prNumber)
	req, err := p.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}

	var files []struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("decode PR files: %w", err)
	}

	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Filename)
	}
	return names, nil
}

// processCheckRuns fetches check runs for each open PR's head SHA and evaluates
// them against the trigger filters. Returns matched events, a rate-limit-low
// flag, and any error.
func (p *RepoPoller) processCheckRuns(ctx context.Context, prs []PRData, trigger *CheckTrigger) ([]MatchedEvent, bool, error) {
	var results []MatchedEvent

	for _, pr := range prs {
		if pr.Head.SHA == "" {
			continue
		}

		runs, rateLow, err := p.fetchCheckRuns(ctx, pr.Head.SHA)
		if err != nil {
			p.logger.Warn("failed to fetch check runs",
				slog.String("repo", p.repo),
				slog.String("sha", pr.Head.SHA),
				slog.String("error", err.Error()),
			)
			continue
		}

		for _, cr := range runs {
			eventKey := fmt.Sprintf("check:%s:%d", p.repo, cr.ID)
			if p.state.IsProcessed(eventKey) {
				continue
			}

			eventData := EventData{
				CheckName:  cr.Name,
				Conclusion: cr.Conclusion,
			}
			if !trigger.Filters.Match(eventData) {
				continue
			}

			variables := ExtractCheckRunVariables(cr, p.repo)
			results = append(results, MatchedEvent{
				Template:  p.template,
				Variables: variables,
				EventKey:  eventKey,
			})

			if err := p.state.MarkProcessed(eventKey); err != nil {
				p.logger.Warn("failed to mark check run processed",
					slog.String("event_key", eventKey),
					slog.String("error", err.Error()),
				)
			}
		}

		if rateLow {
			return results, true, nil
		}
	}

	return results, false, nil
}

// fetchCheckRuns fetches completed check runs for a given commit SHA.
func (p *RepoPoller) fetchCheckRuns(ctx context.Context, sha string) ([]CheckRunData, bool, error) {
	url := fmt.Sprintf(
		"%s/repos/%s/commits/%s/check-runs?per_page=%d",
		p.apiBase, p.repo, sha, checkRunPerPage,
	)
	req, err := p.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, false, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	rateLow := p.checkRateLimit(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, rateLow, fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}

	var payload struct {
		CheckRuns []CheckRunData `json:"check_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, rateLow, fmt.Errorf("decode check runs: %w", err)
	}
	return payload.CheckRuns, rateLow, nil
}

// processIssues fetches open issues and evaluates them against the trigger filters.
// Returns matched events, a rate-limit-low flag, and any error.
func (p *RepoPoller) processIssues(ctx context.Context, trigger *IssueTrigger) ([]MatchedEvent, bool, error) {
	endpoint := fmt.Sprintf(
		"%s/repos/%s/issues?state=open&sort=updated&direction=desc&per_page=%d",
		p.apiBase, p.repo, issuePerPage,
	)

	// Append label filter if specified (URL-encode for labels with special chars)
	if len(trigger.Filters.Labels) > 0 {
		encoded := make([]string, len(trigger.Filters.Labels))
		for i, l := range trigger.Filters.Labels {
			encoded[i] = url.QueryEscape(l)
		}
		endpoint += "&labels=" + strings.Join(encoded, ",")
	}

	req, err := p.newRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, false, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	rateLow := p.checkRateLimit(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, rateLow, fmt.Errorf("GET %s returned %d", endpoint, resp.StatusCode)
	}

	var issues []IssueData
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, rateLow, fmt.Errorf("decode issues: %w", err)
	}

	var results []MatchedEvent
	for _, issue := range issues {
		// Skip pull requests returned by the issues endpoint
		if issue.PullRequest != nil {
			continue
		}

		eventKey := fmt.Sprintf("issue:%s:%d", p.repo, issue.Number)
		if p.state.IsProcessed(eventKey) {
			continue
		}

		labelNames := make([]string, 0, len(issue.Labels))
		for _, l := range issue.Labels {
			labelNames = append(labelNames, l.Name)
		}
		eventData := EventData{
			Labels: labelNames,
			Author: issue.User.Login,
		}
		if !trigger.Filters.Match(eventData) {
			continue
		}

		variables := ExtractIssueVariables(issue, p.repo)
		results = append(results, MatchedEvent{
			Template:  p.template,
			Variables: variables,
			EventKey:  eventKey,
		})

		if err := p.state.MarkProcessed(eventKey); err != nil {
			p.logger.Warn("failed to mark issue processed",
				slog.String("event_key", eventKey),
				slog.String("error", err.Error()),
			)
		}
	}

	return results, rateLow, nil
}

// newRequest creates an HTTP request with GitHub auth headers set.
func (p *RepoPoller) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s %s: %w", method, url, err)
	}
	req.Header.Set("Authorization", "token "+p.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	return req, nil
}

// checkRateLimit inspects X-RateLimit-Remaining and returns true when
// the remaining allowance is below the warning threshold.
func (p *RepoPoller) checkRateLimit(resp *http.Response) bool {
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	if remaining == "" {
		return false
	}
	n, err := strconv.Atoi(remaining)
	if err != nil {
		return false
	}
	return n < rateLimitWarningThresh
}

// isAfter returns true when candidate is lexicographically greater than base.
// GitHub timestamps use RFC3339 (ISO8601) which compares correctly as strings.
func isAfter(candidate, base string) bool {
	return candidate > base
}
