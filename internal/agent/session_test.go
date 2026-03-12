package agent

import (
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func skipIfNopty(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTY tests not supported on Windows")
	}
	// Verify PTY allocation actually works on this system.
	ptmx, err := pty.Start(exec.Command("/bin/true"))
	if ptmx != nil {
		ptmx.Close()
	}
	if err != nil {
		t.Skipf("PTY allocation unavailable: %v", err)
	}
}

func TestSessionSpawnAndRead(t *testing.T) {
	skipIfNopty(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sess, err := NewSession("test-1", "/bin/echo", []string{"hello"}, "", nil, 24, 80, logger)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	var output []byte
	timeout := time.After(5 * time.Second)
	for {
		select {
		case data, ok := <-sess.OutputCh():
			if !ok {
				goto done
			}
			output = append(output, data...)
			if strings.Contains(string(output), "hello") {
				goto done
			}
		case <-timeout:
			t.Fatal("timeout waiting for output")
		}
	}
done:

	if !strings.Contains(string(output), "hello") {
		t.Errorf("expected output to contain 'hello', got %q", string(output))
	}

	// Wait for session to complete.
	deadline := time.After(5 * time.Second)
	for {
		if sess.Status() == "completed" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for status 'completed', got %q", sess.Status())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestSessionWriteInput(t *testing.T) {
	skipIfNopty(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sess, err := NewSession("test-2", "/bin/cat", nil, "", nil, 24, 80, logger)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Write input to cat.
	if err := sess.WriteInput([]byte("test\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}

	var output []byte
	timeout := time.After(5 * time.Second)
	for {
		select {
		case data, ok := <-sess.OutputCh():
			if !ok {
				t.Fatal("output channel closed before receiving echo")
			}
			output = append(output, data...)
			if strings.Contains(string(output), "test") {
				goto done
			}
		case <-timeout:
			t.Fatalf("timeout waiting for echo, got %q", string(output))
		}
	}
done:

	if !strings.Contains(string(output), "test") {
		t.Errorf("expected output to contain 'test', got %q", string(output))
	}

	// Clean up: kill cat.
	_ = sess.Kill("")
}

func TestSessionExitStatus(t *testing.T) {
	skipIfNopty(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Test successful exit.
	sess, err := NewSession("test-3a", "/bin/true", nil, "", nil, 24, 80, logger)
	if err != nil {
		t.Fatalf("NewSession /bin/true: %v", err)
	}

	// Drain output channel until closed.
	timeout := time.After(5 * time.Second)
drainTrue:
	for {
		select {
		case _, ok := <-sess.OutputCh():
			if !ok {
				break drainTrue
			}
		case <-timeout:
			t.Fatal("timeout waiting for /bin/true to finish")
		}
	}

	if sess.Status() != "completed" {
		t.Errorf("expected status 'completed' for /bin/true, got %q", sess.Status())
	}
	if sess.ExitCode() != 0 {
		t.Errorf("expected exit code 0, got %d", sess.ExitCode())
	}

	// Test failed exit.
	sess2, err := NewSession("test-3b", "/bin/false", nil, "", nil, 24, 80, logger)
	if err != nil {
		t.Fatalf("NewSession /bin/false: %v", err)
	}

	timeout2 := time.After(5 * time.Second)
drainFalse:
	for {
		select {
		case _, ok := <-sess2.OutputCh():
			if !ok {
				break drainFalse
			}
		case <-timeout2:
			t.Fatal("timeout waiting for /bin/false to finish")
		}
	}

	if sess2.Status() != "failed" {
		t.Errorf("expected status 'failed' for /bin/false, got %q", sess2.Status())
	}
	if sess2.ExitCode() == 0 {
		t.Error("expected non-zero exit code for /bin/false")
	}
}

func TestSessionResize(t *testing.T) {
	skipIfNopty(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sess, err := NewSession("test-4", "/bin/cat", nil, "", nil, 24, 80, logger)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := sess.Resize(40, 120); err != nil {
		t.Errorf("Resize: %v", err)
	}

	_ = sess.Kill("")
}
