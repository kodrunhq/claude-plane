package event

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// --- helpers ---

// newTestWSPair creates a connected pair of websocket connections via a test
// HTTP server.  The server-side conn is returned as srvConn; the client-side
// conn as cliConn.  Both are closed when t.Cleanup runs.
func newTestWSPair(t *testing.T) (srvConn, cliConn *websocket.Conn) {
	t.Helper()

	ready := make(chan *websocket.Conn, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("server accept: %v", err)
			return
		}
		ready <- c
	}))
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[4:] // replace "http" with "ws"
	cli, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	select {
	case srv2 := <-ready:
		t.Cleanup(func() { srv2.CloseNow() })
		t.Cleanup(func() { cli.CloseNow() })
		return srv2, cli
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server websocket")
		return nil, nil
	}
}

// readJSONWithTimeout reads a JSON websocket message from conn or fails the test.
func readJSONWithTimeout(t *testing.T, conn *websocket.Conn, d time.Duration, dst any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatalf("unmarshal: %v (raw=%s)", err, data)
	}
}

// startReader drains messages from conn in a background goroutine and returns
// a channel of decoded wsEventMsg values.  The goroutine exits when the conn
// closes or the context is cancelled.
func startReader(t *testing.T, conn *websocket.Conn) <-chan wsEventMsg {
	t.Helper()
	ch := make(chan wsEventMsg, 16)
	go func() {
		defer close(ch)
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var msg wsEventMsg
			if err := json.Unmarshal(data, &msg); err != nil {
				t.Logf("startReader: unmarshal error: %v (raw=%s)", err, data)
				continue
			}
			ch <- msg
		}
	}()
	return ch
}

// --- MatchPattern export ---

func TestMatchPatternExported(t *testing.T) {
	cases := []struct {
		pattern   string
		eventType string
		want      bool
	}{
		{"*", "anything", true},
		{"run.*", "run.created", true},
		{"run.*", "session.started", false},
		{"run.created", "run.created", true},
		{"run.created", "run.started", false},
	}
	for _, tc := range cases {
		got := MatchPattern(tc.pattern, tc.eventType)
		if got != tc.want {
			t.Errorf("MatchPattern(%q, %q) = %v, want %v", tc.pattern, tc.eventType, got, tc.want)
		}
	}
}

// --- clientMatchesEvent ---

func TestClientMatchesEvent(t *testing.T) {
	if !clientMatchesEvent([]string{"run.*", "session.*"}, "run.created") {
		t.Error("expected match for run.created with run.*")
	}
	if !clientMatchesEvent([]string{"run.*", "session.*"}, "session.started") {
		t.Error("expected match for session.started with session.*")
	}
	if clientMatchesEvent([]string{"run.*", "session.*"}, "machine.connected") {
		t.Error("expected no match for machine.connected")
	}
	if !clientMatchesEvent([]string{"*"}, "anything") {
		t.Error("expected match for * wildcard")
	}
}

// --- WSFanout ---

