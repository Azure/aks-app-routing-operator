package keyvault

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	kvcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	buildSpcTestIngress = &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &spcTestIngressClassName,
		},
	}

	buildSpcTestNginxIngress = &v1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nic",
			Namespace: "test-namespace",
		},
		Spec: v1alpha1.NginxIngressControllerSpec{
			IngressClassName: spcTestNginxIngressClassName,
			DefaultSSLCertificate: &v1alpha1.DefaultSSLCertificate{
				Secret:      nil,
				KeyVaultURI: &spcTestKeyVaultURI},
		},
	}

	testName      = "testName"
	maxSizeString = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbbbbbbbbbbbbb"
)

func buildTestSpcConfig(clientId, tenantID, cloud, name, certUri string) spcConfig {
	spcTestConf := spcConfig{
		ClientId:        clientId,
		TenantId:        tenantID,
		Cloud:           cloud,
		Name:            name,
		KeyvaultCertUri: certUri,
	}

	return spcTestConf
}

func TestDefaultNginxCertName(t *testing.T) {

	testStr := DefaultNginxCertName(&v1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: testName,
		},
	})
	require.Equal(t, testStr, nginxNamePrefix+testName)

	testStr = DefaultNginxCertName(&v1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: maxSizeString,
		},
	})

	require.NotContains(t, testStr, "b")
	require.Contains(t, testStr, nginxNamePrefix)
}

func TestCertSecretName(t *testing.T) {
	require.Equal(t, "keyvault-ingressname", certSecretName("ingressname"))
	require.Equal(t, "keyvault-anotheringressname", certSecretName("anotheringressname"))
}

func TestIngressSecretProviderClassReconcilershouldDeploySpc(t *testing.T) {
	ing := buildSpcTestIngress.DeepCopy()
	ing.Annotations = map[string]string{
		"kubernetes.azure.com/tls-cert-keyvault-uri": "https://test.vault.azure.net/secrets/test-secret",
	}
	ok := shouldDeploySpc(ing)
	require.True(t, ok, "SPC should be built")

}
func TestIngressSecretProviderClassReconcilerbuildSPCCloud(t *testing.T) {
	cases := []struct {
		name, configCloud, spcCloud string
		expected                    bool
	}{
		{
			name:        "empty config cloud",
			configCloud: "",
			expected:    false,
		},
		{
			name:        "public cloud",
			configCloud: "AzurePublicCloud",
			spcCloud:    "AzurePublicCloud",
			expected:    true,
		},
		{
			name:        "sov cloud",
			configCloud: "AzureUSGovernmentCloud",
			spcCloud:    "AzureUSGovernmentCloud",
			expected:    true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ing := buildSpcTestIngress.DeepCopy()
			ing.Annotations = map[string]string{
				"kubernetes.azure.com/tls-cert-keyvault-uri": "https://test.vault.azure.net/secrets/test-secret",
			}

			spc := &secv1.SecretProviderClass{}
			err := buildSPC(spc, buildTestSpcConfig("test-msi", "test-tenant", c.configCloud, certSecretName(ing.Name), ing.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"]))
			require.NoError(t, err, "building SPC should not error")

			spcCloud, ok := spc.Spec.Parameters[kvcsi.CloudNameParameter]
			require.Equal(t, c.expected, ok, "SPC cloud annotation unexpected")
			require.Equal(t, c.spcCloud, spcCloud, "SPC cloud annotation doesn't match")
		})
	}
}

func TestNginxSecretProviderClassReconcilershouldDeploySpc(t *testing.T) {
	nic := buildSpcTestNginxIngress.DeepCopy()
	testSecretUri := "https://test.vault.azure.net/secrets/test-secret"
	nic.Spec.DefaultSSLCertificate.KeyVaultURI = &testSecretUri

	ok := shouldDeploySpc(nic)
	require.True(t, ok, "SPC should be built")
}

func TestNginxSecretProviderClassReconcilerbuildSPCCloud(t *testing.T) {
	cases := []struct {
		name, configCloud, spcCloud string
		expected                    bool
	}{
		{
			name:        "empty config cloud",
			configCloud: "",
			expected:    false,
		},
		{
			name:        "public cloud",
			configCloud: "AzurePublicCloud",
			spcCloud:    "AzurePublicCloud",
			expected:    true,
		},
		{
			name:        "sov cloud",
			configCloud: "AzureUSGovernmentCloud",
			spcCloud:    "AzureUSGovernmentCloud",
			expected:    true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nic := buildSpcTestNginxIngress.DeepCopy()
			testSecretUri := "https://test.vault.azure.net/secrets/test-secret"

			spc := &secv1.SecretProviderClass{}
			err := buildSPC(spc, buildTestSpcConfig("test-msi", "test-tenant", c.configCloud, DefaultNginxCertName(nic), testSecretUri))
			require.NoError(t, err, "building SPC should not error")

			spcCloud, ok := spc.Spec.Parameters[kvcsi.CloudNameParameter]
			require.Equal(t, c.expected, ok, "SPC cloud annotation unexpected")
			require.Equal(t, c.spcCloud, spcCloud, "SPC cloud annotation doesn't match")
		})
	}
}

