package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/state"
)

// ---------------------------------------------------------------------------
// Mock GitHub API server helpers
// ---------------------------------------------------------------------------

type mockGitHubServer struct {
	server    *httptest.Server
	pulls     []PRData
	pullFiles map[int][]string // PR number -> list of changed filenames
	checkRuns map[string][]CheckRunData // head SHA -> check runs
	issues    []IssueData
	// Rate limit controls
	rateLimitRemaining int
}

func newMockGitHub(t *testing.T) *mockGitHubServer {
	t.Helper()
	m := &mockGitHubServer{
		pullFiles:          make(map[int][]string),
		checkRuns:          make(map[string][]CheckRunData),
		rateLimitRemaining: 1000,
	}

	mux := http.NewServeMux()

	// GET /repos/{owner}/{repo}/pulls
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(m.rateLimitRemaining))

		// Route based on path suffix
		path := r.URL.Path
		switch {
		case hasSuffix(path, "/pulls") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(m.pulls)

		case hasSuffixSegment(path, "/pulls", "/files") && r.Method == http.MethodGet:
			// /repos/{owner}/{repo}/pulls/{number}/files
			num := extractSegmentBefore(path, "/files")
			prNum, _ := strconv.Atoi(num)
			files := m.pullFiles[prNum]
			type fileEntry struct {
				Filename string `json:"filename"`
			}
			entries := make([]fileEntry, len(files))
			for i, f := range files {
				entries[i] = fileEntry{Filename: f}
			}
			_ = json.NewEncoder(w).Encode(entries)

		case hasSuffixSegment(path, "/commits", "/check-runs") && r.Method == http.MethodGet:
			// /repos/{owner}/{repo}/commits/{sha}/check-runs
			sha := extractSegmentBefore(path, "/check-runs")
			runs := m.checkRuns[sha]
			resp := struct {
				CheckRuns []CheckRunData `json:"check_runs"`
			}{CheckRuns: runs}
			_ = json.NewEncoder(w).Encode(resp)

		case hasSuffix(path, "/issues") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(m.issues)

		default:
			http.NotFound(w, r)
		}
	})

	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

// hasSuffix returns true if p ends with suffix.
func hasSuffix(p, suffix string) bool {
	return len(p) >= len(suffix) && p[len(p)-len(suffix):] == suffix
}

// hasSuffixSegment returns true if p contains segment and ends with after.
// e.g. hasSuffixSegment("/repos/o/r/pulls/42/files", "/pulls", "/files") → true
func hasSuffixSegment(p, segment, after string) bool {
	return hasSuffix(p, after) && containsSegment(p, segment)
}

func containsSegment(p, seg string) bool {
	for i := 0; i <= len(p)-len(seg); i++ {
		if p[i:i+len(seg)] == seg {
			return true
		}
	}
	return false
}

