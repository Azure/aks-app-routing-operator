package webhook

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// this file is heavily inspired by
// https://www.velotio.com/engineering-blog/managing-tls-certificate-for-kubernetes-admission-webhook
// https://github.com/Azure/fleet/blob/main/pkg/webhook/webhook.go

type selfSignedCert struct {
	ca, cert, key []byte
}

func genCert(serviceName, ns string) (selfSignedCert, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2023),
		Subject: pkix.Name{
			CommonName: "approuting.kubernetes.azure.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return selfSignedCert{}, fmt.Errorf("generating private key: %w", err)
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return selfSignedCert{}, fmt.Errorf("generating CA certificate: %w", err)
	}

	// PEM encode CA
	var caPEM bytes.Buffer
	if err := pem.Encode(&caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	}); err != nil {
		return selfSignedCert{}, fmt.Errorf("pem encoding CA: %w", err)
	}

	dnsNames := []string{
		fmt.Sprintf("%s.%s.svc", serviceName, ns),
		fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, ns),
	}

	cert := &x509.Certificate{
		DNSNames:     dnsNames,
		SerialNumber: big.NewInt(2023),
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("%s.cert.server", serviceName),
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return selfSignedCert{}, fmt.Errorf("generating private key: %w", err)
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return selfSignedCert{}, fmt.Errorf("generating certificate: %w", err)
	}

	// PEM encode cert
	var certPEM bytes.Buffer
	if err := pem.Encode(&certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}); err != nil {
		return selfSignedCert{}, fmt.Errorf("pem encoding certificate: %w", err)
	}

	var certPrvKeyPEM bytes.Buffer
	if err := pem.Encode(&certPrvKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	}); err != nil {
		return selfSignedCert{}, fmt.Errorf("pem encoding certificate private key: %w", err)
	}

	return selfSignedCert{
		ca:   caPEM.Bytes(),
		cert: certPEM.Bytes(),
		key:  certPrvKeyPEM.Bytes(),
	}, nil

}

func (s selfSignedCert) save(dir string) error {
	// always cleanup old webhook certs
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing dir %s: %w", dir, err)
	}

	certPath := filepath.Join(dir, "tls.crt")

	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return fmt.Errorf("creating dir %s: %w", dir, err)
	}

	certFile, err := os.OpenFile(certPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("opening file %s: %w", certPath, err)
	}
	defer certFile.Close()

	certBlock, _ := pem.Decode(s.cert)
	if certBlock == nil {
		return errors.New("failed to decode certificate PEM")
	}

	if err := pem.Encode(certFile, certBlock); err != nil {
		return fmt.Errorf("writing certificate PEM: %w", err)
	}

	keyPath := filepath.Join(dir, "tls.key")
	keyFile, err := os.OpenFile(keyPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening file %s: %w", keyPath, err)
	}
	defer keyFile.Close()

	keyBlock, _ := pem.Decode(s.key)
	if keyBlock == nil {
		return errors.New("failed to decode key PEM")
	}

	if err := pem.Encode(keyFile, keyBlock); err != nil {
		return fmt.Errorf("writing key PEM: %w", err)
	}

	return nil
}
