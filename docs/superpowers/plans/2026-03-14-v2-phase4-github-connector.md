# Phase 4: GitHub Connector — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a GitHub connector to the bridge that polls repositories for PR, CI, and issue events, matches them against configurable trigger rules and filters, and creates template-based sessions automatically. Add the GitHub connector configuration form to the frontend.

**Architecture:** New connector package implementing the existing `Connector` interface. Polls GitHub REST API per configured watch, evaluates AND-combined filters, deduplicates via state store, and dispatches session creation through the bridge's claude-plane API client. Frontend adds GitHub-specific form to the existing connectors page.

**Tech Stack:** Go (GitHub REST API client, filter engine), React 19, TypeScript

**Design Spec:** `docs/superpowers/specs/2026-03-14-v2-templates-injection-bridge-design.md` — Section 8

**Depends on:** Phase 1 (templates — `useTemplates()` hook used in frontend), Phase 2 (injection — inject endpoint used by bridge), and Phase 3 (bridge core + Telegram — connector interface, bridge lifecycle, state store, API client).

**SDK Decision:** Use `net/http` directly for GitHub REST API calls (no `google/go-github` dependency). GitHub's REST API is simple enough that a thin HTTP client with JSON parsing is cleaner than importing a large SDK. This keeps the bridge binary small and avoids version coupling.

---

## File Map

### Bridge — Create
| File | Responsibility |
|------|---------------|
| `internal/bridge/connector/github/github.go` | GitHub connector: polling loop, event dispatch |
| `internal/bridge/connector/github/github_test.go` | Connector tests |
| `internal/bridge/connector/github/poller.go` | Per-repo polling with high-water mark tracking |
| `internal/bridge/connector/github/poller_test.go` | Poller tests |
| `internal/bridge/connector/github/filters.go` | AND-combined filter evaluation |
| `internal/bridge/connector/github/filters_test.go` | Filter tests |
| `internal/bridge/connector/github/variables.go` | Event → template variables extraction |
| `internal/bridge/connector/github/variables_test.go` | Variable extraction tests |

### Frontend — Create
| File | Responsibility |
|------|---------------|
| `web/src/components/connectors/GithubForm.tsx` | GitHub connector config form with watches |
| `web/src/components/connectors/WatchEditor.tsx` | Watch sub-form (repo, template, triggers, filters) |
| `web/src/components/connectors/TriggerConfig.tsx` | Per-trigger type configuration (enable, filters) |

### Frontend — Modify
| File | Change |
|------|--------|
| `web/src/components/connectors/AddConnectorModal.tsx` | Enable GitHub option (currently disabled/grayed) |

---

## Chunk 1: Filter Engine + Variable Extraction

### Task 1: Filter Evaluation Engine

**Files:**
- Create: `internal/bridge/connector/github/filters.go`
- Create: `internal/bridge/connector/github/filters_test.go`

- [ ] **Step 1: Write failing tests**

Test cases for each filter type:
- `branches`: PR with base `main` passes `["main"]`; PR with base `develop` fails `["main"]`
- `labels`: PR with label `claude-review` passes `["claude-review"]`; PR without fails
- `check_names`: check run `CI / test` passes `["CI / test"]`; `CI / deploy` fails
- `conclusions`: `failure` passes `["failure", "timed_out"]`; `success` fails
- `paths`: changed file `src/main.go` passes `["src/**"]`; `docs/readme.md` fails
- `authors_ignore`: PR by `dependabot[bot]` fails `["dependabot[bot]"]`; PR by `jose` passes
- AND combination: all filters must pass
- Empty filter array: means no filtering (pass all)

Run: `go test -race ./internal/bridge/connector/github/ -run TestFilter -v`
Expected: FAIL

- [ ] **Step 2: Implement filter types**

```go
type Filters struct {
    Branches      []string `json:"branches"`
    Labels        []string `json:"labels"`
    CheckNames    []string `json:"check_names"`
    Conclusions   []string `json:"conclusions"`
    Paths         []string `json:"paths"`
    AuthorsIgnore []string `json:"authors_ignore"`
}

// Match evaluates all configured filters (AND-combined).
// Empty slices are treated as "no filter" (pass all).
func (f *Filters) Match(event EventData) bool
```

`EventData` carries the fields each filter checks against: `BaseBranch`, `Labels`, `CheckName`, `Conclusion`, `ChangedFiles`, `Author`.

For `paths` filter, use `filepath.Match` for glob matching against each changed file.

