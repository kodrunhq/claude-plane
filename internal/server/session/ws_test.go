package session_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/auth"
	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/session"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

const wsTestAttachCommandPollInterval = 10 * time.Millisecond

func setupWSTest(t *testing.T) (*httptest.Server, *session.Registry, *commandRecorder, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cm := connmgr.NewConnectionManager(&mockMachineStore{}, nil)
	reg := session.NewRegistry(slog.Default())
	recorder := &commandRecorder{}

	// Create auth service with real blocklist backed by the store
	bl, err := auth.NewBlocklist(st)
	if err != nil {
		t.Fatalf("NewBlocklist: %v", err)
	}
	authSvc := auth.NewService([]byte("test-secret-key-32-bytes-long!!!"), 15*time.Minute, bl)

	// Create a user and issue token
	user := &store.User{
		UserID:       "test-user",
		Email:        "test@example.com",
		DisplayName:  "Test User",
		PasswordHash: "not-used",
		Role:         "admin",
	}
	if err := st.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := authSvc.IssueToken(user)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	// Create machine and session
	if err := st.UpsertMachine("machine-a", 5); err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	cm.Register("machine-a", &connmgr.ConnectedAgent{
		MachineID:   "machine-a",
		MaxSessions: 5,
		SendCommand: recorder.send,
	})

	sessionID := "test-session-123"
	if err := st.CreateSession(&store.Session{
		SessionID:  sessionID,
		MachineID:  "machine-a",
		UserID:     "test-user",
		Command:    "claude",
		WorkingDir: "/tmp",
		Status:     store.StatusCreated,
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Set up router
	r := chi.NewRouter()
	wsHandler := session.HandleTerminalWS(st, cm, reg, authSvc, slog.Default())
	r.Get("/ws/terminal/{sessionID}", wsHandler)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return srv, reg, recorder, sessionID, token
}

func wsURL(srv *httptest.Server, sessionID string) string {
	return strings.Replace(srv.URL, "http://", "ws://", 1) + "/ws/terminal/" + sessionID
}

// dialWithAuth connects to the WebSocket, performs first-message auth, and
// sends an initial resize (the server waits for a resize before attaching).
func dialWithAuth(t *testing.T, ctx context.Context, srv *httptest.Server, sessionID string, recorder *commandRecorder, token string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.Dial(ctx, wsURL(srv, sessionID), nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	authMsg, _ := json.Marshal(map[string]string{"type": "auth", "token": token})
	if err := conn.Write(ctx, websocket.MessageText, authMsg); err != nil {
		conn.CloseNow()
		t.Fatalf("write auth: %v", err)
	}
	// Send initial resize — the server waits for this before attaching.
	resizeMsg, _ := json.Marshal(map[string]interface{}{"type": "resize", "cols": 80, "rows": 24})
	if err := conn.Write(ctx, websocket.MessageText, resizeMsg); err != nil {
		conn.CloseNow()
		t.Fatalf("write resize: %v", err)
	}
	if recorder != nil {
		waitForAttachCommand(t, ctx, recorder, sessionID)
	}
	return conn
}

func waitForAttachCommand(t *testing.T, ctx context.Context, recorder *commandRecorder, sessionID string) {
	t.Helper()

	ticker := time.NewTicker(wsTestAttachCommandPollInterval)
	defer ticker.Stop()

	for {
		recorder.mu.Lock()
		for _, cmd := range recorder.commands {
			attach := cmd.GetAttachSession()
			if attach != nil && attach.GetSessionId() == sessionID {
				recorder.mu.Unlock()
				return
			}
		}
		recorder.mu.Unlock()

		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for AttachSessionCmd for session %s", sessionID)
		case <-ticker.C:
		}
	}
}

func TestWebSocketBinaryRelay(t *testing.T) {
	srv, reg, recorder, sessionID, token := setupWSTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := dialWithAuth(t, ctx, srv, sessionID, recorder, token)
	defer conn.CloseNow()

	// Give writer goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Publish binary data to registry
	testData := []byte("hello from agent")
	reg.Publish(sessionID, testData)

	// Read from WebSocket
	msgType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	if msgType != websocket.MessageBinary {
		t.Errorf("message type = %v, want Binary", msgType)
	}
	if string(data) != "hello from agent" {
		t.Errorf("data = %q, want %q", data, "hello from agent")
	}
}

func TestWebSocketInputRelay(t *testing.T) {
	srv, _, recorder, sessionID, token := setupWSTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := dialWithAuth(t, ctx, srv, sessionID, recorder, token)
	defer conn.CloseNow()

	// Give time for scrollback request to be sent
	time.Sleep(50 * time.Millisecond)
	initialCount := recorder.count()

	// Send binary data (keystrokes)
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("ls -la\n")); err != nil {
		t.Fatalf("ws write: %v", err)
	}

	// Wait for command to be recorded
	time.Sleep(100 * time.Millisecond)

	newCount := recorder.count()
	if newCount <= initialCount {
		t.Errorf("expected InputDataCmd to be sent, commands before=%d after=%d", initialCount, newCount)
	}

	// Verify the last command is an InputDataCmd
	recorder.mu.Lock()
	lastCmd := recorder.commands[len(recorder.commands)-1]
	recorder.mu.Unlock()

	inputCmd := lastCmd.GetInputData()
	if inputCmd == nil {
		t.Fatal("expected InputDataCmd")
	}
	if string(inputCmd.GetData()) != "ls -la\n" {
		t.Errorf("input data = %q, want %q", inputCmd.GetData(), "ls -la\n")
	}
}

func TestWebSocketResizeMessage(t *testing.T) {
	srv, _, recorder, sessionID, token := setupWSTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := dialWithAuth(t, ctx, srv, sessionID, recorder, token)
	defer conn.CloseNow()

	time.Sleep(50 * time.Millisecond)
	initialCount := recorder.count()

	// Send resize control message
	resizeMsg, _ := json.Marshal(map[string]interface{}{
		"type": "resize",
		"cols": 120,
		"rows": 40,
	})
	if err := conn.Write(ctx, websocket.MessageText, resizeMsg); err != nil {
		t.Fatalf("ws write: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if recorder.count() <= initialCount {
		t.Fatal("expected ResizeTerminalCmd to be sent")
	}

	recorder.mu.Lock()
	lastCmd := recorder.commands[len(recorder.commands)-1]
	recorder.mu.Unlock()

	resizeCmd := lastCmd.GetResizeTerminal()
	if resizeCmd == nil {
		t.Fatal("expected ResizeTerminalCmd")
	}
	if resizeCmd.GetSize().GetCols() != 120 || resizeCmd.GetSize().GetRows() != 40 {
		t.Errorf("resize = %dx%d, want 120x40", resizeCmd.GetSize().GetCols(), resizeCmd.GetSize().GetRows())
	}
}

func TestWebSocketCloseDetaches(t *testing.T) {
	srv, _, recorder, sessionID, token := setupWSTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := dialWithAuth(t, ctx, srv, sessionID, recorder, token)

	time.Sleep(50 * time.Millisecond)
	initialCount := recorder.count()

	// Close WebSocket
	conn.Close(websocket.StatusNormalClosure, "bye")

	// Wait for close to propagate
	time.Sleep(200 * time.Millisecond)

	// Should have sent DetachSessionCmd (not KillSessionCmd)
	if recorder.count() <= initialCount {
		t.Fatal("expected DetachSessionCmd to be sent on close")
	}

	recorder.mu.Lock()
	lastCmd := recorder.commands[len(recorder.commands)-1]
	recorder.mu.Unlock()

	detachCmd := lastCmd.GetDetachSession()
	if detachCmd == nil {
		t.Fatal("expected DetachSessionCmd, got different command type")
	}
	if detachCmd.GetSessionId() != sessionID {
		t.Errorf("session_id = %q, want %q", detachCmd.GetSessionId(), sessionID)
	}
}

func TestWebSocketFlowControl(t *testing.T) {
	srv, reg, recorder, sessionID, token := setupWSTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := dialWithAuth(t, ctx, srv, sessionID, recorder, token)
	defer conn.CloseNow()

	time.Sleep(50 * time.Millisecond)

	// Publish 300 messages rapidly (more than buffer)
	done := make(chan struct{})
	go func() {
		for i := range 300 {
			reg.Publish(sessionID, []byte{byte(i % 256)})
		}
		close(done)
	}()

	// Should not deadlock
	select {
	case <-done:
		// Success: no deadlock
	case <-time.After(3 * time.Second):
		t.Fatal("publishing 300 messages deadlocked")
	}

	// Read some messages to verify WS client receives data
	received := 0
	readCtx, readCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer readCancel()
	for {
		_, _, err := conn.Read(readCtx)
		if err != nil {
			break
		}
		received++
	}

	if received == 0 {
		t.Error("received no messages via WebSocket")
	}
	t.Logf("received %d of 300 messages (some may be dropped by flow control)", received)
}

func TestWebSocketAuthRejection(t *testing.T) {
	srv, _, _, sessionID, _ := setupWSTest(t)

	t.Run("invalid token via first-message", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, _, err := websocket.Dial(ctx, wsURL(srv, sessionID), nil)
		if err != nil {
			t.Fatalf("websocket.Dial: %v", err)
		}
		defer conn.CloseNow()

		authMsg, _ := json.Marshal(map[string]string{"type": "auth", "token": "bad-token"})
		if err := conn.Write(ctx, websocket.MessageText, authMsg); err != nil {
			t.Fatalf("write auth: %v", err)
		}

		_, _, err = conn.Read(ctx)
		if err == nil {
			t.Fatal("expected read error after invalid auth, got nil")
		}
	})

	t.Run("nonexistent session via first-message", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		url := strings.Replace(srv.URL, "http://", "ws://", 1) + "/ws/terminal/nonexistent-session"
		conn, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatalf("websocket.Dial: %v", err)
		}
		defer conn.CloseNow()

		// Even with a valid-format token, session doesn't exist
		authMsg, _ := json.Marshal(map[string]string{"type": "auth", "token": "bad-token"})
		if err := conn.Write(ctx, websocket.MessageText, authMsg); err != nil {
			t.Fatalf("write auth: %v", err)
		}

		_, _, err = conn.Read(ctx)
		if err == nil {
			t.Fatal("expected read error for nonexistent session, got nil")
		}
	})
}

