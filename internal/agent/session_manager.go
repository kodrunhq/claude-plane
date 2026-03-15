package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/shared/status"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SessionManager is a thread-safe registry of PTY sessions that implements
// the SessionProvider interface from client.go.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	logger   *slog.Logger
	cliPath  string
	dataDir  string

	// Attach state: tracks which sessions are attached for live relay.
	attachedMu sync.RWMutex
	attached   map[string]bool

	// Task type tracking: remembers each session's task type for exit handling.
	taskTypeMu sync.RWMutex
	taskTypes  map[string]string // session_id -> "shell" | "claude_session"

	// Per-session output buffers for task value extraction.
	// For shared sessions, the buffer is drained on each StepIdleEvent
	// so that task values are attributed to the correct step.
	outputBufMu sync.Mutex
	outputBufs  map[string]*[]byte // session_id -> output buffer (pointer for in-place updates from relay goroutine)

	// Relay state.
	relayMu     sync.Mutex
	relayCtx    context.Context
	relayCancel context.CancelFunc
	relayWg     sync.WaitGroup
	sendCh      chan<- *pb.AgentEvent

	// Idle detector options passed to each session's IdleDetector.
	idleDetectorOpts []IdleDetectorOption
}

// NewSessionManager creates a new session manager.
// dataDir is the directory for scrollback files.
// Optional IdleDetectorOption values configure prompt detection for all sessions.
func NewSessionManager(cliPath, dataDir string, logger *slog.Logger, idleOpts ...IdleDetectorOption) *SessionManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionManager{
		sessions:         make(map[string]*Session),
		attached:         make(map[string]bool),
		taskTypes:        make(map[string]string),
		outputBufs:       make(map[string]*[]byte),
		logger:           logger,
		cliPath:          cliPath,
		dataDir:          dataDir,
		idleDetectorOpts: idleOpts,
	}
}

// GetStates returns the current state of all sessions (for re-registration).
func (sm *SessionManager) GetStates() []*pb.SessionState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	states := make([]*pb.SessionState, 0, len(sm.sessions))
	for _, sess := range sm.sessions {
		state := &pb.SessionState{
			SessionId: sess.SessionID(),
			Status:    sess.Status(),
			StartedAt: timestamppb.New(sess.StartedAt()),
		}
		if sess.Status() != status.Running {
			ec := int32(sess.ExitCode())
			state.ExitCode = &ec
		}
		states = append(states, state)
	}
	return states
}

// HandleCommand dispatches a server command to the appropriate session.
func (sm *SessionManager) HandleCommand(cmd *pb.ServerCommand) {
	switch c := cmd.GetCommand().(type) {
	case *pb.ServerCommand_CreateSession:
		sm.handleCreate(c.CreateSession)
	case *pb.ServerCommand_InputData:
		sm.handleInput(c.InputData)
	case *pb.ServerCommand_ResizeTerminal:
		sm.handleResize(c.ResizeTerminal)
	case *pb.ServerCommand_KillSession:
		sm.handleKill(c.KillSession)
	case *pb.ServerCommand_AttachSession:
		sm.handleAttach(c.AttachSession)
	case *pb.ServerCommand_DetachSession:
		sm.handleDetach(c.DetachSession)
	case *pb.ServerCommand_RequestScrollback:
		sm.handleRequestScrollback(c.RequestScrollback)
	case *pb.ServerCommand_CleanupScrollback:
		sm.handleCleanupScrollback(c.CleanupScrollback)
	default:
		sm.logger.Warn("unknown command type", "command", cmd)
	}
}