- [ ] **Step 3: Run tests**

Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/bridge/connector/github/filters.go internal/bridge/connector/github/filters_test.go
git commit -m "feat: add GitHub connector filter evaluation engine"
```

---

### Task 2: Variable Extraction

**Files:**
- Create: `internal/bridge/connector/github/variables.go`
- Create: `internal/bridge/connector/github/variables_test.go`

- [ ] **Step 1: Write failing tests**

Test cases:
- PR opened event → extracts `PR_URL`, `PR_TITLE`, `PR_BODY`, `PR_AUTHOR`, `PR_BRANCH`, `PR_BASE`, `PR_NUMBER`, `PR_DIFF_URL`, `REPO_FULL_NAME`
- Check run completed → extracts `CHECK_NAME`, `CHECK_STATUS`, `CHECK_CONCLUSION`, `CHECK_URL`, `CHECK_OUTPUT` (truncated to 4KB), `PR_URL`, `REPO_FULL_NAME`
- Issue labeled → extracts `ISSUE_URL`, `ISSUE_TITLE`, `ISSUE_BODY`, `ISSUE_AUTHOR`, `ISSUE_LABELS`, `ISSUE_NUMBER`, `REPO_FULL_NAME`

- [ ] **Step 2: Implement variable extractors**

```go
// Uses plain structs parsed from GitHub REST API JSON responses (no SDK dependency).
func ExtractPRVariables(pr PRData, repo string) map[string]string
func ExtractCheckRunVariables(cr CheckRunData, repo string) map[string]string
func ExtractIssueVariables(issue IssueData, repo string) map[string]string
```

`PRData`, `CheckRunData`, `IssueData` are lightweight structs defined in `variables.go` with only the fields needed for variable extraction. Parsed directly from `json.Decoder`.

Each returns a flat `map[string]string` ready for template variable substitution. `CHECK_OUTPUT` truncated to 4096 bytes.

- [ ] **Step 3: Run tests and commit**

```
git add internal/bridge/connector/github/variables.go internal/bridge/connector/github/variables_test.go
git commit -m "feat: add GitHub event variable extraction"
```

---

## Chunk 2: Poller + Connector

### Task 3: Per-Repo Poller

**Files:**
- Create: `internal/bridge/connector/github/poller.go`
- Create: `internal/bridge/connector/github/poller_test.go`

- [ ] **Step 1: Write failing tests**

Test cases (use `httptest.Server` to mock GitHub API):
- Poll PRs: returns PRs updated after high-water mark; updates cursor
- Poll check runs: returns completed checks after cursor
- Poll issues: returns issues updated after cursor
- Rate limit handling: backs off when `X-RateLimit-Remaining` < 10%
- Deduplication: already processed event ID skipped

- [ ] **Step 2: Implement poller**

```go
type RepoPoller struct {
    repo         string         // "owner/repo"
    token        string
    triggers     TriggerConfig
    state        *state.Store
    httpClient   *http.Client
    logger       *slog.Logger
}

type TriggerConfig struct {
    PullRequestOpened *PRTrigger    `json:"pull_request_opened"`
    CheckRunCompleted *CheckTrigger `json:"check_run_completed"`
    IssueLabeled      *IssueTrigger `json:"issue_labeled"`
}

// Poll checks all enabled triggers for one repo. Returns matched events.
func (p *RepoPoller) Poll(ctx context.Context) ([]MatchedEvent, error)
```

Each trigger type:
1. Calls appropriate GitHub API endpoint
2. Filters by high-water mark (only events newer than last seen)
3. Evaluates filters
4. Checks deduplication
5. Extracts variables
6. Returns `MatchedEvent{Template, Variables, EventKey}`

After processing, updates high-water mark and marks events as processed.

- [ ] **Step 3: Run tests and commit**

```
git add internal/bridge/connector/github/poller.go internal/bridge/connector/github/poller_test.go
git commit -m "feat: add per-repo GitHub poller with rate limit handling"
```

---

### Task 4: GitHub Connector

**Files:**
- Create: `internal/bridge/connector/github/github.go`
- Create: `internal/bridge/connector/github/github_test.go`

- [ ] **Step 1: Write failing tests**

Test cases:
- `Start`: creates pollers for each configured watch, polls at configured intervals
- Matched event creates session via API client
- Auth validation on startup (`GET /user`)
- Health reporting
- Graceful shutdown

- [ ] **Step 2: Implement GitHub connector**

```go
// WatchConfig represents a single repository watch parsed from connector config JSON.
type WatchConfig struct {
    Repo         string        `json:"repo"`          // "owner/repo"
    Template     string        `json:"template"`      // template name for session creation
    PollInterval string        `json:"poll_interval"` // e.g., "60s"
    Triggers     TriggerConfig `json:"triggers"`
}

type GitHub struct {
    token    string
    watches  []WatchConfig
    client   *client.Client
    state    *state.Store
    logger   *slog.Logger
    healthy  atomic.Bool
}

func (g *GitHub) Name() string    { return "github" }
func (g *GitHub) Healthy() bool   { return g.healthy.Load() }

