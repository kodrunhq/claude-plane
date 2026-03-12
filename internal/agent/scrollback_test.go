package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScrollbackWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	w, err := NewScrollbackWriter(path, 80, 24)
	if err != nil {
		t.Fatalf("NewScrollbackWriter: %v", err)
	}

	entries := []string{"hello", "world", "test"}
	for _, e := range entries {
		if err := w.WriteOutput([]byte(e)); err != nil {
			t.Fatalf("WriteOutput(%q): %v", e, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read file and verify JSONL structure.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 { // 1 header + 3 entries
		t.Fatalf("expected 4 lines, got %d: %v", len(lines), lines)
	}

	// Verify header is valid JSON with version:2.
	var header map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("header is not valid JSON: %v", err)
	}
	if v, ok := header["version"]; !ok || v != float64(2) {
		t.Errorf("header version: got %v, want 2", header["version"])
	}

	// Verify each entry line is a valid JSON array with 3 elements.
	for i, line := range lines[1:] {
		var entry []interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d not valid JSON: %v\nline: %s", i+1, err, line)
		}
		if len(entry) != 3 {
			t.Errorf("line %d: expected 3 elements, got %d", i+1, len(entry))
		}
		// First element should be a float (elapsed seconds).
		if _, ok := entry[0].(float64); !ok {
			t.Errorf("line %d: first element should be float, got %T", i+1, entry[0])
		}
		// Second element should be "o".
		if entry[1] != "o" {
			t.Errorf("line %d: second element should be 'o', got %v", i+1, entry[1])
		}
		// Third element should be a string.
		if _, ok := entry[2].(string); !ok {
			t.Errorf("line %d: third element should be string, got %T", i+1, entry[2])
		}
	}
}

func TestScrollbackSpecialChars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "special.cast")

	w, err := NewScrollbackWriter(path, 80, 24)
	if err != nil {
		t.Fatalf("NewScrollbackWriter: %v", err)
	}

	specials := []string{
		`"quotes"`,
		`back\slash`,
		"new\nline",
		"\t tab",
	}

	for _, s := range specials {
		if err := w.WriteOutput([]byte(s)); err != nil {
			t.Fatalf("WriteOutput: %v", err)
		}
	}
	w.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// Skip header line, verify each entry parses as valid JSON.
	for i, line := range lines[1:] {
		var entry []interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d not valid JSON: %v\nline: %s", i+1, err, line)
		}
		if len(entry) != 3 {
			t.Errorf("line %d: expected 3 elements, got %d", i+1, len(entry))
		}
	}
}

func TestScrollbackChunkReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chunks.cast")

	w, err := NewScrollbackWriter(path, 80, 24)
	if err != nil {
		t.Fatalf("NewScrollbackWriter: %v", err)
	}

	// Write enough data to exceed 256 bytes.
	for i := 0; i < 20; i++ {
		if err := w.WriteOutput([]byte("this is some test data for chunking purposes\n")); err != nil {
			t.Fatalf("WriteOutput: %v", err)
		}
	}
	w.Close()

	// Read with small chunk size.
	chunks, err := ReadScrollbackChunks(path, 128)
	if err != nil {
		t.Fatalf("ReadScrollbackChunks: %v", err)
	}

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	// Verify offsets are sequential.
	var expectedOffset int64
	for i, chunk := range chunks {
		if chunk.Offset != expectedOffset {
			t.Errorf("chunk %d: offset=%d, want %d", i, chunk.Offset, expectedOffset)
		}
		expectedOffset += int64(len(chunk.Data))
	}

	// Verify last chunk has IsFinal.
	if !chunks[len(chunks)-1].IsFinal {
		t.Error("last chunk should have IsFinal=true")
	}

	// Verify no other chunk has IsFinal.
	for i, chunk := range chunks[:len(chunks)-1] {
		if chunk.IsFinal {
			t.Errorf("chunk %d should not have IsFinal=true", i)
		}
	}

	// Verify concatenated chunks equal full file content.
	fullFile, _ := os.ReadFile(path)
	var concatenated []byte
	for _, chunk := range chunks {
		concatenated = append(concatenated, chunk.Data...)
	}
	if string(concatenated) != string(fullFile) {
		t.Errorf("concatenated chunks don't match file content:\ngot %d bytes\nwant %d bytes", len(concatenated), len(fullFile))
	}
}

func TestScrollbackOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "offset.cast")

	w, err := NewScrollbackWriter(path, 80, 24)
	if err != nil {
		t.Fatalf("NewScrollbackWriter: %v", err)
	}

	initialOffset := w.CurrentOffset()
	if initialOffset <= 0 {
		t.Errorf("initial offset should be > 0 (header was written), got %d", initialOffset)
	}

	if err := w.WriteOutput([]byte("first")); err != nil {
		t.Fatalf("WriteOutput: %v", err)
	}
	afterFirst := w.CurrentOffset()
	if afterFirst <= initialOffset {
		t.Errorf("offset should increase after write: before=%d, after=%d", initialOffset, afterFirst)
	}

	if err := w.WriteOutput([]byte("second")); err != nil {
		t.Fatalf("WriteOutput: %v", err)
	}
	afterSecond := w.CurrentOffset()
	if afterSecond <= afterFirst {
		t.Errorf("offset should increase after second write: before=%d, after=%d", afterFirst, afterSecond)
	}

	w.Close()
}
