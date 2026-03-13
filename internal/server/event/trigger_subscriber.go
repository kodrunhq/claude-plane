package event

import (
	"context"
	"fmt"
	"log/slog"
)

// TriggerStore is the persistence interface required by TriggerSubscriber.
type TriggerStore interface {
	ListEnabledTriggers(ctx context.Context) ([]JobTrigger, error)
}

// JobTrigger is the minimal representation of a job trigger needed by
// TriggerSubscriber. It mirrors store.JobTrigger but lives in this package to
// avoid an import cycle.
type JobTrigger struct {
	TriggerID string
	JobID     string
	EventType string
	Filter    string
	Enabled   bool
}

// OrchestratorIface is the interface TriggerSubscriber requires to start runs.
type OrchestratorIface interface {
	CreateRun(ctx context.Context, jobID string, triggerType string) error
}

// TriggerSubscriber subscribes to all events on the bus and fires job runs
// when an enabled trigger's pattern matches the incoming event type.
//
// Loop prevention: if the event payload contains a "trigger_job_id" key whose
// value matches the trigger's job_id, the trigger is skipped to prevent direct
// recursion (job A completes → triggers job A → infinite loop).
type TriggerSubscriber struct {
	store        TriggerStore
	orchestrator OrchestratorIface
	logger       *slog.Logger
}

// NewTriggerSubscriber creates a TriggerSubscriber.
func NewTriggerSubscriber(store TriggerStore, orch OrchestratorIface, logger *slog.Logger) *TriggerSubscriber {
	if logger == nil {
		logger = slog.Default()
	}
	return &TriggerSubscriber{
		store:        store,
		orchestrator: orch,
		logger:       logger,
	}
}

// Handler returns a HandlerFunc suitable for Bus.Subscribe with pattern "*",
// concurrency 2, buffer 256.
func (t *TriggerSubscriber) Handler() HandlerFunc {
	return func(ctx context.Context, e Event) error {
		triggers, err := t.store.ListEnabledTriggers(ctx)
		if err != nil {
			return fmt.Errorf("trigger subscriber: list enabled triggers: %w", err)
		}

		// Extract loop-prevention value from the event payload once.
		triggerJobID := payloadString(e.Payload, "trigger_job_id")

		for _, trigger := range triggers {
			if !MatchPattern(trigger.EventType, e.Type) {
				continue
			}

			// Loop prevention: skip if the triggering job is the same as this trigger's job.
			if triggerJobID != "" && triggerJobID == trigger.JobID {
				t.logger.Info("trigger subscriber: skipping trigger to prevent loop",
					"trigger_id", trigger.TriggerID,
					"job_id", trigger.JobID,
					"event_type", e.Type,
				)
				continue
			}

			if err := t.orchestrator.CreateRun(ctx, trigger.JobID, "event_trigger"); err != nil {
				t.logger.Warn("trigger subscriber: create run failed",
					"trigger_id", trigger.TriggerID,
					"job_id", trigger.JobID,
					"event_type", e.Type,
					"error", err,
				)
				// Continue processing remaining triggers even if one fails.
			}
		}
		return nil
	}
}

// payloadString safely extracts a string value from a payload map.
// Returns empty string if the key is absent or the value is not a string.
func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	v, ok := payload[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
