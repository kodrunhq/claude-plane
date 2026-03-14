// Package connector defines the Connector interface that all bridge connectors implement.
// Each connector integrates an external service (e.g. Telegram, Slack) with claude-plane
// by receiving messages and dispatching actions via the REST API.
package connector

import "context"

// Connector is the interface that all bridge connector implementations must satisfy.
//
// Connectors are started with a context derived from the bridge lifecycle.
// They should run until the context is cancelled and clean up resources on return.
// Start must block until the connector has fully shut down.
type Connector interface {
	// Name returns the human-readable identifier for this connector instance.
	// It is used in logs and health reporting.
	Name() string

	// Start runs the connector until ctx is cancelled.
	// Implementations should return nil on clean shutdown and a descriptive error
	// if the connector encounters a fatal failure.
	Start(ctx context.Context) error

	// Healthy reports whether the connector is currently operational.
	// It is called periodically by the bridge health check endpoint.
	Healthy() bool
}