func (sm *SessionManager) handleCreate(cmd *pb.CreateSessionCmd) {
	var rows, cols uint16 = 24, 80
	if ts := cmd.GetTerminalSize(); ts != nil {
		const maxUint16 = uint32(^uint16(0))
		if r := ts.GetRows(); r >= 1 && r <= maxUint16 {
			rows = uint16(r)
		}
		if c := ts.GetCols(); c >= 1 && c <= maxUint16 {
			cols = uint16(c)
		}
	}

	command := cmd.GetCommand()
	if command == "" {
		command = sm.cliPath
		if command == "" {
			command = "claude"
		}
	}

	sess, err := NewSession(
		cmd.GetSessionId(),
		command,
		cmd.GetArgs(),
		cmd.GetWorkingDir(),
		cmd.GetEnvVars(),
		rows, cols,
		sm.dataDir,
		sm.logger,
	)
	if err != nil {
		sm.logger.Error("failed to create session", "session_id", cmd.GetSessionId(), "error", err)
		return
	}

	sm.mu.Lock()
	existing, hadExisting := sm.sessions[cmd.GetSessionId()]
	sm.sessions[cmd.GetSessionId()] = sess
	sm.mu.Unlock()

	// Kill existing session outside the lock (safe — we already replaced the map entry)
	if hadExisting {
		sm.logger.Warn("duplicate session ID, killing existing session", "session_id", cmd.GetSessionId())
		_ = existing.Kill("")
	}

	// Mark new sessions as attached by default so live relay works immediately.
	sm.attachedMu.Lock()
	sm.attached[cmd.GetSessionId()] = true
	sm.attachedMu.Unlock()

	// Track task type for exit-time handling (task value extraction).
	taskType := cmd.GetTaskType()
	if taskType == "" {
		taskType = "claude_session"
	}
	sm.taskTypeMu.Lock()
	sm.taskTypes[cmd.GetSessionId()] = taskType
	sm.taskTypeMu.Unlock()

	sm.logger.Info("session created", "session_id", cmd.GetSessionId(), "command", command, "task_type", taskType)

	// For Claude sessions with an initial prompt, set up an IdleDetector that
	// watches for Claude CLI's startup prompt (❯) to submit the prompt at
	// exactly the right time, then watches for the completion prompt to either
	// send /exit (normal) or emit a StepIdleEvent (keep-alive shared sessions).
	// Shell tasks skip this entirely.
	if prompt := cmd.GetInitialPrompt(); taskType != "shell" && prompt != "" {
		sessionID := cmd.GetSessionId()
		keepAlive := cmd.GetKeepAlive()

		// onReady: CLI startup prompt detected — submit the initial prompt.
		onReady := func() {
			input := []byte(prompt + "\r")
			if err := sess.WriteInput(input); err != nil {
				sm.logger.Error("failed to write initial prompt",
					"session_id", sessionID,
					"error", err,
				)
			} else {
				sm.logger.Info("initial prompt submitted",
					"session_id", sessionID,
					"prompt_len", len(prompt),
				)
			}
		}

		var onIdle func()
		if keepAlive {
			// Keep-alive (shared session): extract task values from the output
			// accumulated since the last extraction, then signal step completion
			// to the server. The session stays alive for subsequent prompts.
			onIdle = func() {
				sm.logger.Info("idle prompt detected, extracting task values and sending StepIdleEvent (keep-alive)",
					"session_id", sessionID,
				)
				// Extract and send task values for this step before signalling idle.
				sm.extractAndSendStepTaskValues(sessionID)
				sm.sendEvent(&pb.AgentEvent{
					Event: &pb.AgentEvent_StepIdle{
						StepIdle: &pb.StepIdleEvent{
							SessionId: sessionID,
						},
					},
				})
			}
		} else {
			// Normal mode: send /exit to gracefully terminate the session.
			onIdle = func() {
				sm.logger.Info("idle prompt detected, sending /exit",
					"session_id", sessionID,
				)
				if err := sess.WriteInput([]byte("/exit\r")); err != nil {
					sm.logger.Error("failed to send /exit after idle",
						"session_id", sessionID,
						"error", err,
					)
				}
			}
		}

		opts := make([]IdleDetectorOption, len(sm.idleDetectorOpts))
		copy(opts, sm.idleDetectorOpts)
		if keepAlive {
			opts = append(opts, WithKeepAlive(true))
		}

		detector := NewIdleDetector(onReady, onIdle, opts...)
		detector.Start()
		sess.SetOutputObserver(detector.Feed)
	}

	// Send status event.
	sm.sendEvent(&pb.AgentEvent{
		Event: &pb.AgentEvent_SessionStatus{
			SessionStatus: &pb.SessionStatusEvent{
				SessionId: cmd.GetSessionId(),
				Status:    status.Running,
			},
		},
	})

	// Start relay goroutine if relay is active.
	sm.relayMu.Lock()
	if sm.sendCh != nil {
		sm.startSessionRelay(sess)
	}
	sm.relayMu.Unlock()
}

func (sm *SessionManager) handleInput(cmd *pb.InputDataCmd) {
	sess := sm.getSession(cmd.GetSessionId())
	if sess == nil {
		sm.logger.Warn("input for unknown session", "session_id", cmd.GetSessionId())
		return
	}
	if err := sess.WriteInput(cmd.GetData()); err != nil {
		sm.logger.Error("write input failed", "session_id", cmd.GetSessionId(), "error", err)
	}
}

