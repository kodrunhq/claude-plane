package github

import (
	"strconv"
	"strings"
)

const maxCheckOutputLen = 4096

// maxBodyLen caps user-controlled text fields (comment bodies, review bodies,
// release notes) to mitigate prompt-injection risk when these values flow into
// Claude session prompts.
const maxBodyLen = 4096

// truncateSuffix is appended to truncated body text.
const truncateSuffix = "... [truncated]"

// truncateBody returns s truncated to maxBodyLen runes if it exceeds the limit.
// The returned string never exceeds maxBodyLen runes including the suffix.
func truncateBody(s string) string {
	runes := []rune(s)
	if len(runes) <= maxBodyLen {
		return s
	}
	suffixRunes := []rune(truncateSuffix)
	cutAt := maxBodyLen - len(suffixRunes)
	if cutAt < 0 {
		cutAt = 0
	}
	return string(runes[:cutAt]) + truncateSuffix
}

// PRData represents relevant fields from a GitHub Pull Request.
type PRData struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	DiffURL   string `json:"diff_url"`
	UpdatedAt string `json:"updated_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
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
	ID         int64  `json:"id"`
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
// PullRequest is non-nil when the issue is actually a pull request (GitHub
// returns PRs on the issues list endpoint). Consumers should skip such entries.
type IssueData struct {
	Number      int         `json:"number"`
	Title       string      `json:"title"`
	Body        string      `json:"body"`
	HTMLURL     string      `json:"html_url"`
	PullRequest interface{} `json:"pull_request"`
	User        struct {
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
		"PR_BODY":        truncateBody(pr.Body),
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

// IssueCommentData represents a comment from the GitHub issue comments endpoint.
type IssueCommentData struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	UpdatedAt string `json:"updated_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	IssueURL string `json:"issue_url"`
}

// PRCommentData represents a review comment from the GitHub PR comments endpoint.
type PRCommentData struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	UpdatedAt string `json:"updated_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	PullRequestURL string `json:"pull_request_url"`
}

// PRReviewData represents a review from the GitHub PR reviews endpoint.
type PRReviewData struct {
	ID          int64  `json:"id"`
	State       string `json:"state"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	SubmittedAt string `json:"submitted_at"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
}

// ReleaseData represents a release from the GitHub releases endpoint.
type ReleaseData struct {
	ID      int64  `json:"id"`
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Author  struct {
		Login string `json:"login"`
	} `json:"author"`
	PublishedAt string `json:"published_at"`
}

// ExtractIssueCommentVariables returns template variables for an issue comment.
func ExtractIssueCommentVariables(comment IssueCommentData, repo string, issueNumber int, issueTitle, issueURL string) map[string]string {
	return map[string]string{
		"COMMENT_BODY":   truncateBody(comment.Body),
		"COMMENT_AUTHOR": comment.User.Login,
		"COMMENT_URL":    comment.HTMLURL,
		"ISSUE_NUMBER":   strconv.Itoa(issueNumber),
		"ISSUE_TITLE":    issueTitle,
		"ISSUE_URL":      issueURL,
		"REPO_FULL_NAME": repo,
	}
}

// ExtractPRCommentVariables returns template variables for a PR review comment.
func ExtractPRCommentVariables(comment PRCommentData, repo string, prNumber int, prTitle, prURL string) map[string]string {
	return map[string]string{
		"COMMENT_BODY":   truncateBody(comment.Body),
		"COMMENT_AUTHOR": comment.User.Login,
		"COMMENT_URL":    comment.HTMLURL,
		"PR_NUMBER":      strconv.Itoa(prNumber),
		"PR_TITLE":       prTitle,
		"PR_URL":         prURL,
		"REPO_FULL_NAME": repo,
	}
}

// ExtractPRReviewVariables returns template variables for a PR review.
func ExtractPRReviewVariables(review PRReviewData, repo string, prNumber int, prTitle, prURL string) map[string]string {
	return map[string]string{
		"REVIEW_STATE":   strings.ToLower(review.State),
		"REVIEW_AUTHOR":  review.User.Login,
		"REVIEW_BODY":    truncateBody(review.Body),
		"REVIEW_URL":     review.HTMLURL,
		"PR_NUMBER":      strconv.Itoa(prNumber),
		"PR_TITLE":       prTitle,
		"PR_URL":         prURL,
		"REPO_FULL_NAME": repo,
	}
}

// ExtractReleaseVariables returns template variables for a release.
func ExtractReleaseVariables(release ReleaseData, repo string) map[string]string {
	return map[string]string{
		"RELEASE_TAG":    release.TagName,
		"RELEASE_NAME":   release.Name,
		"RELEASE_BODY":   truncateBody(release.Body),
		"RELEASE_URL":    release.HTMLURL,
		"RELEASE_AUTHOR": release.Author.Login,
		"REPO_FULL_NAME": repo,
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
