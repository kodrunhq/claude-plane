package grpc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAsciicastData(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "header + two events",
			raw: `{"version":2,"width":120,"height":40,"timestamp":1700000000}
[0.1,"o","hello "]
[0.5,"o","world"]
`,
			want: "hello world",
		},
		{
			name: "header only",
			raw:  `{"version":2,"width":80,"height":24,"timestamp":1700000000}` + "\n",
			want: "",
		},
		{
			name: "empty input",
			raw:  "",
			want: "",
		},
		{
			name: "multiple events concatenated",
			raw: `{"version":2,"width":80,"height":24}
[0.1,"o","visible"]
[0.2,"o"," text"]
`,
			want: "visible text",
		},
		{
			name: "malformed lines are skipped",
			raw: `{"version":2,"width":80,"height":24}
not-json-at-all
[0.1,"o","good"]
[broken
[0.5,"o"," data"]
`,
			want: "good data",
		},
		{
			name: "escape sequences preserved",
			raw: `{"version":2,"width":80,"height":24}
[0.1,"o","\u001b[32mgreen\u001b[0m"]
`,
			want: "\x1b[32mgreen\x1b[0m",
		},
		{
			name: "long line near scanner limit",
			raw:  buildLongLineInput(100_000),
			want: strings.Repeat("A", 100_000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(parseAsciicastData([]byte(tt.raw)))
			if got != tt.want {
				if len(got) > 200 {
					t.Errorf("got %d bytes, want %d bytes", len(got), len(tt.want))
				} else {
					t.Errorf("got %q, want %q", got, tt.want)
				}
			}
		})
	}
}

// buildLongLineInput creates an asciicast with one event whose data is n bytes.
func buildLongLineInput(n int) string {
	data := strings.Repeat("A", n)
	entry, _ := json.Marshal([]interface{}{0.1, "o", data})
	return `{"version":2,"width":80,"height":24}` + "\n" + string(entry) + "\n"
}