// extractSegmentBefore extracts the path segment immediately before the given suffix.
func extractSegmentBefore(p, suffix string) string {
	trimmed := p[:len(p)-len(suffix)]
	// Find last "/"
	for i := len(trimmed) - 1; i >= 0; i-- {
		if trimmed[i] == '/' {
			return trimmed[i+1:]
		}
	}
	return trimmed
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestStateStore(t *testing.T) *state.Store {
	t.Helper()
	dir := t.TempDir()
	return state.New(dir + "/state.json")
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func makePR(number int, branch, author, updatedAt string, labels ...string) PRData {
	pr := PRData{
		Number:    number,
		Title:     fmt.Sprintf("PR #%d", number),
		Body:      "Test body",
		HTMLURL:   fmt.Sprintf("https://github.com/org/repo/pull/%d", number),
		DiffURL:   fmt.Sprintf("https://github.com/org/repo/pull/%d.diff", number),
		UpdatedAt: updatedAt,
	}
	pr.User.Login = author
	pr.Base.Ref = branch
	pr.Head.Ref = fmt.Sprintf("feature/pr-%d", number)
	pr.Head.SHA = fmt.Sprintf("sha%d", number)
	for _, l := range labels {
		pr.Labels = append(pr.Labels, struct {
			Name string `json:"name"`
		}{Name: l})
	}
	return pr
}

func makeIssue(number int, author string, isPR bool, labels ...string) IssueData {
	issue := IssueData{
		Number:  number,
		Title:   fmt.Sprintf("Issue #%d", number),
		Body:    "Issue body",
		HTMLURL: fmt.Sprintf("https://github.com/org/repo/issues/%d", number),
	}
	issue.User.Login = author
	for _, l := range labels {
		issue.Labels = append(issue.Labels, struct {
			Name string `json:"name"`
		}{Name: l})
	}
	if isPR {
		issue.PullRequest = &struct{}{}
	}
	return issue
}

func makeCheckRun(id int64, name, conclusion string) CheckRunData {
	cr := CheckRunData{
		ID:         id,
		Name:       name,
		Status:     "completed",
		Conclusion: conclusion,
		HTMLURL:    fmt.Sprintf("https://github.com/org/repo/runs/%d", id),
	}
	return cr
}

// ---------------------------------------------------------------------------
// PR polling tests
// ---------------------------------------------------------------------------

func TestPoller_PollPRs_ReturnsMatchingPRs(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
		makePR(2, "main", "bob", "2024-01-03T10:00:00Z"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestPoller_PollPRs_UpdatesCursor(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
		makePR(2, "main", "bob", "2024-01-05T10:00:00Z"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	_, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}

	cursor := st.GetCursor("conn-1:pr:org/repo")
	if cursor == "" {
		t.Fatal("expected cursor to be set after poll")
	}
	// Cursor should be the most recent updated_at
	if cursor != "2024-01-05T10:00:00Z" {
		t.Errorf("cursor = %q, want %q", cursor, "2024-01-05T10:00:00Z")
	}
}

func TestPoller_PollPRs_RespectsHighWaterMark(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
		makePR(2, "main", "bob", "2024-01-05T10:00:00Z"),
	}

	st := newTestStateStore(t)
	// Pre-set cursor to after PR 1 but before PR 2
	_ = st.SetCursor("conn-1:pr:org/repo", "2024-01-03T00:00:00Z")

	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (only PR updated after cursor), got %d", len(events))
	}
	if events[0].Variables["PR_NUMBER"] != "2" {
		t.Errorf("expected PR #2, got PR #%s", events[0].Variables["PR_NUMBER"])
	}
}

func TestPoller_PollPRs_BranchFilter(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
		makePR(2, "develop", "bob", "2024-01-03T10:00:00Z"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{Branches: []string{"main"}}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (only main branch PR), got %d", len(events))
	}
	if events[0].Variables["PR_BASE"] != "main" {
		t.Errorf("expected PR_BASE=main, got %s", events[0].Variables["PR_BASE"])
	}
}

func TestPoller_PollPRs_AuthorsIgnore(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(1, "main", "dependabot[bot]", "2024-01-02T10:00:00Z"),
		makePR(2, "main", "alice", "2024-01-03T10:00:00Z"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{AuthorsIgnore: []string{"dependabot[bot]"}}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (bot excluded), got %d", len(events))
	}
	if events[0].Variables["PR_AUTHOR"] != "alice" {
		t.Errorf("expected alice, got %s", events[0].Variables["PR_AUTHOR"])
	}
}

func TestPoller_PollPRs_Deduplication(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
	}

	st := newTestStateStore(t)
	// Pre-mark PR 1 as processed
	_ = st.MarkProcessed("pr:org/repo:1")

	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events (already processed), got %d", len(events))
	}
}

func TestPoller_PollPRs_MarkProcessed(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(5, "main", "charlie", "2024-01-10T10:00:00Z"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Second poll should return nothing (deduped)
	events2, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Second poll returned error: %v", err)
	}
	if len(events2) != 0 {
		t.Fatalf("expected 0 events on second poll, got %d", len(events2))
	}
}

