package agent

import (
	"log/slog"
	"os"
	"testing"
	"time"

	pb "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
)

func TestSessionManagerCreate(t *testing.T) {
	skipIfNopty(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sm := NewSessionManager("", logger)

	sm.HandleCommand(&pb.ServerCommand{
		Command: &pb.ServerCommand_CreateSession{
			CreateSession: &pb.CreateSessionCmd{
				SessionId: "s1",
				Command:   "/bin/echo",
				Args:      []string{"hello"},
				TerminalSize: &pb.TerminalSize{
					Rows: 24,
					Cols: 80,
				},
			},
		},
	})

	// Give it a moment to start.
	time.Sleep(100 * time.Millisecond)

	states := sm.GetStates()
	if len(states) == 0 {
		t.Fatal("expected at least 1 session in GetStates")
	}

	found := false
	for _, s := range states {
		if s.GetSessionId() == "s1" {
			found = true
		}
	}
	if !found {
		t.Error("session s1 not found in GetStates")
	}
}

func TestSessionManagerInput(t *testing.T) {
	skipIfNopty(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sm := NewSessionManager("", logger)

	sm.HandleCommand(&pb.ServerCommand{
		Command: &pb.ServerCommand_CreateSession{
			CreateSession: &pb.CreateSessionCmd{
				SessionId: "s2",
				Command:   "/bin/cat",
				TerminalSize: &pb.TerminalSize{
					Rows: 24,
					Cols: 80,
				},
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	sm.HandleCommand(&pb.ServerCommand{
		Command: &pb.ServerCommand_InputData{
			InputData: &pb.InputDataCmd{
				SessionId: "s2",
				Data:      []byte("hello\n"),
			},
		},
	})

	// Read directly from session output channel.
	sm.mu.RLock()
	sess := sm.sessions["s2"]
	sm.mu.RUnlock()

	if sess == nil {
		t.Fatal("session s2 not found")
	}

	timeout := time.After(5 * time.Second)
	var output []byte
	for {
		select {
		case data, ok := <-sess.OutputCh():
			if !ok {
				goto done
			}
			output = append(output, data...)
			if len(output) > 0 {
				goto done
			}
		case <-timeout:
			t.Fatal("timeout waiting for input echo")
		}
	}
done:

	// Clean up.
	sm.HandleCommand(&pb.ServerCommand{
		Command: &pb.ServerCommand_KillSession{
			KillSession: &pb.KillSessionCmd{
				SessionId: "s2",
			},
		},
	})
}

func TestSessionManagerRelay(t *testing.T) {
	skipIfNopty(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sm := NewSessionManager("", logger)

	sendCh := make(chan *pb.AgentEvent, 64)
	sm.StartRelay(sendCh)

	sm.HandleCommand(&pb.ServerCommand{
		Command: &pb.ServerCommand_CreateSession{
			CreateSession: &pb.CreateSessionCmd{
				SessionId: "s3",
				Command:   "/bin/echo",
				Args:      []string{"relay-test"},
				TerminalSize: &pb.TerminalSize{
					Rows: 24,
					Cols: 80,
				},
			},
		},
	})

	// Expect status event ("running") and output event.
	var gotOutput, gotStatus bool
	timeout := time.After(5 * time.Second)

	for !gotOutput || !gotStatus {
		select {
		case evt, ok := <-sendCh:
			if !ok {
				t.Fatal("sendCh closed unexpectedly")
			}
			if evt.GetSessionOutput() != nil {
				gotOutput = true
			}
			if evt.GetSessionStatus() != nil {
				gotStatus = true
			}
		case <-timeout:
			t.Fatalf("timeout waiting for relay events (gotOutput=%v, gotStatus=%v)", gotOutput, gotStatus)
		}
	}

	sm.StopRelay()
}

func TestSessionManagerConcurrent(t *testing.T) {
	skipIfNopty(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sm := NewSessionManager("", logger)

	sendCh := make(chan *pb.AgentEvent, 256)
	sm.StartRelay(sendCh)

	done := make(chan struct{})
	for i := 0; i < 5; i++ {
		go func(n int) {
			id := "concurrent-" + string(rune('a'+n))
			sm.HandleCommand(&pb.ServerCommand{
				Command: &pb.ServerCommand_CreateSession{
					CreateSession: &pb.CreateSessionCmd{
						SessionId: id,
						Command:   "/bin/echo",
						Args:      []string{"test"},
						TerminalSize: &pb.TerminalSize{
							Rows: 24,
							Cols: 80,
						},
					},
				},
			})
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 5; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent session creation")
		}
	}

	// Drain sendCh.
	sm.StopRelay()

	states := sm.GetStates()
	if len(states) < 5 {
		t.Errorf("expected 5 sessions, got %d", len(states))
	}
}

func TestSessionManagerGetStates(t *testing.T) {
	skipIfNopty(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sm := NewSessionManager("", logger)

	for _, id := range []string{"gs1", "gs2"} {
		sm.HandleCommand(&pb.ServerCommand{
			Command: &pb.ServerCommand_CreateSession{
				CreateSession: &pb.CreateSessionCmd{
					SessionId: id,
					Command:   "/bin/cat",
					TerminalSize: &pb.TerminalSize{
						Rows: 24,
						Cols: 80,
					},
				},
			},
		})
	}

	time.Sleep(100 * time.Millisecond)

	states := sm.GetStates()
	if len(states) != 2 {
		t.Fatalf("expected 2 sessions in GetStates, got %d", len(states))
	}

	ids := map[string]bool{}
	for _, s := range states {
		ids[s.GetSessionId()] = true
	}
	for _, id := range []string{"gs1", "gs2"} {
		if !ids[id] {
			t.Errorf("session %s not found in GetStates", id)
		}
	}

	// Clean up.
	for _, id := range []string{"gs1", "gs2"} {
		sm.HandleCommand(&pb.ServerCommand{
			Command: &pb.ServerCommand_KillSession{
				KillSession: &pb.KillSessionCmd{
					SessionId: id,
				},
			},
		})
	}
}
