package agent

import (
	"strings"
	"testing"
)

func TestParseTaskValues_SingleValue(t *testing.T) {
	data := `some output
%%TASK_VALUE key=result%%
hello world
%%END_TASK_VALUE%%
more output`

	vals := ParseTaskValues(data)
	if vals == nil {
		t.Fatal("expected non-nil result")
	}
	if got := vals["result"]; got != "hello world\n" {
		t.Errorf("result = %q, want %q", got, "hello world\n")
	}
}

func TestParseTaskValues_MultipleValues(t *testing.T) {
	data := `%%TASK_VALUE key=alpha%%value1%%END_TASK_VALUE%%` +
		`%%TASK_VALUE key=beta%%value2%%END_TASK_VALUE%%`

	vals := ParseTaskValues(data)
	if vals == nil {
		t.Fatal("expected non-nil result")
	}
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
	if vals["alpha"] != "value1" {
		t.Errorf("alpha = %q, want %q", vals["alpha"], "value1")
	}
	if vals["beta"] != "value2" {
		t.Errorf("beta = %q, want %q", vals["beta"], "value2")
	}
}

func TestParseTaskValues_NoValues(t *testing.T) {
	data := "just plain output, no markers"
	vals := ParseTaskValues(data)
	if vals != nil {
		t.Errorf("expected nil, got %v", vals)
	}
}

func TestParseTaskValues_InvalidKey(t *testing.T) {
	// Keys starting with a digit or containing special chars should not match.
	data := `%%TASK_VALUE key=1bad%%value%%END_TASK_VALUE%%` +
		`%%TASK_VALUE key=has-dash%%value%%END_TASK_VALUE%%`
	vals := ParseTaskValues(data)
	if vals != nil {
		t.Errorf("expected nil for invalid keys, got %v", vals)
	}
}

func TestParseTaskValues_TruncateLargeValue(t *testing.T) {
	largeValue := strings.Repeat("x", 40*1024) // 40 KB
	data := "%%TASK_VALUE key=big%%" + largeValue + "%%END_TASK_VALUE%%"

	vals := ParseTaskValues(data)
	if vals == nil {
		t.Fatal("expected non-nil result")
	}
	if len(vals["big"]) != maxTaskValueSize {
		t.Errorf("value length = %d, want %d", len(vals["big"]), maxTaskValueSize)
	}
}

func TestParseTaskValues_MaxValues(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 25; i++ {
		b.WriteString("%%TASK_VALUE key=k")
		b.WriteByte(byte('a' + i))
		b.WriteString("%%v%%END_TASK_VALUE%%")
	}

	vals := ParseTaskValues(b.String())
	if vals == nil {
		t.Fatal("expected non-nil result")
	}
	if len(vals) != maxTaskValueCount {
		t.Errorf("count = %d, want %d", len(vals), maxTaskValueCount)
	}
}

func TestParseTaskValues_NewlineAfterMarker(t *testing.T) {
	// The regex optionally consumes a newline after the opening marker.
	data := "%%TASK_VALUE key=msg%%hello%%END_TASK_VALUE%%"
	vals := ParseTaskValues(data)
	if vals == nil {
		t.Fatal("expected non-nil result")
	}
	if vals["msg"] != "hello" {
		t.Errorf("msg = %q, want %q", vals["msg"], "hello")
	}
}

func TestParseTaskValues_DuplicateKeysLastWins(t *testing.T) {
	data := `%%TASK_VALUE key=dup%%first%%END_TASK_VALUE%%` +
		`%%TASK_VALUE key=dup%%second%%END_TASK_VALUE%%`

	vals := ParseTaskValues(data)
	if vals == nil {
		t.Fatal("expected non-nil result")
	}
	if vals["dup"] != "second" {
		t.Errorf("dup = %q, want %q", vals["dup"], "second")
	}
}
