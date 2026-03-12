package agent_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/claudeplane/claude-plane/internal/agent"
	"github.com/claudeplane/claude-plane/internal/agent/config"
	pb "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
	"github.com/claudeplane/claude-plane/internal/shared/tlsutil"

	servergrpc "github.com/claudeplane/claude-plane/internal/server/grpc"
)

// noopSessionProvider is a no-op SessionProvider for tests.
type noopSessionProvider struct{}

func (n *noopSessionProvider) GetStates() []*pb.SessionState        { return nil }
func (n *noopSessionProvider) HandleCommand(cmd *pb.ServerCommand)   {}
func (n *noopSessionProvider) StartRelay(sendCh chan<- *pb.AgentEvent) {}
func (n *noopSessionProvider) StopRelay()                            {}

// setupTestEnv generates CA, server, and agent certs and returns paths + config.
func setupTestEnv(t *testing.T, machineID string) (caDir, serverDir string, agentCfg *config.AgentConfig) {
	t.Helper()
	base := t.TempDir()

	caDir = filepath.Join(base, "ca")
	serverDir = filepath.Join(base, "server")
	agentDir := filepath.Join(base, "agent")

	for _, d := range []string{caDir, serverDir, agentDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := tlsutil.GenerateCA(caDir); err != nil {
		t.Fatal(err)
	}
	if err := tlsutil.IssueServerCert(caDir, serverDir, nil); err != nil {
		t.Fatal(err)
	}
	if err := tlsutil.IssueAgentCert(caDir, agentDir, machineID); err != nil {
		t.Fatal(err)
	}

	agentCfg = &config.AgentConfig{
		Server: config.ServerConnConfig{Address: ""}, // set after server starts
		TLS: config.TLSConfig{
			CACert:    filepath.Join(caDir, "ca.pem"),
			AgentCert: filepath.Join(agentDir, "agent.pem"),
			AgentKey:  filepath.Join(agentDir, "agent-key.pem"),
		},
		Agent: config.AgentSettings{
			MachineID:   "nuc-01",
			MaxSessions: 5,
		},
	}

	return caDir, serverDir, agentCfg
}

// startServer creates and starts a test gRPC server, returning its address.
func startServer(t *testing.T, caDir, serverDir string) (addr string, srv *servergrpc.GRPCServer, stop func()) {
	t.Helper()

	tlsCfg, err := tlsutil.ServerTLSConfig(
		filepath.Join(caDir, "ca.pem"),
		filepath.Join(serverDir, "server.pem"),
		filepath.Join(serverDir, "server-key.pem"),
	)
	if err != nil {
		t.Fatal(err)
	}

	srv = servergrpc.NewGRPCServer(tlsCfg, nil)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		_ = srv.Serve(lis)
	}()

	return lis.Addr().String(), srv, func() { srv.Stop() }
}

func TestAgentConnectsAndRegisters(t *testing.T) {
	caDir, serverDir, agentCfg := setupTestEnv(t, "agent-nuc-01")
	addr, srv, stop := startServer(t, caDir, serverDir)
	defer stop()

	agentCfg.Server.Address = addr

	client, err := agent.NewAgentClient(agentCfg, &noopSessionProvider{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Run connectAndServe in background with a timeout context.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use Run in a goroutine; cancel context after verifying registration.
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx)
	}()

	// Wait a bit for registration to complete, then verify.
	time.Sleep(500 * time.Millisecond)

	agents := srv.ConnectionManager().List()
	if len(agents) == 0 {
		t.Fatal("expected agent to be registered, got 0 connected agents")
	}
	if agents[0].MachineID != "nuc-01" {
		t.Errorf("expected machine_id=nuc-01, got %s", agents[0].MachineID)
	}

	cancel()
	<-errCh
}

func TestAgentMTLSRejection(t *testing.T) {
	caDir, serverDir, _ := setupTestEnv(t, "agent-nuc-01")
	addr, srv, stop := startServer(t, caDir, serverDir)
	defer stop()

	// Create agent certs from a DIFFERENT CA.
	base := t.TempDir()
	otherCADir := filepath.Join(base, "other-ca")
	otherAgentDir := filepath.Join(base, "other-agent")
	for _, d := range []string{otherCADir, otherAgentDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := tlsutil.GenerateCA(otherCADir); err != nil {
		t.Fatal(err)
	}
	if err := tlsutil.IssueAgentCert(otherCADir, otherAgentDir, "agent-rogue"); err != nil {
		t.Fatal(err)
	}

	badCfg := &config.AgentConfig{
		Server: config.ServerConnConfig{Address: addr},
		TLS: config.TLSConfig{
			CACert:    filepath.Join(otherCADir, "ca.pem"),
			AgentCert: filepath.Join(otherAgentDir, "agent.pem"),
			AgentKey:  filepath.Join(otherAgentDir, "agent-key.pem"),
		},
		Agent: config.AgentSettings{
			MachineID:   "rogue",
			MaxSessions: 1,
		},
	}

	client, err := agent.NewAgentClient(badCfg, &noopSessionProvider{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx)
	}()

	// Give some time for the connection attempt to fail.
	time.Sleep(1 * time.Second)

	// Verify the rogue agent was never registered on the server.
	agents := srv.ConnectionManager().List()
	if len(agents) != 0 {
		t.Errorf("expected 0 registered agents, got %d", len(agents))
	}
	if client.Connected() {
		t.Error("expected rogue agent to NOT be connected, but Connected() returned true")
	}

	cancel()
	<-errCh
}

func TestAgentReconnectsOnDrop(t *testing.T) {
	caDir, serverDir, agentCfg := setupTestEnv(t, "agent-nuc-01")

	// Start server.
	addr, _, stop1 := startServer(t, caDir, serverDir)
	agentCfg.Server.Address = addr

	client, err := agent.NewAgentClient(agentCfg, &noopSessionProvider{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx)
	}()

	// Wait for initial connection.
	time.Sleep(500 * time.Millisecond)
	if !client.Connected() {
		t.Fatal("expected client to be connected")
	}

	// Kill the server.
	stop1()
	time.Sleep(500 * time.Millisecond)

	// Client should detect disconnection.
	if client.Connected() {
		t.Log("client still reports connected (may be in reconnect loop)")
	}

	// Start new server on the SAME address.
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("could not re-listen on %s: %v", addr, err)
	}

	tlsCfg, err := tlsutil.ServerTLSConfig(
		filepath.Join(caDir, "ca.pem"),
		filepath.Join(serverDir, "server.pem"),
		filepath.Join(serverDir, "server-key.pem"),
	)
	if err != nil {
		t.Fatal(err)
	}
	srv2 := servergrpc.NewGRPCServer(tlsCfg, nil)
	go func() {
		_ = srv2.Serve(lis)
	}()
	defer srv2.Stop()

	// Wait for agent to reconnect (backoff starts at ~1s).
	deadline := time.After(8 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	reconnected := false
	for !reconnected {
		select {
		case <-deadline:
			t.Fatal("agent did not reconnect within 8 seconds")
		case <-ticker.C:
			agents := srv2.ConnectionManager().List()
			if len(agents) > 0 {
				reconnected = true
			}
		}
	}

	cancel()
	<-errCh
}
