package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateCA(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateCA(dir); err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// Verify files exist
	certPEM, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
	if err != nil {
		t.Fatalf("ca.pem not found: %v", err)
	}
	keyPEM, err := os.ReadFile(filepath.Join(dir, "ca-key.pem"))
	if err != nil {
		t.Fatalf("ca-key.pem not found: %v", err)
	}

	// Parse cert
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("ca.pem is not a valid PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse CA cert: %v", err)
	}

	// Verify CA properties
	if !cert.IsCA {
		t.Error("CA cert IsCA should be true")
	}
	if cert.Subject.CommonName != "claude-plane-ca" {
		t.Errorf("expected CN=claude-plane-ca, got %s", cert.Subject.CommonName)
	}
	if cert.PublicKeyAlgorithm != x509.ECDSA {
		t.Error("expected ECDSA public key")
	}
	// 10 year validity (within a day tolerance)
	expectedExpiry := time.Now().Add(10 * 365 * 24 * time.Hour)
	if cert.NotAfter.Before(expectedExpiry.Add(-24 * time.Hour)) {
		t.Error("CA cert validity should be ~10 years")
	}
	// Random serial (not zero, not 1)
	if cert.SerialNumber.Cmp(big.NewInt(1)) <= 0 {
		t.Error("serial number should be random, not 0 or 1")
	}

	// Verify key
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil || keyBlock.Type != "EC PRIVATE KEY" {
		t.Fatal("ca-key.pem is not a valid EC private key PEM")
	}
}

func TestIssueServerCert(t *testing.T) {
	caDir := t.TempDir()
	if err := GenerateCA(caDir); err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	serverDir := t.TempDir()
	hostnames := []string{"myhost.example.com", "other.example.com"}
	if err := IssueServerCert(caDir, serverDir, hostnames); err != nil {
		t.Fatalf("IssueServerCert failed: %v", err)
	}

	certPEM, err := os.ReadFile(filepath.Join(serverDir, "server.pem"))
	if err != nil {
		t.Fatalf("server.pem not found: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("server.pem is not a valid PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse server cert: %v", err)
	}

	// Check SANs include localhost, 127.0.0.1, and provided hostnames
	dnsNames := make(map[string]bool)
	for _, n := range cert.DNSNames {
		dnsNames[n] = true
	}
	if !dnsNames["localhost"] {
		t.Error("server cert should contain localhost SAN")
	}
	for _, h := range hostnames {
		if !dnsNames[h] {
			t.Errorf("server cert should contain SAN %s", h)
		}
	}

	// Check IP SANs
	hasLoopback := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.IPv4(127, 0, 0, 1)) {
			hasLoopback = true
		}
	}
	if !hasLoopback {
		t.Error("server cert should contain 127.0.0.1 IP SAN")
	}

	// ExtKeyUsage
	hasServerAuth := false
	for _, u := range cert.ExtKeyUsage {
		if u == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
		}
	}
	if !hasServerAuth {
		t.Error("server cert should have ExtKeyUsageServerAuth")
	}

	// 2 year validity
	expectedExpiry := time.Now().Add(2 * 365 * 24 * time.Hour)
	if cert.NotAfter.Before(expectedExpiry.Add(-24 * time.Hour)) {
		t.Error("server cert validity should be ~2 years")
	}

	if cert.IsCA {
		t.Error("server cert should not be a CA")
	}

	if cert.SerialNumber.Cmp(big.NewInt(1)) <= 0 {
		t.Error("serial number should be random")
	}
}

func TestIssueAgentCert(t *testing.T) {
	caDir := t.TempDir()
	if err := GenerateCA(caDir); err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	agentDir := t.TempDir()
	machineID := "worker-node-42"
	if err := IssueAgentCert(caDir, agentDir, machineID); err != nil {
		t.Fatalf("IssueAgentCert failed: %v", err)
	}

	certPEM, err := os.ReadFile(filepath.Join(agentDir, "agent.pem"))
	if err != nil {
		t.Fatalf("agent.pem not found: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("agent.pem is not a valid PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse agent cert: %v", err)
	}

	if cert.Subject.CommonName != machineID {
		t.Errorf("expected CN=%s, got %s", machineID, cert.Subject.CommonName)
	}

	hasClientAuth := false
	for _, u := range cert.ExtKeyUsage {
		if u == x509.ExtKeyUsageClientAuth {
			hasClientAuth = true
		}
	}
	if !hasClientAuth {
		t.Error("agent cert should have ExtKeyUsageClientAuth")
	}

	expectedExpiry := time.Now().Add(2 * 365 * 24 * time.Hour)
	if cert.NotAfter.Before(expectedExpiry.Add(-24 * time.Hour)) {
		t.Error("agent cert validity should be ~2 years")
	}

	if cert.IsCA {
		t.Error("agent cert should not be a CA")
	}

	if cert.SerialNumber.Cmp(big.NewInt(1)) <= 0 {
		t.Error("serial number should be random")
	}
}

func TestIssueAgentCert_MachineIDValidation(t *testing.T) {
	caDir := t.TempDir()
	if err := GenerateCA(caDir); err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	longID := strings.Repeat("a", 65) // 65 chars exceeds 64 limit

	tests := []struct {
		name      string
		machineID string
		wantErr   bool
	}{
		{"valid simple", "worker-node-42", false},
		{"valid with underscores", "my_worker_01", false},
		{"valid single char", "a", false},
		{"valid max length 64", strings.Repeat("a", 64), false},
		{"path traversal", "../etc/passwd", true},
		{"empty", "", true},
		{"too long 65 chars", longID, true},
		{"special chars", "worker@node!#", true},
		{"starts with hyphen", "-worker", true},
		{"starts with underscore", "_worker", true},
		{"contains spaces", "worker node", true},
		{"contains slash", "worker/node", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outDir := t.TempDir()
			err := IssueAgentCert(caDir, outDir, tt.machineID)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for machineID %q, got nil", tt.machineID)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for machineID %q: %v", tt.machineID, err)
			}
		})
	}
}

