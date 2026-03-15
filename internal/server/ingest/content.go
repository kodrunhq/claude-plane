package ingest

import (
	"bytes"
	"context"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// ansiPattern matches ANSI escape sequences:
// - CSI sequences: \x1b[ ... (any params) ... final byte
// - OSC sequences: \x1b] ... ST (or BEL)
// - Single-char escapes: \x1b followed by one character
// - C0 control chars except \n, \r, \t
var ansiPattern = regexp.MustCompile(
	`\x1b\[[0-9;?]*[a-zA-Z]` + // CSI sequences
		`|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)` + // OSC sequences
		`|\x1b[()][0-9A-B]` + // Character set selection
		`|\x1b[^[\]()0-9A-B]` + // Other single-char escapes
		`|[\x00-\x08\x0b\x0c\x0e-\x1f]`, // C0 controls (keep \n \r \t)
)

// stripANSI removes all ANSI escape sequences and control characters from data,
// keeping only printable text, newlines, carriage returns, and tabs.
func stripANSI(data []byte) []byte {
	return ansiPattern.ReplaceAll(data, nil)
}

// ContentStore is the interface needed by the ingestor.
type ContentStore interface {
	InsertContentLines(ctx context.Context, lines []store.ContentLine) error
	UpsertContentMeta(ctx context.Context, sessionID string, lineCount int) error
}

const (
	batchFlushInterval = 500 * time.Millisecond
	batchFlushSize     = 100
)

// ContentIngestor receives raw terminal output, strips ANSI, splits into lines,
// and batch-inserts into the content store for full-text search.
type ContentIngestor struct {
	store   ContentStore
	logger  *slog.Logger
	buffers sync.Map // map[string]*lineBuffer
	done    chan struct{}
	wg      sync.WaitGroup
}

type lineBuffer struct {
	mu        sync.Mutex
	lineCount int
	partial   []byte              // incomplete line waiting for newline
	batch     []store.ContentLine // buffered complete lines pending flush
	sessionID string
}

// NewContentIngestor creates a new ingestor and starts the background flush ticker.
func NewContentIngestor(st ContentStore, logger *slog.Logger) *ContentIngestor {
	if logger == nil {
		logger = slog.Default()
	}
	ci := &ContentIngestor{
		store:  st,
		logger: logger,
		done:   make(chan struct{}),
	}
	ci.wg.Add(1)
	go ci.flushLoop()
	return ci
}

// Ingest processes raw terminal output for a session.
// Called from the gRPC server goroutine on each SessionOutputEvent.
func (ci *ContentIngestor) Ingest(sessionID string, data []byte) {
	if len(data) == 0 {
		return
	}
	stripped := stripANSI(data)
	if len(stripped) == 0 {
		return
	}

	val, _ := ci.buffers.LoadOrStore(sessionID, &lineBuffer{sessionID: sessionID})
	buf := val.(*lineBuffer)

	buf.mu.Lock()

	// Combine partial line with new data
	combined := make([]byte, len(buf.partial)+len(stripped))
	copy(combined, buf.partial)
	copy(combined[len(buf.partial):], stripped)
	buf.partial = nil

	// Split on newlines
	for {
		idx := bytes.IndexByte(combined, '\n')
		if idx < 0 {
			// No more newlines — save remainder as partial
			if len(combined) > 0 {
				buf.partial = make([]byte, len(combined))
				copy(buf.partial, combined)
			}
			break
		}
		line := combined[:idx]
		combined = combined[idx+1:]

		// Strip trailing carriage returns
		line = bytes.TrimRight(line, "\r")

		buf.lineCount++
		buf.batch = append(buf.batch, store.ContentLine{
			SessionID:  sessionID,
			LineNumber: buf.lineCount,
			Content:    string(line),
		})
	}

	// Extract batch under lock if large enough, then write outside lock
	var toFlush []store.ContentLine
	if len(buf.batch) >= batchFlushSize {
		toFlush = buf.batch
		buf.batch = nil
	}
	buf.mu.Unlock()

	if len(toFlush) > 0 {
		ci.writeBatch(sessionID, toFlush)
	}
}

// FlushSession flushes all buffered content for a session (call when session ends).
func (ci *ContentIngestor) FlushSession(sessionID string) {
	val, ok := ci.buffers.Load(sessionID)
	if !ok {
		return
	}
	buf := val.(*lineBuffer)

	buf.mu.Lock()

	// Flush any remaining partial line
	if len(buf.partial) > 0 {
		partial := bytes.TrimRight(buf.partial, "\r")
		buf.lineCount++
		buf.batch = append(buf.batch, store.ContentLine{
			SessionID:  sessionID,
			LineNumber: buf.lineCount,
			Content:    string(partial),
		})
		buf.partial = nil
	}

	batch := buf.batch
	buf.batch = nil
	lineCount := buf.lineCount
	buf.mu.Unlock()

	if len(batch) > 0 {
		ci.writeBatch(sessionID, batch)
	}

	// Update meta with final line count
	if lineCount > 0 {
		if err := ci.store.UpsertContentMeta(context.Background(), sessionID, lineCount); err != nil {
			ci.logger.Warn("failed to update content meta", "error", err, "session_id", sessionID)
		}
	}

	ci.buffers.Delete(sessionID)
}

// Close stops the background flush ticker and flushes all remaining buffers.
func (ci *ContentIngestor) Close() {
	close(ci.done)
	ci.wg.Wait()

	// Flush all remaining buffers
	ci.buffers.Range(func(key, val any) bool {
		buf := val.(*lineBuffer)
		buf.mu.Lock()
		batch := buf.batch
		buf.batch = nil
		sessionID := buf.sessionID
		buf.mu.Unlock()
		if len(batch) > 0 {
			ci.writeBatch(sessionID, batch)
		}
		return true
	})
}

// flushLoop runs the periodic batch flush.
func (ci *ContentIngestor) flushLoop() {
	defer ci.wg.Done()
	ticker := time.NewTicker(batchFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ci.done:
			return
		case <-ticker.C:
			ci.buffers.Range(func(key, val any) bool {
				buf := val.(*lineBuffer)
				buf.mu.Lock()
				batch := buf.batch
				buf.batch = nil
				sessionID := buf.sessionID
				buf.mu.Unlock()
				if len(batch) > 0 {
					ci.writeBatch(sessionID, batch)
				}
				return true
			})
		}
	}
}

// writeBatch writes a batch of lines to the store. Must be called without holding any mutex.
func (ci *ContentIngestor) writeBatch(sessionID string, batch []store.ContentLine) {
	if err := ci.store.InsertContentLines(context.Background(), batch); err != nil {
		ci.logger.Warn("failed to insert content lines",
			"error", err,
			"session_id", sessionID,
			"line_count", len(batch),
		)
	}
}
