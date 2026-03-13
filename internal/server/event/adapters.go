// Package event — adapters.go provides adapter types that bridge the store
// package types to the event package interfaces, avoiding import cycles.
// (store imports event; event must not import store.)
//
// The adapters accept function closures so that the caller (cmd/server/main.go)
// can provide the translation between store types and event types inline,
// without the event package needing to import the store package.
package event

import (
	"context"
)

// ---- Webhook store adapter --------------------------------------------------

// WebhookStoreFuncs satisfies WebhookStore via injected function closures.
// Create one in the wiring layer with the concrete store wired in.
type WebhookStoreFuncs struct {
	ListWebhooksFn      func(ctx context.Context) ([]Webhook, error)
	CreateDeliveryFn    func(ctx context.Context, d WebhookDelivery) error
	UpdateDeliveryFn    func(ctx context.Context, d WebhookDelivery) error
	PendingDeliveriesFn func(ctx context.Context) ([]WebhookDelivery, error)
	GetEventByIDFn      func(ctx context.Context, eventID string) (*Event, error)
}

func (f *WebhookStoreFuncs) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	return f.ListWebhooksFn(ctx)
}
func (f *WebhookStoreFuncs) CreateDelivery(ctx context.Context, d WebhookDelivery) error {
	return f.CreateDeliveryFn(ctx, d)
}
func (f *WebhookStoreFuncs) UpdateDelivery(ctx context.Context, d WebhookDelivery) error {
	return f.UpdateDeliveryFn(ctx, d)
}
func (f *WebhookStoreFuncs) PendingDeliveries(ctx context.Context) ([]WebhookDelivery, error) {
	return f.PendingDeliveriesFn(ctx)
}
func (f *WebhookStoreFuncs) GetEventByID(ctx context.Context, eventID string) (*Event, error) {
	return f.GetEventByIDFn(ctx, eventID)
}

// ---- Trigger store adapter --------------------------------------------------

// TriggerStoreFuncs satisfies TriggerStore via injected function closures.
type TriggerStoreFuncs struct {
	ListEnabledTriggersFn func(ctx context.Context) ([]JobTrigger, error)
}

func (f *TriggerStoreFuncs) ListEnabledTriggers(ctx context.Context) ([]JobTrigger, error) {
	return f.ListEnabledTriggersFn(ctx)
}

// ---- Orchestrator adapter ---------------------------------------------------

// OrchestratorFuncs satisfies OrchestratorIface via an injected function closure.
type OrchestratorFuncs struct {
	CreateRunFn func(ctx context.Context, jobID string, triggerType string) error
}

func (f *OrchestratorFuncs) CreateRun(ctx context.Context, jobID string, triggerType string) error {
	return f.CreateRunFn(ctx, jobID, triggerType)
}
