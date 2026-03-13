// Package scheduler provides cron-based job scheduling for claude-plane server.
// It loads enabled schedules from the store at startup, fires trigger events
// through the event bus, and supports dynamic reload/remove without restart.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

// CronSchedule mirrors store.CronSchedule to avoid import cycles.
type CronSchedule struct {
	ScheduleID      string
	JobID           string
	CronExpr        string
	Timezone        string
	Enabled         bool
	NextRunAt       *time.Time
	LastTriggeredAt *time.Time
}

// Event mirrors event.Event to avoid import cycles.
type Event struct {
	EventID   string         `json:"event_id"`
	Type      string         `json:"event_type"`
	Timestamp time.Time      `json:"timestamp"`
	Source    string         `json:"source"`
	Payload   map[string]any `json:"payload"`
}

// ScheduleStore is the persistence interface required by the Scheduler.
// Satisfied by store.Store.
type ScheduleStore interface {
	ListEnabledSchedules(ctx context.Context) ([]CronSchedule, error)
	GetSchedule(ctx context.Context, scheduleID string) (*CronSchedule, error)
	UpdateScheduleTimestamps(ctx context.Context, scheduleID string, lastTriggered, nextRun time.Time) error
}

// EventPublisher publishes events to the event bus.
type EventPublisher interface {
	Publish(ctx context.Context, event Event) error
}

// Scheduler manages cron-based schedule entries and fires trigger events.
type Scheduler struct {
	cron     *cron.Cron
	store    ScheduleStore
	eventBus EventPublisher
	mu       sync.Mutex
	entries  map[string]cron.EntryID // schedule_id -> cron entry ID
	logger   *slog.Logger
}

// NewScheduler creates a new Scheduler backed by the given store and event bus.
// The cron parser supports both 5-field (minute-resolution) and 6-field
// (second-resolution) expressions as well as @hourly/@daily descriptors.
// If logger is nil, slog.Default() is used.
func NewScheduler(store ScheduleStore, eventBus EventPublisher, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}

	parser := cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)

	return &Scheduler{
		cron:     cron.New(cron.WithParser(parser)),
		store:    store,
		eventBus: eventBus,
		entries:  make(map[string]cron.EntryID),
		logger:   logger,
	}
}

// Start loads all enabled schedules from the store, registers cron entries for
// each, then starts the cron runner. It spawns a goroutine that stops the cron
// when ctx is cancelled. Returns an error only if the initial store load fails.
func (s *Scheduler) Start(ctx context.Context) error {
	schedules, err := s.store.ListEnabledSchedules(ctx)
	if err != nil {
		return fmt.Errorf("scheduler start: list enabled schedules: %w", err)
	}

	s.mu.Lock()
	for _, sc := range schedules {
		if _, addErr := s.addEntryLocked(sc); addErr != nil {
			s.logger.Error("failed to add schedule entry",
				"schedule_id", sc.ScheduleID,
				"error", addErr,
			)
		}
	}
	s.mu.Unlock()

	s.cron.Start()

	go func() {
		<-ctx.Done()
		stopCtx := s.cron.Stop()
		<-stopCtx.Done()
	}()

	s.logger.Info("scheduler started", "schedules_loaded", len(schedules))
	return nil
}

// addEntryLocked registers a single CronSchedule with the underlying cron runner
// and returns the computed next run time. Must be called with s.mu held.
func (s *Scheduler) addEntryLocked(schedule CronSchedule) (time.Time, error) {
	if _, err := time.LoadLocation(schedule.Timezone); err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %q for schedule %s: %w", schedule.Timezone, schedule.ScheduleID, err)
	}

	spec := "CRON_TZ=" + schedule.Timezone + " " + schedule.CronExpr

	entryID, err := s.cron.AddFunc(spec, s.buildFuncJob(schedule))
	if err != nil {
		return time.Time{}, fmt.Errorf("add cron entry for schedule %s: %w", schedule.ScheduleID, err)
	}

	s.entries[schedule.ScheduleID] = entryID
	nextRun := s.cron.Entry(entryID).Next
	s.logger.Debug("registered schedule", "schedule_id", schedule.ScheduleID, "spec", spec, "next_run", nextRun)
	return nextRun, nil
}