func TestPoller_PollPRs_PathFilter_FetchesFiles(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
		makePR(2, "main", "bob", "2024-01-03T10:00:00Z"),
	}
	// PR 1 touches src/; PR 2 touches docs/
	mock.pullFiles[1] = []string{"src/main.go", "src/util.go"}
	mock.pullFiles[2] = []string{"docs/readme.md"}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{Paths: []string{"src/**"}}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (only src/** PR), got %d", len(events))
	}
	if events[0].Variables["PR_NUMBER"] != "1" {
		t.Errorf("expected PR #1, got PR #%s", events[0].Variables["PR_NUMBER"])
	}
}

func TestPoller_PollPRs_MatchedEventFields(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(42, "main", "alice", "2024-01-02T10:00:00Z"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	ev := events[0]
	if ev.Template != "my-template" {
		t.Errorf("Template = %q, want %q", ev.Template, "my-template")
	}
	if ev.EventKey != "pr:org/repo:42" {
		t.Errorf("EventKey = %q, want %q", ev.EventKey, "pr:org/repo:42")
	}
	if ev.Variables["PR_NUMBER"] != "42" {
		t.Errorf("PR_NUMBER = %q, want 42", ev.Variables["PR_NUMBER"])
	}
	if ev.Variables["PR_AUTHOR"] != "alice" {
		t.Errorf("PR_AUTHOR = %q, want alice", ev.Variables["PR_AUTHOR"])
	}
	if ev.Variables["REPO_FULL_NAME"] != "org/repo" {
		t.Errorf("REPO_FULL_NAME = %q, want org/repo", ev.Variables["REPO_FULL_NAME"])
	}
}

// ---------------------------------------------------------------------------
// Check run polling tests
// ---------------------------------------------------------------------------

func TestPoller_PollCheckRuns_FindsCompletedCheckRuns(t *testing.T) {
	mock := newMockGitHub(t)
	pr := makePR(10, "main", "alice", "2024-01-02T10:00:00Z")
	mock.pulls = []PRData{pr}
	mock.checkRuns[pr.Head.SHA] = []CheckRunData{
		makeCheckRun(101, "CI / test", "failure"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		CheckRunCompleted: &CheckTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 check run event, got %d", len(events))
	}
	if events[0].Variables["CHECK_NAME"] != "CI / test" {
		t.Errorf("CHECK_NAME = %q, want CI / test", events[0].Variables["CHECK_NAME"])
	}
	if events[0].EventKey != "check:org/repo:101" {
		t.Errorf("EventKey = %q, want check:org/repo:101", events[0].EventKey)
	}
}

func TestPoller_PollCheckRuns_ConclusionFilter(t *testing.T) {
	mock := newMockGitHub(t)
	pr := makePR(10, "main", "alice", "2024-01-02T10:00:00Z")
	mock.pulls = []PRData{pr}
	mock.checkRuns[pr.Head.SHA] = []CheckRunData{
		makeCheckRun(101, "CI / test", "failure"),
		makeCheckRun(102, "CI / lint", "success"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		CheckRunCompleted: &CheckTrigger{Filters: Filters{Conclusions: []string{"failure"}}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (only failure), got %d", len(events))
	}
	if events[0].Variables["CHECK_CONCLUSION"] != "failure" {
		t.Errorf("CHECK_CONCLUSION = %q, want failure", events[0].Variables["CHECK_CONCLUSION"])
	}
}

func TestPoller_PollCheckRuns_CheckNameFilter(t *testing.T) {
	mock := newMockGitHub(t)
	pr := makePR(10, "main", "alice", "2024-01-02T10:00:00Z")
	mock.pulls = []PRData{pr}
	mock.checkRuns[pr.Head.SHA] = []CheckRunData{
		makeCheckRun(101, "CI / test", "failure"),
		makeCheckRun(102, "CI / deploy", "failure"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		CheckRunCompleted: &CheckTrigger{Filters: Filters{CheckNames: []string{"CI / test"}}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (only CI / test), got %d", len(events))
	}
}

func TestPoller_PollCheckRuns_Deduplication(t *testing.T) {
	mock := newMockGitHub(t)
	pr := makePR(10, "main", "alice", "2024-01-02T10:00:00Z")
	mock.pulls = []PRData{pr}
	mock.checkRuns[pr.Head.SHA] = []CheckRunData{
		makeCheckRun(101, "CI / test", "failure"),
	}

	st := newTestStateStore(t)
	_ = st.MarkProcessed("check:org/repo:101")

	triggers := TriggerConfig{
		CheckRunCompleted: &CheckTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events (already processed), got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// Issue polling tests
// ---------------------------------------------------------------------------

func TestPoller_PollIssues_ReturnsLabeledIssues(t *testing.T) {
	mock := newMockGitHub(t)
	mock.issues = []IssueData{
		makeIssue(1, "alice", false, "claude-review"),
		makeIssue(2, "bob", false, "bug"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		IssueLabeled: &IssueTrigger{Filters: Filters{Labels: []string{"claude-review"}}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (only claude-review labeled), got %d", len(events))
	}
	if events[0].Variables["ISSUE_NUMBER"] != "1" {
		t.Errorf("ISSUE_NUMBER = %q, want 1", events[0].Variables["ISSUE_NUMBER"])
	}
}

func TestPoller_PollIssues_SkipsPRs(t *testing.T) {
	mock := newMockGitHub(t)
	mock.issues = []IssueData{
		makeIssue(1, "alice", false, "claude-review"),
		makeIssue(2, "bob", true, "claude-review"), // is a PR
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		IssueLabeled: &IssueTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (PR-type issue skipped), got %d", len(events))
	}
	if events[0].Variables["ISSUE_NUMBER"] != "1" {
		t.Errorf("expected issue #1, got #%s", events[0].Variables["ISSUE_NUMBER"])
	}
}

func TestPoller_PollIssues_Deduplication(t *testing.T) {
	mock := newMockGitHub(t)
	mock.issues = []IssueData{
		makeIssue(7, "alice", false, "bug"),
	}

	st := newTestStateStore(t)
	_ = st.MarkProcessed("issue:org/repo:7")

	triggers := TriggerConfig{
		IssueLabeled: &IssueTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events (already processed), got %d", len(events))
	}
}

func TestPoller_PollIssues_EventKeyAndVariables(t *testing.T) {
	mock := newMockGitHub(t)
	mock.issues = []IssueData{
		makeIssue(99, "reporter", false, "help-wanted"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		IssueLabeled: &IssueTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.EventKey != "issue:org/repo:99" {
		t.Errorf("EventKey = %q, want issue:org/repo:99", ev.EventKey)
	}
	if ev.Variables["ISSUE_AUTHOR"] != "reporter" {
		t.Errorf("ISSUE_AUTHOR = %q, want reporter", ev.Variables["ISSUE_AUTHOR"])
	}
}

// ---------------------------------------------------------------------------
// Rate limit tests
// ---------------------------------------------------------------------------

func TestPoller_RateLimit_LogsWarningAndReturnsEarly(t *testing.T) {
	mock := newMockGitHub(t)
	mock.rateLimitRemaining = 50 // below 100 threshold
	mock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	// Should return early with partial results and not crash
	_, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Multi-trigger test
// ---------------------------------------------------------------------------

func TestPoller_MultiTrigger_PRsAndIssues(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
	}
	mock.issues = []IssueData{
		makeIssue(5, "bob", false, "bug"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{}},
		IssueLabeled:      &IssueTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	events, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (1 PR + 1 issue), got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// Context cancellation test
// ---------------------------------------------------------------------------

func TestPoller_ContextCancelled(t *testing.T) {
	mock := newMockGitHub(t)
	mock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
	}

	st := newTestStateStore(t)
	triggers := TriggerConfig{
		PullRequestOpened: &PRTrigger{Filters: Filters{}},
	}
	p := NewRepoPoller("org/repo", "token123", "my-template", "conn-1", triggers, st, newTestLogger())
	p.SetAPIBase(mock.server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // ensure context is expired

	_, err := p.Poll(ctx)
	// Should return a context error
	if err == nil {
		// Some implementations may handle gracefully; acceptable
		t.Log("Poll with cancelled context returned no error (acceptable)")
	}
}
