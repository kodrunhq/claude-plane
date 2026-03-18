package cliutil_test

import (
	"slices"
	"testing"

	"github.com/kodrunhq/claude-plane/internal/shared/cliutil"
)

func TestStripFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		flag string
		want []string
	}{
		{
			name: "removes single occurrence",
			args: []string{"--dangerously-skip-permissions", "--verbose"},
			flag: "--dangerously-skip-permissions",
			want: []string{"--verbose"},
		},
		{
			name: "removes multiple occurrences",
			args: []string{"--foo", "--bar", "--foo"},
			flag: "--foo",
			want: []string{"--bar"},
		},
		{
			name: "no match leaves args unchanged",
			args: []string{"--verbose", "--model", "opus"},
			flag: "--dangerously-skip-permissions",
			want: []string{"--verbose", "--model", "opus"},
		},
		{
			name: "empty args returns empty slice",
			args: []string{},
			flag: "--foo",
			want: []string{},
		},
		{
			name: "all args removed",
			args: []string{"--foo", "--foo"},
			flag: "--foo",
			want: []string{},
		},
		{
			name: "nil args returns empty slice",
			args: nil,
			flag: "--foo",
			want: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cliutil.StripFlag(tc.args, tc.flag)
			if !slices.Equal(got, tc.want) {
				t.Errorf("StripFlag(%v, %q) = %v, want %v", tc.args, tc.flag, got, tc.want)
			}
		})
	}
}

func TestStripFlagWithValue(t *testing.T) {
	tests := []struct {
		name string
		args []string
		flag string
		want []string
	}{
		{
			name: "removes flag and its value",
			args: []string{"--model", "opus", "--verbose"},
			flag: "--model",
			want: []string{"--verbose"},
		},
		{
			name: "flag at end without value is removed",
			args: []string{"--verbose", "--model"},
			flag: "--model",
			want: []string{"--verbose"},
		},
		{
			name: "no match leaves args unchanged",
			args: []string{"--verbose", "--output", "file.txt"},
			flag: "--model",
			want: []string{"--verbose", "--output", "file.txt"},
		},
		{
			name: "empty args returns empty slice",
			args: []string{},
			flag: "--model",
			want: []string{},
		},
		{
			name: "only flag and value",
			args: []string{"--model", "sonnet"},
			flag: "--model",
			want: []string{},
		},
		{
			name: "nil args returns empty slice",
			args: nil,
			flag: "--model",
			want: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cliutil.StripFlagWithValue(tc.args, tc.flag)
			if !slices.Equal(got, tc.want) {
				t.Errorf("StripFlagWithValue(%v, %q) = %v, want %v", tc.args, tc.flag, got, tc.want)
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name     string
		snapshot string
		want     []string
	}{
		{
			name:     "valid JSON array",
			snapshot: `["--model","opus","--verbose"]`,
			want:     []string{"--model", "opus", "--verbose"},
		},
		{
			name:     "empty string returns empty slice",
			snapshot: "",
			want:     []string{},
		},
		{
			name:     "whitespace-only string returns empty slice",
			snapshot: "   ",
			want:     []string{},
		},
		{
			name:     "invalid JSON returns empty slice",
			snapshot: "not-json",
			want:     []string{},
		},
		{
			name:     "empty JSON array",
			snapshot: "[]",
			want:     []string{},
		},
		{
			name:     "single element array",
			snapshot: `["--dangerously-skip-permissions"]`,
			want:     []string{"--dangerously-skip-permissions"},
		},
		{
			name:     "JSON object instead of array returns empty slice",
			snapshot: `{"key":"value"}`,
			want:     []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cliutil.ParseArgs(tc.snapshot)
			if got == nil {
				got = []string{}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("ParseArgs(%q) = %v, want %v", tc.snapshot, got, tc.want)
			}
		})
	}
}
