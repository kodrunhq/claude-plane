package github

import (
	"testing"
)

func TestFilters_Match_Branches(t *testing.T) {
	t.Run("passes when base branch is in filter list", func(t *testing.T) {
		f := &Filters{Branches: []string{"main"}}
		event := EventData{BaseBranch: "main"}
		if !f.Match(event) {
			t.Error("expected match for branch 'main' in filter ['main']")
		}
	})

	t.Run("fails when base branch is not in filter list", func(t *testing.T) {
		f := &Filters{Branches: []string{"main"}}
		event := EventData{BaseBranch: "develop"}
		if f.Match(event) {
			t.Error("expected no match for branch 'develop' in filter ['main']")
		}
	})

	t.Run("passes when branches filter is empty", func(t *testing.T) {
		f := &Filters{Branches: []string{}}
		event := EventData{BaseBranch: "any-branch"}
		if !f.Match(event) {
			t.Error("expected match when branches filter is empty")
		}
	})
}

func TestFilters_Match_Labels(t *testing.T) {
	t.Run("passes when at least one event label matches filter", func(t *testing.T) {
		f := &Filters{Labels: []string{"claude-review"}}
		event := EventData{Labels: []string{"bug", "claude-review"}}
		if !f.Match(event) {
			t.Error("expected match for label 'claude-review'")
		}
	})

	t.Run("fails when no event label matches filter", func(t *testing.T) {
		f := &Filters{Labels: []string{"claude-review"}}
		event := EventData{Labels: []string{"bug", "enhancement"}}
		if f.Match(event) {
			t.Error("expected no match when no labels intersect")
		}
	})

	t.Run("fails when event has no labels and filter is non-empty", func(t *testing.T) {
		f := &Filters{Labels: []string{"claude-review"}}
		event := EventData{Labels: []string{}}
		if f.Match(event) {
			t.Error("expected no match when event has no labels")
		}
	})

	t.Run("passes when labels filter is empty", func(t *testing.T) {
		f := &Filters{Labels: []string{}}
		event := EventData{Labels: []string{"any-label"}}
		if !f.Match(event) {
			t.Error("expected match when labels filter is empty")
		}
	})
}

func TestFilters_Match_CheckNames(t *testing.T) {
	t.Run("passes when check name matches filter", func(t *testing.T) {
		f := &Filters{CheckNames: []string{"CI / test"}}
		event := EventData{CheckName: "CI / test"}
		if !f.Match(event) {
			t.Error("expected match for check name 'CI / test'")
		}
	})

	t.Run("fails when check name does not match filter", func(t *testing.T) {
		f := &Filters{CheckNames: []string{"CI / test"}}
		event := EventData{CheckName: "CI / deploy"}
		if f.Match(event) {
			t.Error("expected no match for check name 'CI / deploy' in filter ['CI / test']")
		}
	})

	t.Run("passes when check names filter is empty", func(t *testing.T) {
		f := &Filters{CheckNames: []string{}}
		event := EventData{CheckName: "any-check"}
		if !f.Match(event) {
			t.Error("expected match when check names filter is empty")
		}
	})
}

func TestFilters_Match_Conclusions(t *testing.T) {
	t.Run("passes when conclusion matches filter", func(t *testing.T) {
		f := &Filters{Conclusions: []string{"failure", "timed_out"}}
		event := EventData{Conclusion: "failure"}
		if !f.Match(event) {
			t.Error("expected match for conclusion 'failure'")
		}
	})

	t.Run("passes when conclusion matches second item in filter", func(t *testing.T) {
		f := &Filters{Conclusions: []string{"failure", "timed_out"}}
		event := EventData{Conclusion: "timed_out"}
		if !f.Match(event) {
			t.Error("expected match for conclusion 'timed_out'")
		}
	})

	t.Run("fails when conclusion is not in filter list", func(t *testing.T) {
		f := &Filters{Conclusions: []string{"failure", "timed_out"}}
		event := EventData{Conclusion: "success"}
		if f.Match(event) {
			t.Error("expected no match for conclusion 'success' in filter ['failure', 'timed_out']")
		}
	})

	t.Run("passes when conclusions filter is empty", func(t *testing.T) {
		f := &Filters{Conclusions: []string{}}
		event := EventData{Conclusion: "any-conclusion"}
		if !f.Match(event) {
			t.Error("expected match when conclusions filter is empty")
		}
	})
}

