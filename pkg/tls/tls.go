package tls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"
)

// CertificateInfo contains information about a parsed TLS certificate
type CertificateInfo struct {
	Subject   string
	Issuer    string
	NotBefore time.Time
	NotAfter  time.Time
	DNSNames  []string
}

// ParseTLSCertificate parses and validates a TLS certificate from PEM-encoded cert and key data
func ParseTLSCertificate(certPEM, keyPEM []byte) (*CertificateInfo, error) {
	// Parse the certificate
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("failed to decode PEM certificate block")
	}

	if certBlock.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid certificate block type: %s", certBlock.Type)
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Parse the private key
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode PEM key block")
	}

	// Validate that the certificate and key pair work together
	_, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("certificate and key do not match: %w", err)
	}

	// Create certificate info
	info := &CertificateInfo{
		Subject:   cert.Subject.String(),
		Issuer:    cert.Issuer.String(),
		NotBefore: cert.NotBefore,
		NotAfter:  cert.NotAfter,
		DNSNames:  cert.DNSNames,
	}

	return info, nil
}
