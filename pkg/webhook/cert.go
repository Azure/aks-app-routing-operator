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
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
)

// this file is heavily inspired by Fleet's webhook cert gen https://github.com/Azure/fleet/blob/main/pkg/webhook/webhook.go
type cert struct {
	caPem   []byte
	certPem []byte
	keyPem  []byte
}

func (c *config) newCert() (*cert, error) {
	// CA config
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2023),
		Subject: pkix.Name{
			CommonName: "approuting.kubernetes.azure.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 years
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// CA private key
	caPrvKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, fmt.Errorf("generating ca private key: %w", err)
	}

	// self-signed CA certificate
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrvKey.PublicKey, caPrvKey)
	if err != nil {
		return nil, fmt.Errorf("creating self-signed ca certificate: %w", err)
	}

	// PEM encode CA cert
	caPEM := new(bytes.Buffer)
	if err := pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	}); err != nil {
		return nil, fmt.Errorf("pem encoding ca certificate: %w", err)
	}

	var dnsNames []string
	if c.serviceName != "" {
		dnsNames = append(dnsNames, fmt.Sprintf("%s.%s.svc", c.serviceName, c.namespace))
	}
	if c.serviceUrl != "" {
		serviceUrl, err := url.Parse(c.serviceUrl)
		if err != nil {
			return nil, fmt.Errorf("parsing service url: %w", err)
		}
		dnsNames = append(dnsNames, serviceUrl.Hostname())
	}

	// server cert config
	certCfg := &x509.Certificate{
		DNSNames:     dnsNames,
		SerialNumber: big.NewInt(2023),
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("%s.cert.server", c.serviceName),
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 5},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// server private key
	certPrvKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, fmt.Errorf("generating server cert private key: %w", err)
	}

	// sign the server cert
	certBytes, err := x509.CreateCertificate(rand.Reader, certCfg, ca, &certPrvKey.PublicKey, caPrvKey)
	if err != nil {
		return nil, fmt.Errorf("signing the server cert: %w", err)
	}

	// PEM encode the  server cert and key
	certPEM := new(bytes.Buffer)
	if err := pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}); err != nil {
		return nil, fmt.Errorf("pem encoding the server cert: %w", err)
	}

	certPrvKeyPEM := new(bytes.Buffer)
	if err := pem.Encode(certPrvKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrvKey),
	}); err != nil {
		return nil, fmt.Errorf("pem encoding the cert private key: %w", err)
	}

	return &cert{
		caPem:   caPEM.Bytes(),
		certPem: certPEM.Bytes(),
		keyPem:  certPrvKeyPEM.Bytes(),
	}, nil
}

func (c *config) EnsureCertificates(lgr logr.Logger) error {
	lgr.Info("ensuring certificates")

	lgr.Info("checking if certs exists")
	needToGen := false
	certsFiles := []string{c.certName, c.keyName, c.caName}
	for _, certFile := range certsFiles {
		path := filepath.Join(c.certDir, certFile)
		if _, err := os.ReadFile(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				needToGen = true
				break
			}

			return fmt.Errorf("reading cert file %s: %w", path, err)
		}
	}

	if !needToGen {
		lgr.Info("certs already exist")
		return nil
	}

	lgr.Info("certs do not exist, creating new certs")
	newCert, err := c.newCert()
	if err != nil {
		return fmt.Errorf("creating new cert: %w", err)
	}

	// we need to fully clean any certs so we can ensure that our certs are the ones being used
	lgr.Info("fully cleaning any old certs")
	if err := os.RemoveAll(c.certDir); err != nil {
		return fmt.Errorf("removing old certs: %w", err)
	}

	lgr.Info("creating new certs dir")
	if err := os.MkdirAll(c.certDir, 0755); err != nil {
		return fmt.Errorf("creating new certs dir: %w", err)
	}

	// we use O_EXCL to ensure the file doesn't exist when we open it.
	// this ensures that another replica doesn't attempt to write the same file at the same time
	// which would cause unintended sideeffects. Only one instance of the operator should be
	// generating certs at a time which this guarantees.
	openFileFlags := os.O_CREATE | os.O_EXCL | os.O_RDWR

	lgr.Info("writing cert")
	certPath := filepath.Join(c.certDir, c.certName)
	certFile, err := os.OpenFile(certPath, openFileFlags, 0600)
	if err != nil {
		return fmt.Errorf("opening cert file: %w", err)
	}
	defer certFile.Close()
	if _, err := certFile.Write(newCert.certPem); err != nil {
		return fmt.Errorf("writing cert: %w", err)
	}

	lgr.Info("writing key")
	keyPath := filepath.Join(c.certDir, c.keyName)
	keyFile, err := os.OpenFile(keyPath, openFileFlags, 0600)
	if err != nil {
		return fmt.Errorf("opening key file: %w", err)
	}
	defer keyFile.Close()
	if _, err := keyFile.Write(newCert.keyPem); err != nil {
		return fmt.Errorf("writing key: %w", err)
	}

	lgr.Info("writing ca")
	caPath := filepath.Join(c.certDir, c.caName)
	caFile, err := os.OpenFile(caPath, openFileFlags, 0600)
	if err != nil {
		return fmt.Errorf("opening ca file: %w", err)
	}
	defer caFile.Close()
	if _, err := caFile.Write(newCert.caPem); err != nil {
		return fmt.Errorf("writing ca: %w", err)
	}

	lgr.Info("certs created")
	return nil
}
