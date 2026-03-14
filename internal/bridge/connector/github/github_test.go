package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
	"github.com/kodrunhq/claude-plane/internal/bridge/state"
)

// ---------------------------------------------------------------------------
// Mock claude-plane API server
// ---------------------------------------------------------------------------

type mockPlaneServer struct {
	server          *httptest.Server
	mu              sync.Mutex
	sessionRequests []client.CreateSessionRequest
	callCount       atomic.Int64
}

func newMockPlaneServer(t *testing.T) *mockPlaneServer {
	t.Helper()
	m := &mockPlaneServer{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req client.CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.sessionRequests = append(m.sessionRequests, req)
		m.mu.Unlock()
		m.callCount.Add(1)

		resp := client.Session{
			SessionID: "sess-123",
			MachineID: req.MachineID,
			Status:    "running",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

// ---------------------------------------------------------------------------
// Mock GitHub API server with /user endpoint
// ---------------------------------------------------------------------------

type mockGitHubConnectorServer struct {
	server             *httptest.Server
	userLogin          string
	userStatusCode     int
	pulls              []PRData
	pullFiles          map[int][]string
	checkRuns          map[string][]CheckRunData
	issues             []IssueData
	rateLimitRemaining int
}

func newMockGitHubConnector(t *testing.T) *mockGitHubConnectorServer {
	t.Helper()
	m := &mockGitHubConnectorServer{
		pullFiles:          make(map[int][]string),
		checkRuns:          make(map[string][]CheckRunData),
		rateLimitRemaining: 1000,
		userLogin:          "testuser",
		userStatusCode:     http.StatusOK,
	}

	mux := http.NewServeMux()

	// GET /user — token validation
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(m.userStatusCode)
		if m.userStatusCode == http.StatusOK {
			_ = json.NewEncoder(w).Encode(map[string]string{"login": m.userLogin})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Bad credentials"})
		}
	})

	// GET /repos/...
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "1000")

		path := r.URL.Path
		switch {
		case hasSuffix(path, "/pulls") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(m.pulls)

		case hasSuffixSegment(path, "/pulls", "/files") && r.Method == http.MethodGet:
			num := extractSegmentBefore(path, "/files")
			var prNum int
			for i := 0; i < len(num); i++ {
				prNum = prNum*10 + int(num[i]-'0')
			}
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

// ---------------------------------------------------------------------------
// Connector test helpers
// ---------------------------------------------------------------------------

func newTestConnector(
	t *testing.T,
	cfg Config,
	planeURL string,
	stateStore *state.Store,
) *GitHub {
	t.Helper()
	apiClient := client.New(planeURL, "test-api-key")
	return New("test-gh-connector", cfg, apiClient, stateStore, newTestLogger())
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestGitHub_Start_ValidToken_Authenticates(t *testing.T) {
	ghMock := newMockGitHubConnector(t)
	planeMock := newMockPlaneServer(t)

	// A PR that will match immediately so we can verify session creation.
	ghMock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
	}

	cfg := Config{
		Token: "valid-token",
		Watches: []WatchConfig{
			{
				Repo:         "org/repo",
				Template:     "review-template",
				MachineID:    "machine-1",
				PollInterval: "50ms", // fast for tests
				Triggers: TriggerConfig{
					PullRequestOpened: &PRTrigger{Filters: Filters{}},
				},
			},
		},
	}

	st := newTestStateStore(t)
	g := newTestConnector(t, cfg, planeMock.server.URL, st)
	g.SetAPIBase(ghMock.server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := g.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// Session should have been created for the matched PR.
	if planeMock.callCount.Load() == 0 {
		t.Error("expected at least one session creation call")
	}
}

func TestGitHub_Start_InvalidToken_ReturnsError(t *testing.T) {
	ghMock := newMockGitHubConnector(t)
	ghMock.userStatusCode = http.StatusUnauthorized

	planeMock := newMockPlaneServer(t)

	cfg := Config{
		Token: "bad-token",
		Watches: []WatchConfig{
			{
				Repo:         "org/repo",
				Template:     "review-template",
				PollInterval: "1s",
				Triggers: TriggerConfig{
					PullRequestOpened: &PRTrigger{Filters: Filters{}},
				},
			},
		},
	}

	st := newTestStateStore(t)
	g := newTestConnector(t, cfg, planeMock.server.URL, st)
	g.SetAPIBase(ghMock.server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := g.Start(ctx)
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}
}

func TestGitHub_Healthy_TrueAfterStart_FalseAfterShutdown(t *testing.T) {
	ghMock := newMockGitHubConnector(t)
	planeMock := newMockPlaneServer(t)

	cfg := Config{
		Token: "valid-token",
		Watches: []WatchConfig{
			{
				Repo:         "org/repo",
				Template:     "tmpl",
				PollInterval: "1s",
				Triggers: TriggerConfig{
					PullRequestOpened: &PRTrigger{Filters: Filters{}},
				},
			},
		},
	}

	st := newTestStateStore(t)
	g := newTestConnector(t, cfg, planeMock.server.URL, st)
	g.SetAPIBase(ghMock.server.URL)

	if g.Healthy() {
		t.Error("expected Healthy() = false before Start()")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start runs until context cancelled; check health while running.
	healthyCh := make(chan bool, 1)
	go func() {
		// Small delay to let Start() set healthy = true.
		time.Sleep(50 * time.Millisecond)
		healthyCh <- g.Healthy()
	}()

	_ = g.Start(ctx)

	healthyDuringRun := <-healthyCh
	if !healthyDuringRun {
		t.Error("expected Healthy() = true while connector is running")
	}

	if g.Healthy() {
		t.Error("expected Healthy() = false after Start() returns")
	}
}

func TestGitHub_Start_GracefulShutdown_OnContextCancel(t *testing.T) {
	ghMock := newMockGitHubConnector(t)
	planeMock := newMockPlaneServer(t)

	cfg := Config{
		Token: "valid-token",
		Watches: []WatchConfig{
			{
				Repo:         "org/repo",
				Template:     "tmpl",
				PollInterval: "10s", // long so we rely on ctx cancel
				Triggers: TriggerConfig{
					PullRequestOpened: &PRTrigger{Filters: Filters{}},
				},
			},
		},
	}

	st := newTestStateStore(t)
	g := newTestConnector(t, cfg, planeMock.server.URL, st)
	g.SetAPIBase(ghMock.server.URL)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- g.Start(ctx)
	}()

	// Let it start.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error on graceful shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancel within 2s")
	}
}

func TestGitHub_Start_SessionCreation_UsesTemplateAndMachine(t *testing.T) {
	ghMock := newMockGitHubConnector(t)
	planeMock := newMockPlaneServer(t)

	ghMock.pulls = []PRData{
		makePR(42, "main", "alice", "2024-01-02T10:00:00Z"),
	}

	cfg := Config{
		Token: "valid-token",
		Watches: []WatchConfig{
			{
				Repo:         "org/repo",
				Template:     "pr-review",
				MachineID:    "worker-1",
				PollInterval: "50ms",
				Triggers: TriggerConfig{
					PullRequestOpened: &PRTrigger{Filters: Filters{}},
				},
			},
		},
	}

	st := newTestStateStore(t)
	g := newTestConnector(t, cfg, planeMock.server.URL, st)
	g.SetAPIBase(ghMock.server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = g.Start(ctx)

	planeMock.mu.Lock()
	reqs := planeMock.sessionRequests
	planeMock.mu.Unlock()
	if len(reqs) == 0 {
		t.Fatal("expected session creation request, got none")
	}

	req := reqs[0]
	if req.TemplateName != "pr-review" {
		t.Errorf("TemplateName = %q, want %q", req.TemplateName, "pr-review")
	}
	if req.MachineID != "worker-1" {
		t.Errorf("MachineID = %q, want %q", req.MachineID, "worker-1")
	}
	// Variables should include PR_NUMBER.
	if req.Vars["PR_NUMBER"] != "42" {
		t.Errorf("Vars[PR_NUMBER] = %q, want 42", req.Vars["PR_NUMBER"])
	}
}

func TestGitHub_Start_MultipleWatches_PollIndependently(t *testing.T) {
	ghMock := newMockGitHubConnector(t)
	planeMock := newMockPlaneServer(t)

	// Both repos have one matching PR each.
	ghMock.pulls = []PRData{
		makePR(1, "main", "alice", "2024-01-02T10:00:00Z"),
	}

	cfg := Config{
		Token: "valid-token",
		Watches: []WatchConfig{
			{
				Repo:         "org/repo-a",
				Template:     "tmpl-a",
				PollInterval: "50ms",
				Triggers: TriggerConfig{
					PullRequestOpened: &PRTrigger{Filters: Filters{}},
				},
			},
			{
				Repo:         "org/repo-b",
				Template:     "tmpl-b",
				PollInterval: "50ms",
				Triggers: TriggerConfig{
					PullRequestOpened: &PRTrigger{Filters: Filters{}},
				},
			},
		},
	}

	st := newTestStateStore(t)
	g := newTestConnector(t, cfg, planeMock.server.URL, st)
	g.SetAPIBase(ghMock.server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = g.Start(ctx)

	// Each watch should create at least one session (one for each repo).
	if planeMock.callCount.Load() < 2 {
		t.Errorf("expected at least 2 session creations (one per watch), got %d", planeMock.callCount.Load())
	}
}

func TestGitHub_Name_ReturnsConnectorID(t *testing.T) {
	st := newTestStateStore(t)
	apiClient := client.New("http://localhost", "key")
	g := New("my-github-conn", Config{}, apiClient, st, newTestLogger())

	if g.Name() != "my-github-conn" {
		t.Errorf("Name() = %q, want %q", g.Name(), "my-github-conn")
	}
}

func TestGitHub_Start_NoMatches_NoSessionsCreated(t *testing.T) {
	ghMock := newMockGitHubConnector(t)
	planeMock := newMockPlaneServer(t)

	// No PRs, no matches.
	ghMock.pulls = []PRData{}

	cfg := Config{
		Token: "valid-token",
		Watches: []WatchConfig{
			{
				Repo:         "org/repo",
				Template:     "tmpl",
				PollInterval: "50ms",
				Triggers: TriggerConfig{
					PullRequestOpened: &PRTrigger{Filters: Filters{}},
				},
			},
		},
	}

	st := newTestStateStore(t)
	g := newTestConnector(t, cfg, planeMock.server.URL, st)
	g.SetAPIBase(ghMock.server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_ = g.Start(ctx)

	if planeMock.callCount.Load() != 0 {
		t.Errorf("expected 0 session calls, got %d", planeMock.callCount.Load())
	}
}

func TestGitHub_Start_DefaultPollInterval_UsedWhenEmpty(t *testing.T) {
	ghMock := newMockGitHubConnector(t)
	planeMock := newMockPlaneServer(t)

	cfg := Config{
		Token: "valid-token",
		Watches: []WatchConfig{
			{
				Repo:         "org/repo",
				Template:     "tmpl",
				PollInterval: "", // empty → default 60s
				Triggers: TriggerConfig{
					PullRequestOpened: &PRTrigger{Filters: Filters{}},
				},
			},
		},
	}

	st := newTestStateStore(t)
	g := newTestConnector(t, cfg, planeMock.server.URL, st)
	g.SetAPIBase(ghMock.server.URL)

	// Context cancelled quickly; connector should start without panic.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := g.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}
