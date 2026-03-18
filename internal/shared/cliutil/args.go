// Package cliutil provides shared helpers for parsing and manipulating
// CLI argument slices, used by the executor and session handler.
package cliutil

import (
	"encoding/json"
	"strings"
)

// StripFlag removes all occurrences of a standalone flag from args.
func StripFlag(args []string, flag string) []string {
	result := make([]string, 0, len(args))
	for _, a := range args {
		if a != flag {
			result = append(result, a)
		}
	}
	return result
}

// StripFlagWithValue removes a flag and its following value from args
// (e.g., --model opus).
func StripFlagWithValue(args []string, flag string) []string {
	result := make([]string, 0, len(args))
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == flag {
			skip = true
			continue
		}
		result = append(result, a)
	}
	return result
}

// ParseArgs parses a JSON-encoded array string into a []string.
// Returns an empty slice on empty input or any parse error.
func ParseArgs(argsSnapshot string) []string {
	trimmed := strings.TrimSpace(argsSnapshot)
	if trimmed == "" {
		return []string{}
	}
	var args []string
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return []string{}
	}
	if args == nil {
		return []string{}
	}
	return args
}
