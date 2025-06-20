package util

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func Test_gatewayServiceAccountIndexFn(t *testing.T) {
	tcs := []struct {
		name                    string
		gateway                 *gatewayv1.Gateway
		expectedServiceAccounts []string
	}{
		{
			name:    "no listeners",
			gateway: &gatewayv1.Gateway{Spec: gatewayv1.GatewaySpec{}},
		},
		{
			name: "no tls",
			gateway: &gatewayv1.Gateway{
				Spec: gatewayv1.GatewaySpec{
					Listeners: []gatewayv1.Listener{
						{
							Name: "test-listener",
						},
					},
				},
			},
		},
		{
			name: "no tls options",
			gateway: &gatewayv1.Gateway{
				Spec: gatewayv1.GatewaySpec{
					Listeners: []gatewayv1.Listener{
						{
							Name: "test-listener",
							TLS:  &gatewayv1.GatewayTLSConfig{},
						},
					},
				},
			},
		},
		{
			name: "no service accounts",
			gateway: &gatewayv1.Gateway{
				Spec: gatewayv1.GatewaySpec{
					Listeners: []gatewayv1.Listener{
						{
							Name: "test-listener",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"test-key": "test-value",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple service accounts",
			gateway: &gatewayv1.Gateway{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Gateway",
					APIVersion: "gateway.networking.k8s.io/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gw",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "istio",
					Listeners: []gatewayv1.Listener{
						{
							Name: "test-listener",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
						{
							Name: "test-listener-2",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a35",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa-2",
								},
							},
						},
					},
				},
			},
			expectedServiceAccounts: []string{"test-sa", "test-sa-2"},
		},
		{
			name: "duplicate service accounts",
			gateway: &gatewayv1.Gateway{
				Spec: gatewayv1.GatewaySpec{
					Listeners: []gatewayv1.Listener{
						{
							Name: "test-listener",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
						{
							Name: "test-listener-2",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			expectedServiceAccounts: []string{"test-sa"},
		},
	}

	for _, tc := range tcs {
		actual := gatewayServiceAccountIndexFn(tc.gateway)
		require.ElementsMatch(t, tc.expectedServiceAccounts, actual)
	}
}

func Test_generateGatewayGetter(t *testing.T) {
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
			filepath.Join("..", "..", "config", "gatewaycrd"),
		},
	}

	testRestConfig, err := testenv.Start()

	require.NoError(t, err)

	m, err := manager.New(testRestConfig, manager.Options{
		Scheme: s,
	})
	require.NoError(t, err)

	require.NoError(t, AddGatewaySerAddGatewayServiceAccountIndex(m.GetFieldIndexer(), "spec.listeners.tls.options.kubernetes.azure.com/tls-cert-service-account"))
	require.NoError(t, m.GetClient().Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}))
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
			name: "matching gateways",
			serviceAccountObj: &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceAccount",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"azure.workload.identity/client-id": "test-client-id",
					},
				},
			},
			existingGateways: []client.Object{&gatewayv1.Gateway{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Gateway",
					APIVersion: "gateway.networking.k8s.io/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gw",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "istio",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "test-listener",
							Port:     20,
							Protocol: gatewayv1.ProtocolType("HTTPS"),
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
						{
							Name:     "test-listener-2",
							Port:     21,
							Protocol: gatewayv1.ProtocolType("HTTPS"),
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a35",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa-2",
								},
							},
						},
					},
				},
			}},
			expectedReqs: []ctrl.Request{
				{
					NamespacedName: types.NamespacedName{Name: "test-gw", Namespace: "test-ns"},
				},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err = m.Start(ctx)
		require.NoError(t, err)
	}()

	for _, tc := range tests {
		for _, gw := range tc.existingGateways {
			err = m.GetClient().Create(ctx, gw)
			require.NoError(t, err)
		}

		time.Sleep(1 * time.Second) // wait for manager to start + cache update

		testFunc := GenerateGatewayGetter(m, "spec.listeners.tls.options.kubernetes.azure.com/tls-cert-service-account")
		actualReqs := testFunc(ctx, tc.serviceAccountObj)
		require.ElementsMatch(t, tc.expectedReqs, actualReqs)

		// clean up
		for _, gw := range tc.existingGateways {
			err = m.GetClient().Delete(ctx, gw)
			require.NoError(t, err)
		}

	}
	// done with tests, so shut down the manager
	cancel()
}
