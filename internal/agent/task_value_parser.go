package agent

import "regexp"

const (
	maxTaskValueSize     = 32 * 1024 // 32 KB
	maxTaskValueCount    = 20
)

// taskValuePattern matches %%TASK_VALUE key=<key>%%<value>%%END_TASK_VALUE%% markers
// in session output. Keys must start with a letter and contain only alphanumerics
// and underscores.
var taskValuePattern = regexp.MustCompile(`%%TASK_VALUE key=([a-zA-Z][a-zA-Z0-9_]*)%%\n?([\s\S]*?)%%END_TASK_VALUE%%`)

// ParseTaskValues extracts task value markers from raw session output.
// Returns a map of key → value. Values exceeding 32 KB are truncated.
// At most 20 values are extracted; additional matches are ignored.
func ParseTaskValues(data string) map[string]string {
	matches := taskValuePattern.FindAllStringSubmatch(data, -1)
	if len(matches) == 0 {
		return nil
	}

	result := make(map[string]string, len(matches))
	count := 0
	for _, m := range matches {
		if count >= maxTaskValueCount {
			break
		}
		key := m[1]
		value := m[2]
		if len(value) > maxTaskValueSize {
			value = value[:maxTaskValueSize]
		}
		result[key] = value
		count++
	}
	return result
}
