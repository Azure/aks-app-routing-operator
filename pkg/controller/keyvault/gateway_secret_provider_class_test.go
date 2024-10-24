package keyvault

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func Test_GenerateGwListenerCertName(t *testing.T) {
	gwName := "test-gateway"
	gwListener := "test-listener"
	require.Equal(t, "kv-gw-cert-test-gateway-test-listener", GenerateGwListenerCertName(gwName, gatewayv1.SectionName(gwListener)))

	longName := make([]byte, 255)
	for i := 0; i < len(longName); i++ {
		longName[i] = 'a'
	}

	prefix := "kv-gw-cert-"
	for i := 0; i < 253-len(prefix); i++ {
		prefix += "a"
	}

	require.Equal(t, prefix, GenerateGwListenerCertName(gwName, gatewayv1.SectionName(gwListener)))

}

func Test_listenerIsKvEnabled(t *testing.T) {
	enabledListener := gatewayv1.Listener{
		TLS: &gatewayv1.GatewayTLSConfig{
			Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri": "test-cert-uri",
			},
		},
	}

	require.Equal(t, true, listenerIsKvEnabled(enabledListener))

	onlySaListener := gatewayv1.Listener{
		TLS: &gatewayv1.GatewayTLSConfig{
			Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-service-account": "test-SA",
			},
		},
	}
	require.Equal(t, false, listenerIsKvEnabled(onlySaListener))

	onlyCidListener := gatewayv1.Listener{
		TLS: &gatewayv1.GatewayTLSConfig{
			Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-client-id": "test-client-id",
			},
		},
	}
	require.Equal(t, false, listenerIsKvEnabled(onlyCidListener))

	nilTlsListener := gatewayv1.Listener{}
	require.Equal(t, false, listenerIsKvEnabled(nilTlsListener))

}

func Test_retrieveClientIdFromListener(t *testing.T) {
	tcs := []struct {
		name                string
		namespace           string
		options             map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue
		generateClientState func() client.Client
		expectedClientId    string
		expectedError       error
	}{
		{
			name: "cert URI without sa or cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
			},
			generateClientState: func() client.Client { return fake.NewClientBuilder().Build() },
			expectedClientId:    "",
			expectedError:       errors.New("user specified cert URI but no serviceaccount or clientid in a listener"),
		},
		{
			name: "cert URI with nonexistent sa",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			generateClientState: func() client.Client { return fake.NewClientBuilder().Build() },
			expectedClientId:    "",
			expectedError:       errors.New("user-specified serviceAccount test-sa does not exist"),
		},
		{
			name: "cert URI with sa with no annotation, without cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			namespace: "test-ns",
			generateClientState: func() client.Client {
				return fake.NewClientBuilder().WithObjects(
					&corev1.ServiceAccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ServiceAccount",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-sa",
							Namespace: "test-ns",
						},
					}).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("user-specified service account doesn't contain annotation with clientId"),
		},
		{
			name: "cert URI with sa with correct annotation, without cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			namespace: "test-ns",
			generateClientState: func() client.Client {
				return fake.NewClientBuilder().WithObjects(
					&corev1.ServiceAccount{
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
					}).Build()
			},
			expectedClientId: "test-client-id",
			expectedError:    nil,
		},
		{
			name: "cert URI without sa with cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
				"kubernetes.azure.com/tls-cert-client-id":    "test-client-id",
			},
			generateClientState: func() client.Client { return fake.NewClientBuilder().Build() },
			expectedClientId:    "test-client-id",
			expectedError:       nil,
		},
		{
			name: "cert URI with sa and cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
				"kubernetes.azure.com/tls-cert-client-id":       "test-client-id",
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			generateClientState: func() client.Client { return fake.NewClientBuilder().Build() },
			expectedClientId:    "",
			expectedError:       errors.New("user specified both serviceaccount and a clientId in the same listener"),
		},
		{
			name:                "no cert URI without sa or cid",
			options:             map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{},
			generateClientState: func() client.Client { return fake.NewClientBuilder().Build() },
			expectedClientId:    "",
			expectedError:       errors.New("none of the required TLS options were specified"),
		},
		{
			name: "no cert URI with sa not cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			generateClientState: func() client.Client { return fake.NewClientBuilder().Build() },
			expectedClientId:    "",
			expectedError:       errors.New("user specified clientId or SA but no cert URI in a listener"),
		},
		{
			name: "no cert URI without sa with cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-client-id": "test-client-id",
			},
			generateClientState: func() client.Client { return fake.NewClientBuilder().Build() },
			expectedClientId:    "",
			expectedError:       errors.New("user specified clientId or SA but no cert URI in a listener"),
		},
		{
			name: "no cert URI with sa and cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-client-id":       "test-client-id",
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			generateClientState: func() client.Client { return fake.NewClientBuilder().Build() },
			expectedClientId:    "",
			expectedError:       errors.New("user specified both serviceaccount and a clientId in the same listener"),
		},
	}

	for _, tc := range tcs {
		clientId, err := retrieveClientIdFromListener(context.Background(), tc.generateClientState(), tc.namespace, tc.options)
		if tc.expectedError != nil {
			require.Equal(t, tc.expectedError.Error(), err.Error())
		} else {
			require.Equal(t, nil, err)
			require.Equal(t, tc.expectedClientId, clientId)
		}

	}
}

