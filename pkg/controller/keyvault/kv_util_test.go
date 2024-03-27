package keyvault

import (
	"errors"
	"fmt"
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	kvcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
	"testing"
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
			ok, err := buildSPC(ing, spc, buildTestSpcConfig("test-msi", "test-tenant", c.configCloud))
			require.NoError(t, err, "building SPC should not error")
			require.True(t, ok, "SPC should be built")

			spcCloud, ok := spc.Spec.Parameters[kvcsi.CloudNameParameter]
			require.Equal(t, c.expected, ok, "SPC cloud annotation unexpected")
			require.Equal(t, c.spcCloud, spcCloud, "SPC cloud annotation doesn't match")
		})
	}
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
			nic.Spec.DefaultSSLCertificate.KeyVaultURI = &testSecretUri

			spc := &secv1.SecretProviderClass{}
			ok, err := buildSPC(nic, spc, buildTestSpcConfig("test-msi", "test-tenant", c.configCloud))
			require.NoError(t, err, "building SPC should not error")
			require.True(t, ok, "SPC should be built")

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

	t.Run("nil annotations", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()

		ok, err := buildSPC(ing, &secv1.SecretProviderClass{}, spcTestDefaultConf)
		assert.False(t, ok)
		require.NoError(t, err)
	})

	t.Run("empty url", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": ""}

		ok, err := buildSPC(ing, &secv1.SecretProviderClass{}, spcTestDefaultConf)
		assert.False(t, ok)
		require.NoError(t, err)
	})

	t.Run("url with control character", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		cc := string([]byte{0x7f})
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": cc}

		ok, err := buildSPC(ing, &secv1.SecretProviderClass{}, spcTestDefaultConf)
		assert.False(t, ok)
		_, expectedErr := url.Parse(cc) // the exact error depends on operating system
		require.EqualError(t, err, fmt.Sprintf("%s", expectedErr))
	})

	t.Run("url with one path segment", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": "http://test.com/foo"}

		ok, err := buildSPC(ing, &secv1.SecretProviderClass{}, spcTestDefaultConf)
		assert.False(t, ok)
		require.EqualError(t, err, "uri Path contains too few segments: has: 2 requires greater than: 3 uri path: /foo")
	})
}

func TestBuildSPCWithWrongObject(t *testing.T) {
	var obj client.Object

	ok, err := buildSPC(obj, &secv1.SecretProviderClass{}, spcTestDefaultConf)
	assert.False(t, ok)
	require.EqualError(t, err, fmt.Sprintf("incorrect object type: %s", obj))
}

func TestUserErrors(t *testing.T) {
	testMsg := "test error message"
	testError := newBuildSPCUserError(errors.New("test"), testMsg)
	var userErr userError
	var buildSPCUserErr buildSPCUserError

	assert.True(t, testError.UserError() == testMsg)
	assert.True(t, errors.As(testError, &userErr))
	assert.True(t, errors.As(testError, &buildSPCUserErr))
}