func TestMTLSHandshake(t *testing.T) {
	// Generate all certs
	caDir := t.TempDir()
	if err := GenerateCA(caDir); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	serverDir := t.TempDir()
	if err := IssueServerCert(caDir, serverDir, nil); err != nil {
		t.Fatalf("IssueServerCert: %v", err)
	}

	agentDir := t.TempDir()
	if err := IssueAgentCert(caDir, agentDir, "test-agent"); err != nil {
		t.Fatalf("IssueAgentCert: %v", err)
	}

	// Build TLS configs
	caCertPath := filepath.Join(caDir, "ca.pem")

	serverTLS, err := ServerTLSConfig(caCertPath, filepath.Join(serverDir, "server.pem"), filepath.Join(serverDir, "server-key.pem"))
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}

	agentTLS, err := AgentTLSConfig(caCertPath, filepath.Join(agentDir, "agent.pem"), filepath.Join(agentDir, "agent-key.pem"))
	if err != nil {
		t.Fatalf("AgentTLSConfig: %v", err)
	}

	// Do a real TLS handshake
	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	defer listener.Close()

	type serverResult struct {
		err     error
		peerCN  string
		hasCert bool
	}
	resultCh := make(chan serverResult, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			resultCh <- serverResult{err: err}
			return
		}
		defer conn.Close()
		tlsConn := conn.(*tls.Conn)
		if err := tlsConn.Handshake(); err != nil {
			resultCh <- serverResult{err: err}
			return
		}
		state := tlsConn.ConnectionState()
		res := serverResult{hasCert: len(state.PeerCertificates) > 0}
		if res.hasCert {
			res.peerCN = state.PeerCertificates[0].Subject.CommonName
		}
		resultCh <- res
	}()

	agentTLS.ServerName = "localhost"
	conn, err := tls.Dial("tcp", listener.Addr().String(), agentTLS)
	if err != nil {
		t.Fatalf("agent dial failed: %v", err)
	}
	defer conn.Close()

	res := <-resultCh
	if res.err != nil {
		t.Fatalf("server handshake failed: %v", res.err)
	}

	// Verify the agent's identity from the server side
	if !res.hasCert {
		t.Fatal("server should see agent's client certificate")
	}
	if res.peerCN != "test-agent" {
		t.Errorf("server saw client CN=%q, want %q", res.peerCN, "test-agent")
	}
}

func TestServerTLSConfig(t *testing.T) {
	caDir := t.TempDir()
	if err := GenerateCA(caDir); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	serverDir := t.TempDir()
	if err := IssueServerCert(caDir, serverDir, nil); err != nil {
		t.Fatalf("IssueServerCert: %v", err)
	}

	cfg, err := ServerTLSConfig(
		filepath.Join(caDir, "ca.pem"),
		filepath.Join(serverDir, "server.pem"),
		filepath.Join(serverDir, "server-key.pem"),
	)
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}

	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Error("server TLS should require and verify client certs")
	}
	if cfg.ClientCAs == nil {
		t.Error("server TLS should have ClientCAs pool")
	}
	if len(cfg.Certificates) != 1 {
		t.Error("server TLS should have exactly one certificate")
	}
}

func TestAgentTLSConfig(t *testing.T) {
	caDir := t.TempDir()
	if err := GenerateCA(caDir); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	agentDir := t.TempDir()
	if err := IssueAgentCert(caDir, agentDir, "test-machine"); err != nil {
		t.Fatalf("IssueAgentCert: %v", err)
	}

	cfg, err := AgentTLSConfig(
		filepath.Join(caDir, "ca.pem"),
		filepath.Join(agentDir, "agent.pem"),
		filepath.Join(agentDir, "agent-key.pem"),
	)
	if err != nil {
		t.Fatalf("AgentTLSConfig: %v", err)
	}

	if cfg.RootCAs == nil {
		t.Error("agent TLS should have RootCAs pool")
	}
	if len(cfg.Certificates) != 1 {
		t.Error("agent TLS should have exactly one certificate")
	}
}

func TestGenerateCA_UniqueSerials(t *testing.T) {
	// Generate two CAs and ensure they have different serial numbers
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	if err := GenerateCA(dir1); err != nil {
		t.Fatal(err)
	}
	if err := GenerateCA(dir2); err != nil {
		t.Fatal(err)
	}

	cert1 := loadCertFromDir(t, dir1, "ca.pem")
	cert2 := loadCertFromDir(t, dir2, "ca.pem")

	if cert1.SerialNumber.Cmp(cert2.SerialNumber) == 0 {
		t.Error("two CA certs should have different serial numbers")
	}
}

func loadCertFromDir(t *testing.T, dir, name string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("%s is not a valid PEM certificate", name)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return cert
}

// Verify that the private key type is ECDSA P-256
func TestGenerateCA_KeyType(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateCA(dir); err != nil {
		t.Fatal(err)
	}

	keyPEM, err := os.ReadFile(filepath.Join(dir, "ca-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "EC PRIVATE KEY" {
		t.Fatal("ca-key.pem is not a valid EC private key PEM")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}

	if key.Curve.Params().Name != "P-256" {
		t.Errorf("expected P-256, got %s", key.Curve.Params().Name)
	}
}
