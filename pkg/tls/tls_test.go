package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

// generateTestCertificate creates a test certificate and private key for testing
func generateTestCertificate(t *testing.T, notBefore, notAfter time.Time, dnsNames []string, subject, issuer pkix.Name) ([]byte, []byte) {
	// Generate a private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               subject,
		Issuer:                issuer,
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

// getDefaultSubject returns a default subject for test certificates
func getDefaultSubject() pkix.Name {
	return pkix.Name{
		Organization:  []string{"Test Org"},
		Country:       []string{"US"},
		Province:      []string{""},
		Locality:      []string{"San Francisco"},
		StreetAddress: []string{""},
		PostalCode:    []string{""},
	}
}

// getDefaultIssuer returns a default issuer for test certificates
func getDefaultIssuer() pkix.Name {
	return pkix.Name{
		Organization: []string{"Test CA"},
		Country:      []string{"US"},
		Province:     []string{""},
		Locality:     []string{"San Francisco"},
		CommonName:   "Test CA",
	}
}

func TestParseTLSCertificate_Valid(t *testing.T) {
	now := time.Now()
	notBefore := now.Add(-24 * time.Hour)
	notAfter := now.Add(24 * time.Hour)
	dnsNames := []string{"example.com", "www.example.com"}

	expectedSubject := pkix.Name{
		Organization:  []string{"Test Organization"},
		Country:       []string{"US"},
		Province:      []string{"California"},
		Locality:      []string{"San Francisco"},
		StreetAddress: []string{"123 Test St"},
		PostalCode:    []string{"94105"},
		CommonName:    "example.com",
	}

	// For self-signed certificates, issuer will be the same as subject
	expectedIssuer := expectedSubject

	certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter, dnsNames, expectedSubject, expectedIssuer)

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

	// Test subject field parsing
	if info.Subject != expectedSubject.String() {
		t.Errorf("Subject mismatch: expected %s, got %s", expectedSubject.String(), info.Subject)
	}

	// Test issuer field parsing (should be same as subject for self-signed cert)
	if info.Issuer != expectedIssuer.String() {
		t.Errorf("Issuer mismatch: expected %s, got %s", expectedIssuer.String(), info.Issuer)
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

	certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter, []string{"example.com"}, getDefaultSubject(), getDefaultIssuer())

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
	_, keyPEM := generateTestCertificate(t, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), []string{"example.com"}, getDefaultSubject(), getDefaultIssuer())

	_, err := ParseTLSCertificate(invalidCertPEM, keyPEM)
	if err == nil {
		t.Fatal("Expected error for invalid certificate PEM")
	}

	expectedErrMsg := "failed to decode PEM certificate block"
	if err.Error() != expectedErrMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrMsg, err.Error())
	}
}

func TestParseTLSCertificate_InvalidKeyPEM(t *testing.T) {
	certPEM, _ := generateTestCertificate(t, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), []string{"example.com"}, getDefaultSubject(), getDefaultIssuer())
	invalidKeyPEM := []byte("invalid key")

	_, err := ParseTLSCertificate(certPEM, invalidKeyPEM)
	if err == nil {
		t.Fatal("Expected error for invalid key PEM")
	}

	expectedErrMsg := "failed to decode PEM key block"
	if err.Error() != expectedErrMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrMsg, err.Error())
	}
}

func TestParseTLSCertificate_MismatchedKeyPair(t *testing.T) {
	// Generate two separate certificate/key pairs
	certPEM1, _ := generateTestCertificate(t, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), []string{"example.com"}, getDefaultSubject(), getDefaultIssuer())
	_, keyPEM2 := generateTestCertificate(t, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), []string{"other.com"}, getDefaultSubject(), getDefaultIssuer())

	_, err := ParseTLSCertificate(certPEM1, keyPEM2)
	if err == nil {
		t.Fatal("Expected error for mismatched certificate and key")
	}

	expectedErrPrefix := "certificate and key do not match:"
	if !strings.Contains(err.Error(), expectedErrPrefix) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrPrefix, err.Error())
	}
}

func TestParseTLSCertificate_SubjectIssuerParsing(t *testing.T) {
	now := time.Now()
	notBefore := now.Add(-24 * time.Hour)
	notAfter := now.Add(24 * time.Hour)

	testSubject := pkix.Name{
		Organization:       []string{"Acme Corp"},
		OrganizationalUnit: []string{"Engineering", "Security"},
		Country:            []string{"CA"},
		Province:           []string{"Ontario"},
		Locality:           []string{"Toronto"},
		StreetAddress:      []string{"456 Business Ave"},
		PostalCode:         []string{"M5V 3A8"},
		CommonName:         "test.acme.com",
	}

	testIssuer := testSubject // Self-signed

	certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter, []string{"test.acme.com"}, testSubject, testIssuer)

	info, err := ParseTLSCertificate(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("ParseTLSCertificate failed: %v", err)
	}

	// Verify subject parsing contains expected components
	expectedComponents := []string{"CN=test.acme.com", "O=Acme Corp", "L=Toronto", "ST=Ontario", "C=CA", "POSTALCODE=M5V 3A8", "STREET=456 Business Ave", "OU=Engineering", "OU=Security"}
	for _, component := range expectedComponents {
		if !strings.Contains(info.Subject, component) {
			t.Errorf("Subject missing expected component %s, got: %s", component, info.Subject)
		}
	}

	// Verify issuer parsing (should be same as subject for self-signed cert)
	if info.Subject != info.Issuer {
		t.Error("Expected subject and issuer to be the same for self-signed certificate")
	}
}