// buildFuncJob returns the closure executed by the cron runner each time the
// schedule fires. It publishes a trigger.cron event and updates timestamps.
func (s *Scheduler) buildFuncJob(schedule CronSchedule) func() {
	return func() {
		now := time.Now().UTC()
		ctx := context.Background()

		ev := Event{
			EventID:   uuid.New().String(),
			Type:      "trigger.cron",
			Timestamp: now,
			Source:    "scheduler",
			Payload: map[string]any{
				"schedule_id": schedule.ScheduleID,
				"job_id":      schedule.JobID,
				"cron_expr":   schedule.CronExpr,
				"fired_at":    now.Format(time.RFC3339),
			},
		}

		if err := s.eventBus.Publish(ctx, ev); err != nil {
			s.logger.Error("failed to publish cron trigger event",
				"schedule_id", schedule.ScheduleID,
				"error", err,
			)
		}

		// Determine next run time from the live cron entry.
		s.mu.Lock()
		entryID, ok := s.entries[schedule.ScheduleID]
		var nextRun time.Time
		if ok {
			nextRun = s.cron.Entry(entryID).Next
		}
		s.mu.Unlock()

		if nextRun.IsZero() {
			nextRun = now // fallback: avoid storing a zero time
		}

		if err := s.store.UpdateScheduleTimestamps(ctx, schedule.ScheduleID, now, nextRun); err != nil {
			s.logger.Error("failed to update schedule timestamps",
				"schedule_id", schedule.ScheduleID,
				"error", err,
			)
		}
	}
}

// ReloadSchedule removes any existing cron entry for scheduleID, fetches
// the current schedule from the store, and re-registers it if enabled.
// The mutex is released during the store call to avoid blocking fire callbacks.
func (s *Scheduler) ReloadSchedule(ctx context.Context, scheduleID string) error {
	// Step 1: remove old entry under lock.
	s.mu.Lock()
	if entryID, ok := s.entries[scheduleID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, scheduleID)
	}
	s.mu.Unlock()

	// Step 2: fetch from store without holding the lock.
	sc, err := s.store.GetSchedule(ctx, scheduleID)
	if err != nil {
		return fmt.Errorf("reload schedule %s: get from store: %w", scheduleID, err)
	}
	if sc == nil {
		return fmt.Errorf("reload schedule %s: not found", scheduleID)
	}

	if !sc.Enabled {
		s.logger.Debug("schedule is disabled, not re-registering", "schedule_id", scheduleID)
		return nil
	}

	cronSched := CronSchedule{
		ScheduleID:      sc.ScheduleID,
		JobID:           sc.JobID,
		CronExpr:        sc.CronExpr,
		Timezone:        sc.Timezone,
		Enabled:         sc.Enabled,
		NextRunAt:       sc.NextRunAt,
		LastTriggeredAt: sc.LastTriggeredAt,
	}

	// Step 3: re-add entry under lock.
	s.mu.Lock()
	nextRun, addErr := s.addEntryLocked(cronSched)
	s.mu.Unlock()

	if addErr != nil {
		return fmt.Errorf("reload schedule %s: add entry: %w", scheduleID, addErr)
	}

	// Step 4: persist computed next_run_at so the UI shows it immediately.
	if !nextRun.IsZero() {
		lastTriggered := time.Time{}
		if sc.LastTriggeredAt != nil {
			lastTriggered = *sc.LastTriggeredAt
		}
		if err := s.store.UpdateScheduleTimestamps(ctx, scheduleID, lastTriggered, nextRun); err != nil {
			s.logger.Warn("failed to persist next_run_at after reload",
				"schedule_id", scheduleID, "error", err)
		}
	}

	s.logger.Info("schedule reloaded", "schedule_id", scheduleID)
	return nil
}

// RemoveSchedule removes the cron entry for scheduleID if one exists.
// It is a no-op if the schedule is not currently registered.
func (s *Scheduler) RemoveSchedule(scheduleID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entries[scheduleID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, scheduleID)
		s.logger.Info("schedule removed", "schedule_id", scheduleID)
	}
}

// Stop gracefully stops the cron runner, waiting for any in-flight jobs to
// complete before returning.
func (s *Scheduler) Stop() {
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
}

// ScheduleStoreFuncs satisfies ScheduleStore via injected function closures.
type ScheduleStoreFuncs struct {
	ListEnabledSchedulesFn     func(ctx context.Context) ([]CronSchedule, error)
	GetScheduleFn              func(ctx context.Context, scheduleID string) (*CronSchedule, error)
	UpdateScheduleTimestampsFn func(ctx context.Context, scheduleID string, lastTriggered, nextRun time.Time) error
}

func (f *ScheduleStoreFuncs) ListEnabledSchedules(ctx context.Context) ([]CronSchedule, error) {
	return f.ListEnabledSchedulesFn(ctx)
}

func (f *ScheduleStoreFuncs) GetSchedule(ctx context.Context, scheduleID string) (*CronSchedule, error) {
	return f.GetScheduleFn(ctx, scheduleID)
}

func (f *ScheduleStoreFuncs) UpdateScheduleTimestamps(ctx context.Context, scheduleID string, lastTriggered, nextRun time.Time) error {
	return f.UpdateScheduleTimestampsFn(ctx, scheduleID, lastTriggered, nextRun)
}

// EventPublisherFuncs satisfies EventPublisher via an injected function closure.
type EventPublisherFuncs struct {
	PublishFn func(ctx context.Context, event Event) error
}

func (f *EventPublisherFuncs) Publish(ctx context.Context, event Event) error {
	return f.PublishFn(ctx, event)
}