func TestFilters_Match_Paths(t *testing.T) {
	t.Run("passes when changed file matches glob pattern with **", func(t *testing.T) {
		f := &Filters{Paths: []string{"src/**"}}
		event := EventData{ChangedFiles: []string{"src/main.go"}}
		if !f.Match(event) {
			t.Error("expected match for 'src/main.go' against pattern 'src/**'")
		}
	})

	t.Run("fails when changed file does not match glob pattern with **", func(t *testing.T) {
		f := &Filters{Paths: []string{"src/**"}}
		event := EventData{ChangedFiles: []string{"docs/readme.md"}}
		if f.Match(event) {
			t.Error("expected no match for 'docs/readme.md' against pattern 'src/**'")
		}
	})

	t.Run("passes when changed file matches single-segment glob", func(t *testing.T) {
		f := &Filters{Paths: []string{"src/*.go"}}
		event := EventData{ChangedFiles: []string{"src/main.go"}}
		if !f.Match(event) {
			t.Error("expected match for 'src/main.go' against pattern 'src/*.go'")
		}
	})

	t.Run("fails when changed file path crosses separator with single *", func(t *testing.T) {
		f := &Filters{Paths: []string{"src/*.go"}}
		event := EventData{ChangedFiles: []string{"src/sub/file.go"}}
		if f.Match(event) {
			t.Error("expected no match for 'src/sub/file.go' against pattern 'src/*.go' (filepath.Match does not cross /)")
		}
	})

	t.Run("passes when at least one changed file matches", func(t *testing.T) {
		f := &Filters{Paths: []string{"src/**"}}
		event := EventData{ChangedFiles: []string{"docs/readme.md", "src/main.go"}}
		if !f.Match(event) {
			t.Error("expected match when at least one changed file matches pattern")
		}
	})

	t.Run("passes when paths filter is empty", func(t *testing.T) {
		f := &Filters{Paths: []string{}}
		event := EventData{ChangedFiles: []string{"any/file.go"}}
		if !f.Match(event) {
			t.Error("expected match when paths filter is empty")
		}
	})
}

func TestFilters_Match_AuthorsIgnore(t *testing.T) {
	t.Run("fails when author is in ignore list", func(t *testing.T) {
		f := &Filters{AuthorsIgnore: []string{"dependabot[bot]"}}
		event := EventData{Author: "dependabot[bot]"}
		if f.Match(event) {
			t.Error("expected no match for author 'dependabot[bot]' in ignore list")
		}
	})

	t.Run("passes when author is not in ignore list", func(t *testing.T) {
		f := &Filters{AuthorsIgnore: []string{"dependabot[bot]"}}
		event := EventData{Author: "jose"}
		if !f.Match(event) {
			t.Error("expected match for author 'jose' not in ignore list")
		}
	})

	t.Run("passes when authors_ignore filter is empty", func(t *testing.T) {
		f := &Filters{AuthorsIgnore: []string{}}
		event := EventData{Author: "anyone"}
		if !f.Match(event) {
			t.Error("expected match when authors_ignore filter is empty")
		}
	})
}

func TestFilters_Match_AndCombination(t *testing.T) {
	t.Run("passes when all filters match", func(t *testing.T) {
		f := &Filters{
			Branches:    []string{"main"},
			Labels:      []string{"claude-review"},
			Conclusions: []string{"failure"},
		}
		event := EventData{
			BaseBranch: "main",
			Labels:     []string{"claude-review"},
			Conclusion: "failure",
		}
		if !f.Match(event) {
			t.Error("expected match when all filters pass")
		}
	})

	t.Run("fails when one filter does not match", func(t *testing.T) {
		f := &Filters{
			Branches: []string{"main"},
			Labels:   []string{"claude-review"},
		}
		event := EventData{
			BaseBranch: "main",
			Labels:     []string{"bug"}, // does not match
		}
		if f.Match(event) {
			t.Error("expected no match when labels filter fails")
		}
	})

	t.Run("fails when authors_ignore blocks even though other filters pass", func(t *testing.T) {
		f := &Filters{
			Branches:      []string{"main"},
			AuthorsIgnore: []string{"dependabot[bot]"},
		}
		event := EventData{
			BaseBranch: "main",
			Author:     "dependabot[bot]",
		}
		if f.Match(event) {
			t.Error("expected no match when author is in ignore list")
		}
	})
}

func TestFilters_Match_AllEmpty(t *testing.T) {
	t.Run("passes when all filter slices are empty", func(t *testing.T) {
		f := &Filters{}
		event := EventData{
			BaseBranch:   "any-branch",
			Labels:       []string{"any-label"},
			CheckName:    "any-check",
			Conclusion:   "any-conclusion",
			ChangedFiles: []string{"any/file.go"},
			Author:       "anyone",
		}
		if !f.Match(event) {
			t.Error("expected match when all filter slices are empty (no filtering)")
		}
	})
}
