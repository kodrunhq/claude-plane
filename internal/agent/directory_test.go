package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// newTestClient creates a minimal AgentClient for testing directory listing.
func newTestClient() *AgentClient {
	return &AgentClient{
		logger: slog.Default(),
	}
}

func TestHandleListDirectory_ValidPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create known contents.
	if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	c := newTestClient()
	sendCh := make(chan *pb.AgentEvent, 4)

	cmd := &pb.ListDirectoryCmd{
		RequestId: "test-req-1",
		Path:      tmpDir,
	}

	c.handleListDirectory(cmd, sendCh)

	select {
	case evt := <-sendCh:
		dl := evt.GetDirectoryListing()
		if dl == nil {
			t.Fatal("expected DirectoryListingEvent, got nil")
		}
		if dl.GetRequestId() != "test-req-1" {
			t.Errorf("expected request ID test-req-1, got %s", dl.GetRequestId())
		}
		if dl.GetError() != "" {
			t.Errorf("expected no error, got %s", dl.GetError())
		}
		if dl.GetPath() != tmpDir {
			t.Errorf("expected path %s, got %s", tmpDir, dl.GetPath())
		}
		if dl.GetParent() != filepath.Dir(tmpDir) {
			t.Errorf("expected parent %s, got %s", filepath.Dir(tmpDir), dl.GetParent())
		}
		entries := dl.GetEntries()
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
		// Entries are sorted by os.ReadDir (alphabetical).
		nameMap := make(map[string]string)
		for _, e := range entries {
			nameMap[e.GetName()] = e.GetType()
		}
		if nameMap["file.txt"] != "file" {
			t.Errorf("expected file.txt to be type file, got %s", nameMap["file.txt"])
		}
		if nameMap["subdir"] != "directory" {
			t.Errorf("expected subdir to be type directory, got %s", nameMap["subdir"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for directory listing event")
	}
}

func TestHandleListDirectory_InvalidPath(t *testing.T) {
	c := newTestClient()
	sendCh := make(chan *pb.AgentEvent, 4)

	cmd := &pb.ListDirectoryCmd{
		RequestId: "test-req-2",
		Path:      "/nonexistent/path/that/does/not/exist",
	}

	c.handleListDirectory(cmd, sendCh)

	select {
	case evt := <-sendCh:
		dl := evt.GetDirectoryListing()
		if dl == nil {
			t.Fatal("expected DirectoryListingEvent, got nil")
		}
		if dl.GetRequestId() != "test-req-2" {
			t.Errorf("expected request ID test-req-2, got %s", dl.GetRequestId())
		}
		if dl.GetError() == "" {
			t.Error("expected error message for nonexistent path, got empty string")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error event")
	}
}

func TestHandleListDirectory_RelativePath(t *testing.T) {
	c := newTestClient()
	sendCh := make(chan *pb.AgentEvent, 4)

	cmd := &pb.ListDirectoryCmd{
		RequestId: "test-req-3",
		Path:      "relative/path",
	}

	c.handleListDirectory(cmd, sendCh)

	select {
	case evt := <-sendCh:
		dl := evt.GetDirectoryListing()
		if dl == nil {
			t.Fatal("expected DirectoryListingEvent, got nil")
		}
		if dl.GetRequestId() != "test-req-3" {
			t.Errorf("expected request ID test-req-3, got %s", dl.GetRequestId())
		}
		if dl.GetError() == "" {
			t.Error("expected error for relative path, got empty string")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error event")
	}
}
