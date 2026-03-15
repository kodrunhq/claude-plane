package retention

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// Cleaner periodically removes expired session content and notifies agents.
type Cleaner struct {
	store       *store.Store
	connMgr     *connmgr.ConnectionManager
	logger      *slog.Logger
	defaultDays int
	done        chan struct{}
	wg          sync.WaitGroup
}

// NewCleaner creates a new retention cleaner.
func NewCleaner(st *store.Store, cm *connmgr.ConnectionManager, logger *slog.Logger, defaultDays int) *Cleaner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Cleaner{
		store:       st,
		connMgr:     cm,
		logger:      logger,
		defaultDays: defaultDays,
		done:        make(chan struct{}),
	}
}

// Start begins the hourly cleanup sweep in a background goroutine.
func (c *Cleaner) Start() {
	c.wg.Add(1)
	go c.loop()
}

// Stop halts the cleaner and waits for the background goroutine to finish.
func (c *Cleaner) Stop() {
	close(c.done)
	c.wg.Wait()
}

func (c *Cleaner) loop() {
	defer c.wg.Done()

	// Run once on startup after a short delay
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-timer.C:
			c.sweep()
			timer.Reset(1 * time.Hour)
		}
	}
}

func (c *Cleaner) sweep() {
	ctx := context.Background()

	// Determine retention period
	days := c.defaultDays
	if val, err := c.store.GetSetting(ctx, "retention_days"); err == nil && val != "" {
		if d, err := strconv.Atoi(val); err == nil && d > 0 {
			days = d
		} else if val == "0" {
			return // unlimited retention
		}
	}

	expired, err := c.store.ListExpiredContentSessions(ctx, days)
	if err != nil {
		c.logger.Warn("retention sweep: failed to list expired sessions", "error", err)
		return
	}

	if len(expired) == 0 {
		return
	}

	c.logger.Info("retention sweep: cleaning expired sessions", "count", len(expired), "retention_days", days)

	for _, s := range expired {
		if err := c.store.DeleteSessionContent(ctx, s.SessionID); err != nil {
			c.logger.Warn("retention sweep: failed to delete content",
				"error", err, "session_id", s.SessionID)
			continue
		}

		// Notify agent to delete .cast file
		agent := c.connMgr.GetAgent(s.MachineID)
		if agent != nil {
			cmd := &pb.ServerCommand{
				Command: &pb.ServerCommand_CleanupScrollback{
					CleanupScrollback: &pb.CleanupScrollbackCmd{
						SessionId: s.SessionID,
					},
				},
			}
			if err := agent.SendCommand(cmd); err != nil {
				c.logger.Warn("retention sweep: failed to send cleanup to agent",
					"error", err, "machine_id", s.MachineID, "session_id", s.SessionID)
				// Queue for later
				if err := c.store.AddPendingCleanup(ctx, s.SessionID, s.MachineID); err != nil {
					c.logger.Warn("failed to queue pending cleanup", "error", err, "session_id", s.SessionID)
				}
			}
		} else {
			// Agent offline — queue for reconnect
			if err := c.store.AddPendingCleanup(ctx, s.SessionID, s.MachineID); err != nil {
				c.logger.Warn("failed to queue pending cleanup", "error", err, "session_id", s.SessionID)
			}
		}
	}

	// Optimize FTS5 index after bulk deletes
	if err := c.store.OptimizeFTS(ctx); err != nil {
		c.logger.Warn("retention sweep: FTS optimize failed", "error", err)
	}
}
