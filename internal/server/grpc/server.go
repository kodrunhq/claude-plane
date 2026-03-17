package grpc

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/ingest"
	"github.com/kodrunhq/claude-plane/internal/server/session"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
	"github.com/kodrunhq/claude-plane/internal/shared/status"
	"github.com/kodrunhq/claude-plane/internal/shared/version"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// GRPCServer wraps a gRPC server with mTLS, auth interceptors, and agent service.
type GRPCServer struct {
	*grpc.Server
	streams      *StreamRegistry
	agentConnMgr *connmgr.ConnectionManager
	agentSvc     *agentService
	logger       *slog.Logger
}

// SessionStore is the interface the gRPC server uses to persist session status changes.
type SessionStore interface {
	UpdateSessionStatus(id, status string) error
	UpdateSessionStatusIfNotTerminal(id, status string) error
}

// RunStepLookup maps session IDs to run step IDs. Used to determine where
// to persist task values received from agents.
type RunStepLookup interface {
	RunStepIDForSession(sessionID string) (runStepID string, found bool)
}

// TaskValueStore persists task values extracted by agents.
type TaskValueStore interface {
	SetTaskValue(ctx context.Context, runStepID, key, value string) error
}

// StepIdleHandler handles StepIdleEvent from agents, signalling that a shared
// session step has returned to the idle prompt.
type StepIdleHandler interface {
	OnStepIdle(sessionID string)
}

// cleanupStore is the subset of store methods needed for pending cleanup dispatch.
type cleanupStore interface {
	ListPendingCleanups(ctx context.Context, machineID string) ([]string, error)
	DeletePendingCleanups(ctx context.Context, machineID string) error
}

// agentService implements the AgentServiceServer interface.
type agentService struct {
	pb.UnimplementedAgentServiceServer
	streams         *StreamRegistry
	agentConnMgr    *connmgr.ConnectionManager
	registry        *session.Registry
	sessionStore    SessionStore
	eventPublisher  event.Publisher
	runStepLookup   RunStepLookup
	taskValueStore  TaskValueStore
	stepIdleHandler StepIdleHandler
	cleanupStore    cleanupStore
	ingestor        *ingest.ContentIngestor
	logger          *slog.Logger
}

// NewGRPCServer creates a gRPC server configured with mTLS, keepalive, and auth interceptors.
// The agentConnMgr parameter provides DB-backed connection tracking; it may be nil
// during tests or when DB-backed tracking is not needed.
// If logger is nil, slog.Default() is used.
func NewGRPCServer(tlsCfg *tls.Config, agentConnMgr *connmgr.ConnectionManager, logger *slog.Logger) *GRPCServer {
	if logger == nil {
		logger = slog.Default()
	}

	streams := NewStreamRegistry()

	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsCfg)),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             15 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.ChainUnaryInterceptor(MachineAuthUnaryInterceptor()),
		grpc.ChainStreamInterceptor(MachineAuthStreamInterceptor()),
	)

	svc := &agentService{
		streams:      streams,
		agentConnMgr: agentConnMgr,
		logger:       logger,
	}
	pb.RegisterAgentServiceServer(srv, svc)

	return &GRPCServer{
		Server:       srv,
		streams:      streams,
		agentConnMgr: agentConnMgr,
		agentSvc:     svc,
		logger:       logger,
	}
}

// SetRegistry sets the session registry for routing agent events to WebSocket subscribers.
func (s *GRPCServer) SetRegistry(r *session.Registry) {
	s.agentSvc.registry = r
}

// SetSessionStore sets the session store for persisting session status changes.
func (s *GRPCServer) SetSessionStore(store SessionStore) {
	s.agentSvc.sessionStore = store
}

// SetRunStepLookup sets the lookup used to map session IDs to run step IDs
// for task value persistence.
func (s *GRPCServer) SetRunStepLookup(lookup RunStepLookup) {
	s.agentSvc.runStepLookup = lookup
}

// SetTaskValueStore sets the store used to persist task values from agents.
func (s *GRPCServer) SetTaskValueStore(store TaskValueStore) {
	s.agentSvc.taskValueStore = store
}

// SetStepIdleHandler sets the handler for StepIdleEvent from agents.
func (s *GRPCServer) SetStepIdleHandler(handler StepIdleHandler) {
	s.agentSvc.stepIdleHandler = handler
}

