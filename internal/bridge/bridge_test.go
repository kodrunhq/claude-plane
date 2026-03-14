package bridge_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge"
	"github.com/kodrunhq/claude-plane/internal/bridge/state"
)

// mockConnector satisfies the connector.Connector interface for testing.
type mockConnector struct {
	name    string
	started atomic.Bool
	healthy atomic.Bool
}

func (m *mockConnector) Name() string { return m.name }
func (m *mockConnector) Start(ctx context.Context) error {
	m.started.Store(true)
	<-ctx.Done()
	return nil
}
func (m *mockConnector) Healthy() bool { return m.healthy.Load() }

// mockAPIClient satisfies bridge.APIClient for testing.
type mockAPIClient struct {
	restartResult bool
	restartErr    error
	calls         atomic.Int64
}

func (m *mockAPIClient) CheckRestartSignal(_ context.Context, _ time.Time) (bool, error) {
	m.calls.Add(1)
	return m.restartResult, m.restartErr
}

func newTestBridge(t *testing.T, apiClient bridge.APIClient, healthAddr string) *bridge.Bridge {
	t.Helper()
	st := state.New(t.TempDir() + "/state.json")
	return bridge.New(apiClient, st, healthAddr, nil)
}

// TestBridge_ConnectorStarted verifies that Run calls Start on each registered connector.
func TestBridge_ConnectorStarted(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{name: "test-connector"}
	apiClient := &mockAPIClient{restartResult: false}

	b := newTestBridge(t, apiClient, "127.0.0.1:0")
	b.AddConnector(mc)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- b.Run(ctx)
	}()

	// Wait for connector to start.
	deadline := time.Now().Add(time.Second)
	for !mc.started.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if !mc.started.Load() {
		t.Fatal("connector Start was not called within deadline")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

// TestBridge_GracefulShutdown verifies that cancelling ctx causes Run to return nil.
func TestBridge_GracefulShutdown(t *testing.T) {
	t.Parallel()

	apiClient := &mockAPIClient{restartResult: false}
	b := newTestBridge(t, apiClient, "127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- b.Run(ctx)
	}()

	// Allow the bridge to start.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on graceful shutdown, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

// TestBridge_RestartSignal verifies that a restart signal causes Run to return nil.
func TestBridge_RestartSignal(t *testing.T) {
	t.Parallel()

	// Return restart=true immediately so the bridge exits on first poll.
	apiClient := &mockAPIClient{restartResult: true}
	b := newTestBridge(t, apiClient, "127.0.0.1:0")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		// Use a shorter poll interval via the exported test helper.
		done <- b.RunWithInterval(ctx, 50*time.Millisecond)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on restart signal, got: %v", err)
		}
		if apiClient.calls.Load() == 0 {
			t.Fatal("CheckRestartSignal was never called")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after restart signal")
	}
}

// TestBridge_HealthEndpoint verifies the /healthz HTTP endpoint returns connector status.
func TestBridge_HealthEndpoint(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{name: "my-connector"}
	mc.healthy.Store(true)

	apiClient := &mockAPIClient{restartResult: false}

	// Use a random free port by binding to :0 via the test server approach.
	// We pass an explicit addr here so the bridge binds and we can query it.
	srv := httptest.NewServer(nil) // just to discover a free port
	addr := srv.Listener.Addr().String()
	srv.Close()

	b := newTestBridge(t, apiClient, addr)
	b.AddConnector(mc)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- b.RunWithInterval(ctx, time.Hour) // long interval so restart check doesn't interfere
	}()

	// Wait for the health server to start.
	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		resp, err = http.Get("http://" + addr + "/healthz")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if resp == nil {
		t.Fatal("health endpoint did not become available within deadline")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /healthz, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}

	connectors, ok := body["connectors"].([]interface{})
	if !ok {
		t.Fatalf("connectors field missing or wrong type: %T", body["connectors"])
	}
	if len(connectors) != 1 {
		t.Fatalf("expected 1 connector, got %d", len(connectors))
	}

	c0 := connectors[0].(map[string]interface{})
	if c0["name"] != "my-connector" {
		t.Errorf("expected name=my-connector, got %v", c0["name"])
	}
	if c0["healthy"] != true {
		t.Errorf("expected healthy=true, got %v", c0["healthy"])
	}

	cancel()
	<-done
}

// TestBridge_MultipleConnectors verifies all connectors are started.
func TestBridge_MultipleConnectors(t *testing.T) {
	t.Parallel()

	mc1 := &mockConnector{name: "connector-1"}
	mc2 := &mockConnector{name: "connector-2"}
	apiClient := &mockAPIClient{restartResult: false}

	b := newTestBridge(t, apiClient, "127.0.0.1:0")
	b.AddConnector(mc1)
	b.AddConnector(mc2)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- b.Run(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for (!mc1.started.Load() || !mc2.started.Load()) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if !mc1.started.Load() {
		t.Error("connector-1 Start was not called")
	}
	if !mc2.started.Load() {
		t.Error("connector-2 Start was not called")
	}

	cancel()
	<-done
}
