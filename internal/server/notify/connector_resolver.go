package notify

import "context"

// ConnectorResolver resolves a bridge connector ID to a merged notification
// config JSON (bot_token, chat_id, topic_id). Handles secret decryption internally.
type ConnectorResolver interface {
	ResolveConnectorConfig(ctx context.Context, connectorID string) (string, error)
}
