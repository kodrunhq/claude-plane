package grpc

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/session"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"

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

// agentService implements the AgentServiceServer interface.
type agentService struct {
	pb.UnimplementedAgentServiceServer
	streams      *StreamRegistry
	agentConnMgr *connmgr.ConnectionManager
	registry     *session.Registry
	logger       *slog.Logger
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
		ServerVersion: "0.1.0",
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
				}
				if sc := res.event.GetScrollbackChunk(); sc != nil {
					s.registry.Publish(sc.GetSessionId(), sc.GetData())
					if sc.GetIsFinal() {
						s.registry.PublishControl(sc.GetSessionId(), []byte(`{"type":"scrollback_end"}`))
					}
				}
			}
		}
	}
}
