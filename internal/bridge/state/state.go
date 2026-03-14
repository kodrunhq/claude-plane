// Package state provides thread-safe, persistent state for the bridge binary.
// It tracks per-connector high-water mark cursors and processed event IDs to
// ensure at-least-once delivery without re-processing.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

const defaultMaxAge = 7 * 24 * time.Hour

// Store is a thread-safe, JSON-backed state store.
type Store struct {
	path string
	mu   sync.RWMutex
	data stateData
}

// stateData is the JSON-serialisable payload written to disk.
type stateData struct {
	Cursors   map[string]string    `json:"cursors"`   // connector_id -> cursor
	Processed map[string]time.Time `json:"processed"` // event_id -> processed_at
}

// New creates a Store that persists state to path.
// Call Load to restore previously saved state.
func New(path string) *Store {
	return &Store{
		path: path,
		data: stateData{
			Cursors:   make(map[string]string),
			Processed: make(map[string]time.Time),
		},
	}
}

// GetCursor returns the stored cursor for connectorID, or "" if none exists.
func (s *Store) GetCursor(connectorID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Cursors[connectorID]
}

// SetCursor persists a new cursor value for connectorID and flushes to disk.
func (s *Store) SetCursor(connectorID, cursor string) error {
	s.mu.Lock()
	s.data.Cursors[connectorID] = cursor
	s.mu.Unlock()
	return s.Save()
}

// IsProcessed reports whether eventID has already been processed.
func (s *Store) IsProcessed(eventID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data.Processed[eventID]
	return ok
}

// MarkProcessed records eventID as processed at the current time and flushes to disk.
func (s *Store) MarkProcessed(eventID string) error {
	s.mu.Lock()
	s.data.Processed[eventID] = time.Now().UTC()
	s.mu.Unlock()
	return s.Save()
}

// Prune removes processed entries whose recorded time is older than maxAge.
// Pass 0 to use the default of 7 days.
func (s *Store) Prune(maxAge time.Duration) error {
	if maxAge <= 0 {
		maxAge = defaultMaxAge
	}
	cutoff := time.Now().UTC().Add(-maxAge)

	s.mu.Lock()
	pruned := make(map[string]time.Time, len(s.data.Processed))
	for id, at := range s.data.Processed {
		if at.After(cutoff) {
			pruned[id] = at
		}
	}
	s.data.Processed = pruned
	s.mu.Unlock()

	return s.Save()
}

// Save writes the current state to the configured path atomically.
// It first writes to a temp file then renames to avoid partial writes.
func (s *Store) Save() error {
	s.mu.RLock()
	data, err := json.Marshal(s.data)
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write state to %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename state file: %w", err)
	}
	return nil
}

// Load reads previously saved state from disk.
// If the file does not exist the store is left in its initial (empty) state.
func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read state file %s: %w", s.path, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var loaded stateData
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("parse state file %s: %w", s.path, err)
	}

	// Merge maps, preserving nil safety.
	if loaded.Cursors != nil {
		s.data.Cursors = loaded.Cursors
	}
	if loaded.Processed != nil {
		s.data.Processed = loaded.Processed
	}

	return nil
}