func (sm *SessionManager) handleResize(cmd *pb.ResizeTerminalCmd) {
	sess := sm.getSession(cmd.GetSessionId())
	if sess == nil {
		sm.logger.Warn("resize for unknown session", "session_id", cmd.GetSessionId())
		return
	}
	size := cmd.GetSize()
	if size == nil {
		return
	}
	const maxUint16 = uint32(^uint16(0))
	r, c := size.GetRows(), size.GetCols()
	if r < 1 || r > maxUint16 || c < 1 || c > maxUint16 {
		sm.logger.Warn("invalid resize dimensions", "session_id", cmd.GetSessionId(), "rows", r, "cols", c)
		return
	}
	if err := sess.Resize(uint16(r), uint16(c)); err != nil {
		sm.logger.Error("resize failed", "session_id", cmd.GetSessionId(), "error", err)
	}
}

func (sm *SessionManager) handleKill(cmd *pb.KillSessionCmd) {
	sess := sm.getSession(cmd.GetSessionId())
	if sess == nil {
		sm.logger.Warn("kill for unknown session", "session_id", cmd.GetSessionId())
		return
	}
	if err := sess.Kill(cmd.GetSignal()); err != nil {
		sm.logger.Error("kill failed", "session_id", cmd.GetSessionId(), "error", err)
	}
}

func (sm *SessionManager) handleAttach(cmd *pb.AttachSessionCmd) {
	sessionID := cmd.GetSessionId()
	sess := sm.getSession(sessionID)

	if sess != nil {
		// Live session: send scrollback then enable live relay.
		sm.sendScrollbackChunks(sessionID, sess.ScrollbackPath())

		sm.attachedMu.Lock()
		sm.attached[sessionID] = true
		sm.attachedMu.Unlock()

		sm.logger.Info("session attached", "session_id", sessionID)
		return
	}

	// Session no longer in memory (already exited). Try to replay scrollback
	// from the persisted .cast file on disk.
	scrollbackPath := filepath.Join(sm.dataDir, sessionID+".cast")
	sm.logger.Info("session not in memory, replaying scrollback from disk",
		"session_id", sessionID,
		"path", scrollbackPath,
	)
	sm.sendScrollbackChunks(sessionID, scrollbackPath)
}

func (sm *SessionManager) handleDetach(cmd *pb.DetachSessionCmd) {
	sess := sm.getSession(cmd.GetSessionId())
	if sess == nil {
		sm.logger.Warn("detach for unknown session", "session_id", cmd.GetSessionId())
		return
	}

	// Mark session as detached — stop live relay but keep PTY running.
	sm.attachedMu.Lock()
	sm.attached[cmd.GetSessionId()] = false
	sm.attachedMu.Unlock()

	sm.logger.Info("session detached", "session_id", cmd.GetSessionId())
}

func (sm *SessionManager) handleRequestScrollback(cmd *pb.RequestScrollbackCmd) {
	sess := sm.getSession(cmd.GetSessionId())
	var scrollbackPath string
	if sess != nil {
		scrollbackPath = sess.ScrollbackPath()
	} else {
		// Session may have been cleaned up after exit. Try the default path.
		scrollbackPath = filepath.Join(sm.dataDir, cmd.GetSessionId()+".cast")
	}

	sm.sendScrollbackChunks(cmd.GetSessionId(), scrollbackPath)
}

func (sm *SessionManager) handleCleanupScrollback(cmd *pb.CleanupScrollbackCmd) {
	sessionID := cmd.GetSessionId()
	castPath := filepath.Join(sm.dataDir, sessionID+".cast")
	if err := os.Remove(castPath); err != nil {
		if !os.IsNotExist(err) {
			sm.logger.Warn("failed to delete scrollback file",
				slog.String("session_id", sessionID),
				slog.String("path", castPath),
				slog.String("error", err.Error()),
			)
		}
		return
	}
	sm.logger.Info("deleted scrollback file",
		slog.String("session_id", sessionID),
		slog.String("path", castPath),
	)
}

func (sm *SessionManager) sendScrollbackChunks(sessionID, path string) {
	f, err := os.Open(path)
	if err != nil {
		sm.logger.Error("failed to open scrollback", "session_id", sessionID, "error", err)
		// Send final marker so clients don't get stuck in "replaying" state.
		sm.sendFinalScrollbackMarker(sessionID, 0)
		return
	}
	defer f.Close()

	const chunkSize = 32768
	buf := make([]byte, chunkSize)
	var offset int64

	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			sm.sendEvent(&pb.AgentEvent{
				Event: &pb.AgentEvent_ScrollbackChunk{
					ScrollbackChunk: &pb.ScrollbackChunkEvent{
						SessionId:  sessionID,
						Data:       data,
						Offset:     offset,
						TotalBytes: offset + int64(n),
					},
				},
			})
			offset += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			sm.logger.Error("failed to read scrollback chunk", "session_id", sessionID, "error", readErr)
			break
		}
	}

	// Always send a final marker so clients can transition to live mode.
	sm.sendFinalScrollbackMarker(sessionID, offset)
}

