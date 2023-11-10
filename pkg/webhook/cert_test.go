package webhook

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

func TestNewCert(t *testing.T) {
	c := &config{
		serviceName: "test-service",
		namespace:   "test-namespace",
	}
	cert, err := c.newCert()
	require.NoError(t, err, "expected no error creating new cert")
	require.NotNil(t, cert, "expected cert to not be nil")
	require.NotNil(t, cert.caPem, "expected caPem to not be nil")
	require.NotNil(t, cert.certPem, "expected certPem to not be nil")
	require.NotNil(t, cert.keyPem, "expected keyPem to not be nil")

	// Verify that the CA cert is valid
	caCertBlock, _ := pem.Decode(cert.caPem)
	require.NotNil(t, caCertBlock, "expected caCertBlock to not be nil")
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	require.NoError(t, err, "expected no error parsing ca cert")
	require.Equal(t, "approuting.kubernetes.azure.com", caCert.Subject.CommonName, "expected common name to match")
	require.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, caCert.ExtKeyUsage, "expected ext key usage to match")
	require.Equal(t, x509.KeyUsageDigitalSignature|x509.KeyUsageCertSign, caCert.KeyUsage, "expected key usage to match")
	require.True(t, caCert.IsCA, "expected IsCA to be true")

	// Verify that the server cert is valid
	serverCertBlock, _ := pem.Decode(cert.certPem)
	require.NotNil(t, serverCertBlock, "expected serverCertBlock to not be nil")
	serverCert, err := x509.ParseCertificate(serverCertBlock.Bytes)
	require.NoError(t, err, "expected no error parsing server cert")
	require.Equal(t, []string{
		fmt.Sprintf("%s.%s.svc", c.serviceName, c.namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", c.serviceName, c.namespace),
	}, serverCert.DNSNames, "expected DNS names to match")
	require.Equal(t, fmt.Sprintf("%s.cert.server", c.serviceName), serverCert.Subject.CommonName, "expected common name to match")
	require.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, serverCert.ExtKeyUsage, "expected ext key usage to match")
	require.Equal(t, x509.KeyUsageDigitalSignature, serverCert.KeyUsage, "expected key usage to match")

	// Verify that the server cert is signed by the CA
	err = serverCert.CheckSignatureFrom(caCert)
	require.NoError(t, err, "expected no error checking signature")

	// Verify that the server key is valid
	serverKeyBlock, _ := pem.Decode(cert.keyPem)
	require.NotNil(t, serverKeyBlock, "expected serverKeyBlock to not be nil")
	serverKey, err := x509.ParsePKCS1PrivateKey(serverKeyBlock.Bytes)
	require.NoError(t, err, "expected no error parsing server key")
	require.Equal(t, serverCert.PublicKey, serverKey.Public(), "expected public key to match")
}
func TestEnsureCertificates(t *testing.T) {

	lgr := logr.Discard()

	t.Run("cert already exists", func(t *testing.T) {
		c := &config{
			certDir:  "testcerts",
			certName: "tls.crt",
			keyName:  "tls.key",
			caName:   "ca.crt",
		}

		err := c.EnsureCertificates(lgr)
		require.NoError(t, err, "expected no error")
	})

	t.Run("create new certs", func(t *testing.T) {

		c := &config{
			certDir:  "test-dir",
			certName: "tls.crt",
			keyName:  "tls.key",
			caName:   "ca.crt",
		}

		err := c.EnsureCertificates(lgr)
		require.NoError(t, err, "expected no error")
		require.FileExists(t, "test-dir/tls.crt", "expected tls.crt to exist")
		require.FileExists(t, "test-dir/tls.key", "expected tls.key to exist")
		require.FileExists(t, "test-dir/ca.crt", "expected ca.crt to exist")

		os.RemoveAll("test-dir")

	})
}
