package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/agent/config"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
	"github.com/kodrunhq/claude-plane/internal/shared/tlsutil"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// SessionProvider is the interface the agent client uses to interact with sessions.
// Implemented by the session manager (wired in Plan 02).
type SessionProvider interface {
	// GetStates returns the current session states for re-registration.
	GetStates() []*pb.SessionState
	// HandleCommand dispatches a server command to the appropriate session.
	HandleCommand(cmd *pb.ServerCommand)
	// StartRelay begins sending session events to the given channel.
	StartRelay(sendCh chan<- *pb.AgentEvent)
	// StopRelay stops the event relay.
	StopRelay()
	// ActiveSessionCount returns the number of active sessions.
	ActiveSessionCount() int32
}

// AgentClient manages the gRPC connection to the server with reconnection.
type AgentClient struct {
	cfg      *config.AgentConfig
	creds    credentials.TransportCredentials
	sessions SessionProvider
	logger   *slog.Logger
	backoff  *Backoff

	mu        sync.Mutex
	conn      *grpc.ClientConn
	connected bool
}

// NewAgentClient creates a new agent client from the given configuration.
// It loads TLS credentials from the config paths.
func NewAgentClient(cfg *config.AgentConfig, sessions SessionProvider, logger *slog.Logger) (*AgentClient, error) {
	if logger == nil {
		logger = slog.Default()
	}

	tlsCfg, err := tlsutil.AgentTLSConfig(cfg.TLS.CACert, cfg.TLS.AgentCert, cfg.TLS.AgentKey)
	if err != nil {
		return nil, fmt.Errorf("load agent TLS config: %w", err)
	}

	return &AgentClient{
		cfg:      cfg,
		creds:    credentials.NewTLS(tlsCfg),
		sessions: sessions,
		logger:   logger,
		backoff:  NewBackoff(1*time.Second, 60*time.Second),
	}, nil
}

// Run enters the reconnect loop, calling connectAndServe repeatedly.
// It returns only when ctx is cancelled.
func (c *AgentClient) Run(ctx context.Context) error {
	for {
		err := c.connectAndServe(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			c.logger.Warn("connection lost, reconnecting", "error", err)
		} else {
			c.logger.Info("connection closed, reconnecting")
		}
		if err := c.waitWithBackoff(ctx); err != nil {
			return err
		}
	}
}

// connectAndServe dials the server, registers, opens the bidi stream, and runs the event loop.
func (c *AgentClient) connectAndServe(ctx context.Context) error {
	conn, err := grpc.NewClient(c.cfg.Server.Address,
		grpc.WithTransportCredentials(c.creds),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return fmt.Errorf("dial server: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	defer func() {
		conn.Close()
		c.mu.Lock()
		c.conn = nil
		c.connected = false
		c.mu.Unlock()
	}()

	client := pb.NewAgentServiceClient(conn)

	// Register with server.
	var existingSessions []*pb.SessionState
	if c.sessions != nil {
		existingSessions = c.sessions.GetStates()
	}

	resp, err := client.Register(ctx, &pb.RegisterRequest{
		MachineId:        c.cfg.Agent.MachineID,
		MaxSessions:      int32(c.cfg.Agent.MaxSessions),
		ExistingSessions: existingSessions,
	})
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	if !resp.Accepted {
		return fmt.Errorf("registration rejected: %s", resp.RejectReason)
	}

	c.logger.Info("registered with server",
		"machine_id", c.cfg.Agent.MachineID,
		"server_version", resp.ServerVersion,
	)

	// Open bidirectional stream.
	stream, err := client.CommandStream(ctx)
	if err != nil {
		return fmt.Errorf("open command stream: %w", err)
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()
	c.backoff.Reset()

	// Sender goroutine: reads from sendCh and calls stream.Send.
	// This prevents concurrent Send calls on the stream.
	sendCh := make(chan *pb.AgentEvent, 64)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range sendCh {
			if err := stream.Send(event); err != nil {
				c.logger.Debug("send error", "error", err)
				return
			}
		}
	}()

	// Start session relay (sends events into sendCh).
	if c.sessions != nil {
		c.sessions.StartRelay(sendCh)
	}

	// Periodic health reporting goroutine.
	healthCtx, healthCancel := context.WithCancel(ctx)
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.sendHealthEvents(healthCtx, sendCh)
	}()
	// Receive loop: dispatch server commands.
	var recvErr error
	for {
		cmd, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			recvErr = err
			break
		}
		if c.sessions != nil {
			c.sessions.HandleCommand(cmd)
		}
	}

	// Cleanup: stop health reporting, relay, close sendCh, wait for goroutines.
	healthCancel()
	if c.sessions != nil {
		c.sessions.StopRelay()
	}
	close(sendCh)
	wg.Wait()

	if recvErr != nil {
		return fmt.Errorf("stream recv: %w", recvErr)
	}
	return nil
}

// waitWithBackoff waits for the backoff duration or ctx cancellation.
func (c *AgentClient) waitWithBackoff(ctx context.Context) error {
	d := c.backoff.Next()
	c.logger.Info("waiting before reconnect", "duration", d)
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sendHealthEvents periodically sends health metrics to the server.
func (c *AgentClient) sendHealthEvents(ctx context.Context, sendCh chan<- *pb.AgentEvent) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Send an initial health event immediately on connect.
	c.emitHealthEvent(sendCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.emitHealthEvent(sendCh)
		}
	}
}

func (c *AgentClient) emitHealthEvent(sendCh chan<- *pb.AgentEvent) {
	var activeSessions int32
	if c.sessions != nil {
		activeSessions = c.sessions.ActiveSessionCount()
	}
	res := CollectResources(activeSessions, int32(c.cfg.Agent.MaxSessions))
	evt := &pb.AgentEvent{
		Event: &pb.AgentEvent_Health{
			Health: &pb.HealthEvent{
				CpuCores:        res.CPUCores,
				MemoryTotalMb:   res.TotalMemoryMB,
				MemoryUsedMb:    res.UsedMemoryMB,
				ActiveSessions:  res.ActiveSessions,
				MaxSessions:     res.MaxSessions,
			},
		},
	}
	select {
	case sendCh <- evt:
	default:
		c.logger.Debug("send channel full, dropping health event")
	}
}

// Connected returns whether the client is currently connected and streaming.
func (c *AgentClient) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}
