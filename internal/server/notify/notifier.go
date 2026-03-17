// Package notify provides notification delivery for the claude-plane server.
// Notifiers send messages to configured channels (email, Telegram, etc.).
// The Dispatcher subscribes to the event bus and fans out to matching notifiers.
package notify

import "context"

// Notifier sends a notification to a specific channel type.
type Notifier interface {
	// Send delivers a notification using the given channel configuration.
	Send(ctx context.Context, channelConfig string, subject, body string) error
	// Type returns the channel type this notifier handles (e.g. "email", "telegram").
	Type() string
}