func (sm *SessionManager) sendFinalScrollbackMarker(sessionID string, offset int64) {
	sm.sendEvent(&pb.AgentEvent{
		Event: &pb.AgentEvent_ScrollbackChunk{
			ScrollbackChunk: &pb.ScrollbackChunkEvent{
				SessionId:  sessionID,
				Offset:     offset,
				TotalBytes: offset,
				IsFinal:    true,
			},
		},
	})
}

func (sm *SessionManager) isAttached(sessionID string) bool {
	sm.attachedMu.RLock()
	defer sm.attachedMu.RUnlock()
	return sm.attached[sessionID]
}

func (sm *SessionManager) getSession(id string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

// removeSession deletes a session from the sessions, attached, taskTypes, and outputBufs maps.
func (sm *SessionManager) removeSession(sessionID string) {
	sm.mu.Lock()
	delete(sm.sessions, sessionID)
	sm.mu.Unlock()

	sm.attachedMu.Lock()
	delete(sm.attached, sessionID)
	sm.attachedMu.Unlock()

	sm.taskTypeMu.Lock()
	delete(sm.taskTypes, sessionID)
	sm.taskTypeMu.Unlock()

	sm.outputBufMu.Lock()
	delete(sm.outputBufs, sessionID)
	sm.outputBufMu.Unlock()
}

// StartRelay begins sending session output events to sendCh.
func (sm *SessionManager) StartRelay(sendCh chan<- *pb.AgentEvent) {
	sm.relayMu.Lock()
	defer sm.relayMu.Unlock()

	sm.sendCh = sendCh
	sm.relayCtx, sm.relayCancel = context.WithCancel(context.Background())

	// Start relay for existing sessions.
	sm.mu.RLock()
	for _, sess := range sm.sessions {
		sm.startSessionRelay(sess)
	}
	sm.mu.RUnlock()
}

// maxOutputCapture is the maximum amount of raw output to retain in memory
// for task value extraction on session exit.
const maxOutputCapture = 64 * 1024

// startSessionRelay starts a goroutine that reads from the session's output channel
// and forwards to the gRPC send channel. Must be called with relayMu held.
func (sm *SessionManager) startSessionRelay(sess *Session) {
	// Register a shared output buffer for this session so that the onIdle
	// callback (for shared sessions) can drain it for per-step task values.
	sessionID := sess.SessionID()
	outputBuf := make([]byte, 0)
	sm.outputBufMu.Lock()
	sm.outputBufs[sessionID] = &outputBuf
	sm.outputBufMu.Unlock()

	sm.relayWg.Add(1)
	go func() {
		defer sm.relayWg.Done()

		for {
			select {
			case <-sm.relayCtx.Done():
				return
			case data, ok := <-sess.OutputCh():
				if !ok {
					// Channel closed: session exited. Send final status.
					sm.sendEvent(&pb.AgentEvent{
						Event: &pb.AgentEvent_SessionStatus{
							SessionStatus: &pb.SessionStatusEvent{
								SessionId: sessionID,
								Status:    sess.Status(),
							},
						},
					})

					// Extract and send task values before exit event.
					sm.outputBufMu.Lock()
					bufSnapshot := make([]byte, len(*sm.outputBufs[sessionID]))
					copy(bufSnapshot, *sm.outputBufs[sessionID])
					sm.outputBufMu.Unlock()
					sm.extractAndSendTaskValues(sessionID, bufSnapshot)

					// Send exit event with exit code so the server can persist the final status.
					sm.sendEvent(&pb.AgentEvent{
						Event: &pb.AgentEvent_SessionExit{
							SessionExit: &pb.SessionExitEvent{
								SessionId: sessionID,
								ExitCode:  int32(sess.ExitCode()),
								ExitedAt:  timestamppb.Now(),
							},
						},
					})
					// Clean up maps to prevent leak.
					sm.removeSession(sessionID)
					return
				}

				// Accumulate output for task value extraction (keep last maxOutputCapture bytes).
				sm.outputBufMu.Lock()
				if bp := sm.outputBufs[sessionID]; bp != nil {
					*bp = appendCapped(*bp, data, maxOutputCapture)
				}
				sm.outputBufMu.Unlock()

				// Only relay live output if session is attached.
				if sm.isAttached(sessionID) {
					sm.sendEvent(&pb.AgentEvent{
						Event: &pb.AgentEvent_SessionOutput{
							SessionOutput: &pb.SessionOutputEvent{
								SessionId: sessionID,
								Data:      data,
								Timestamp: float64(time.Now().UnixMilli()) / 1000.0,
							},
						},
					})
				}
			}
		}
	}()
}

// appendCapped appends data to buf, keeping only the last maxSize bytes.
func appendCapped(buf, data []byte, maxSize int) []byte {
	buf = append(buf, data...)
	if len(buf) > maxSize {
		buf = buf[len(buf)-maxSize:]
	}
	return buf
}

// extractAndSendStepTaskValues drains the per-session output buffer and sends
// task values for the current step. Used by shared (keep-alive) sessions on
// idle to attribute task values to the correct step before completion.
func (sm *SessionManager) extractAndSendStepTaskValues(sessionID string) {
	sm.outputBufMu.Lock()
	bp := sm.outputBufs[sessionID]
	if bp == nil || len(*bp) == 0 {
		sm.outputBufMu.Unlock()
		return
	}
	// Drain: snapshot the buffer and reset it.
	bufSnapshot := make([]byte, len(*bp))
	copy(bufSnapshot, *bp)
	*bp = (*bp)[:0]
	sm.outputBufMu.Unlock()

	sm.extractAndSendTaskValues(sessionID, bufSnapshot)
}

// extractAndSendTaskValues parses task value markers from session output and
// sends a TaskValuesEvent if any values were found. For shell tasks, the last
// 32 KB of stdout is also captured as an automatic "stdout" value.
func (sm *SessionManager) extractAndSendTaskValues(sessionID string, outputBuf []byte) {
	if len(outputBuf) == 0 {
		return
	}

	outputStr := string(outputBuf)
	vals := ParseTaskValues(outputStr)

	// For shell tasks, capture stdout as an automatic value.
	sm.taskTypeMu.RLock()
	taskType := sm.taskTypes[sessionID]
	sm.taskTypeMu.RUnlock()

	if taskType == "shell" && len(vals) < maxTaskValueCount {
		stdout := outputStr
		if len(stdout) > maxTaskValueSize {
			stdout = stdout[len(stdout)-maxTaskValueSize:]
		}
		if vals == nil {
			vals = make(map[string]string)
		}
		vals["stdout"] = stdout
	}

	if len(vals) == 0 {
		return
	}

	sm.logger.Info("extracted task values",
		"session_id", sessionID,
		"count", len(vals),
		"task_type", taskType,
	)

	sm.sendEvent(&pb.AgentEvent{
		Event: &pb.AgentEvent_TaskValues{
			TaskValues: &pb.TaskValuesEvent{
				SessionId: sessionID,
				Values:    vals,
			},
		},
	})
}

// sendEvent sends an event to the send channel if relay is active.
func (sm *SessionManager) sendEvent(evt *pb.AgentEvent) {
	sm.relayMu.Lock()
	ch := sm.sendCh
	sm.relayMu.Unlock()

	if ch == nil {
		return
	}

	select {
	case ch <- evt:
	default:
		// Log event type for debugging which events are being lost.
		var eventType string
		switch e := evt.GetEvent().(type) {
		case *pb.AgentEvent_SessionOutput:
			eventType = "session_output"
			sm.logger.Warn("send channel full, dropping event",
				"event_type", eventType,
				"session_id", e.SessionOutput.GetSessionId(),
				"bytes", len(e.SessionOutput.GetData()),
			)
		case *pb.AgentEvent_SessionStatus:
			eventType = "session_status"
			sm.logger.Warn("send channel full, dropping event",
				"event_type", eventType,
				"session_id", e.SessionStatus.GetSessionId(),
				"status", e.SessionStatus.GetStatus(),
			)
		default:
			sm.logger.Warn("send channel full, dropping event",
				"event_type", fmt.Sprintf("%T", evt.GetEvent()),
			)
		}
	}
}

// StopRelay stops the output relay and waits for goroutines to finish.
func (sm *SessionManager) StopRelay() {
	sm.relayMu.Lock()
	if sm.relayCancel != nil {
		sm.relayCancel()
	}
	sm.relayMu.Unlock()

	sm.relayWg.Wait()

	sm.relayMu.Lock()
	sm.sendCh = nil
	sm.relayMu.Unlock()
}

// ActiveSessionCount returns the number of active sessions.
func (sm *SessionManager) ActiveSessionCount() int32 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return int32(len(sm.sessions))
}

// Compile-time interface check.
var _ SessionProvider = (*SessionManager)(nil)
