package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// generateTestCertificate creates a test certificate and private key for testing
func generateTestCertificate(t *testing.T, notBefore, notAfter time.Time, dnsNames []string) ([]byte, []byte) {
	// Generate a private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test Org"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
	}

	// Create the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to marshal private key: %v", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	})

	return certPEM, keyPEM
}

func TestParseTLSCertificate_Valid(t *testing.T) {
	now := time.Now()
	notBefore := now.Add(-24 * time.Hour)
	notAfter := now.Add(24 * time.Hour)
	dnsNames := []string{"example.com", "www.example.com"}

	certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter, dnsNames)

	info, err := ParseTLSCertificate(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("ParseTLSCertificate failed: %v", err)
	}

	if info == nil {
		t.Fatal("Expected certificate info, got nil")
	}

	if len(info.DNSNames) != 2 {
		t.Errorf("Expected 2 DNS names, got %d", len(info.DNSNames))
	}

	if info.NotBefore.UTC().Truncate(time.Second) != notBefore.UTC().Truncate(time.Second) {
		t.Errorf("NotBefore mismatch: expected %v, got %v", notBefore.UTC(), info.NotBefore.UTC())
	}

	if info.NotAfter.UTC().Truncate(time.Second) != notAfter.UTC().Truncate(time.Second) {
		t.Errorf("NotAfter mismatch: expected %v, got %v", notAfter.UTC(), info.NotAfter.UTC())
	}
}

func TestParseTLSCertificate_ExpiredCertificate(t *testing.T) {
	now := time.Now()
	notBefore := now.Add(-48 * time.Hour)
	notAfter := now.Add(-24 * time.Hour) // Expired yesterday

	certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter, []string{"example.com"})

	info, err := ParseTLSCertificate(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("ParseTLSCertificate failed: %v", err)
	}

	if info == nil {
		t.Fatal("Expected certificate info, got nil")
	}

	if !now.After(info.NotAfter) {
		t.Error("Expected certificate to be expired")
	}
}

func TestParseTLSCertificate_InvalidCertPEM(t *testing.T) {
	invalidCertPEM := []byte("invalid certificate")
	_, keyPEM := generateTestCertificate(t, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), []string{"example.com"})

	_, err := ParseTLSCertificate(invalidCertPEM, keyPEM)
	if err == nil {
		t.Fatal("Expected error for invalid certificate PEM")
	}
}

func TestParseTLSCertificate_InvalidKeyPEM(t *testing.T) {
	certPEM, _ := generateTestCertificate(t, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), []string{"example.com"})
	invalidKeyPEM := []byte("invalid key")

	_, err := ParseTLSCertificate(certPEM, invalidKeyPEM)
	if err == nil {
		t.Fatal("Expected error for invalid key PEM")
	}
}
