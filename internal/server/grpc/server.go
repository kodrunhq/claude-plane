package grpc

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"time"

	pb "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// GRPCServer wraps a gRPC server with mTLS, auth interceptors, and agent service.
type GRPCServer struct {
	*grpc.Server
	connMgr *ConnectionManager
	logger  *slog.Logger
}

// agentService implements the AgentServiceServer interface.
type agentService struct {
	pb.UnimplementedAgentServiceServer
	connMgr *ConnectionManager
	logger  *slog.Logger
}

// NewGRPCServer creates a gRPC server configured with mTLS, keepalive, and auth interceptors.
// If logger is nil, slog.Default() is used.
func NewGRPCServer(tlsCfg *tls.Config, logger *slog.Logger) *GRPCServer {
	if logger == nil {
		logger = slog.Default()
	}

	connMgr := NewConnectionManager()

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
		connMgr: connMgr,
		logger:  logger,
	}
	pb.RegisterAgentServiceServer(srv, svc)

	return &GRPCServer{
		Server:  srv,
		connMgr: connMgr,
		logger:  logger,
	}
}

// Serve starts the gRPC server on the given listener.
// This method blocks until the server is stopped.
func (s *GRPCServer) Serve(lis net.Listener) error {
	s.logger.Info("gRPC server starting", "addr", lis.Addr().String())
	return s.Server.Serve(lis)
}

// ConnectionManager returns the server's connection manager.
func (s *GRPCServer) ConnectionManager() *ConnectionManager {
	return s.connMgr
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
	s.connMgr.Add(machineID, &ConnectedAgent{
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
// It holds the stream open, receiving agent events and (in a future server-core phase) dispatching server commands.
// For now, received events are logged; server-side command dispatch is intentionally not implemented yet.
func (s *agentService) CommandStream(stream grpc.BidiStreamingServer[pb.AgentEvent, pb.ServerCommand]) error {
	machineID, err := MachineIDFromContext(stream.Context())
	if err != nil {
		return err
	}

	// Capture the current stream token so we only remove our own entry on close,
	// not a newer connection from the same agent that reconnected.
	var streamToken uint64
	if agent, ok := s.connMgr.Get(machineID); ok {
		streamToken = agent.StreamToken
	}

	s.logger.Info("agent stream opened", "machine_id", machineID)
	defer func() {
		s.connMgr.RemoveIfToken(machineID, streamToken)
		s.logger.Info("agent stream closed", "machine_id", machineID)
	}()

	// Receive loop: read events from agent until stream closes or error.
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		s.logger.Debug("agent event received", "machine_id", machineID, "event", event)
	}
}
