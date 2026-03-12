package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	pb "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
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

	// Relay state.
	relayMu     sync.Mutex
	relayCtx    context.Context
	relayCancel context.CancelFunc
	relayWg     sync.WaitGroup
	sendCh      chan<- *pb.AgentEvent
}

// NewSessionManager creates a new session manager.
// dataDir is the directory for scrollback files.
func NewSessionManager(cliPath, dataDir string, logger *slog.Logger) *SessionManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionManager{
		sessions: make(map[string]*Session),
		attached: make(map[string]bool),
		logger:   logger,
		cliPath:  cliPath,
		dataDir:  dataDir,
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
		if sess.Status() != "running" {
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
	if existing, ok := sm.sessions[cmd.GetSessionId()]; ok {
		sm.mu.Unlock()
		sm.logger.Warn("duplicate session ID, killing existing session", "session_id", cmd.GetSessionId())
		_ = existing.Kill("")
		sm.mu.Lock()
	}
	sm.sessions[cmd.GetSessionId()] = sess
	sm.mu.Unlock()

	// Mark new sessions as attached by default so live relay works immediately.
	sm.attachedMu.Lock()
	sm.attached[cmd.GetSessionId()] = true
	sm.attachedMu.Unlock()

	sm.logger.Info("session created", "session_id", cmd.GetSessionId(), "command", command)

	// Send status event.
	sm.sendEvent(&pb.AgentEvent{
		Event: &pb.AgentEvent_SessionStatus{
			SessionStatus: &pb.SessionStatusEvent{
				SessionId: cmd.GetSessionId(),
				Status:    "running",
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
	sess := sm.getSession(cmd.GetSessionId())
	if sess == nil {
		sm.logger.Warn("attach for unknown session", "session_id", cmd.GetSessionId())
		return
	}

	// Send scrollback chunks first to catch up the viewer.
	sm.sendScrollbackChunks(cmd.GetSessionId(), sess.ScrollbackPath())

	// Mark session as attached for live relay.
	sm.attachedMu.Lock()
	sm.attached[cmd.GetSessionId()] = true
	sm.attachedMu.Unlock()

	sm.logger.Info("session attached", "session_id", cmd.GetSessionId())
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
	if sess == nil {
		sm.logger.Warn("scrollback request for unknown session", "session_id", cmd.GetSessionId())
		return
	}

	sm.sendScrollbackChunks(cmd.GetSessionId(), sess.ScrollbackPath())
}

func (sm *SessionManager) sendScrollbackChunks(sessionID, path string) {
	chunks, err := ReadScrollbackChunks(path, 32768) // 32KB chunks
	if err != nil {
		sm.logger.Error("failed to read scrollback", "session_id", sessionID, "error", err)
		return
	}

	for _, chunk := range chunks {
		sm.sendEvent(&pb.AgentEvent{
			Event: &pb.AgentEvent_ScrollbackChunk{
				ScrollbackChunk: &pb.ScrollbackChunkEvent{
					SessionId:  sessionID,
					Data:       chunk.Data,
					Offset:     chunk.Offset,
					IsFinal:    chunk.IsFinal,
					TotalBytes: chunk.Offset + int64(len(chunk.Data)),
				},
			},
		})
	}
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

// startSessionRelay starts a goroutine that reads from the session's output channel
// and forwards to the gRPC send channel. Must be called with relayMu held.
func (sm *SessionManager) startSessionRelay(sess *Session) {
	sm.relayWg.Add(1)
	go func() {
		defer sm.relayWg.Done()
		sessionID := sess.SessionID()

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
					return
				}
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
		sm.logger.Warn("send channel full, dropping event")
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

// Compile-time interface check.
var _ SessionProvider = (*SessionManager)(nil)
