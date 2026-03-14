package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

const (
	defaultQueueCapacity  = 32
	defaultIdleTimeout    = 5 * time.Minute
	defaultStaleThreshold = 5 * time.Minute
)

// ErrQueueFull is returned when the injection queue for a session is at capacity.
var ErrQueueFull = errors.New("injection queue full")

// ErrSessionNotRunning is returned when the target session is in a terminal status.
var ErrSessionNotRunning = errors.New("session not running")

// InjectionAuditStore is the narrow interface for injection audit operations.
type InjectionAuditStore interface {
	UpdateInjectionDelivered(ctx context.Context, injectionID string, deliveredAt time.Time) error
	UpdateInjectionFailed(ctx context.Context, injectionID string, reason string) error
}

// SessionStatusChecker matches store.GetSession (no context param).
type SessionStatusChecker interface {
	GetSession(id string) (*store.Session, error)
}

// InjectionQueue manages per-session channels that deliver injected text to
// running sessions via gRPC InputDataCmd. Each session with pending injections
// gets a dedicated drainer goroutine that forwards items to the agent.
type InjectionQueue struct {
	mu           sync.Mutex
	wg           sync.WaitGroup
	queues       map[string]*sessionQueue
	connMgr      *connmgr.ConnectionManager
	auditStore   InjectionAuditStore
	sessionStore SessionStatusChecker
	subscriber   event.Subscriber
	unsubscribe  func()
	idleTimeout  time.Duration
	logger       *slog.Logger
}

type sessionQueue struct {
	items     chan queueItem
	done      chan struct{}
	closeOnce sync.Once
	machineID string
}

type queueItem struct {
	InjectionID string
	Data        []byte
	DelayMs     int
	QueuedAt    time.Time
}

// NewInjectionQueue creates an InjectionQueue that subscribes to session events
// so it can tear down drainers when sessions exit.
func NewInjectionQueue(
	connMgr *connmgr.ConnectionManager,
	auditStore InjectionAuditStore,
	sessionStore SessionStatusChecker,
	subscriber event.Subscriber,
	logger *slog.Logger,
) *InjectionQueue {
	if logger == nil {
		logger = slog.Default()
	}
	q := &InjectionQueue{
		queues:       make(map[string]*sessionQueue),
		connMgr:      connMgr,
		auditStore:   auditStore,
		sessionStore: sessionStore,
		subscriber:   subscriber,
		idleTimeout:  defaultIdleTimeout,
		logger:       logger,
	}
	q.unsubscribe = subscriber.Subscribe("session.*", q.handleSessionEvent,
		event.SubscriberOptions{BufferSize: 64, Concurrency: 1})
	return q
}

// Enqueue adds data to the injection queue for the given session. If no drainer
// goroutine exists for the session, one is started. Returns an error if the
// session is in a terminal status or the queue is full.
func (q *InjectionQueue) Enqueue(ctx context.Context, sessionID, machineID string, data []byte, delayMs int, injectionID string) error {
	sess, err := q.sessionStore.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("check session status: %w", err)
	}
	if isTerminalStatus(sess.Status) {
		return fmt.Errorf("session %s is in terminal status %q: %w", sessionID, sess.Status, ErrSessionNotRunning)
	}

	sq := q.getOrCreateQueue(sessionID, machineID)

	item := queueItem{
		InjectionID: injectionID,
		Data:        data,
		DelayMs:     delayMs,
		QueuedAt:    time.Now(),
	}

	select {
	case sq.items <- item:
		return nil
	default:
		return fmt.Errorf("injection queue full for session %s (capacity %d): %w", sessionID, defaultQueueCapacity, ErrQueueFull)
	}
}

// getOrCreateQueue returns the existing sessionQueue or creates a new one with
// a drainer goroutine.
func (q *InjectionQueue) getOrCreateQueue(sessionID, machineID string) *sessionQueue {
	q.mu.Lock()
	defer q.mu.Unlock()

	if sq, ok := q.queues[sessionID]; ok {
		return sq
	}

	sq := &sessionQueue{
		items:     make(chan queueItem, defaultQueueCapacity),
		done:      make(chan struct{}),
		machineID: machineID,
	}
	q.queues[sessionID] = sq
	q.wg.Add(1)
	go q.drainSession(sessionID, sq)
	return sq
}

// drainSession reads items from the session queue and sends them to the agent
// via gRPC. It exits when the done channel is closed, the idle timeout fires,
// or the queue is garbage-collected.
func (q *InjectionQueue) drainSession(sessionID string, sq *sessionQueue) {
	defer q.wg.Done()
	defer q.removeQueue(sessionID)

	idleTimer := time.NewTimer(q.idleTimeout)
	defer idleTimer.Stop()

	for {
		select {
		case item, ok := <-sq.items:
			if !ok {
				return
			}
			// Reset idle timer on activity.
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(q.idleTimeout)

			q.processItem(sessionID, sq.machineID, item, sq)

		case <-sq.done:
			q.drainRemaining(sessionID, sq)
			return

		case <-idleTimer.C:
			// Re-check: an item may have raced with the timer.
			select {
			case item, ok := <-sq.items:
				if ok {
					q.processItem(sessionID, sq.machineID, item, sq)
					idleTimer.Reset(q.idleTimeout)
					continue // back to main select
				}
			default:
				// truly idle
			}
			q.logger.Info("injection drainer idle timeout", "session_id", sessionID)
			return
		}
	}
}

