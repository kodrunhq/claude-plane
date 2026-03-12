package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// ScrollbackChunk represents a chunk of scrollback data read from a file.
type ScrollbackChunk struct {
	Data    []byte
	Offset  int64
	IsFinal bool
}

// ScrollbackWriter writes PTY output to an asciicast v2 JSONL file.
type ScrollbackWriter struct {
	file    *os.File
	mu      sync.Mutex
	offset  int64
	started time.Time
}

// NewScrollbackWriter creates a new scrollback file with an asciicast v2 header.
func NewScrollbackWriter(path string, cols, rows uint32) (*ScrollbackWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create scrollback file: %w", err)
	}

	header := fmt.Sprintf("{\"version\":2,\"width\":%d,\"height\":%d,\"timestamp\":%d}\n",
		cols, rows, time.Now().Unix())
	n, err := f.WriteString(header)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("write header: %w", err)
	}

	return &ScrollbackWriter{
		file:    f,
		offset:  int64(n),
		started: time.Now(),
	}, nil
}

// WriteOutput writes a timestamped output entry in asciicast v2 format.
func (w *ScrollbackWriter) WriteOutput(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	elapsed := time.Since(w.started).Seconds()

	// Use json.Marshal to properly escape control chars and quotes. Note: invalid UTF-8
	// bytes are replaced with U+FFFD per encoding/json semantics, which matches asciicast
	// v2's expectation of valid UTF-8 terminal text.
	escapedData, err := json.Marshal(string(data))
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	// escapedData includes surrounding quotes from json.Marshal, so use it directly.
	line := fmt.Sprintf("[%f,\"o\",%s]\n", elapsed, escapedData)
	n, err := w.file.WriteString(line)
	if err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	w.offset += int64(n)
	return nil
}

// CurrentOffset returns the current byte offset in the scrollback file.
func (w *ScrollbackWriter) CurrentOffset() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.offset
}

// Close flushes and closes the scrollback file.
func (w *ScrollbackWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// ReadScrollbackChunks reads a scrollback file in chunks of the given size.
// Each chunk includes its byte offset and a flag indicating if it is the last chunk.
func ReadScrollbackChunks(path string, chunkSize int) ([]ScrollbackChunk, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open scrollback file: %w", err)
	}
	defer f.Close()

	var chunks []ScrollbackChunk
	buf := make([]byte, chunkSize)
	var offset int64

	for {
		n, err := f.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			chunks = append(chunks, ScrollbackChunk{
				Data:   data,
				Offset: offset,
			})
			offset += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read chunk: %w", err)
		}
	}

	if len(chunks) > 0 {
		chunks[len(chunks)-1].IsFinal = true
	}

	return chunks, nil
}
