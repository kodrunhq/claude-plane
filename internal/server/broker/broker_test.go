package broker_test

import (
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/broker"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

func TestBroker_RegisterAndResolve(t *testing.T) {
	b := broker.New()
	ch := b.Register("req-1")

	evt := &pb.DirectoryListingEvent{
		RequestId: "req-1",
		Path:      "/home",
		Entries: []*pb.DirectoryEntry{
			{Name: "user", Type: "directory"},
		},
	}

	b.Resolve("req-1", evt)

	select {
	case got := <-ch:
		if got.GetRequestId() != "req-1" {
			t.Errorf("expected request ID req-1, got %s", got.GetRequestId())
		}
		if got.GetPath() != "/home" {
			t.Errorf("expected path /home, got %s", got.GetPath())
		}
		if len(got.GetEntries()) != 1 {
			t.Errorf("expected 1 entry, got %d", len(got.GetEntries()))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for resolve")
	}
}

func TestBroker_Cancel(t *testing.T) {
	b := broker.New()
	_ = b.Register("req-cancel")

	b.Cancel("req-cancel")

	// Resolve after cancel should not panic and should be a no-op.
	evt := &pb.DirectoryListingEvent{RequestId: "req-cancel", Path: "/tmp"}
	b.Resolve("req-cancel", evt)
}

func TestBroker_ResolveUnknown(t *testing.T) {
	b := broker.New()

	// Resolving a request ID that was never registered should not panic.
	evt := &pb.DirectoryListingEvent{RequestId: "unknown", Path: "/tmp"}
	b.Resolve("unknown", evt)
}

func TestBroker_Timeout(t *testing.T) {
	b := broker.New()
	ch := b.Register("req-timeout")

	// Simulate a timeout — nobody calls Resolve.
	select {
	case <-ch:
		t.Fatal("expected timeout, but got a response")
	case <-time.After(50 * time.Millisecond):
		// Expected: timed out.
	}

	// Clean up the pending request.
	b.Cancel("req-timeout")
}
