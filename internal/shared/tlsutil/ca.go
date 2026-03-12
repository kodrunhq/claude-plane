// Package tlsutil provides mTLS certificate authority tooling for claude-plane.
// It generates a self-signed CA and issues server/agent leaf certificates
// for mutual TLS authentication between server and agent components.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// validMachineID matches alphanumeric IDs with hyphens/underscores, 1-64 chars,
// starting with an alphanumeric character.
var validMachineID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// GenerateCA creates a self-signed CA certificate and private key in outDir.
// Files written: ca.pem (certificate), ca-key.pem (ECDSA P-256 private key).
// The CA cert has a 10-year validity period and CN="claude-plane-ca".
func GenerateCA(outDir string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "claude-plane-ca",
		},
		NotBefore:             now,
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create CA certificate: %w", err)
	}

	return writePEMFiles(outDir, "ca.pem", "ca-key.pem", certDER, key)
}

// IssueServerCert creates a server certificate signed by the CA in caDir.
// Files written to outDir: server.pem, server-key.pem.
// The cert includes SANs for localhost, 127.0.0.1, and any additional hostnames.
// Validity is 2 years. ExtKeyUsage is ServerAuth.
func IssueServerCert(caDir, outDir string, hostnames []string) error {
	caCert, caKey, err := loadCA(caDir)
	if err != nil {
		return err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate server key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	// Build SANs: always include localhost and 127.0.0.1
	dnsNames := []string{"localhost"}
	ipAddrs := []net.IP{net.IPv4(127, 0, 0, 1)}

	for _, h := range hostnames {
		if ip := net.ParseIP(h); ip != nil {
			ipAddrs = append(ipAddrs, ip)
		} else {
			dnsNames = append(dnsNames, h)
		}
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "claude-plane-server",
		},
		NotBefore:   now,
		NotAfter:    now.Add(2 * 365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    dnsNames,
		IPAddresses: ipAddrs,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create server certificate: %w", err)
	}

	return writePEMFiles(outDir, "server.pem", "server-key.pem", certDER, key)
}

// IssueAgentCert creates an agent certificate signed by the CA in caDir.
// Files written to outDir: agent.pem, agent-key.pem.
// The cert has CN=machineID for agent identity. Validity is 2 years.
// ExtKeyUsage is ClientAuth.
func IssueAgentCert(caDir, outDir, machineID string) error {
	if !validMachineID.MatchString(machineID) {
		return fmt.Errorf("invalid machineID %q: must match %s", machineID, validMachineID.String())
	}

	caCert, caKey, err := loadCA(caDir)
	if err != nil {
		return err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate agent key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: machineID,
		},
		NotBefore:   now,
		NotAfter:    now.Add(2 * 365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create agent certificate: %w", err)
	}

	return writePEMFiles(outDir, "agent.pem", "agent-key.pem", certDER, key)
}

// loadCA reads the CA certificate and private key from caDir.
func loadCA(caDir string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(filepath.Join(caDir, "ca.pem"))
	if err != nil {
		return nil, nil, fmt.Errorf("read CA cert: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("decode CA cert PEM: no PEM data found")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA cert: %w", err)
	}

	keyPEM, err := os.ReadFile(filepath.Join(caDir, "ca-key.pem"))
	if err != nil {
		return nil, nil, fmt.Errorf("read CA key: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("decode CA key PEM: no PEM data found")
	}

	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA key: %w", err)
	}

	return cert, key, nil
}

// writePEMFiles writes a certificate and ECDSA private key as PEM files.
func writePEMFiles(dir, certName, keyName string, certDER []byte, key *ecdsa.PrivateKey) (err error) {
	certFile, err := os.Create(filepath.Join(dir, certName))
	if err != nil {
		return fmt.Errorf("create %s: %w", certName, err)
	}
	defer func() {
		if cerr := certFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", certName, cerr)
		}
	}()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return fmt.Errorf("encode cert PEM: %w", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal EC key: %w", err)
	}

	keyFile, err := os.OpenFile(filepath.Join(dir, keyName), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create %s: %w", keyName, err)
	}
	defer func() {
		if kerr := keyFile.Close(); kerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", keyName, kerr)
		}
	}()

	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return fmt.Errorf("encode key PEM: %w", err)
	}

	return nil
}

// randomSerial generates a random 128-bit serial number for certificates.
// Per RFC 5280, serial numbers must be positive integers. We ensure the
// result is at least 2 to satisfy both the RFC and test assertions.
func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	for {
		serial, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return nil, fmt.Errorf("generate serial number: %w", err)
		}
		if serial.Cmp(big.NewInt(1)) > 0 {
			return serial, nil
		}
	}
}
