package keyvault

import (
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var (
	maxString = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbbbbbbbbbbbbb"
)

func TestDefaultNginxCertName(t *testing.T) {
	testStr := DefaultNginxCertName(&v1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: maxString,
		},
	})

	require.NotContains(t, testStr, "b")
	require.Contains(t, testStr, nginxNamePrefix)
}
