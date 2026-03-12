package grpc_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pb "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
	"github.com/claudeplane/claude-plane/internal/shared/tlsutil"

	servergrpc "github.com/claudeplane/claude-plane/internal/server/grpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// setupTestCerts generates ephemeral CA, server, and agent certificates.
func setupTestCerts(t *testing.T, machineID string) (caDir, serverDir, agentDir string) {
	t.Helper()
	base := t.TempDir()

	caDir = filepath.Join(base, "ca")
	serverDir = filepath.Join(base, "server")
	agentDir = filepath.Join(base, "agent")

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

	return caDir, serverDir, agentDir
}

// startTestGRPCServer creates a gRPC server with mTLS and returns its address.
func startTestGRPCServer(t *testing.T, caDir, serverDir string) (addr string, stop func()) {
	t.Helper()

	tlsCfg, err := tlsutil.ServerTLSConfig(
		filepath.Join(caDir, "ca.pem"),
		filepath.Join(serverDir, "server.pem"),
		filepath.Join(serverDir, "server-key.pem"),
	)
	if err != nil {
		t.Fatal(err)
	}

	srv := servergrpc.NewGRPCServer(tlsCfg, nil)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		_ = srv.Serve(lis)
	}()

	return lis.Addr().String(), func() { srv.Stop() }
}

// dialAgent creates a gRPC client connection using agent mTLS credentials.
func dialAgent(t *testing.T, addr, caDir, agentDir string) *grpc.ClientConn {
	t.Helper()

	tlsCfg, err := tlsutil.AgentTLSConfig(
		filepath.Join(caDir, "ca.pem"),
		filepath.Join(agentDir, "agent.pem"),
		filepath.Join(agentDir, "agent-key.pem"),
	)
	if err != nil {
		t.Fatal(err)
	}

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestExtractMachineID_ValidCert(t *testing.T) {
	caDir, serverDir, agentDir := setupTestCerts(t, "agent-nuc-01")
	addr, stop := startTestGRPCServer(t, caDir, serverDir)
	defer stop()

	conn := dialAgent(t, addr, caDir, agentDir)
	client := pb.NewAgentServiceClient(conn)

	resp, err := client.Register(context.Background(), &pb.RegisterRequest{
		MachineId:   "nuc-01",
		MaxSessions: 5,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("expected accepted=true, got false: %s", resp.RejectReason)
	}
}

func TestExtractMachineID_NoPeerInfo(t *testing.T) {
	_, err := servergrpc.ExtractMachineIDForTest(context.Background())
	if err == nil {
		t.Fatal("expected error for context without peer info")
	}
}

func TestExtractMachineID_InvalidCN(t *testing.T) {
	// Issue agent cert with CN "bad-nuc-01" (no "agent-" prefix)
	caDir, serverDir, agentDir := setupTestCerts(t, "bad-nuc-01")
	addr, stop := startTestGRPCServer(t, caDir, serverDir)
	defer stop()

	conn := dialAgent(t, addr, caDir, agentDir)
	client := pb.NewAgentServiceClient(conn)

	_, err := client.Register(context.Background(), &pb.RegisterRequest{
		MachineId:   "bad-nuc-01",
		MaxSessions: 5,
	})
	if err == nil {
		t.Fatal("expected error for cert without agent- prefix in CN")
	}
}

func TestConnectionManager_AddRemoveList(t *testing.T) {
	cm := servergrpc.NewConnectionManager()

	cm.Add("nuc-01", &servergrpc.ConnectedAgent{
		MachineID:   "nuc-01",
		MaxSessions: 5,
		ConnectedAt: time.Now(),
	})
	cm.Add("nuc-02", &servergrpc.ConnectedAgent{
		MachineID:   "nuc-02",
		MaxSessions: 3,
		ConnectedAt: time.Now(),
	})

	agents := cm.List()
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	a, ok := cm.Get("nuc-01")
	if !ok || a.MachineID != "nuc-01" {
		t.Fatal("expected to find nuc-01")
	}

	_, ok = cm.Get("nuc-99")
	if ok {
		t.Fatal("expected not to find nuc-99")
	}

	cm.Remove("nuc-01")
	agents = cm.List()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent after remove, got %d", len(agents))
	}

	// Concurrent access safety
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "concurrent-" + string(rune('a'+i%26))
			cm.Add(id, &servergrpc.ConnectedAgent{
				MachineID:   id,
				ConnectedAt: time.Now(),
			})
			cm.List()
			cm.Remove(id)
		}(i)
	}
	wg.Wait()
}
