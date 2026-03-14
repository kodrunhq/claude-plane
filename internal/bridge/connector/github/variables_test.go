package github

import (
	"strings"
	"testing"
)

func TestExtractPRVariables(t *testing.T) {
	pr := PRData{
		Number:  42,
		Title:   "Add new feature",
		Body:    "This PR adds a new feature.",
		HTMLURL: "https://github.com/org/repo/pull/42",
		DiffURL: "https://github.com/org/repo/pull/42.diff",
	}
	pr.User.Login = "octocat"
	pr.Head.Ref = "feature/my-branch"
	pr.Base.Ref = "main"
	pr.Labels = []struct {
		Name string `json:"name"`
	}{
		{Name: "bug"},
		{Name: "enhancement"},
	}

	got := ExtractPRVariables(pr, "org/repo")

	cases := map[string]string{
		"PR_URL":        "https://github.com/org/repo/pull/42",
		"PR_TITLE":      "Add new feature",
		"PR_BODY":       "This PR adds a new feature.",
		"PR_AUTHOR":     "octocat",
		"PR_BRANCH":     "feature/my-branch",
		"PR_BASE":       "main",
		"PR_NUMBER":     "42",
		"PR_DIFF_URL":   "https://github.com/org/repo/pull/42.diff",
		"REPO_FULL_NAME": "org/repo",
	}

	for key, want := range cases {
		if got[key] != want {
			t.Errorf("PR variable %s: got %q, want %q", key, got[key], want)
		}
	}

	if len(got) != len(cases) {
		t.Errorf("expected %d PR variables, got %d", len(cases), len(got))
	}
}

func TestExtractPRVariables_EmptyFields(t *testing.T) {
	pr := PRData{}

	got := ExtractPRVariables(pr, "")

	for _, key := range []string{"PR_URL", "PR_TITLE", "PR_BODY", "PR_AUTHOR", "PR_BRANCH", "PR_BASE", "PR_DIFF_URL", "REPO_FULL_NAME"} {
		if got[key] != "" {
			t.Errorf("PR variable %s: expected empty string, got %q", key, got[key])
		}
	}

	if got["PR_NUMBER"] != "0" {
		t.Errorf("PR_NUMBER with zero value: got %q, want %q", got["PR_NUMBER"], "0")
	}
}

func TestExtractCheckRunVariables(t *testing.T) {
	cr := CheckRunData{
		Name:       "CI / build",
		Status:     "completed",
		Conclusion: "success",
		HTMLURL:    "https://github.com/org/repo/runs/999",
	}
	cr.Output.Title = "Build passed"
	cr.Output.Summary = "All checks passed."
	cr.Output.Text = "No errors found."
	cr.PullRequests = []struct {
		URL string `json:"url"`
	}{
		{URL: "https://api.github.com/repos/org/repo/pulls/42"},
	}

	got := ExtractCheckRunVariables(cr, "org/repo")

	cases := map[string]string{
		"CHECK_NAME":       "CI / build",
		"CHECK_STATUS":     "completed",
		"CHECK_CONCLUSION": "success",
		"CHECK_URL":        "https://github.com/org/repo/runs/999",
		"PR_URL":           "https://api.github.com/repos/org/repo/pulls/42",
		"REPO_FULL_NAME":   "org/repo",
	}

	for key, want := range cases {
		if got[key] != want {
			t.Errorf("CheckRun variable %s: got %q, want %q", key, got[key], want)
		}
	}

	wantOutput := "Build passed\nAll checks passed.\nNo errors found."
	if got["CHECK_OUTPUT"] != wantOutput {
		t.Errorf("CHECK_OUTPUT: got %q, want %q", got["CHECK_OUTPUT"], wantOutput)
	}

	if len(got) != 7 {
		t.Errorf("expected 7 CheckRun variables, got %d", len(got))
	}
}

func TestExtractCheckRunVariables_OutputTruncated(t *testing.T) {
	cr := CheckRunData{
		Name:       "long-output",
		Status:     "completed",
		Conclusion: "failure",
		HTMLURL:    "https://github.com/org/repo/runs/1",
	}
	// Build output that exceeds 4096 bytes
	cr.Output.Title = strings.Repeat("A", 1000)
	cr.Output.Summary = strings.Repeat("B", 2000)
	cr.Output.Text = strings.Repeat("C", 2000)

	got := ExtractCheckRunVariables(cr, "org/repo")

	if len(got["CHECK_OUTPUT"]) != maxCheckOutputLen {
		t.Errorf("CHECK_OUTPUT length: got %d, want %d", len(got["CHECK_OUTPUT"]), maxCheckOutputLen)
	}
}

func TestExtractCheckRunVariables_NoPRs(t *testing.T) {
	cr := CheckRunData{
		Name:       "lint",
		Status:     "completed",
		Conclusion: "success",
		HTMLURL:    "https://github.com/org/repo/runs/2",
	}

	got := ExtractCheckRunVariables(cr, "org/repo")

	if got["PR_URL"] != "" {
		t.Errorf("PR_URL with no pull requests: got %q, want empty string", got["PR_URL"])
	}
}

func TestExtractIssueVariables(t *testing.T) {
	issue := IssueData{
		Number:  7,
		Title:   "Bug report",
		Body:    "Something is broken.",
		HTMLURL: "https://github.com/org/repo/issues/7",
	}
	issue.User.Login = "reporter"
	issue.Labels = []struct {
		Name string `json:"name"`
	}{
		{Name: "bug"},
		{Name: "priority:high"},
	}

	got := ExtractIssueVariables(issue, "org/repo")

	cases := map[string]string{
		"ISSUE_URL":      "https://github.com/org/repo/issues/7",
		"ISSUE_TITLE":    "Bug report",
		"ISSUE_BODY":     "Something is broken.",
		"ISSUE_AUTHOR":   "reporter",
		"ISSUE_LABELS":   "bug,priority:high",
		"ISSUE_NUMBER":   "7",
		"REPO_FULL_NAME": "org/repo",
	}

	for key, want := range cases {
		if got[key] != want {
			t.Errorf("Issue variable %s: got %q, want %q", key, got[key], want)
		}
	}

	if len(got) != len(cases) {
		t.Errorf("expected %d Issue variables, got %d", len(cases), len(got))
	}
}

func TestExtractIssueVariables_NoLabels(t *testing.T) {
	issue := IssueData{
		Number:  1,
		Title:   "Feature request",
		HTMLURL: "https://github.com/org/repo/issues/1",
	}
	issue.User.Login = "user1"

	got := ExtractIssueVariables(issue, "org/repo")

	if got["ISSUE_LABELS"] != "" {
		t.Errorf("ISSUE_LABELS with no labels: got %q, want empty string", got["ISSUE_LABELS"])
	}
}

func TestExtractIssueVariables_EmptyBody(t *testing.T) {
	issue := IssueData{
		Number:  3,
		Title:   "No body issue",
		HTMLURL: "https://github.com/org/repo/issues/3",
	}
	issue.User.Login = "user2"

	got := ExtractIssueVariables(issue, "org/repo")

	if got["ISSUE_BODY"] != "" {
		t.Errorf("ISSUE_BODY with empty body: got %q, want empty string", got["ISSUE_BODY"])
	}
}
