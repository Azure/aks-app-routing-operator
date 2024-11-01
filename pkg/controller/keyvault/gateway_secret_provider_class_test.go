package keyvault

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func Test_GenerateGwListenerCertName(t *testing.T) {
	gwName := "test-gateway"
	gwListener := "test-listener"
	require.Equal(t, "kv-gw-cert-test-gateway-test-listener", generateGwListenerCertName(gwName, gatewayv1.SectionName(gwListener)))

	longName := make([]byte, 255)
	for i := 0; i < len(longName); i++ {
		longName[i] = 'a'
	}

	prefix := "kv-gw-cert-"
	aCount := 253 - len(prefix)
	for i := 0; i < aCount; i++ {
		prefix += "a"
	}

	require.Equal(t, prefix, generateGwListenerCertName(string(longName), gatewayv1.SectionName(gwListener)))

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
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("user specified cert URI but no serviceaccount or clientid in a listener"),
		},
		{
			name: "cert URI with nonexistent sa",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("user-specified serviceAccount test-sa does not exist"),
		},
		{
			name: "cert URI with sa with no annotation, without cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			namespace: "test-ns",
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(
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
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(
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
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "test-client-id",
			expectedError:    nil,
		},
		{
			name: "cert URI with sa and cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
				"kubernetes.azure.com/tls-cert-client-id":       "test-client-id",
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("user specified both serviceaccount and a clientId in the same listener"),
		},
		{
			name:    "no cert URI without sa or cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("none of the required TLS options were specified"),
		},
		{
			name: "no cert URI with sa not cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("user specified clientId or SA but no cert URI in a listener"),
		},
		{
			name: "no cert URI without sa with cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-client-id": "test-client-id",
			},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("user specified clientId or SA but no cert URI in a listener"),
		},
		{
			name: "no cert URI with sa and cid",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-client-id":       "test-client-id",
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("user specified clientId or SA but no cert URI in a listener"),
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
	tcs := []struct {
		name                string
		gwObj               *gatewayv1.Gateway
		generateClientState func() client.Client
		expectedSpcs        []*secv1.SecretProviderClass
		expectedError       error
		expectedUserErr     string
	}{
		{
			name:         "cert URI without sa or cid",
			gwObj:        gwWithCertWithoutOthers,
			expectedSpcs: nil,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gwWithCertWithoutOthers).Build()
			},
			expectedError:   nil,
			expectedUserErr: "Warning InvalidInput invalid TLS configuration: detected Keyvault Cert URI, but no ServiceAccount or Client ID was provided",
		},
		{
			name:  "cert URI with nonexistent sa",
			gwObj: gwWithSa,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gwWithSa).Build()
			},
			expectedSpcs:    nil,
			expectedError:   nil,
			expectedUserErr: "Warning InvalidInput invalid TLS configuration: serviceAccount test-sa does not exist",
		},
		{
			name:  "cert URI with sa with no annotation, without cid",
			gwObj: gwWithSa,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(
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
			expectedSpcs:    nil,
			expectedError:   nil,
			expectedUserErr: "Warning InvalidInput invalid TLS configuration: workload identity MSI client ID must be specified for serviceAccount test-sa with annotation azure.workload.identity/client-id",
		},
		{
			name:  "cert URI with sa with correct annotation, without cid",
			gwObj: gwWithSa,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(
					gwWithSa,
					annotatedServiceAccount,
				).Build()
			},
			expectedSpcs:  []*secv1.SecretProviderClass{clientIdSpc},
			expectedError: nil,
		},
		{
			name:  "cert URI without sa with cid",
			gwObj: gatewayWithCid,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gatewayWithCid).Build()
			},
			expectedSpcs:  []*secv1.SecretProviderClass{clientIdSpc},
			expectedError: nil,
		},
		{
			name:  "cert URI with sa listener and cid listener",
			gwObj: gatewayWithCidListenerAndSaListener,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gatewayWithCidListenerAndSaListener, annotatedServiceAccount).Build()
			},
			expectedSpcs:  []*secv1.SecretProviderClass{clientIdSpc, serviceAccountSpc},
			expectedError: nil,
		},
		{
			name:  "cert URI with sa and cid",
			gwObj: gwWithCidAndSaInSameListener,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gwWithCidAndSaInSameListener).Build()
			},
			expectedSpcs:    nil,
			expectedError:   nil,
			expectedUserErr: "Warning InvalidInput invalid TLS configuration: both ServiceAccount name and clientId have been specified, please specify one or the other",
		},
		{
			name:  "non-Istio gateway",
			gwObj: nonIstioGateway,
			// ensure it was originally there and that reconciler deletes it
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme), secv1.AddToScheme, gatewayv1.Install).WithObjects(nonIstioGateway).Build()
			},
			expectedSpcs:  nil,
			expectedError: nil,
		},
	}

	for _, tc := range tcs {
		t.Logf("starting case %s", tc.name)
		ctx := logr.NewContext(context.Background(), logr.Discard())
		c := tc.generateClientState()
		recorder := record.NewFakeRecorder(1)
		g := GatewaySecretProviderClassReconciler{
			client: c,
			config: &config.Config{
				TenantID: "test-tenant-id",
			},
			events: recorder,
		}

		// Define initial metrics
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: tc.gwObj.Namespace, Name: tc.gwObj.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, gatewaySecretProviderControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, gatewaySecretProviderControllerName, metrics.LabelSuccess)

		_, err = g.Reconcile(ctx, req)

		if tc.expectedError == nil {
			require.Nil(t, err)
			require.Equal(t, testutils.GetErrMetricCount(t, gatewaySecretProviderControllerName), beforeErrCount)
			require.Greater(t, testutils.GetReconcileMetricCount(t, gatewaySecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)
		} else {
			t.Logf("expected error: %s", tc.expectedError.Error())
			require.Equal(t, tc.expectedError.Error(), err.Error())
			require.Greater(t, testutils.GetErrMetricCount(t, gatewaySecretProviderControllerName), beforeErrCount)
		}

		if tc.expectedUserErr != "" {
			require.Greater(t, len(recorder.Events), 0)
			require.Equal(t, tc.expectedUserErr, <-recorder.Events)
		} else {
			require.Equal(t, 0, len(recorder.Events))
		}

		if tc.expectedSpcs == nil {
			actualSpcs := &secv1.SecretProviderClassList{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "secrets-store.csi.x-k8s.io/v1",
					Kind:       "SecretProviderClass",
				},
			}
			err = c.List(ctx, actualSpcs)
			require.Equal(t, nil, err)
			require.Equal(t, 0, len(actualSpcs.Items))

		} else {
			reconciledGw := &gatewayv1.Gateway{}
			err = c.Get(ctx, types.NamespacedName{Namespace: tc.gwObj.Namespace, Name: tc.gwObj.Name}, reconciledGw)
			require.Equal(t, nil, err)
			for _, expectedSpc := range tc.expectedSpcs {
				actualSpc := &secv1.SecretProviderClass{}
				err = c.Get(ctx, types.NamespacedName{Namespace: expectedSpc.Namespace, Name: expectedSpc.Name}, actualSpc)
				require.Nil(t, err)
				require.Equal(t, expectedSpc.TypeMeta, actualSpc.TypeMeta)
				require.Equal(t, expectedSpc.ObjectMeta.Name, actualSpc.ObjectMeta.Name)
				require.Equal(t, expectedSpc.ObjectMeta.Namespace, actualSpc.ObjectMeta.Namespace)
				require.Equal(t, expectedSpc.ObjectMeta.Labels, actualSpc.ObjectMeta.Labels)
				require.Equal(t, expectedSpc.ObjectMeta.OwnerReferences, actualSpc.ObjectMeta.OwnerReferences)
				require.Equal(t, expectedSpc.Spec, actualSpc.Spec)

				// find and verify listener
				matchingListenerName := strings.Replace(expectedSpc.Name, "kv-gw-cert-test-gw-", "", 1)
				foundListener := false
				for _, listener := range reconciledGw.Spec.Listeners {
					if string(listener.Name) == matchingListenerName {
						foundListener = true
						require.Equal(t, expectedSpc.Name, string(listener.TLS.CertificateRefs[0].Name))
						require.Equal(t, "Secret", string(*listener.TLS.CertificateRefs[0].Kind))
					}
				}
				require.True(t, foundListener)
			}
		}
	}
}
