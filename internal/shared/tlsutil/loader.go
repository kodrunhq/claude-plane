package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// ServerTLSConfig builds a *tls.Config for the server side of an mTLS connection.
// It loads the CA certificate for client verification and the server's own certificate.
// ClientAuth is set to RequireAndVerifyClientCert.
func ServerTLSConfig(caCertPath, serverCertPath, serverKeyPath string) (*tls.Config, error) {
	caPool, err := loadCACertPool(caCertPath)
	if err != nil {
		return nil, err
	}

	serverCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load server certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// AgentTLSConfig builds a *tls.Config for the agent (client) side of an mTLS connection.
// It loads the CA certificate for server verification and the agent's own certificate.
func AgentTLSConfig(caCertPath, agentCertPath, agentKeyPath string) (*tls.Config, error) {
	caPool, err := loadCACertPool(caCertPath)
	if err != nil {
		return nil, err
	}

	agentCert, err := tls.LoadX509KeyPair(agentCertPath, agentKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load agent certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{agentCert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// loadCACertPool reads a PEM-encoded CA certificate file and returns a cert pool.
func loadCACertPool(caCertPath string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to add CA cert to pool")
	}

	return pool, nil
}
