// Package provision provides the provisioning service for creating agent install tokens.
package provision

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	"github.com/kodrunhq/claude-plane/internal/shared/tlsutil"
)

// isUniqueConstraintError checks if the error is a SQLite UNIQUE constraint violation.
func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// ErrInvalidMachineID is returned when the machine ID fails validation.
var ErrInvalidMachineID = errors.New("invalid machine ID")

// ErrUnsupportedPlatform is returned when the OS/arch combination is not supported.
var ErrUnsupportedPlatform = errors.New("unsupported platform")

// ErrInvalidTTL is returned when the token TTL is not positive.
var ErrInvalidTTL = errors.New("invalid TTL")

// ProvisionResult is the result returned after successfully creating a provisioning token.
type ProvisionResult struct {
	Token       string    `json:"token"`
	ShortCode   string    `json:"short_code"`
	ExpiresAt   time.Time `json:"expires_at"`
	CurlCommand string    `json:"curl_command"`
	JoinCommand string    `json:"join_command"`
}

// Service creates and manages agent provisioning tokens.
type Service struct {
	store       *store.Store
	caDir       string
	httpAddress string
	grpcAddress string
}

// NewService creates a new provisioning Service.
func NewService(s *store.Store, caDir, httpAddress, grpcAddress string) *Service {
	return &Service{
		store:       s,
		caDir:       caDir,
		httpAddress: httpAddress,
		grpcAddress: grpcAddress,
	}
}

// validPlatforms is the set of supported OS/arch combinations.
var validPlatforms = map[string]bool{
	"linux-amd64":  true,
	"linux-arm64":  true,
	"darwin-amd64": true,
	"darwin-arm64": true,
}

// CreateAgentProvision creates a provisioning token for a new agent machine.
// It issues a TLS certificate for the machine, embeds it in the token, and
// returns the token along with a ready-to-run curl install command.
func (svc *Service) CreateAgentProvision(ctx context.Context, machineID, targetOS, targetArch, createdBy string, ttl time.Duration) (*ProvisionResult, error) {
	if !tlsutil.ValidMachineID.MatchString(machineID) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidMachineID, machineID)
	}
	// gRPC auth requires CN="agent-{machineID}"; ValidMachineID allows up to 64 chars,
	// so the full CN must also fit within that limit.
	cn := "agent-" + machineID
	if !tlsutil.ValidMachineID.MatchString(cn) {
		return nil, fmt.Errorf("%w: %q (too long with agent- prefix)", ErrInvalidMachineID, machineID)
	}

	if ttl <= 0 {
		return nil, fmt.Errorf("%w: must be positive", ErrInvalidTTL)
	}

	if targetOS == "" {
		targetOS = "linux"
	}
	if targetArch == "" {
		targetArch = "amd64"
	}

	if !validPlatforms[targetOS+"-"+targetArch] {
		return nil, fmt.Errorf("%w: %s-%s", ErrUnsupportedPlatform, targetOS, targetArch)
	}

	// gRPC mTLS auth requires CN="agent-{machineID}", so prefix the CN accordingly.
	certPEM, keyPEM, err := tlsutil.IssueAgentCertPEM(svc.caDir, cn)
	if err != nil {
		return nil, fmt.Errorf("issue agent cert: %w", err)
	}

	caCertPEM, err := tlsutil.ReadCACertPEM(svc.caDir)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	now := time.Now().UTC()
	tokenID := uuid.New().String()
	expiresAt := now.Add(ttl)

	// Generate short code with retry on collision (UNIQUE constraint).
	const maxRetries = 3
	var shortCode string
	for attempt := range maxRetries {
		code, err := GenerateShortCode()
		if err != nil {
			return nil, fmt.Errorf("generate short code: %w", err)
		}

		token := store.ProvisioningToken{
			Token:         tokenID,
			ShortCode:     code,
			MachineID:     machineID,
			TargetOS:      targetOS,
			TargetArch:    targetArch,
			CACertPEM:     string(caCertPEM),
			AgentCertPEM:  string(certPEM),
			AgentKeyPEM:   string(keyPEM),
			ServerAddress: svc.httpAddress,
			GRPCAddress:   svc.grpcAddress,
			CreatedBy:     createdBy,
			CreatedAt:     now,
			ExpiresAt:     expiresAt,
		}

		if err := svc.store.CreateProvisioningToken(ctx, token); err != nil {
			if attempt < maxRetries-1 && isUniqueConstraintError(err) {
				continue // Retry with a new short code.
			}
			return nil, fmt.Errorf("create token: %w", err)
		}
		shortCode = code
		break
	}

	curlCmd := fmt.Sprintf("curl -sfL %s/api/v1/provision/%s/script | sudo bash", svc.httpAddress, tokenID)
	joinCmd := fmt.Sprintf("claude-plane-agent join %s --server %s", shortCode, svc.httpAddress)

	return &ProvisionResult{
		Token:       tokenID,
		ShortCode:   shortCode,
		ExpiresAt:   expiresAt,
		CurlCommand: curlCmd,
		JoinCommand: joinCmd,
	}, nil
}
