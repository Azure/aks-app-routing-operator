package keyvault

import (
	"context"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	cfgv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func Test_generateGatewayGetter(t *testing.T) {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(secv1.Install(s))
	utilruntime.Must(cfgv1alpha2.AddToScheme(s))
	utilruntime.Must(policyv1alpha1.AddToScheme(s))
	utilruntime.Must(approutingv1alpha1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))
	utilruntime.Must(gatewayv1.Install(s))

	type testcase struct {
		name              string
		serviceAccountObj client.Object
		existingGateways  []client.Object
		expectedReqs      []ctrl.Request
	}
	tests := []testcase{
		{
			name:              "non serviceaccount object",
			serviceAccountObj: &corev1.Pod{},
		},
		{
			name: "no matching gateways",
			serviceAccountObj: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "test-ns",
				},
			},
		},
	}

	for _, tc := range tests {
		ctx := context.Background()
		m, err := ctrl.NewManager(restConfig, ctrl.Options{
			Scheme: s,
		})
		require.NoError(t, err)
		require.NoError(t, AddGatewayServiceAccountIndex(m.GetFieldIndexer(), "spec.listeners.tls.options.kubernetes.azure.com/tls-cert-service-account"))

		testFunc := generateGatewayGetter(m, "spec.listeners.tls.options.kubernetes.azure.com/tls-cert-service-account")
		for _, gw := range tc.existingGateways {
			require.NoError(t, m.GetClient().Create(ctx, gw))
		}
		actualReqs := testFunc(ctx, tc.serviceAccountObj)
		require.ElementsMatch(t, tc.expectedReqs, actualReqs)

	}
}
