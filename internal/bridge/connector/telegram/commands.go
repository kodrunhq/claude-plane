package telegram

import (
	"fmt"
	"strings"
)

// validCommands is the set of recognised command names.
var validCommands = map[string]bool{
	"start":    true,
	"list":     true,
	"status":   true,
	"kill":     true,
	"inject":   true,
	"machines": true,
	"help":     true,
}

// Command is a parsed Telegram slash command.
type Command struct {
	// Name is the command name without the leading slash.
	Name string
	// Args holds positional arguments from the command text (left of the | delimiter).
	Args []string
	// Vars holds key=value variable pairs parsed from the right of the | delimiter.
	Vars map[string]string
}

// ParseCommand parses a Telegram message text into a Command.
//
// The expected format is:
//
//	/<name> [args…] [| VAR1=val1 VAR2=val2]
//
// Rules:
//   - The first non-space word must start with "/".
//   - The command name (after stripping "/") must be in the known command set.
//   - If a "|" separator is present, everything to the right is parsed as VAR=val pairs.
//   - Each variable token must contain at least one "=" (values may contain "=").
func ParseCommand(text string) (*Command, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty command text")
	}

	// Split into command+args part and optional variables part.
	var cmdPart, varPart string
	if idx := strings.Index(text, "|"); idx >= 0 {
		cmdPart = strings.TrimSpace(text[:idx])
		varPart = strings.TrimSpace(text[idx+1:])
	} else {
		cmdPart = text
	}

	tokens := strings.Fields(cmdPart)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty command text")
	}

	first := tokens[0]
	if !strings.HasPrefix(first, "/") {
		return nil, fmt.Errorf("command must start with '/': got %q", first)
	}

	name := strings.TrimPrefix(first, "/")
	if !validCommands[name] {
		return nil, fmt.Errorf("unknown command %q", name)
	}

	args := tokens[1:]

	vars := map[string]string{}
	if varPart != "" {
		varTokens := strings.Fields(varPart)
		for _, tok := range varTokens {
			eqIdx := strings.Index(tok, "=")
			if eqIdx < 0 {
				return nil, fmt.Errorf("malformed variable %q: expected KEY=VALUE", tok)
			}
			k := tok[:eqIdx]
			v := tok[eqIdx+1:]
			vars[k] = v
		}
	}

	return &Command{
		Name: name,
		Args: args,
		Vars: vars,
	}, nil
}