// SetCleanupStore sets the store used for dispatching pending cleanups on agent reconnect.
func (s *GRPCServer) SetCleanupStore(cs cleanupStore) {
	s.agentSvc.cleanupStore = cs
}

// SetEventPublisher sets the event bus publisher for broadcasting session lifecycle
// events (terminated, exited). These events feed the /ws/events endpoint, which the
// frontend uses to invalidate cached session lists.
func (s *GRPCServer) SetEventPublisher(p event.Publisher) {
	s.agentSvc.eventPublisher = p
}

// SetContentIngestor sets the content ingestor for search indexing.
func (s *GRPCServer) SetContentIngestor(ci *ingest.ContentIngestor) {
	s.agentSvc.ingestor = ci
}

// Serve starts the gRPC server on the given listener.
// This method blocks until the server is stopped.
func (s *GRPCServer) Serve(lis net.Listener) error {
	s.logger.Info("gRPC server starting", "addr", lis.Addr().String())
	return s.Server.Serve(lis)
}

// StreamRegistry returns the server's in-memory stream registry
// used for tracking active gRPC streams and their tokens.
func (s *GRPCServer) StreamRegistry() *StreamRegistry {
	return s.streams
}

// AgentConnectionManager returns the DB-backed connection manager
// used for agent status tracking.
func (s *GRPCServer) AgentConnectionManager() *connmgr.ConnectionManager {
	return s.agentConnMgr
}

// Register handles an agent registration request.
// It extracts the machine-id from the interceptor-enriched context,
// validates the request, and adds the agent to the connection manager.
func (s *agentService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	machineID, err := MachineIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	s.logger.Info("agent registered",
		"machine_id", machineID,
		"max_sessions", req.GetMaxSessions(),
		"existing_sessions", len(req.GetExistingSessions()),
	)

	token := NextStreamToken()
	s.streams.Add(machineID, &StreamEntry{
		MachineID:     machineID,
		StreamToken:   token,
		MaxSessions:   req.GetMaxSessions(),
		ConnectedAt:   time.Now(),
		SessionStates: req.GetExistingSessions(),
	})

	return &pb.RegisterResponse{
		Accepted:      true,
		ServerVersion: version.Version,
	}, nil
}

