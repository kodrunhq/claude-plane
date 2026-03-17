// Command generate-event-types parses event type constants from
// internal/server/event/event.go and writes them to event_types.json.
// This keeps the frontend TypeScript constants in sync with Go via CI.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// EventTypeEntry represents a single event type constant.
type EventTypeEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func main() {
	root, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error finding project root: %v\n", err)
		os.Exit(1)
	}

	srcPath := filepath.Join(root, "internal", "server", "event", "event.go")
	entries, err := extractEventTypes(srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error extracting event types: %v\n", err)
		os.Exit(1)
	}

	outPath := filepath.Join(root, "internal", "server", "event", "event_types.json")
	if err := writeJSON(outPath, entries); err != nil {
		fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %d event types to %s\n", len(entries), outPath)
}

// findProjectRoot walks up from the current working directory until it finds go.mod.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in any parent of %s", dir)
		}
		dir = parent
	}
}

// extractEventTypes parses the Go source file and returns all const
// declarations whose names start with "Type" and have string literal values.
func extractEventTypes(path string) ([]EventTypeEntry, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	var entries []EventTypeEntry

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for i, name := range valueSpec.Names {
				if !strings.HasPrefix(name.Name, "Type") {
					continue
				}
				if i >= len(valueSpec.Values) {
					continue
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				entries = append(entries, EventTypeEntry{
					Name:  name.Name,
					Value: val,
				})
			}
		}
	}

	return entries, nil
}

// writeJSON marshals entries to indented JSON and writes to path.
func writeJSON(path string, entries []EventTypeEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}
