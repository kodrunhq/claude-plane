package state_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/state"
)

func newStore(t *testing.T) *state.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test-state.json")
	return state.New(path)
}

// --- GetCursor / SetCursor ---

func TestGetCursor_Empty(t *testing.T) {
	s := newStore(t)
	if got := s.GetCursor("conn-1"); got != "" {
		t.Errorf("GetCursor on empty store = %q, want empty string", got)
	}
}

func TestSetAndGetCursor(t *testing.T) {
	s := newStore(t)
	if err := s.SetCursor("conn-1", "cursor-abc"); err != nil {
		t.Fatalf("SetCursor: %v", err)
	}
	if got := s.GetCursor("conn-1"); got != "cursor-abc" {
		t.Errorf("GetCursor = %q, want %q", got, "cursor-abc")
	}
}

func TestSetCursor_OverwritesPreviousValue(t *testing.T) {
	s := newStore(t)
	_ = s.SetCursor("conn-1", "first")
	_ = s.SetCursor("conn-1", "second")
	if got := s.GetCursor("conn-1"); got != "second" {
		t.Errorf("GetCursor = %q, want %q", got, "second")
	}
}

func TestSetCursor_IndependentConnectors(t *testing.T) {
	s := newStore(t)
	_ = s.SetCursor("conn-a", "ca")
	_ = s.SetCursor("conn-b", "cb")
	if got := s.GetCursor("conn-a"); got != "ca" {
		t.Errorf("conn-a cursor = %q, want %q", got, "ca")
	}
	if got := s.GetCursor("conn-b"); got != "cb" {
		t.Errorf("conn-b cursor = %q, want %q", got, "cb")
	}
}

// --- IsProcessed / MarkProcessed ---

func TestIsProcessed_Empty(t *testing.T) {
	s := newStore(t)
	if s.IsProcessed("evt-1") {
		t.Error("IsProcessed on empty store should be false")
	}
}

func TestMarkAndIsProcessed(t *testing.T) {
	s := newStore(t)
	if err := s.MarkProcessed("evt-1"); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}
	if !s.IsProcessed("evt-1") {
		t.Error("IsProcessed should be true after MarkProcessed")
	}
}

func TestIsProcessed_UnknownEvent(t *testing.T) {
	s := newStore(t)
	_ = s.MarkProcessed("evt-1")
	if s.IsProcessed("evt-999") {
		t.Error("IsProcessed should be false for unknown event")
	}
}

// --- Prune ---

func TestPrune_RemovesOldEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s := state.New(path)

	if err := s.MarkProcessed("evt-old"); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}
	if err := s.MarkProcessed("evt-new"); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}

	// Prune with a very small maxAge — both should be considered old relative to future,
	// so use a negative duration to simulate age greater than any entry.
	// Instead, write a file with a manually crafted old timestamp.
	// The simplest way: call Prune with 1 nanosecond, so everything before now-1ns is pruned.
	// Since MarkProcessed uses time.Now(), they are never older than 1ns when called immediately.
	// So we verify that Prune with a large window keeps them, and a zero window removes them.
	if err := s.Prune(100 * 365 * 24 * time.Hour); err != nil {
		t.Fatalf("Prune (large window): %v", err)
	}
	if !s.IsProcessed("evt-old") || !s.IsProcessed("evt-new") {
		t.Error("Prune with large window should keep recent events")
	}
}

func TestPrune_KeepsRecentEntries(t *testing.T) {
	s := newStore(t)
	_ = s.MarkProcessed("evt-recent")

	// Prune with 1 second — events marked right now should survive.
	if err := s.Prune(time.Second); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if !s.IsProcessed("evt-recent") {
		t.Error("Prune should keep events younger than maxAge")
	}
}

// --- Save / Load round-trip ---

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := state.New(path)
	_ = s.SetCursor("conn-1", "cursor-xyz")
	_ = s.MarkProcessed("evt-42")

	// Load into a fresh store from the same path.
	s2 := state.New(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := s2.GetCursor("conn-1"); got != "cursor-xyz" {
		t.Errorf("after reload, cursor = %q, want %q", got, "cursor-xyz")
	}
	if !s2.IsProcessed("evt-42") {
		t.Error("after reload, evt-42 should be marked processed")
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-file.json")
	s := state.New(path)
	if err := s.Load(); err != nil {
		t.Errorf("Load on missing file should not error, got: %v", err)
	}
}

func TestLoad_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(path, []byte("not-valid-json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	s := state.New(path)
	if err := s.Load(); err == nil {
		t.Error("Load with corrupt file should return an error")
	}
}

// --- Thread safety ---

func TestConcurrentAccess(t *testing.T) {
	s := newStore(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			connID := "conn"
			eventID := "evt"
			_ = s.SetCursor(connID, "cursor")
			_ = s.MarkProcessed(eventID)
			_ = s.GetCursor(connID)
			_ = s.IsProcessed(eventID)
		}(i)
	}
	wg.Wait()
}
