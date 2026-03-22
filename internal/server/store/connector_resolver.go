package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// ConnectorConfigResolver implements notify.ConnectorResolver by looking up a
// bridge connector and merging its public config with the decrypted secret.
// The encryption key is held here so the notify package never sees it.
type ConnectorConfigResolver struct {
	store  *Store
	encKey []byte
}

// NewConnectorConfigResolver returns a resolver that decrypts connector secrets
// using encKey.
func NewConnectorConfigResolver(store *Store, encKey []byte) *ConnectorConfigResolver {
	return &ConnectorConfigResolver{store: store, encKey: encKey}
}

// ResolveConnectorConfig fetches the connector identified by connectorID,
// decrypts its secret, and returns a merged JSON object containing:
//
//	bot_token  (string)  — from the encrypted secret
//	chat_id    (string)  — stringified group_id from the public config
//	topic_id   (int)     — events_topic_id from the public config (omitted when 0)
func (r *ConnectorConfigResolver) ResolveConnectorConfig(ctx context.Context, connectorID string) (string, error) {
	connector, secretJSON, err := r.store.GetConnectorWithSecret(ctx, connectorID, r.encKey)
	if err != nil {
		return "", fmt.Errorf("resolve connector %s: %w", connectorID, err)
	}

	// Parse public config for group_id and events_topic_id.
	var publicCfg struct {
		GroupID       int64 `json:"group_id"`
		EventsTopicID int   `json:"events_topic_id"`
	}
	if connector.Config != "" {
		if err := json.Unmarshal([]byte(connector.Config), &publicCfg); err != nil {
			return "", fmt.Errorf("parse connector public config: %w", err)
		}
	}

	// Parse secret for bot_token.
	var secretCfg struct {
		BotToken string `json:"bot_token"`
	}
	if len(secretJSON) > 0 {
		if err := json.Unmarshal(secretJSON, &secretCfg); err != nil {
			return "", fmt.Errorf("parse connector secret config: %w", err)
		}
	}

	// Build merged result.
	merged := map[string]any{
		"bot_token": secretCfg.BotToken,
		"chat_id":   strconv.FormatInt(publicCfg.GroupID, 10),
	}
	if publicCfg.EventsTopicID > 0 {
		merged["topic_id"] = publicCfg.EventsTopicID
	}

	out, err := json.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("marshal merged connector config: %w", err)
	}
	return string(out), nil
}
