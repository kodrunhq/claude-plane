package agent

import (
	"context"
	"log/slog"
	"testing"
	"time"

	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

func TestGRPCSinkHandler_FlushesOnClose(t *testing.T) {
	sendCh := make(chan *pb.AgentEvent, 10)
	h := NewGRPCSinkHandler(sendCh)

	logger := slog.New(h)
	logger.Info("msg-one")
	logger.Info("msg-two")
	logger.Warn("msg-three")

	h.Close()

	// Drain all events from the channel.
	var entries []*pb.LogEntry
	for {
		select {
		case evt := <-sendCh:
			batch := evt.GetLogBatch()
			if batch != nil {
				entries = append(entries, batch.Entries...)
			}
		default:
			goto done
		}
	}
done:

	if len(entries) != 3 {
		t.Fatalf("expected 3 log entries, got %d", len(entries))
	}

	want := []string{"msg-one", "msg-two", "msg-three"}
	for i, e := range entries {
		if e.Message != want[i] {
			t.Errorf("entry[%d].Message = %q, want %q", i, e.Message, want[i])
		}
	}
}

func TestGRPCSinkHandler_FlushesOnSize(t *testing.T) {
	sendCh := make(chan *pb.AgentEvent, 10)
	h := NewGRPCSinkHandler(sendCh)
	defer h.Close()

	logger := slog.New(h)

	// Emit exactly logFlushSize records to trigger a size-based flush.
	for i := 0; i < logFlushSize; i++ {
		logger.Info("bulk-msg")
	}

	// The flush should have happened synchronously during Handle.
	// Give a tiny window for the non-blocking send to land.
	select {
	case evt := <-sendCh:
		batch := evt.GetLogBatch()
		if batch == nil {
			t.Fatal("expected LogBatch event")
		}
		if len(batch.Entries) != logFlushSize {
			t.Errorf("batch size = %d, want %d", len(batch.Entries), logFlushSize)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for size-triggered flush")
	}
}

func TestGRPCSinkHandler_FeedbackLoopGuard(t *testing.T) {
	sendCh := make(chan *pb.AgentEvent, 10)
	h := NewGRPCSinkHandler(sendCh)
	defer h.Close()

	// Simulate the sending flag being true (as if a flush is in progress).
	h.state.sending.Store(true)

	logger := slog.New(h)
	logger.Info("should-be-dropped")

	h.state.sending.Store(false)

	// Nothing should be buffered.
	h.state.mu.Lock()
	bufLen := len(h.state.buf)
	h.state.mu.Unlock()

	if bufLen != 0 {
		t.Errorf("expected 0 buffered entries while sending=true, got %d", bufLen)
	}
}

func TestGRPCSinkHandler_WithAttrs_ExtractsComponent(t *testing.T) {
	sendCh := make(chan *pb.AgentEvent, 10)
	h := NewGRPCSinkHandler(sendCh)

	withComp := h.WithAttrs([]slog.Attr{slog.String("component", "grpc")})
	logger := slog.New(withComp)
	logger.Info("component test")

	h.Close()

	select {
	case evt := <-sendCh:
		batch := evt.GetLogBatch()
		if batch == nil || len(batch.Entries) == 0 {
			t.Fatal("expected at least 1 log entry")
		}
		if batch.Entries[0].Component != "grpc" {
			t.Errorf("Component = %q, want %q", batch.Entries[0].Component, "grpc")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for flush")
	}
}

func TestGRPCSinkHandler_WithGroup(t *testing.T) {
	sendCh := make(chan *pb.AgentEvent, 10)
	h := NewGRPCSinkHandler(sendCh)

	grouped := h.WithGroup("mygroup")
	logger := slog.New(grouped)
	logger.Info("group test")

	h.Close()

	// WithGroup on GRPCSinkHandler sets the group field, but it does NOT
	// automatically map to the proto Component field (only WithAttrs with
	// key="component" does that via applyAttr). Verify the entry was received.
	select {
	case evt := <-sendCh:
		batch := evt.GetLogBatch()
		if batch == nil || len(batch.Entries) == 0 {
			t.Fatal("expected at least 1 log entry")
		}
		// The group field is stored on the handler but not wired to Component
		// by the current implementation. Just verify the message arrived.
		if batch.Entries[0].Message != "group test" {
			t.Errorf("Message = %q, want %q", batch.Entries[0].Message, "group test")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for flush")
	}
}

func TestMultiHandler_CallsAll(t *testing.T) {
	h1 := &recordingHandler{}
	h2 := &recordingHandler{}

	multi := NewMultiHandler(h1, h2)
	logger := slog.New(multi)
	logger.Info("multi test")

	if len(h1.messages) != 1 {
		t.Errorf("handler1 got %d calls, want 1", len(h1.messages))
	}
	if len(h2.messages) != 1 {
		t.Errorf("handler2 got %d calls, want 1", len(h2.messages))
	}
	if len(h1.messages) > 0 && h1.messages[0] != "multi test" {
		t.Errorf("handler1 msg = %q, want %q", h1.messages[0], "multi test")
	}
}

// recordingHandler is a minimal slog.Handler that records messages for testing.
type recordingHandler struct {
	messages []string
}

func (r *recordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (r *recordingHandler) Handle(_ context.Context, rec slog.Record) error {
	r.messages = append(r.messages, rec.Message)
	return nil
}
func (r *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return r }
func (r *recordingHandler) WithGroup(_ string) slog.Handler      { return r }