// CommandStream handles the bidirectional streaming RPC.
// It registers the agent with the DB-backed connection manager on stream start
// and disconnects on stream end. Received events are logged; server-side command
// dispatch is intentionally not implemented yet.
func (s *agentService) CommandStream(stream grpc.BidiStreamingServer[pb.AgentEvent, pb.ServerCommand]) error {
	machineID, err := MachineIDFromContext(stream.Context())
	if err != nil {
		return err
	}

	// Capture the current stream token so we only remove our own entry on close,
	// not a newer connection from the same agent that reconnected.
	var streamToken uint64
	if entry, ok := s.streams.Get(machineID); ok {
		streamToken = entry.StreamToken
	}

	// Create a cancellable context for this stream so replacement connections
	// can cancel the old stream's receive loop.
	ctx, cancel := context.WithCancel(stream.Context())

	// Build a thread-safe SendCommand function using a mutex to protect stream.Send.
	var sendMu sync.Mutex
	sendCommand := func(cmd *pb.ServerCommand) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return stream.Send(cmd)
	}

	// Register with the DB-backed connection manager if available.
	if s.agentConnMgr != nil {
		var maxSessions int32
		if entry, ok := s.streams.Get(machineID); ok {
			maxSessions = entry.MaxSessions
		}
		ca := &connmgr.ConnectedAgent{
			MachineID:    machineID,
			RegisteredAt: time.Now(),
			MaxSessions:  maxSessions,
			Cancel:       cancel,
			Stream:       stream,
			SendCommand:  sendCommand,
		}
		if regErr := s.agentConnMgr.Register(machineID, ca); regErr != nil {
			cancel()
			s.logger.Error("failed to register agent with connection manager",
				"machine_id", machineID, "error", regErr)
			return regErr
		}
	}

	// Dispatch any pending scrollback cleanups queued while the agent was offline.
	if s.cleanupStore != nil && s.agentConnMgr != nil {
		agent := s.agentConnMgr.GetAgent(machineID)
		if agent != nil {
			go func() {
				cleanups, err := s.cleanupStore.ListPendingCleanups(ctx, machineID)
				if err != nil {
					s.logger.Warn("failed to list pending cleanups", "error", err, "machine_id", machineID)
					return
				}
				if len(cleanups) == 0 {
					return
				}
				for _, sessionID := range cleanups {
					cmd := &pb.ServerCommand{
						Command: &pb.ServerCommand_CleanupScrollback{
							CleanupScrollback: &pb.CleanupScrollbackCmd{SessionId: sessionID},
						},
					}
					if err := agent.SendCommand(cmd); err != nil {
						s.logger.Warn("failed to send pending cleanup", "error", err, "machine_id", machineID)
						return
					}
				}
				if err := s.cleanupStore.DeletePendingCleanups(ctx, machineID); err != nil {
					s.logger.Warn("failed to delete pending cleanups", "error", err, "machine_id", machineID)
				}
				s.logger.Info("sent pending cleanups to agent", "machine_id", machineID, "count", len(cleanups))
			}()
		}
	}

	s.logger.Info("agent stream opened", "machine_id", machineID)
	defer func() {
		s.streams.RemoveIfToken(machineID, streamToken)
		if s.agentConnMgr != nil && ctx.Err() == nil {
			s.agentConnMgr.Disconnect(machineID)
		}
		cancel()
		s.logger.Info("agent stream closed", "machine_id", machineID)
	}()

	// Receive loop: run Recv in a goroutine so ctx cancellation (from a
	// replacement connection) can terminate the loop even when Recv is blocked.
	type recvResult struct {
		event *pb.AgentEvent
		err   error
	}
	recvCh := make(chan recvResult, 1)

	// Single goroutine for receiving — exits when stream closes or context cancels
	go func() {
		for {
			event, err := stream.Recv()
			select {
			case recvCh <- recvResult{event, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res := <-recvCh:
			if res.err == io.EOF {
				return nil
			}
			if res.err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				s.logger.Error("stream receive error",
					"machine_id", machineID,
					"error", res.err,
				)
				return res.err
			}
			s.logger.Debug("agent event received", "machine_id", machineID, "event", res.event)

			// Route terminal events to session registry for WebSocket forwarding
			if s.registry != nil {
				if out := res.event.GetSessionOutput(); out != nil {
					s.registry.Publish(out.GetSessionId(), out.GetData())
					// Tee output to content ingestor for search indexing
					if s.ingestor != nil {
						s.ingestor.Ingest(out.GetSessionId(), out.GetData())
					}
				}
				if sc := res.event.GetScrollbackChunk(); sc != nil {
					// Scrollback data is asciicast v2 JSONL. Parse each line
					// to extract raw terminal output, skipping the header.
					termData := parseAsciicastData(sc.GetData())
					if len(termData) > 0 {
						s.registry.Publish(sc.GetSessionId(), termData)
					}
					if sc.GetIsFinal() {
						s.registry.PublishControl(sc.GetSessionId(), []byte(`{"type":"scrollback_end"}`))
					}
				}
			}

			// Handle health events — store in the connected agent's in-memory state.
			if health := res.event.GetHealth(); health != nil {
				if s.agentConnMgr != nil {
					if agent := s.agentConnMgr.GetAgent(machineID); agent != nil {
						agent.UpdateHealth(&connmgr.HealthInfo{
							CPUCores:       health.GetCpuCores(),
							MemoryTotalMB:  health.GetMemoryTotalMb(),
							MemoryUsedMB:   health.GetMemoryUsedMb(),
							ActiveSessions: health.GetActiveSessions(),
							MaxSessions:    health.GetMaxSessions(),
						})
					}
				}
			}

			// Handle session lifecycle events to update DB status.
			if ss := res.event.GetSessionStatus(); ss != nil {
				newStatus := ss.GetStatus()
				if s.sessionStore != nil {
					if err := s.sessionStore.UpdateSessionStatus(ss.GetSessionId(), newStatus); err != nil {
						s.logger.Warn("failed to update session status from agent event",
							"session_id", ss.GetSessionId(), "status", newStatus, "error", err)
					}
				}
				// Notify any connected browser WebSocket that the session status changed.
				// This lets the frontend show "Session ended" instead of a dead terminal.
				if s.registry != nil && (newStatus == status.Terminated || newStatus == status.Failed || newStatus == status.Completed) {
					controlMsg := []byte(`{"type":"session_ended","status":"` + newStatus + `"}`)
					s.registry.PublishControl(ss.GetSessionId(), controlMsg)
				}
				// Publish to event bus so /ws/events subscribers (sessions list) get invalidated.
				if s.eventPublisher != nil {
					var evType string
					switch newStatus {
					case status.Terminated:
						evType = event.TypeSessionTerminated
					case status.Failed, status.Completed:
						evType = event.TypeSessionExited
					}
					if evType != "" {
						if err := s.eventPublisher.Publish(ctx, event.NewSessionEvent(evType, ss.GetSessionId(), machineID, "", "")); err != nil {
							s.logger.Warn("failed to publish session event", "type", evType, "error", err)
						}
					}
				}
			}
			if se := res.event.GetSessionExit(); se != nil {
				exitStatus := status.Completed
				if se.GetExitCode() != 0 {
					exitStatus = status.Failed
				}
				if s.sessionStore != nil {
					// Only update if not already in a terminal state (e.g., user-initiated "terminated").
					if err := s.sessionStore.UpdateSessionStatusIfNotTerminal(se.GetSessionId(), exitStatus); err != nil {
						s.logger.Warn("failed to update session status on exit",
							"session_id", se.GetSessionId(), "exit_code", se.GetExitCode(), "error", err)
					}
				}
				s.logger.Info("session exit event",
					"machine_id", machineID,
					"session_id", se.GetSessionId(),
					"exit_code", se.GetExitCode(),
				)
				// Publish to event bus so sessions list auto-refreshes.
				if s.eventPublisher != nil {
					if err := s.eventPublisher.Publish(ctx, event.NewSessionEvent(event.TypeSessionExited, se.GetSessionId(), machineID, "", "")); err != nil {
						s.logger.Warn("failed to publish session exit event", "error", err)
					}
				}
				// Flush content ingestor for this session
				if s.ingestor != nil {
					s.ingestor.FlushSession(se.GetSessionId())
				}
			}

			// Handle task values from agent — persist to the run step's value store.
			if tv := res.event.GetTaskValues(); tv != nil {
				if s.runStepLookup != nil && s.taskValueStore != nil {
					if runStepID, ok := s.runStepLookup.RunStepIDForSession(tv.GetSessionId()); ok {
						for k, v := range tv.GetValues() {
							if err := s.taskValueStore.SetTaskValue(ctx, runStepID, k, v); err != nil {
								s.logger.Warn("failed to store task value",
									"session_id", tv.GetSessionId(),
									"run_step_id", runStepID,
									"key", k,
									"error", err,
								)
							}
						}
						s.logger.Info("task values persisted",
							"machine_id", machineID,
							"session_id", tv.GetSessionId(),
							"run_step_id", runStepID,
							"count", len(tv.GetValues()),
						)
					} else {
						s.logger.Debug("task values for unknown session (no run step mapping)",
							"session_id", tv.GetSessionId(),
						)
					}
				}
			}

			// Handle step idle events from shared sessions — signal step completion.
			if si := res.event.GetStepIdle(); si != nil {
				if s.stepIdleHandler != nil {
					s.stepIdleHandler.OnStepIdle(si.GetSessionId())
				}
			}
		}
	}
}