func TestIngressSecretProviderClassReconcilerBuildSPCInvalidURLs(t *testing.T) {
	invalidURLIng := &netv1.Ingress{
		Spec: netv1.IngressSpec{
			IngressClassName: &spcTestIngressClassName,
		},
	}

	invalidUrlNic := &v1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nic",
			Namespace: "test-namespace",
		},
		Spec: v1alpha1.NginxIngressControllerSpec{
			IngressClassName: spcTestNginxIngressClassName,
			DefaultSSLCertificate: &v1alpha1.DefaultSSLCertificate{
				Secret:      nil,
				KeyVaultURI: nil},
		},
	}

	t.Run("nil annotations ing", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()

		ok := shouldDeploySpc(ing)
		assert.False(t, ok)
	})

	t.Run("nil cert nic", func(t *testing.T) {
		nic := invalidUrlNic.DeepCopy()

		ok := shouldDeploySpc(nic)
		assert.False(t, ok)
	})

	t.Run("empty url ing", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": ""}

		ok := shouldDeploySpc(ing)
		assert.False(t, ok)
	})

	t.Run("empty url nic", func(t *testing.T) {
		nic := invalidUrlNic.DeepCopy()
		nic.Spec.DefaultSSLCertificate.KeyVaultURI = to.Ptr("")

		ok := shouldDeploySpc(nic)
		assert.False(t, ok)
	})

	t.Run("url with control character", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		cc := string([]byte{0x7f})
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": cc}

		err := buildSPC(&secv1.SecretProviderClass{}, buildTestSpcConfig("test-client-id", "test-tenant-id", "AzurePublicCloud", certSecretName(ing.Name), ing.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"]))

		_, expectedErr := url.Parse(cc) // the exact error depends on operating system
		require.EqualError(t, err, fmt.Sprintf("%s", expectedErr))
	})

	t.Run("url with one path segment", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": "http://test.com/foo"}

		err := buildSPC(&secv1.SecretProviderClass{}, buildTestSpcConfig("test-client-id", "test-tenant-id", "AzurePublicCloud", certSecretName(ing.Name), ing.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"]))
		require.EqualError(t, err, "uri Path contains too few segments: has: 2 requires greater than: 3 uri path: /foo")
	})
}

func TestBuildSPCWithWrongObject(t *testing.T) {
	var obj client.Object

	ok := shouldDeploySpc(obj)
	assert.False(t, ok)
}

func TestUserErrors(t *testing.T) {
	testMsg := "test error message"
	testError := newUserError(errors.New("test"), testMsg)
	var userErr userError

	assert.True(t, testError.UserError() == testMsg)
	assert.True(t, errors.As(testError, &userErr))
}

func TestShouldReconcileGateway(t *testing.T) {
	nonIstioGateway := &gatewayv1.Gateway{
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "test-gateway-class",
		},
	}

	require.False(t, shouldReconcileGateway(nonIstioGateway))

	istioGateway := &gatewayv1.Gateway{
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
		},
	}

	require.True(t, shouldReconcileGateway(istioGateway))
}