func Test_GatewaySecretClassProviderReconciler(t *testing.T) {
	var (
		gwWithCertWithoutOthers = &gatewayv1.Gateway{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Gateway",
				APIVersion: "gateway.networking.k8s.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gw",
				Namespace: "test-ns",
			},
			Spec: gatewayv1.GatewaySpec{
				Listeners: []gatewayv1.Listener{
					{
						Name: "test-listener",
						TLS: &gatewayv1.GatewayTLSConfig{
							Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
								"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
							},
						},
					},
				},
			},
		}

		gwWithSa = &gatewayv1.Gateway{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Gateway",
				APIVersion: "gateway.networking.k8s.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gw",
				Namespace: "test-ns",
			},
			Spec: gatewayv1.GatewaySpec{
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
				},
			},
		}

		gatewayWithCid = &gatewayv1.Gateway{
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{},
			Spec: gatewayv1.GatewaySpec{
				Listeners: []gatewayv1.Listener{
					{
						Name: "test-listener",
						TLS: &gatewayv1.GatewayTLSConfig{
							Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
								"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
								"kubernetes.azure.com/tls-cert-client-id":    "test-client-id",
							},
						},
					},
				},
			},
		}

		gwWithCidAndSa = &gatewayv1.Gateway{
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{},
			Spec: gatewayv1.GatewaySpec{
				Listeners: []gatewayv1.Listener{
					{
						Name: "test-listener",
						TLS: &gatewayv1.GatewayTLSConfig{
							Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
								"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
								"kubernetes.azure.com/tls-cert-client-id":       "test-client-id",
								"kubernetes.azure.com/tls-cert-service-account": "test-sa",
							},
						},
					},
				},
			},
		}

		gwWithoutTls = &gatewayv1.Gateway{
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{},
			Spec: gatewayv1.GatewaySpec{
				Listeners: []gatewayv1.Listener{
					{
						Name: "test-listener",
						TLS: &gatewayv1.GatewayTLSConfig{
							Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{},
						},
					},
				},
			},
		}

		validSpc = &secv1.SecretProviderClass{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "secrets-store.csi.x-k8s.io/v1",
				Kind:       "SecretProviderClass",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kv-gw-cert-test-gw-test-listener",
				Namespace: "test-ns",
				Labels:    manifests.GetTopLevelLabels(),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "gateway.networking.k8s.io/v1",
					Controller: util.ToPtr(true),
					Kind:       "Gateway",
					Name:       "test-gw",
				}},
			},
			Spec: secv1.SecretProviderClassSpec{
				Provider: secv1.Provider("azure"),
				SecretObjects: []*secv1.SecretObject{{
					SecretName: "kv-gw-cert-test-gw-test-listener",
					Type:       "kubernetes.io/tls",
					Data: []*secv1.SecretObjectData{
						{
							ObjectName: "testcert",
							Key:        "tls.key",
						},
						{
							ObjectName: "testcert",
							Key:        "tls.crt",
						},
					},
				}},
				// https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/getting-started/usage/#create-your-own-secretproviderclass-object
				Parameters: map[string]string{
					"keyvaultName":           "testvault",
					"useVMManagedIdentity":   "true",
					"userAssignedIdentityID": "test-client-id",
					"tenantId":               "test-tenant-id",
					"objects":                `{"array": [{"objectName": "testcert", "objectType": "secret", "objectVersion": "f8982febc6894c0697b884f946fb1a34"}]}`,
				},
			},
		}
	)

	tcs := []struct {
		name                string
		gwObj               *gatewayv1.Gateway
		generateClientState func() client.Client
		expectedSpc         *secv1.SecretProviderClass
		expectedError       error
	}{
		{
			name:                "cert URI without sa or cid",
			gwObj:               gwWithCertWithoutOthers,
			expectedSpc:         nil,
			generateClientState: func() client.Client { return fake.NewClientBuilder().WithObjects(gwWithCertWithoutOthers).Build() },
			expectedError:       nil,
		},
		{
			name:                "cert URI with nonexistent sa",
			gwObj:               gwWithSa,
			generateClientState: func() client.Client { return fake.NewClientBuilder().WithObjects(gwWithSa).Build() },
			expectedSpc:         nil,
			expectedError:       nil,
		},
		{
			name:  "cert URI with sa with no annotation, without cid",
			gwObj: gwWithSa,
			generateClientState: func() client.Client {
				return fake.NewClientBuilder().WithObjects(
					gwWithSa,
					&corev1.ServiceAccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ServiceAccount",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-sa",
							Namespace: "test-ns",
						},
					}).Build()
			},
			expectedSpc:   nil,
			expectedError: nil,
		},
		{
			name:  "cert URI with sa with correct annotation, without cid",
			gwObj: gwWithSa,
			generateClientState: func() client.Client {
				return fake.NewClientBuilder().WithObjects(
					gwWithSa,
					&corev1.ServiceAccount{
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
					}).Build()
			},
			expectedSpc:   validSpc,
			expectedError: nil,
		},
		{
			name:                "cert URI without sa with cid",
			gwObj:               gatewayWithCid,
			generateClientState: func() client.Client { return fake.NewClientBuilder().WithObjects(gatewayWithCid).Build() },
			expectedSpc:         validSpc,
			expectedError:       nil,
		},
		{
			name:                "cert URI with sa and cid",
			gwObj:               gwWithCidAndSa,
			generateClientState: func() client.Client { return fake.NewClientBuilder().WithObjects(gwWithCidAndSa).Build() },
			expectedSpc:         nil,
			expectedError:       errors.New("user specified both serviceaccount and a clientId in the same listener"),
		},
		{
			name:  "no cert URI specified",
			gwObj: gwWithoutTls,
			// ensure it was originally there and that reconciler deletes it
			generateClientState: func() client.Client { return fake.NewClientBuilder().WithObjects(gwWithoutTls, validSpc).Build() },
			expectedSpc:         nil,
		},
	}

	for _, tc := range tcs {
		// ensure spc is blank if need be
		c := tc.generateClientState()
		g := GatewaySecretProviderClassReconciler{
			client: c,
			config: &config.Config{
				TenantID: "test-tenant-id",
			},
		}

		// Define initial metrics
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: tc.gwObj.Namespace, Name: tc.gwObj.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, gatewaySecretProviderControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, gatewaySecretProviderControllerName, metrics.LabelSuccess)

		_, err := g.Reconcile(context.Background(), req)

		if tc.expectedError == nil {
			require.Equal(t, testutils.GetErrMetricCount(t, kvSaControllerName), beforeErrCount)
			require.Greater(t, testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)
		} else {
			require.Equal(t, tc.expectedError.Error(), err.Error())
			require.Greater(t, testutils.GetErrMetricCount(t, kvSaControllerName), beforeErrCount)
		}

		actualSpc := &secv1.SecretProviderClass{}
		err = c.Get(context.Background(), types.NamespacedName{Namespace: validSpc.Namespace, Name: validSpc.Name}, actualSpc)

		if tc.expectedSpc == nil {
			require.Nil(t, client.IgnoreNotFound(err))
			require.NotNil(t, err)

		} else {
			require.Equal(t, tc.expectedSpc.TypeMeta, actualSpc.TypeMeta)
			require.Equal(t, tc.expectedSpc.ObjectMeta, actualSpc.ObjectMeta)
			require.Equal(t, tc.expectedSpc.Spec, actualSpc.Spec)
		}

	}
}
