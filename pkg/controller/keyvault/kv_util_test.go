package keyvault

import (
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var (
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
