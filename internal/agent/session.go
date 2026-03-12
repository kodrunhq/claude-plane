package agent

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Session manages a single PTY-backed process.
type Session struct {
	id        string
	cmd       *exec.Cmd
	ptyFile   *os.File
	outputCh  chan []byte
	startedAt time.Time

	mu       sync.Mutex
	status   string // "running", "completed", "failed"
	exitCode int
	logger   *slog.Logger
}

// NewSession spawns a process in a PTY and starts read/wait goroutines.
func NewSession(id, command string, args []string, workDir string, envVars map[string]string, rows, cols uint16, logger *slog.Logger) (*Session, error) {
	if logger == nil {
		logger = slog.Default()
	}

	cmd := exec.Command(command, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = os.Environ()
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}

	s := &Session{
		id:        id,
		cmd:       cmd,
		ptyFile:   ptmx,
		outputCh:  make(chan []byte, 256),
		startedAt: time.Now(),
		status:    "running",
		logger:    logger.With("session_id", id),
	}

	go s.readLoop()
	go s.waitForExit()

	return s, nil
}

// readLoop reads from the PTY fd in 4096-byte chunks and sends to outputCh.
func (s *Session) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptyFile.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
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
			s.status = "failed"
		} else {
			s.exitCode = -1
			s.status = "failed"
		}
	} else {
		s.exitCode = 0
		s.status = "completed"
	}
	s.mu.Unlock()

	// Close PTY fd (stops readLoop).
	s.ptyFile.Close()

	// Close output channel to signal consumers.
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
			status := s.status
			s.mu.Unlock()
			if status == "running" {
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