func (g *GitHub) Start(ctx context.Context) error {
    // 1. Validate token: GET https://api.github.com/user
    // 2. Create RepoPoller per watch
    // 3. For each poller, run poll loop at configured interval
    // 4. On matched event: client.CreateSession(template_name, variables)
    // 5. Block until ctx cancelled
}
```

Each watch's poll loop runs in its own goroutine. State store prunes old entries on each cycle.

- [ ] **Step 3: Run tests and commit**

```
git add internal/bridge/connector/github/github.go internal/bridge/connector/github/github_test.go
git commit -m "feat: add GitHub connector with polling and session creation"
```

---

### Task 5: Register GitHub Connector in Bridge

**Files:**
- Modify: `internal/bridge/bridge.go`

- [ ] **Step 1: Add GitHub connector instantiation**

In the connector factory logic (where bridge creates connectors from config), add:
```go
case "github":
    conn, err = github.New(connConfig, b.client, b.state, b.logger)
```

- [ ] **Step 2: Add integration test for GitHub factory path**

In `internal/bridge/bridge_test.go`, add a test case that verifies the factory instantiates a GitHub connector from a connector config with `connector_type: "github"`. Use a mock HTTP server for GitHub API validation.

- [ ] **Step 3: Build and verify**

Run: `go build -o claude-plane-bridge ./cmd/bridge`
Expected: builds successfully

Run: `go test -race ./internal/bridge/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add internal/bridge/bridge.go internal/bridge/bridge_test.go
git commit -m "feat: register GitHub connector in bridge factory"
```

---

## Chunk 3: Frontend

### Task 6: GitHub Connector Form

**Files:**
- Create: `web/src/components/connectors/GithubForm.tsx`
- Create: `web/src/components/connectors/WatchEditor.tsx`
- Create: `web/src/components/connectors/TriggerConfig.tsx`
- Modify: `web/src/components/connectors/AddConnectorModal.tsx`

- [ ] **Step 1: Create TriggerConfig component**

Per-trigger-type configuration panel:
- Enable/disable toggle
- Conditional filter fields based on trigger type:
  - PR opened: `branches` (tag input), `labels` (tag input), `authors_ignore` (tag input), `paths` (tag input)
  - Check run completed: `check_names` (tag input), `conclusions` (multi-select: success, failure, timed_out, cancelled)
  - Issue labeled: `labels` (tag input)
- Each filter field is a tag-style multi-input (type value, press Enter to add, click x to remove)

- [ ] **Step 2: Create WatchEditor component**

A single watch configuration:
- Repository input (`owner/repo` format) with validation
- Template dropdown (populated from `useTemplates()`)
- Poll interval select (30s, 60s, 120s, 300s)
- Expandable trigger sections using `TriggerConfig`
- Remove watch button

- [ ] **Step 3: Create GithubForm component**

GitHub connector configuration form:
- Token input (password field) with scope requirements help text (`repo` scope)
- "Test Connection" button — validates token, shows authenticated username on success
- Watches section:
  - List of `WatchEditor` components
  - "Add Watch" button
- The form manages the full config JSON shape for the GitHub connector type

- [ ] **Step 4: Enable GitHub in AddConnectorModal**

Remove the disabled/grayed state from the GitHub option in the type picker.

- [ ] **Step 5: Run frontend lint + tests**

Run: `cd web && npx vitest run && npm run lint`
Expected: PASS

- [ ] **Step 6: Commit**

```
git add web/src/components/connectors/GithubForm.tsx web/src/components/connectors/WatchEditor.tsx web/src/components/connectors/TriggerConfig.tsx web/src/components/connectors/AddConnectorModal.tsx
git commit -m "feat: add GitHub connector configuration form with watches and triggers"
```

---

## Chunk 4: Verification

### Task 7: Full Verification

- [ ] **Step 1: Run all Go tests**

Run: `go test -race ./...`
Expected: PASS

- [ ] **Step 2: Build all binaries**

Run: `go build -o claude-plane-server ./cmd/server && go build -o claude-plane-agent ./cmd/agent && go build -o claude-plane-bridge ./cmd/bridge`
Expected: all build

- [ ] **Step 3: Run all frontend tests**

Run: `cd web && npx vitest run && npm run lint`
Expected: PASS

- [ ] **Step 4: Manual smoke test**

1. Configure GitHub connector via UI with a test repo
2. Add a watch with PR opened trigger, `branches: ["main"]`
3. Apply & Restart bridge
4. Open a PR on the test repo
5. Verify bridge detects the PR, creates a session from template
6. Verify deduplication: same PR doesn't create a second session
7. Test filter: PR targeting non-main branch should be ignored
8. Test `authors_ignore`: PR by ignored author should be skipped

- [ ] **Step 5: Commit any fixes**

```
git commit -m "fix: address integration issues from Phase 4 smoke test"
```
