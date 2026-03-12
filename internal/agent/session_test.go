package agent

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/shared/status"
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
	sess, err := NewSession("test-1", "/bin/echo", []string{"hello"}, "", nil, 24, 80, t.TempDir(), logger)
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
		if sess.Status() == status.Completed {
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
	sess, err := NewSession("test-2", "/bin/cat", nil, "", nil, 24, 80, t.TempDir(), logger)
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
	sess, err := NewSession("test-3a", "/bin/true", nil, "", nil, 24, 80, t.TempDir(), logger)
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

	if sess.Status() != status.Completed {
		t.Errorf("expected status %q for /bin/true, got %q", status.Completed, sess.Status())
	}
	if sess.ExitCode() != 0 {
		t.Errorf("expected exit code 0, got %d", sess.ExitCode())
	}

	// Test failed exit.
	sess2, err := NewSession("test-3b", "/bin/false", nil, "", nil, 24, 80, t.TempDir(), logger)
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

	if sess2.Status() != status.Failed {
		t.Errorf("expected status %q for /bin/false, got %q", status.Failed, sess2.Status())
	}
	if sess2.ExitCode() == 0 {
		t.Error("expected non-zero exit code for /bin/false")
	}
}

func TestSessionResize(t *testing.T) {
	skipIfNopty(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sess, err := NewSession("test-4", "/bin/cat", nil, "", nil, 24, 80, t.TempDir(), logger)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := sess.Resize(40, 120); err != nil {
		t.Errorf("Resize: %v", err)
	}

	_ = sess.Kill("")
}

func TestSessionScrollbackCreated(t *testing.T) {
	skipIfNopty(t)

	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sess, err := NewSession("scroll-1", "/bin/echo", []string{"hello"}, "", nil, 24, 80, dir, logger)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Drain output and wait for exit.
	timeout := time.After(5 * time.Second)
drain:
	for {
		select {
		case _, ok := <-sess.OutputCh():
			if !ok {
				break drain
			}
		case <-timeout:
			t.Fatal("timeout waiting for session to finish")
		}
	}

	// Verify scrollback file exists and is non-empty.
	path := sess.ScrollbackPath()
	expectedPath := filepath.Join(dir, "scroll-1.cast")
	if path != expectedPath {
		t.Errorf("ScrollbackPath: got %q, want %q", path, expectedPath)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("scrollback file does not exist: %v", err)
	}
	if info.Size() == 0 {
		t.Error("scrollback file is empty")
	}
}

func TestSessionScrollbackContent(t *testing.T) {
	skipIfNopty(t)

	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sess, err := NewSession("scroll-2", "/bin/echo", []string{"hello"}, "", nil, 24, 80, dir, logger)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Drain output and wait for exit.
	timeout := time.After(5 * time.Second)
drain:
	for {
		select {
		case _, ok := <-sess.OutputCh():
			if !ok {
				break drain
			}
		case <-timeout:
			t.Fatal("timeout waiting for session to finish")
		}
	}

	// Read scrollback file and verify JSONL structure.
	data, err := os.ReadFile(sess.ScrollbackPath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines (header + output), got %d", len(lines))
	}

	// Verify header.
	var header map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("header not valid JSON: %v", err)
	}
	if header["version"] != float64(2) {
		t.Errorf("header version: got %v, want 2", header["version"])
	}

	// Verify at least one output entry contains "hello".
	foundHello := false
	for _, line := range lines[1:] {
		var entry []interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("entry not valid JSON: %v\nline: %s", err, line)
		}
		if len(entry) == 3 {
			if s, ok := entry[2].(string); ok && strings.Contains(s, "hello") {
				foundHello = true
			}
		}
	}
	if !foundHello {
		t.Error("scrollback file does not contain 'hello' in any output entry")
	}
}

func TestSessionDetachKeepsPTYRunning(t *testing.T) {
	skipIfNopty(t)

	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sess, err := NewSession("detach-1", "/bin/cat", nil, "", nil, 24, 80, dir, logger)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Verify running.
	if sess.Status() != status.Running {
		t.Fatalf("expected status %q, got %q", status.Running, sess.Status())
	}

	// Write input and verify PTY echoes it back (proving it's still alive).
	if err := sess.WriteInput([]byte("still alive\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}

	var output []byte
	timeout := time.After(5 * time.Second)
	for {
		select {
		case data, ok := <-sess.OutputCh():
			if !ok {
				t.Fatal("output channel closed unexpectedly")
			}
			output = append(output, data...)
			if strings.Contains(string(output), "still alive") {
				goto done
			}
		case <-timeout:
			t.Fatalf("timeout waiting for echo, got %q", string(output))
		}
	}
done:

	// PTY still alive.
	if sess.Status() != status.Running {
		t.Errorf("expected status still %q after detach, got %q", status.Running, sess.Status())
	}

	_ = sess.Kill("")
}
