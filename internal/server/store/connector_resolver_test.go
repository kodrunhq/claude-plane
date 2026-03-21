package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestConnectorConfigResolver_Resolve(t *testing.T) {
	s := newTestStoreForBridge(t)
	userID := createTestUserForBridge(t, s, "resolver@test.com")

	publicCfg := `{"group_id":-1001234567890,"events_topic_id":5}`
	secretCfg := []byte(`{"bot_token":"123456:ABC-DEF"}`)

	created, err := s.CreateConnector(context.Background(), &BridgeConnector{
		ConnectorType: "telegram",
		Name:          "test-tg",
		Enabled:       true,
		Config:        publicCfg,
		CreatedBy:     userID,
	}, secretCfg, testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	resolver := NewConnectorConfigResolver(s, testEncKey)
	result, err := resolver.ResolveConnectorConfig(context.Background(), created.ConnectorID)
	if err != nil {
		t.Fatalf("ResolveConnectorConfig: %v", err)
	}

	var merged map[string]any
	if err := json.Unmarshal([]byte(result), &merged); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if got := merged["bot_token"]; got != "123456:ABC-DEF" {
		t.Errorf("bot_token = %v, want 123456:ABC-DEF", got)
	}
	if got := merged["chat_id"]; got != "-1001234567890" {
		t.Errorf("chat_id = %v, want -1001234567890", got)
	}
	topicID, ok := merged["topic_id"]
	if !ok {
		t.Fatal("topic_id missing from merged config")
	}
	// JSON numbers unmarshal as float64.
	if got, want := topicID.(float64), float64(5); got != want {
		t.Errorf("topic_id = %v, want %v", got, want)
	}
}

func TestConnectorConfigResolver_Resolve_NoTopicID(t *testing.T) {
	s := newTestStoreForBridge(t)
	userID := createTestUserForBridge(t, s, "resolver-notopic@test.com")

	publicCfg := `{"group_id":-1001234567890}`
	secretCfg := []byte(`{"bot_token":"tok"}`)

	created, err := s.CreateConnector(context.Background(), &BridgeConnector{
		ConnectorType: "telegram",
		Name:          "no-topic",
		Enabled:       true,
		Config:        publicCfg,
		CreatedBy:     userID,
	}, secretCfg, testEncKey)
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	resolver := NewConnectorConfigResolver(s, testEncKey)
	result, err := resolver.ResolveConnectorConfig(context.Background(), created.ConnectorID)
	if err != nil {
		t.Fatalf("ResolveConnectorConfig: %v", err)
	}

	var merged map[string]any
	if err := json.Unmarshal([]byte(result), &merged); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if _, ok := merged["topic_id"]; ok {
		t.Error("topic_id should be omitted when events_topic_id is 0")
	}
}

func TestConnectorConfigResolver_Resolve_NotFound(t *testing.T) {
	s := newTestStoreForBridge(t)
	resolver := NewConnectorConfigResolver(s, testEncKey)

	_, err := resolver.ResolveConnectorConfig(context.Background(), "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent connector, got nil")
	}
}