func TestWebSocketFirstMessageAuth(t *testing.T) {
	srv, reg, _, sessionID, token := setupWSTest(t)

	t.Run("valid first-message auth", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		url := strings.Replace(srv.URL, "http://", "ws://", 1) + "/ws/terminal/" + sessionID
		conn, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatalf("websocket.Dial: %v", err)
		}
		defer conn.CloseNow()

		// Send auth as first message
		authMsg, _ := json.Marshal(map[string]string{"type": "auth", "token": token})
		if err := conn.Write(ctx, websocket.MessageText, authMsg); err != nil {
			t.Fatalf("write auth: %v", err)
		}

		// Send initial resize — the server waits for this before attaching.
		resizeMsg, _ := json.Marshal(map[string]interface{}{"type": "resize", "cols": 80, "rows": 24})
		if err := conn.Write(ctx, websocket.MessageText, resizeMsg); err != nil {
			t.Fatalf("write resize: %v", err)
		}

		// Give server time to process auth, resize, and start relay
		time.Sleep(200 * time.Millisecond)

		// Publish data and verify we receive it (proves auth succeeded)
		reg.Publish(sessionID, []byte("auth-ok"))
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if msgType != websocket.MessageBinary {
			t.Errorf("message type = %v, want Binary", msgType)
		}
		if string(data) != "auth-ok" {
			t.Errorf("data = %q, want %q", data, "auth-ok")
		}
	})

	t.Run("invalid first-message auth", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		url := strings.Replace(srv.URL, "http://", "ws://", 1) + "/ws/terminal/" + sessionID
		conn, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatalf("websocket.Dial: %v", err)
		}
		defer conn.CloseNow()

		// Send invalid auth
		authMsg, _ := json.Marshal(map[string]string{"type": "auth", "token": "bad-token"})
		if err := conn.Write(ctx, websocket.MessageText, authMsg); err != nil {
			t.Fatalf("write auth: %v", err)
		}

		// Server should close the connection
		_, _, err = conn.Read(ctx)
		if err == nil {
			t.Fatal("expected read error after invalid auth, got nil")
		}
	})

	t.Run("no auth message sent", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		url := strings.Replace(srv.URL, "http://", "ws://", 1) + "/ws/terminal/" + sessionID
		conn, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatalf("websocket.Dial: %v", err)
		}
		defer conn.CloseNow()

		// Don't send auth — server should close after 5s timeout
		_, _, err = conn.Read(ctx)
		if err == nil {
			t.Fatal("expected read error after auth timeout, got nil")
		}
	})
}