// processItem handles a single queue item: checks staleness, applies delay,
// sends the InputDataCmd, and updates the audit record.
func (q *InjectionQueue) processItem(sessionID, machineID string, item queueItem, sq *sessionQueue) {
	// Drop stale items silently.
	if time.Since(item.QueuedAt) > defaultStaleThreshold {
		q.logger.Debug("dropping stale injection", "session_id", sessionID, "injection_id", item.InjectionID)
		return
	}

	// Re-check session status before delivery (mitigates Enqueue TOCTOU race).
	sess, err := q.sessionStore.GetSession(sessionID)
	if err != nil {
		q.logger.Warn("failed to check session status before delivery",
			"session_id", sessionID, "injection_id", item.InjectionID, "error", err)
		return
	}
	if isTerminalStatus(sess.Status) {
		q.logger.Info("session terminated before injection delivery",
			"session_id", sessionID, "injection_id", item.InjectionID)
		if err := q.auditStore.UpdateInjectionFailed(
			context.Background(), item.InjectionID, "session terminated",
		); err != nil {
			q.logger.Error("failed to mark injection failed",
				"injection_id", item.InjectionID, "error", err)
		}
		return
	}

	if item.DelayMs > 0 {
		timer := time.NewTimer(time.Duration(item.DelayMs) * time.Millisecond)
		select {
		case <-timer.C:
			// delay elapsed, proceed
		case <-sq.done:
			timer.Stop()
			return // session terminated during delay
		}
	}

	agent := q.connMgr.GetAgent(machineID)
	if agent == nil {
		q.logger.Warn("agent not connected, dropping injection",
			"session_id", sessionID, "machine_id", machineID, "injection_id", item.InjectionID)
		return
	}

	cmd := &pb.ServerCommand{
		Command: &pb.ServerCommand_InputData{
			InputData: &pb.InputDataCmd{
				SessionId: sessionID,
				Data:      item.Data,
			},
		},
	}

	if err := agent.SendCommand(cmd); err != nil {
		q.logger.Error("failed to send injection",
			"session_id", sessionID, "injection_id", item.InjectionID, "error", err)
		return
	}

	deliveredAt := time.Now().UTC()
	if err := q.auditStore.UpdateInjectionDelivered(context.Background(), item.InjectionID, deliveredAt); err != nil {
		q.logger.Error("failed to update injection delivery",
			"injection_id", item.InjectionID, "error", err)
	}
}

// drainRemaining reads any buffered items from the session queue and marks
// each as failed. Called during shutdown to avoid silently discarding items.
func (q *InjectionQueue) drainRemaining(sessionID string, sq *sessionQueue) {
	for {
		select {
		case item, ok := <-sq.items:
			if !ok {
				return
			}
			q.logger.Warn("marking buffered injection as failed on shutdown",
				"session_id", sessionID, "injection_id", item.InjectionID)
			if err := q.auditStore.UpdateInjectionFailed(context.Background(), item.InjectionID, "queue shutdown"); err != nil {
				q.logger.Error("failed to mark injection as failed",
					"injection_id", item.InjectionID, "error", err)
			}
		default:
			return
		}
	}
}

// handleSessionEvent reacts to session lifecycle events. When a session exits
// or is terminated, the corresponding drainer is shut down.
func (q *InjectionQueue) handleSessionEvent(_ context.Context, evt event.Event) error {
	if evt.Type != event.TypeSessionExited && evt.Type != event.TypeSessionTerminated {
		return nil
	}

	sessionID, ok := evt.Payload["session_id"].(string)
	if !ok || sessionID == "" {
		return nil
	}

	q.mu.Lock()
	sq, exists := q.queues[sessionID]
	q.mu.Unlock()

	if exists {
		sq.closeOnce.Do(func() { close(sq.done) })
	}
	return nil
}

// removeQueue removes the session queue entry under the lock.
func (q *InjectionQueue) removeQueue(sessionID string) {
	q.mu.Lock()
	delete(q.queues, sessionID)
	q.mu.Unlock()
}

// Close unsubscribes from events and shuts down all drainer goroutines.
func (q *InjectionQueue) Close() {
	if q.unsubscribe != nil {
		q.unsubscribe()
	}

	q.mu.Lock()
	queues := make(map[string]*sessionQueue, len(q.queues))
	for k, v := range q.queues {
		queues[k] = v
	}
	q.mu.Unlock()

	for _, sq := range queues {
		sq.closeOnce.Do(func() { close(sq.done) })
	}

	q.wg.Wait()
}

// isTerminalStatus returns true if the session status indicates it has ended.
func isTerminalStatus(status string) bool {
	return status == store.StatusCompleted ||
		status == store.StatusFailed ||
		status == store.StatusTerminated
}
