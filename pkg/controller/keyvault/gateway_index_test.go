package keyvault

import (
	"context"
	"path/filepath"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	cfgv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func Test_generateGatewayGetter(t *testing.T) {
	c := testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(secv1.Install(s))
	utilruntime.Must(cfgv1alpha2.AddToScheme(s))
	utilruntime.Must(policyv1alpha1.AddToScheme(s))
	utilruntime.Must(approutingv1alpha1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))
	utilruntime.Must(gatewayv1.Install(s))

	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "gatewaycrd"),
		},
	}

	testRestConfig, err := testenv.Start()
	require.NoError(t, err)

	m, err := manager.New(testRestConfig, manager.Options{
		Scheme: s,
		NewClient: func(config *rest.Config, options client.Options) (client.Client, error) {
			return c, nil
		},
	})

	require.NoError(t, AddGatewayServiceAccountIndex(m.GetFieldIndexer(), "spec.listeners.tls.options.kubernetes.azure.com/tls-cert-service-account"))

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
		{
			name:              "matching gateways",
			serviceAccountObj: annotatedServiceAccount,
			existingGateways:  []client.Object{gatewayWithTwoServiceAccounts},
			expectedReqs: []ctrl.Request{
				{
					NamespacedName: types.NamespacedName{Name: gatewayWithTwoServiceAccounts.Name, Namespace: gatewayWithTwoServiceAccounts.Namespace},
				},
			},
		},
	}
	for _, tc := range tests {
		ctx := context.Background()

		for _, gw := range tc.existingGateways {
			err = m.GetClient().Create(ctx, gw)
			require.NoError(t, err)
		}

		go func() {
			err = m.Start(ctx)
			//require.NoError(t, err)
		}()

		testFunc := generateGatewayGetter(m, "spec.listeners.tls.options.kubernetes.azure.com/tls-cert-service-account")
		actualReqs := testFunc(ctx, tc.serviceAccountObj)
		require.ElementsMatch(t, tc.expectedReqs, actualReqs)
		ctx.Done()

		// clean up
		for _, gw := range tc.existingGateways {
			err = m.GetClient().Delete(ctx, gw)
			require.NoError(t, err)
		}
	}
}
