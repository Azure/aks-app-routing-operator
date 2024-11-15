package keyvault

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/go-logr/logr"
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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
			name: "cert URI without sa",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
			},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("user specified cert URI but no ServiceAccount in a listener"),
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
			expectedError:    errors.New("serviceaccounts \"test-sa\" not found"),
		},
		{
			name: "cert URI with sa with no annotation",
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
			expectedError:    errors.New("user-specified service account does not contain WI annotation"),
		},
		{
			name: "cert URI with sa with correct annotation",
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
			name:    "no cert URI without sa",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("none of the required TLS options were specified"),
		},
		{
			name: "no cert URI with sa",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"kubernetes.azure.com/tls-cert-service-account": "test-sa",
			},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("user specified ServiceAccount but no cert URI in a listener"),
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
		expectedGateways    []*gatewayv1.Gateway
		expectedError       error
		expectedUserErr     string
	}{
		{
			name:         "cert URI without sa",
			gwObj:        gwWithCertWithoutOthers,
			expectedSpcs: nil,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gwWithCertWithoutOthers).Build()
			},
			expectedError:   nil,
			expectedUserErr: "Warning InvalidInput invalid TLS configuration: detected Keyvault Cert URI, but no ServiceAccount was provided",
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
			name:  "cert URI with sa with no annotation",
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
			expectedUserErr: "Warning InvalidInput invalid TLS configuration: serviceAccount test-sa was specified in Gateway but does not include necessary annotation for workload identity",
		},
		{
			name:  "cert URI with sa with correct annotation",
			gwObj: gwWithSa,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(
					gwWithSa,
					annotatedServiceAccount,
				).Build()
			},
			expectedSpcs:  []*secv1.SecretProviderClass{serviceAccountSpc},
			expectedError: nil,
		},
		{
			name:  "cert URI with two different SA listeners",
			gwObj: gatewayWithTwoServiceAccounts,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gatewayWithTwoServiceAccounts, annotatedServiceAccount, annotatedServiceAccountTwo).Build()
			},
			expectedSpcs:  []*secv1.SecretProviderClass{serviceAccountSpc, serviceAccountTwoSpc},
			expectedError: nil,
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
		{
			name:  "no listeners",
			gwObj: noListenersGateway,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme), secv1.AddToScheme, gatewayv1.Install).WithObjects(noListenersGateway).Build()
			},
			expectedSpcs:  nil,
			expectedError: nil,
		},
		{
			name:  "nil options",
			gwObj: nilOptionsGateway,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(nilOptionsGateway).Build()
			},
			expectedSpcs:  nil,
			expectedError: nil,
		},
		{
			name:  "gateway with only one active listener",
			gwObj: gatewayWithOnlyOneActiveListener,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(
					gatewayWithOnlyOneActiveListener,
					annotatedServiceAccount,
				).Build()
			},
			expectedSpcs: []*secv1.SecretProviderClass{serviceAccountSpc},
			expectedGateways: []*gatewayv1.Gateway{modifyGateway(gatewayWithOnlyOneActiveListener,
				func(gwObj *gatewayv1.Gateway) {
					gwObj.Spec.Listeners[0].TLS.CertificateRefs = []gatewayv1.SecretObjectReference{
						{
							Namespace: to.Ptr(gatewayv1.Namespace("test-ns")),
							Group:     to.Ptr(gatewayv1.Group(corev1.GroupName)),
							Kind:      to.Ptr(gatewayv1.Kind("Secret")),
							Name:      gatewayv1.ObjectName("kv-gw-cert-test-gw-test-listener"),
						},
					}
				})},
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
			if len(recorder.Events) > 0 {
				t.Errorf("unexpected user error: %s", <-recorder.Events)
			}
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

func Test_GatewaySecretProviderReconciler_ServiceAccountChangeIntegration(t *testing.T) {
	ctx := logr.NewContext(context.Background(), logr.Discard())
	c := testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gatewayWithTwoServiceAccounts, annotatedServiceAccount, annotatedServiceAccountTwo, serviceAccountSpc).Build()
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
			filepath.Join("..", "..", "..", "config", "crds"),
			filepath.Join("..", "..", "..", "vendor", "github.com", "kubernetes-sigs", "gateway-api", "config", "crds", "standard"),
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

	require.NoError(t, err)
	k8sClient := m.GetClient()
	err = NewGatewaySecretClassProviderReconciler(m, &config.Config{TenantID: "test-tenant-id"}, "spec.listeners.tls.options.kubernetes.azure.com/tls-cert-service-account")
	require.NoError(t, err)

	go func() {
		err = m.Start(ctx)
		require.NoError(t, err)
	}()
	defer ctx.Done()

	// deploy initial resources
	err = k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}})
	require.NoError(t, err)

	//saToCreate := annotatedServiceAccount.DeepCopy()
	//saToCreate.ResourceVersion = ""
	//err = k8sClient.Create(ctx, saToCreate)
	//require.NoError(t, err)
	//
	//saToCreateTwo := annotatedServiceAccountTwo.DeepCopy()
	//saToCreateTwo.ResourceVersion = ""
	//err = k8sClient.Create(ctx, saToCreateTwo)
	//require.NoError(t, err)

	gwToCreate := gatewayWithTwoServiceAccounts.DeepCopy()
	//gwToCreate.ResourceVersion = ""
	err = k8sClient.Update(ctx, gwToCreate)
	require.NoError(t, err)

	// ensure initial resources are deployed correctly
	reconciledSpc := &secv1.SecretProviderClass{}
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: serviceAccountSpc.Namespace, Name: serviceAccountSpc.Name}, reconciledSpc)
	require.Nil(t, err)
	require.Equal(t, serviceAccountSpc.Spec, reconciledSpc.Spec)

	// now update serviceaccount
	modifiedSA := annotatedServiceAccount.DeepCopy()
	modifiedSA.Annotations["azure.workload.identity/client-id"] = "new-client-id"
	err = k8sClient.Update(ctx, modifiedSA)
	require.NoError(t, err)

	// ensure update took place correctly
	reconciledSA := &corev1.ServiceAccount{}
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: modifiedSA.Namespace, Name: modifiedSA.Name}, reconciledSA)
	require.Nil(t, err)
	require.Equal(t, "new-client-id", reconciledSA.Annotations["azure.workload.identity/client-id"])

	// Ensure clientid was updated on spc
	reconciledSpc = &secv1.SecretProviderClass{}
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: serviceAccountSpc.Namespace, Name: serviceAccountSpc.Name}, reconciledSpc)
	require.Nil(t, err)
	require.Equal(t, "new-client-id", reconciledSpc.Spec.Parameters["userAssignedIdentityID"])
}