const (
	defaultScannerBufSize = 64 * 1024  // 64 KiB
	maxScannerBufSize     = 1024 * 1024 // 1 MiB
)

// parseAsciicastData extracts raw terminal output bytes from asciicast v2 JSONL data.
// It skips the header line ({"version":2,...}) and parses each event line
// [timestamp, "o", "data"] to extract the data field.
func parseAsciicastData(raw []byte) []byte {
	var buf bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	// Asciicast lines can be large (e.g. big output bursts); raise the
	// default 64 KiB token limit to 1 MiB to avoid silent truncation.
	scanner.Buffer(make([]byte, 0, defaultScannerBufSize), maxScannerBufSize)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Skip the header object (starts with '{')
		if line[0] == '{' {
			continue
		}
		// Event lines are JSON arrays: [timestamp, "o", "data"]
		if line[0] != '[' {
			continue
		}
		var entry []json.RawMessage
		if err := json.Unmarshal(line, &entry); err != nil || len(entry) < 3 {
			continue
		}
		var data string
		if err := json.Unmarshal(entry[2], &data); err != nil {
			continue
		}
		buf.WriteString(data)
	}
	if err := scanner.Err(); err != nil {
		// Log would require passing a logger; return what we have so far.
		// Partial data is better than nothing for scrollback replay.
		return buf.Bytes()
	}
	return buf.Bytes()
}
