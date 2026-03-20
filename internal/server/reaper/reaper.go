// Package reaper terminates idle standalone sessions that exceed their
// staleness timeout. It runs as a background goroutine on the server.
package reaper

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

const (
	// DefaultSweepInterval is how often the reaper checks for stale sessions.
	DefaultSweepInterval = 15 * time.Minute

	// DefaultStaleTimeout is the default idle timeout (3 days in minutes).
	DefaultStaleTimeout = 4320
)

// prefsPayload extracts the session_stale_timeout from the preferences JSON.
type prefsPayload struct {
	SessionStaleTimeout *int `json:"session_stale_timeout"`
}

// Reaper terminates idle standalone sessions that exceed their staleness timeout.
type Reaper struct {
	store     *store.Store
	connMgr   *connmgr.ConnectionManager
	publisher event.Publisher
	logger    *slog.Logger
	interval  time.Duration
}

// New creates a Reaper with the given sweep interval.
func New(s *store.Store, cm *connmgr.ConnectionManager, pub event.Publisher, logger *slog.Logger) *Reaper {
	return &Reaper{
		store:     s,
		connMgr:   cm,
		publisher: pub,
		logger:    logger,
		interval:  DefaultSweepInterval,
	}
}

// Start runs the reaper loop in a background goroutine. It stops when ctx is cancelled.
func (r *Reaper) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.sweep(ctx)
			}
		}
	}()
	r.logger.Info("session reaper started", "interval", r.interval)
}

// sweep checks for stale waiting_for_input sessions and terminates them.
func (r *Reaper) sweep(ctx context.Context) {
	sessions, err := r.store.ListSessionsByStatus(store.StatusWaitingForInput)
	if err != nil {
		r.logger.Warn("reaper: failed to list idle sessions", "error", err)
		return
	}

	if len(sessions) == 0 {
		return
	}

	// Build a cache of per-user timeouts to avoid repeated DB reads.
	userTimeouts := make(map[string]int) // user_id -> timeout in minutes (0 = disabled)
	now := time.Now()

	for _, sess := range sessions {
		timeout, ok := userTimeouts[sess.UserID]
		if !ok {
			timeout = r.loadUserTimeout(ctx, sess.UserID)
			userTimeouts[sess.UserID] = timeout
		}

		if timeout <= 0 {
			continue // user opted out of auto-terminate
		}

		idleDuration := now.Sub(sess.UpdatedAt)
		if idleDuration > time.Duration(timeout)*time.Minute {
			r.terminateSession(ctx, sess)
		}
	}
}

// loadUserTimeout reads the session_stale_timeout preference for a user.
// Returns DefaultStaleTimeout if the user has no custom setting.
func (r *Reaper) loadUserTimeout(ctx context.Context, userID string) int {
	prefs, err := r.store.GetUserPreferences(ctx, userID)
	if err != nil {
		r.logger.Warn("reaper: failed to load user preferences, using default",
			"user_id", userID, "error", err)
		return DefaultStaleTimeout
	}

	var p prefsPayload
	if err := json.Unmarshal([]byte(prefs.Preferences), &p); err != nil {
		return DefaultStaleTimeout
	}

	if p.SessionStaleTimeout == nil {
		return DefaultStaleTimeout
	}
	return *p.SessionStaleTimeout
}

// terminateSession sends a kill command and updates the session status.
func (r *Reaper) terminateSession(ctx context.Context, sess store.Session) {
	agent := r.connMgr.GetAgent(sess.MachineID)
	if agent != nil && agent.SendCommand != nil {
		cmd := &pb.ServerCommand{
			Command: &pb.ServerCommand_KillSession{
				KillSession: &pb.KillSessionCmd{
					SessionId: sess.SessionID,
					Signal:    "SIGTERM",
				},
			},
		}
		if err := agent.SendCommand(cmd); err != nil {
			r.logger.Error("reaper: failed to send kill command",
				"session_id", sess.SessionID, "error", err)
		}
	}

	if err := r.store.UpdateSessionStatus(sess.SessionID, store.StatusTerminated); err != nil {
		r.logger.Error("reaper: failed to update session status",
			"session_id", sess.SessionID, "error", err)
		return
	}

	if r.publisher != nil {
		if err := r.publisher.Publish(ctx, event.NewSessionEvent(
			event.TypeSessionTerminated, sess.SessionID, sess.MachineID, "", "",
		)); err != nil {
			r.logger.Warn("reaper: failed to publish termination event",
				"session_id", sess.SessionID, "error", err)
		}
	}

	r.logger.Info("reaper: terminated stale session",
		"session_id", sess.SessionID,
		"user_id", sess.UserID,
		"idle_duration", time.Since(sess.UpdatedAt),
	)
}
