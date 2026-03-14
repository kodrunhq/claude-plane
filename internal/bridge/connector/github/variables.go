package github

import (
	"strconv"
	"strings"
)

const maxCheckOutputLen = 4096

// PRData represents relevant fields from a GitHub Pull Request.
type PRData struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	DiffURL string `json:"diff_url"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// CheckRunData represents relevant fields from a GitHub Check Run.
type CheckRunData struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
	Output     struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Text    string `json:"text"`
	} `json:"output"`
	PullRequests []struct {
		URL string `json:"url"`
	} `json:"pull_requests"`
}

// IssueData represents relevant fields from a GitHub Issue.
type IssueData struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// ExtractPRVariables returns a map of template variables derived from a GitHub
// Pull Request payload. All 9 keys are always present; empty fields produce
// empty strings.
func ExtractPRVariables(pr PRData, repo string) map[string]string {
	return map[string]string{
		"PR_URL":         pr.HTMLURL,
		"PR_TITLE":       pr.Title,
		"PR_BODY":        pr.Body,
		"PR_AUTHOR":      pr.User.Login,
		"PR_BRANCH":      pr.Head.Ref,
		"PR_BASE":        pr.Base.Ref,
		"PR_NUMBER":      strconv.Itoa(pr.Number),
		"PR_DIFF_URL":    pr.DiffURL,
		"REPO_FULL_NAME": repo,
	}
}

// ExtractCheckRunVariables returns a map of template variables derived from a
// GitHub Check Run payload. All 7 keys are always present. CHECK_OUTPUT is
// truncated to maxCheckOutputLen bytes. PR_URL is the URL of the first
// associated pull request, or an empty string if none.
func ExtractCheckRunVariables(cr CheckRunData, repo string) map[string]string {
	prURL := ""
	if len(cr.PullRequests) > 0 {
		prURL = cr.PullRequests[0].URL
	}

	parts := []string{cr.Output.Title, cr.Output.Summary, cr.Output.Text}
	output := strings.Join(parts, "\n")
	if len(output) > maxCheckOutputLen {
		output = output[:maxCheckOutputLen]
	}

	return map[string]string{
		"CHECK_NAME":       cr.Name,
		"CHECK_STATUS":     cr.Status,
		"CHECK_CONCLUSION": cr.Conclusion,
		"CHECK_URL":        cr.HTMLURL,
		"CHECK_OUTPUT":     output,
		"PR_URL":           prURL,
		"REPO_FULL_NAME":   repo,
	}
}

// ExtractIssueVariables returns a map of template variables derived from a
// GitHub Issue payload. All 7 keys are always present. ISSUE_LABELS is a
// comma-separated list of label names, or an empty string when no labels exist.
func ExtractIssueVariables(issue IssueData, repo string) map[string]string {
	labelNames := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labelNames = append(labelNames, l.Name)
	}

	return map[string]string{
		"ISSUE_URL":      issue.HTMLURL,
		"ISSUE_TITLE":    issue.Title,
		"ISSUE_BODY":     issue.Body,
		"ISSUE_AUTHOR":   issue.User.Login,
		"ISSUE_LABELS":   strings.Join(labelNames, ","),
		"ISSUE_NUMBER":   strconv.Itoa(issue.Number),
		"REPO_FULL_NAME": repo,
	}
}