func TestWSFanoutDeliversToClient(t *testing.T) {
	bus := NewBus(nullLogger())
	defer bus.Close()

	fanout := NewWSFanout(bus, nullLogger())
	fanout.Start()
	defer fanout.Close()

	srvConn, cliConn := newTestWSPair(t)
	fanout.AddClient(srvConn, []string{"*"})

	ev := makeEvent(TypeRunCreated)
	if err := bus.Publish(context.Background(), ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	var msg wsEventMsg
	readJSONWithTimeout(t, cliConn, 2*time.Second, &msg)

	if msg.Type != "event" {
		t.Errorf("msg.Type = %q, want %q", msg.Type, "event")
	}
	if msg.EventType != TypeRunCreated {
		t.Errorf("msg.EventType = %q, want %q", msg.EventType, TypeRunCreated)
	}
	if msg.EventID != ev.EventID {
		t.Errorf("msg.EventID = %q, want %q", msg.EventID, ev.EventID)
	}
}

func TestWSFanoutPatternFiltering(t *testing.T) {
	bus := NewBus(nullLogger())
	defer bus.Close()

	fanout := NewWSFanout(bus, nullLogger())
	fanout.Start()
	defer fanout.Close()

	// runClient subscribes only to run.* events.
	runSrvConn, runCliConn := newTestWSPair(t)
	fanout.AddClient(runSrvConn, []string{"run.*"})

	// sessionClient subscribes only to session.* events.
	sessSrvConn, sessCliConn := newTestWSPair(t)
	fanout.AddClient(sessSrvConn, []string{"session.*"})

	// Start non-blocking readers for both clients so we can check what arrives.
	runCh := startReader(t, runCliConn)
	sessCh := startReader(t, sessCliConn)

	// Publish a run event.
	runEv := makeEvent(TypeRunCreated)
	if err := bus.Publish(context.Background(), runEv); err != nil {
		t.Fatalf("Publish run: %v", err)
	}

	// runClient should receive it.
	select {
	case msg := <-runCh:
		if msg.EventType != TypeRunCreated {
			t.Errorf("runClient: got %q, want %q", msg.EventType, TypeRunCreated)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runClient: timed out waiting for run event")
	}

	// sessClient should NOT receive the run event within 200ms.
	select {
	case msg := <-sessCh:
		t.Errorf("sessClient should not receive run event, got %q", msg.EventType)
	case <-time.After(200 * time.Millisecond):
		// Good: no spurious delivery.
	}

	// Publish a session event.
	sessEv := makeEvent(TypeSessionStarted)
	if err := bus.Publish(context.Background(), sessEv); err != nil {
		t.Fatalf("Publish session: %v", err)
	}

	// sessClient should receive it.
	select {
	case msg := <-sessCh:
		if msg.EventType != TypeSessionStarted {
			t.Errorf("sessClient: got %q, want %q", msg.EventType, TypeSessionStarted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sessClient: timed out waiting for session event")
	}
}

func TestWSFanoutRemoveClient(t *testing.T) {
	bus := NewBus(nullLogger())
	defer bus.Close()

	fanout := NewWSFanout(bus, nullLogger())
	fanout.Start()
	defer fanout.Close()

	srvConn, cliConn := newTestWSPair(t)
	fanout.AddClient(srvConn, []string{"*"})
	fanout.RemoveClient(srvConn)

	// Publish after removal — client should receive nothing.
	if err := bus.Publish(context.Background(), makeEvent(TypeRunCreated)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	noMsgCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _, err := cliConn.Read(noMsgCtx)
	if err == nil {
		t.Error("client should not receive event after removal")
	}
}

func TestWSFanoutMultipleClients(t *testing.T) {
	bus := NewBus(nullLogger())
	defer bus.Close()

	fanout := NewWSFanout(bus, nullLogger())
	fanout.Start()
	defer fanout.Close()

	const n = 3
	cliConns := make([]*websocket.Conn, n)
	for i := 0; i < n; i++ {
		srv, cli := newTestWSPair(t)
		fanout.AddClient(srv, []string{"*"})
		cliConns[i] = cli
	}

	ev := makeEvent(TypeMachineConnected)
	if err := bus.Publish(context.Background(), ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	for i, cli := range cliConns {
		var msg wsEventMsg
		readJSONWithTimeout(t, cli, 2*time.Second, &msg)
		if msg.EventType != TypeMachineConnected {
			t.Errorf("client %d: got %q, want %q", i, msg.EventType, TypeMachineConnected)
		}
	}
}

func TestWSFanoutDefaultsToWildcard(t *testing.T) {
	bus := NewBus(nullLogger())
	defer bus.Close()

	fanout := NewWSFanout(bus, nullLogger())
	fanout.Start()
	defer fanout.Close()

	srvConn, cliConn := newTestWSPair(t)
	// Pass empty patterns — should default to ["*"].
	fanout.AddClient(srvConn, nil)

	if err := bus.Publish(context.Background(), makeEvent(TypeRunCreated)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	var msg wsEventMsg
	readJSONWithTimeout(t, cliConn, 2*time.Second, &msg)
	if msg.EventType != TypeRunCreated {
		t.Errorf("got %q, want %q", msg.EventType, TypeRunCreated)
	}
}

// TestWSFanoutClosedClientRemovedAutomatically verifies that writing to a
// closed client connection causes it to be silently removed from the fan-out.
func TestWSFanoutClosedClientRemovedAutomatically(t *testing.T) {
	bus := NewBus(nullLogger())
	defer bus.Close()

	fanout := NewWSFanout(bus, nullLogger())
	fanout.Start()
	defer fanout.Close()

	srvConn, _ := newTestWSPair(t)
	fanout.AddClient(srvConn, []string{"*"})

	// Close the server-side connection directly — subsequent writes will fail.
	srvConn.CloseNow()

	// Publishing should not panic and the failed client should be removed.
	if err := bus.Publish(context.Background(), makeEvent(TypeRunCreated)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Give the fan-out handler time to attempt the write and remove the client.
	// The bus handler is async; wait up to 1s for the client map to drain.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		fanout.mu.RLock()
		count := len(fanout.clients)
		fanout.mu.RUnlock()
		if count == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	fanout.mu.RLock()
	count := len(fanout.clients)
	fanout.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 clients after closed-conn removal, got %d", count)
	}
}

