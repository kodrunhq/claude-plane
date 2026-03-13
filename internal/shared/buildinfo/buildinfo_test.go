package buildinfo

import (
	"testing"
)

func TestString_DefaultValues(t *testing.T) {
	// Reset to known defaults before testing.
	origVersion, origCommit, origDate := Version, Commit, Date
	Version = "dev"
	Commit = "unknown"
	Date = "unknown"
	defer func() {
		Version = origVersion
		Commit = origCommit
		Date = origDate
	}()

	want := "claude-plane dev (unknown, unknown)"
	got := String()
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestString_CustomValues(t *testing.T) {
	origVersion, origCommit, origDate := Version, Commit, Date
	Version = "1.2.3"
	Commit = "abc1234"
	Date = "2026-03-13"
	defer func() {
		Version = origVersion
		Commit = origCommit
		Date = origDate
	}()

	want := "claude-plane 1.2.3 (abc1234, 2026-03-13)"
	got := String()
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
