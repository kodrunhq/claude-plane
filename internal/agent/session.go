package agent

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kodrunhq/claude-plane/internal/shared/status"
	"github.com/creack/pty"
)

// Session manages a single PTY-backed process.
type Session struct {
	id        string
	cmd       *exec.Cmd
	ptyFile   *os.File
	outputCh  chan []byte
	readDone  chan struct{} // closed when readLoop exits
	startedAt time.Time

	scrollback     *ScrollbackWriter
	scrollbackPath string

	mu       sync.Mutex
	status   string // status.Running, status.Completed, status.Failed
	exitCode int
	logger   *slog.Logger
}

// NewSession spawns a process in a PTY and starts read/wait goroutines.
// dataDir specifies where scrollback files are stored.
func NewSession(id, command string, args []string, workDir string, envVars map[string]string, rows, cols uint16, dataDir string, logger *slog.Logger) (*Session, error) {
	if logger == nil {
		logger = slog.Default()
	}

	cmd := exec.Command(command, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	// Build a clean environment, stripping CLAUDECODE= env vars to prevent
	// "nested session" detection when spawning Claude CLI processes.
	environ := os.Environ()
	cmd.Env = make([]string, 0, len(environ)+len(envVars)+1)
	hasTERM := false
	for _, e := range environ {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		if strings.HasPrefix(e, "TERM=") {
			hasTERM = true
		}
		cmd.Env = append(cmd.Env, e)
	}
	for k, v := range envVars {
		if k == "TERM" {
			hasTERM = true
		}
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// Default to xterm-256color so CLI tools render Unicode and colors correctly.
	if !hasTERM {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		ptmx.Close()
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	sbPath := filepath.Join(dataDir, id+".cast")
	sb, err := NewScrollbackWriter(sbPath, uint32(cols), uint32(rows))
	if err != nil {
		ptmx.Close()
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("create scrollback writer: %w", err)
	}

	s := &Session{
		id:             id,
		cmd:            cmd,
		ptyFile:        ptmx,
		outputCh:       make(chan []byte, 256),
		readDone:       make(chan struct{}),
		startedAt:      time.Now(),
		status:         status.Running,
		scrollback:     sb,
		scrollbackPath: sbPath,
		logger:         logger.With("session_id", id),
	}

	go s.readLoop()
	go s.waitForExit()

	return s, nil
}

// readLoop reads from the PTY fd in 4096-byte chunks and sends to outputCh.
// readLoop signals readDone when done. waitForExit closes outputCh after status is set.
// All PTY output is also teed to the scrollback writer.
func (s *Session) readLoop() {
	defer close(s.readDone)

	buf := make([]byte, 4096)
	for {
		n, err := s.ptyFile.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])

			// Write to scrollback (log errors but don't stop).
			if s.scrollback != nil {
				if sbErr := s.scrollback.WriteOutput(data); sbErr != nil {
					s.logger.Warn("scrollback write failed", "error", sbErr)
				}
			}

			// Non-blocking send: drop data if channel is full.
			select {
			case s.outputCh <- data:
			default:
				s.logger.Warn("output channel full, dropping data", "bytes", n)
			}
		}
		if err != nil {
			s.logger.Debug("read loop ended", "error", err)
			return
		}
	}
}

// waitForExit waits for the process to exit, sets status and exit code, and closes resources.
func (s *Session) waitForExit() {
	err := s.cmd.Wait()

	s.mu.Lock()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			s.exitCode = exitErr.ExitCode()
			s.status = status.Failed
		} else {
			s.exitCode = -1
			s.status = status.Failed
		}
	} else {
		s.exitCode = 0
		s.status = status.Completed
	}
	s.mu.Unlock()

	// Close PTY fd — this causes readLoop's Read to return an error and exit.
	s.ptyFile.Close()

	// Wait for readLoop to stop sending to outputCh.
	<-s.readDone

	// Close scrollback writer after readLoop finishes (no more writes).
	if s.scrollback != nil {
		if err := s.scrollback.Close(); err != nil {
			s.logger.Warn("scrollback close failed", "error", err)
		}
	}

	// Now safe to close outputCh: readLoop is done, status is already set.
	close(s.outputCh)

	s.logger.Info("session exited", "status", s.status, "exit_code", s.exitCode)
}

// WriteInput writes data to the PTY (stdin of the process).
func (s *Session) WriteInput(data []byte) error {
	_, err := s.ptyFile.Write(data)
	return err
}

// Resize changes the PTY window size.
func (s *Session) Resize(rows, cols uint16) error {
	return pty.Setsize(s.ptyFile, &pty.Winsize{Rows: rows, Cols: cols})
}

// Kill sends a signal to the process. Default is SIGTERM with SIGKILL escalation after 5s.
func (s *Session) Kill(signal string) error {
	if s.cmd.Process == nil {
		return fmt.Errorf("process not started")
	}

	sig := syscall.SIGTERM
	switch signal {
	case "SIGKILL", "9":
		sig = syscall.SIGKILL
	case "SIGINT", "2":
		sig = syscall.SIGINT
	}

	if err := s.cmd.Process.Signal(sig); err != nil {
		return fmt.Errorf("send signal: %w", err)
	}

	// If not SIGKILL, escalate after 5 seconds.
	if sig != syscall.SIGKILL {
		go func() {
			time.Sleep(5 * time.Second)
			s.mu.Lock()
			currStatus := s.status
			s.mu.Unlock()
			if currStatus == status.Running {
				s.logger.Warn("escalating to SIGKILL after timeout")
				_ = s.cmd.Process.Signal(syscall.SIGKILL)
			}
		}()
	}

	return nil
}

// Status returns the current session status.
func (s *Session) Status() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// ExitCode returns the process exit code.
func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

// OutputCh returns the read-only output channel.
func (s *Session) OutputCh() <-chan []byte {
	return s.outputCh
}

// SessionID returns the session identifier.
func (s *Session) SessionID() string {
	return s.id
}

// StartedAt returns when the session was created.
func (s *Session) StartedAt() time.Time {
	return s.startedAt
}

// ScrollbackPath returns the path to the scrollback file.
func (s *Session) ScrollbackPath() string {
	return s.scrollbackPath
}

// ScrollbackOffset returns the current byte offset of the scrollback file.
func (s *Session) ScrollbackOffset() int64 {
	if s.scrollback == nil {
		return 0
	}
	return s.scrollback.CurrentOffset()
}
