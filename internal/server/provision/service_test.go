package provision_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/provision"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	"github.com/kodrunhq/claude-plane/internal/shared/tlsutil"
)

// newTestStore creates an in-memory SQLite store for testing.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newTestCA initialises a CA in a temp dir and returns the CA directory path.
func newTestCA(t *testing.T) string {
	t.Helper()
	caDir := t.TempDir()
	if err := tlsutil.GenerateCA(caDir); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	return caDir
}

// newTestService creates a Service wired to a real store and CA.
func newTestService(t *testing.T) (*provision.Service, *store.Store, string) {
	t.Helper()
	s := newTestStore(t)
	caDir := newTestCA(t)
	svc := provision.NewService(s, caDir, "http://localhost:8080", "localhost:9090")
	return svc, s, caDir
}

func TestCreateAgentProvision_Success(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	result, err := svc.CreateAgentProvision(ctx, "my-machine", "linux", "amd64", "admin-user", time.Hour)
	if err != nil {
		t.Fatalf("CreateAgentProvision: %v", err)
	}

	if result.Token == "" {
		t.Error("expected non-empty token")
	}
	if result.ExpiresAt.IsZero() {
		t.Error("expected non-zero ExpiresAt")
	}
	if result.ExpiresAt.Before(time.Now()) {
		t.Error("expected ExpiresAt to be in the future")
	}

	wantCurlPrefix := "curl -sfL http://localhost:8080/api/v1/provision/" + result.Token
	if !strings.HasPrefix(result.CurlCommand, wantCurlPrefix) {
		t.Errorf("CurlCommand = %q, want prefix %q", result.CurlCommand, wantCurlPrefix)
	}
	if !strings.Contains(result.CurlCommand, "sudo bash") {
		t.Errorf("CurlCommand missing 'sudo bash': %q", result.CurlCommand)
	}
}

func TestCreateAgentProvision_TokenStoredInDB(t *testing.T) {
	svc, s, _ := newTestService(t)
	ctx := context.Background()

	result, err := svc.CreateAgentProvision(ctx, "db-machine", "linux", "amd64", "admin", time.Hour)
	if err != nil {
		t.Fatalf("CreateAgentProvision: %v", err)
	}

	got, err := s.GetProvisioningToken(ctx, result.Token)
	if err != nil {
		t.Fatalf("GetProvisioningToken: %v", err)
	}

	if got.MachineID != "db-machine" {
		t.Errorf("MachineID = %q, want %q", got.MachineID, "db-machine")
	}
	if got.TargetOS != "linux" {
		t.Errorf("TargetOS = %q, want %q", got.TargetOS, "linux")
	}
	if got.TargetArch != "amd64" {
		t.Errorf("TargetArch = %q, want %q", got.TargetArch, "amd64")
	}
	if got.CACertPEM == "" {
		t.Error("expected non-empty CACertPEM")
	}
	if got.AgentCertPEM == "" {
		t.Error("expected non-empty AgentCertPEM")
	}
	if got.AgentKeyPEM == "" {
		t.Error("expected non-empty AgentKeyPEM")
	}
	if got.ServerAddress != "http://localhost:8080" {
		t.Errorf("ServerAddress = %q, want %q", got.ServerAddress, "http://localhost:8080")
	}
	if got.GRPCAddress != "localhost:9090" {
		t.Errorf("GRPCAddress = %q, want %q", got.GRPCAddress, "localhost:9090")
	}
	if got.CreatedBy != "admin" {
		t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, "admin")
	}
}

func TestCreateAgentProvision_DefaultOSArch(t *testing.T) {
	svc, s, _ := newTestService(t)
	ctx := context.Background()

	result, err := svc.CreateAgentProvision(ctx, "default-machine", "", "", "cli", time.Hour)
	if err != nil {
		t.Fatalf("CreateAgentProvision: %v", err)
	}

	got, err := s.GetProvisioningToken(ctx, result.Token)
	if err != nil {
		t.Fatalf("GetProvisioningToken: %v", err)
	}

	if got.TargetOS != "linux" {
		t.Errorf("TargetOS = %q, want %q", got.TargetOS, "linux")
	}
	if got.TargetArch != "amd64" {
		t.Errorf("TargetArch = %q, want %q", got.TargetArch, "amd64")
	}
}

func TestCreateAgentProvision_InvalidMachineID(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.CreateAgentProvision(ctx, "bad machine id!", "linux", "amd64", "admin", time.Hour)
	if err == nil {
		t.Fatal("expected error for invalid machine ID")
	}
	if !errors.Is(err, provision.ErrInvalidMachineID) {
		t.Errorf("expected ErrInvalidMachineID, got %v", err)
	}
}

func TestCreateAgentProvision_UnsupportedPlatform(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.CreateAgentProvision(ctx, "my-machine", "windows", "amd64", "admin", time.Hour)
	if err == nil {
		t.Fatal("expected error for unsupported platform")
	}
	if !errors.Is(err, provision.ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

func TestCreateAgentProvision_CurlCommandContainsToken(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	result, err := svc.CreateAgentProvision(ctx, "curl-machine", "darwin", "arm64", "admin", 30*time.Minute)
	if err != nil {
		t.Fatalf("CreateAgentProvision: %v", err)
	}

	if !strings.Contains(result.CurlCommand, result.Token) {
		t.Errorf("CurlCommand %q does not contain token %q", result.CurlCommand, result.Token)
	}
}

func TestCreateAgentProvision_ValidPlatforms(t *testing.T) {
	platforms := []struct {
		os   string
		arch string
	}{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
	}

	for _, p := range platforms {
		t.Run(p.os+"-"+p.arch, func(t *testing.T) {
			svc, _, _ := newTestService(t)
			ctx := context.Background()

			machineID := "machine-" + p.os + "-" + p.arch
			_, err := svc.CreateAgentProvision(ctx, machineID, p.os, p.arch, "admin", time.Hour)
			if err != nil {
				t.Errorf("CreateAgentProvision(%s, %s): %v", p.os, p.arch, err)
			}
		})
	}
}
