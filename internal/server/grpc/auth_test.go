package grpc_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/session"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
	"github.com/kodrunhq/claude-plane/internal/shared/tlsutil"

	servergrpc "github.com/kodrunhq/claude-plane/internal/server/grpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
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

	srv := servergrpc.NewGRPCServer(tlsCfg, nil, nil)

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

func TestStreamRegistry_AddRemoveList(t *testing.T) {
	sr := servergrpc.NewStreamRegistry()

	sr.Add("nuc-01", &servergrpc.StreamEntry{
		MachineID:   "nuc-01",
		MaxSessions: 5,
		ConnectedAt: time.Now(),
	})
	sr.Add("nuc-02", &servergrpc.StreamEntry{
		MachineID:   "nuc-02",
		MaxSessions: 3,
		ConnectedAt: time.Now(),
	})

	entries := sr.List()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	e, ok := sr.Get("nuc-01")
	if !ok || e.MachineID != "nuc-01" {
		t.Fatal("expected to find nuc-01")
	}

	_, ok = sr.Get("nuc-99")
	if ok {
		t.Fatal("expected not to find nuc-99")
	}

	sr.Remove("nuc-01")
	entries = sr.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after remove, got %d", len(entries))
	}

	// Concurrent access safety
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "concurrent-" + string(rune('a'+i%26))
			sr.Add(id, &servergrpc.StreamEntry{
				MachineID:   id,
				ConnectedAt: time.Now(),
			})
			sr.List()
			sr.Remove(id)
		}(i)
	}
	wg.Wait()
}

// mockServerStream is a minimal grpc.ServerStream for interceptor unit tests.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func TestMachineAuthStreamInterceptor_NoPeerInfo(t *testing.T) {
	interceptor := servergrpc.MachineAuthStreamInterceptor()
	stream := &mockServerStream{ctx: context.Background()}

	handlerCalled := false
	handler := func(srv any, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error for context without peer info")
	}
	if handlerCalled {
		t.Error("handler should not be called when auth fails")
	}
}

func TestMachineAuthStreamInterceptor_NoCertificate(t *testing.T) {
	interceptor := servergrpc.MachineAuthStreamInterceptor()

	// Peer with TLS info but no client certificate
	p := &peer.Peer{
		Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345},
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: nil,
			},
		},
	}
	ctx := peer.NewContext(context.Background(), p)
	stream := &mockServerStream{ctx: ctx}

	handlerCalled := false
	handler := func(srv any, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error for peer with no certificates")
	}
	if handlerCalled {
		t.Error("handler should not be called when auth fails")
	}
}

func TestMachineAuthStreamInterceptor_InvalidCNPrefix(t *testing.T) {
	interceptor := servergrpc.MachineAuthStreamInterceptor()

	// Peer with a certificate that has wrong CN prefix
	cert := &x509.Certificate{
		Subject: pkix.Name{CommonName: "bad-prefix-nuc-01"},
	}
	p := &peer.Peer{
		Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345},
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{cert},
			},
		},
	}
	ctx := peer.NewContext(context.Background(), p)
	stream := &mockServerStream{ctx: ctx}

	handlerCalled := false
	handler := func(srv any, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error for certificate with invalid CN prefix")
	}
	if handlerCalled {
		t.Error("handler should not be called when auth fails")
	}
}

func TestMachineAuthStreamInterceptor_ValidCert(t *testing.T) {
	interceptor := servergrpc.MachineAuthStreamInterceptor()

	cert := &x509.Certificate{
		Subject: pkix.Name{CommonName: "agent-nuc-42"},
	}
	p := &peer.Peer{
		Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345},
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{cert},
			},
		},
	}
	ctx := peer.NewContext(context.Background(), p)
	stream := &mockServerStream{ctx: ctx}

	var capturedCtx context.Context
	handler := func(srv any, ss grpc.ServerStream) error {
		capturedCtx = ss.Context()
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedCtx == nil {
		t.Fatal("handler was not called")
	}

	// Verify machine-id was injected into context
	machineID, err := servergrpc.MachineIDFromContextForTest(capturedCtx)
	if err != nil {
		t.Fatalf("MachineIDFromContext: %v", err)
	}
	if machineID != "nuc-42" {
		t.Errorf("machineID = %q, want %q", machineID, "nuc-42")
	}
}

func TestMachineAuthStreamInterceptor_EmptyMachineID(t *testing.T) {
	interceptor := servergrpc.MachineAuthStreamInterceptor()

	// CN = "agent-" with empty machine ID after prefix
	cert := &x509.Certificate{
		Subject: pkix.Name{CommonName: "agent-"},
	}
	p := &peer.Peer{
		Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345},
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{cert},
			},
		},
	}
	ctx := peer.NewContext(context.Background(), p)
	stream := &mockServerStream{ctx: ctx}

	handlerCalled := false
	handler := func(srv any, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error for empty machine-id in certificate CN")
	}
	if handlerCalled {
		t.Error("handler should not be called when auth fails")
	}
}

// startTestGRPCServerWithRegistry creates a gRPC server with mTLS and a session
// registry for event forwarding tests.
func startTestGRPCServerWithRegistry(t *testing.T, caDir, serverDir string, reg *session.Registry) (addr string, stop func()) {
	t.Helper()

	tlsCfg, err := tlsutil.ServerTLSConfig(
		filepath.Join(caDir, "ca.pem"),
		filepath.Join(serverDir, "server.pem"),
		filepath.Join(serverDir, "server-key.pem"),
	)
	if err != nil {
		t.Fatal(err)
	}

	srv := servergrpc.NewGRPCServer(tlsCfg, nil, nil)
	srv.SetRegistry(reg)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		_ = srv.Serve(lis)
	}()

	return lis.Addr().String(), func() { srv.Stop() }
}

func TestCommandStream_EventForwarding(t *testing.T) {
	caDir, serverDir, agentDir := setupTestCerts(t, "agent-stream-01")
	reg := session.NewRegistry(slog.Default())
	addr, stop := startTestGRPCServerWithRegistry(t, caDir, serverDir, reg)
	defer stop()

	conn := dialAgent(t, addr, caDir, agentDir)
	client := pb.NewAgentServiceClient(conn)

	// Register first
	resp, err := client.Register(context.Background(), &pb.RegisterRequest{
		MachineId:   "stream-01",
		MaxSessions: 5,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("Register rejected: %s", resp.RejectReason)
	}

	// Subscribe to session output in the registry before sending events
	sessionID := "test-session-abc"
	subCh := reg.Subscribe(sessionID)
	defer reg.Unsubscribe(sessionID, subCh)

	// Open CommandStream
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.CommandStream(ctx)
	if err != nil {
		t.Fatalf("CommandStream: %v", err)
	}

	// Send a SessionOutput event
	testData := []byte("hello from agent stream")
	err = stream.Send(&pb.AgentEvent{
		Event: &pb.AgentEvent_SessionOutput{
			SessionOutput: &pb.SessionOutputEvent{
				SessionId: sessionID,
				Data:      testData,
			},
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Verify the event was forwarded to the registry subscriber
	select {
	case msg := <-subCh:
		if string(msg.Data) != string(testData) {
			t.Errorf("forwarded data = %q, want %q", msg.Data, testData)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for event to be forwarded to registry")
	}
}
