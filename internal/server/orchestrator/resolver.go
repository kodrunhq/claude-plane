package orchestrator

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// JobMeta holds job-level metadata available for template resolution.
type JobMeta struct {
	Name        string
	RunID       string
	RunNumber   int
	StartTime   string
	TriggerType string
}

// StepResult captures the outcome of a completed step for template resolution.
type StepResult struct {
	Status   string
	ExitCode int
}

// ResolveContext holds all data needed to resolve template references in a step's
// prompt or parameters at execution time.
type ResolveContext struct {
	RunParams   map[string]string
	JobMeta     JobMeta
	StepValues  map[string]map[string]string // stepName -> key -> value
	StepResults map[string]StepResult        // stepName -> result
}

var (
	// shorthandPattern matches ${VAR} parameter references.
	shorthandPattern = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
	// templatePattern matches {{...}} references for job metadata and step values.
	templatePattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)
)

// ResolveReferences performs two-phase template resolution on a string:
//  1. ${VAR} shorthand is replaced from runParams.
//  2. {{...}} references are resolved against job metadata and step values/results.
//
// Unresolved references are left as-is.
func ResolveReferences(template string, runParams map[string]string, meta JobMeta, stepValues map[string]map[string]string, stepResults map[string]StepResult) string {
	if template == "" {
		return ""
	}

	// Phase 1: ${VAR} shorthand from run parameters
	result := shorthandPattern.ReplaceAllStringFunc(template, func(match string) string {
		groups := shorthandPattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		key := groups[1]
		if val, ok := runParams[key]; ok {
			return val
		}
		return match
	})

	// Phase 2: {{...}} references
	result = templatePattern.ReplaceAllStringFunc(result, func(match string) string {
		groups := templatePattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		ref := strings.TrimSpace(groups[1])

		if strings.HasPrefix(ref, "job.") {
			return resolveJobRef(ref, meta)
		}
		if strings.HasPrefix(ref, "steps.") {
			return resolveStepRef(ref, stepValues, stepResults)
		}

		return match
	})

	return result
}

// resolveJobRef resolves a {{job.*}} reference against job metadata.
func resolveJobRef(ref string, meta JobMeta) string {
	switch ref {
	case "job.name":
		return meta.Name
	case "job.run_id":
		return meta.RunID
	case "job.run_number":
		return fmt.Sprintf("%d", meta.RunNumber)
	case "job.start_time":
		return meta.StartTime
	case "job.trigger_type":
		return meta.TriggerType
	default:
		return "{{" + ref + "}}"
	}
}

// resolveStepRef resolves a {{steps.<name>.<field>}} reference against step
// values and results. Supported fields:
//
//	steps.<name>.values.<key> — task value from a completed step
//	steps.<name>.status       — completion status
//	steps.<name>.exit_code    — exit code
func resolveStepRef(ref string, stepValues map[string]map[string]string, stepResults map[string]StepResult) string {
	// Expected format: steps.<name>.<field>[.<subfield>]
	parts := strings.SplitN(ref, ".", 4)
	if len(parts) < 3 {
		return "{{" + ref + "}}"
	}

	stepName := parts[1]
	field := parts[2]

	switch field {
	case "values":
		if len(parts) < 4 {
			return "{{" + ref + "}}"
		}
		key := parts[3]
		if vals, ok := stepValues[stepName]; ok {
			if v, ok := vals[key]; ok {
				return v
			}
		}
		return "{{" + ref + "}}"

	case "status":
		if result, ok := stepResults[stepName]; ok {
			return result.Status
		}
		return "{{" + ref + "}}"

	case "exit_code":
		if result, ok := stepResults[stepName]; ok {
			return fmt.Sprintf("%d", result.ExitCode)
		}
		return "{{" + ref + "}}"

	default:
		return "{{" + ref + "}}"
	}
}

// resolveParameters merges job default parameters (JSON) with runtime overrides.
// Overrides take precedence. Invalid JSON in jobDefaultsJSON is logged and ignored.
func resolveParameters(jobDefaultsJSON string, overrides map[string]string) map[string]string {
	defaults := make(map[string]string)
	if jobDefaultsJSON != "" {
		if err := json.Unmarshal([]byte(jobDefaultsJSON), &defaults); err != nil {
			slog.Warn("invalid job parameters JSON", "error", err)
		}
	}
	for k, v := range overrides {
		defaults[k] = v
	}
	return defaults
}
