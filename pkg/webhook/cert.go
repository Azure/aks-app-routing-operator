package webhook

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	dnsNames := []string{
		fmt.Sprintf("%s.%s.svc", c.serviceName, c.namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", c.serviceName, c.namespace),
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

func (c *config) EnsureCertificates(ctx context.Context, lgr logr.Logger, cl client.Client) error {
	lgr.Info("ensuring certificates")

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.certSecret,
			Namespace: c.namespace,
		},
	}
	lgr.Info("checking if secret exists")
	err := cl.Get(ctx, client.ObjectKeyFromObject(secret), secret)
	if err == nil { // secret exists
		lgr.Info("secret already exists")
		return nil
	}
	if err != nil && !k8serrors.IsNotFound(err) {
		lgr.Error(err, "failed to get secret")
		return fmt.Errorf("getting secret: %w", err)
	}

	newCert, err := c.newCert()
	if err != nil {
		return fmt.Errorf("creating new cert: %w", err)
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.certSecret,
			Namespace: c.namespace,
		},
		Data: map[string][]byte{
			c.certName: newCert.certPem,
			c.keyName:  newCert.keyPem,
			c.caName:   newCert.caPem,
		},
	}
	lgr.Info("creating new secret")
	if err := cl.Create(ctx, newSecret); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			// if secret already exists assume it was a race condition with another instance of this
			// todo: rotate pods
			lgr.Info("secret already exists")
			lgr.Info("exiting so pod can be restarted to mount new secret faster")
			os.Exit(1)
			return nil
		}

		lgr.Error(err, "failed to create secret")
		return fmt.Errorf("creating secret: %w", err)
	}

	lgr.Info("secret created")
	lgr.Info("exiting so pod can be restarted to mount new secret faster")
	os.Exit(1) // it's not actually an error but for init containers to cause a pod restart we need this to be error signal

	return nil
}
