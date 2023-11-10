package webhook

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	ctx := context.Background()
	lgr := logr.Discard()
	cl := fake.NewClientBuilder().Build()

	c := &config{
		serviceName: "test-service",
		namespace:   "test-namespace",
		certSecret:  "test-secret",
		certName:    "test-cert",
		keyName:     "test-key",
		caName:      "test-ca",
	}

	t.Run("secret already exists", func(t *testing.T) {
		// create a secret to simulate it already existing
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      c.certSecret,
				Namespace: c.namespace,
			},
		}
		require.NoError(t, cl.Create(ctx, secret), "expected no error creating secret")

		err := c.EnsureCertificates(ctx, lgr, cl)
		require.NoError(t, err, "expected no error")
		cl.Delete(ctx, secret)
	})

	t.Run("create new secret", func(t *testing.T) {
		var exitCode int
    	osExit = func(code int) {
       		exitCode = code
    	}
		err := c.EnsureCertificates(ctx, lgr, cl)
		require.Equal(t, 1, exitCode, "expected exit code 1")
		require.NoError(t, err, "expected no error")

		// verify that the secret was created with the expected data
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      c.certSecret,
				Namespace: c.namespace,
			},
		}
		require.NoError(t, cl.Get(ctx, client.ObjectKeyFromObject(secret), secret), "expected no error getting secret")
		require.Equal(t, c.certSecret, secret.Name, "expected secret name to match")
		require.Equal(t, c.namespace, secret.Namespace, "expected secret namespace to match")
		require.NotNil(t, secret.Data[c.certName], "expected cert data to not be nil")
		require.NotNil(t, secret.Data[c.keyName], "expected key data to not be nil")
		require.NotNil(t, secret.Data[c.caName], "expected ca data to not be nil")
	})
}
