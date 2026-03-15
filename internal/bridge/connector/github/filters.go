package github

import (
	"path"
	"strings"
)

// Filters holds AND-combined filter criteria for a trigger.
// Empty slices are treated as "no filter" (pass all).
type Filters struct {
	Branches      []string `json:"branches"`
	Labels        []string `json:"labels"`
	CheckNames    []string `json:"check_names"`
	Conclusions   []string `json:"conclusions"`
	Paths         []string `json:"paths"`
	AuthorsIgnore []string `json:"authors_ignore"`
	ReviewStates  []string `json:"review_states,omitempty"`
	TagPatterns   []string `json:"tag_patterns,omitempty"`
}

// EventData carries the fields each filter checks against.
type EventData struct {
	BaseBranch   string
	Labels       []string
	CheckName    string
	Conclusion   string
	ChangedFiles []string
	Author       string
	ReviewState  string
	Tag          string
}

// Match evaluates all configured filters (AND-combined).
// Empty slices are treated as "no filter" (pass all).
func (f *Filters) Match(event EventData) bool {
	if !matchBranches(f.Branches, event.BaseBranch) {
		return false
	}
	if !matchLabels(f.Labels, event.Labels) {
		return false
	}
	if !matchStringInList(f.CheckNames, event.CheckName) {
		return false
	}
	if !matchStringInList(f.Conclusions, event.Conclusion) {
		return false
	}
	if !matchPaths(f.Paths, event.ChangedFiles) {
		return false
	}
	if !matchAuthorsIgnore(f.AuthorsIgnore, event.Author) {
		return false
	}
	if !matchReviewStates(f.ReviewStates, event.ReviewState) {
		return false
	}
	if !matchTagPatterns(f.TagPatterns, event.Tag) {
		return false
	}
	return true
}

// matchBranches returns true if filter is empty or baseBranch is in the list.
func matchBranches(filter []string, baseBranch string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, b := range filter {
		if b == baseBranch {
			return true
		}
	}
	return false
}

// matchLabels returns true if filter is empty or at least one event label is in the filter list.
func matchLabels(filter []string, eventLabels []string) bool {
	if len(filter) == 0 {
		return true
	}
	filterSet := make(map[string]struct{}, len(filter))
	for _, l := range filter {
		filterSet[l] = struct{}{}
	}
	for _, el := range eventLabels {
		if _, ok := filterSet[el]; ok {
			return true
		}
	}
	return false
}

// matchStringInList returns true if filter is empty or value is in the list.
func matchStringInList(filter []string, value string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, item := range filter {
		if item == value {
			return true
		}
	}
	return false
}

// matchPaths returns true if filter is empty or at least one changed file matches
// at least one glob pattern in the filter.
func matchPaths(filter []string, changedFiles []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, file := range changedFiles {
		for _, pattern := range filter {
			if matchPath(pattern, file) {
				return true
			}
		}
	}
	return false
}

// matchPath checks whether filePath matches the given glob pattern.
// Patterns ending in "/**" match any file under the specified directory.
// All other patterns use path.Match standard glob semantics (does not cross /).
func matchPath(pattern, filePath string) bool {
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(filePath, prefix+"/")
	}
	matched, _ := path.Match(pattern, filePath)
	return matched
}

// matchAuthorsIgnore returns true if filter is empty or author is NOT in the ignore list.
func matchAuthorsIgnore(filter []string, author string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, ignored := range filter {
		if ignored == author {
			return false
		}
	}
	return true
}

// matchReviewStates returns true if filter is empty or state is in the list.
func matchReviewStates(states []string, state string) bool {
	if len(states) == 0 {
		return true
	}
	for _, s := range states {
		if s == state {
			return true
		}
	}
	return false
}

// matchTagPatterns returns true if filter is empty or tag matches at least one
// glob pattern. Uses path.Match semantics.
func matchTagPatterns(patterns []string, tag string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		matched, _ := path.Match(p, tag)
		if matched {
			return true
		}
	}
	return false
}
