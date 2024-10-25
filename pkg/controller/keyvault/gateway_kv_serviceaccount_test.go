package keyvault

import (
	"context"
	"fmt"
	"testing"

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
)

var (
	singleClientIdGateway = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener-1",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-client-id": "test-client-id",
						},
					},
				},
			},
		},
	}
	multiClientIdGateway = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener-1",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-client-id": "test-client-id",
						},
					},
				},
				{
					Name: "test-listener-2",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-client-id": "test-client-id",
						},
					},
				},
			},
		},
	}

	nilOptionsGateway = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener-1",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: nil,
					},
				},
				{
					Name: "test-listener-2",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: nil,
					},
				},
			},
		},
	}

	noListenersGateway = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{},
		},
	}

	saGateway = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener-1",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-service-account": "test-sa-1",
						},
					},
				},
				{
					Name: "test-listener-2",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-service-account": "test-sa-2",
						},
					},
				},
			},
		},
	}

	multiUniqueCidGateway = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener-1",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-client-id": "test-client-id-1",
						},
					},
				},
				{
					Name: "test-listener-2",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-client-id": "test-client-id-2",
						},
					},
				},
			},
		},
	}
)

func Test_extractClientId(t *testing.T) {
	tcs := []struct {
		name             string
		gwObj            *gatewayv1.Gateway
		expectedClientId string
		expectedErr      error
	}{
		{
			name:             "happy path single client id",
			gwObj:            singleClientIdGateway,
			expectedClientId: "test-client-id",
			expectedErr:      nil,
		},
		{
			name:             "happy path multiple client id",
			gwObj:            multiClientIdGateway,
			expectedClientId: "test-client-id",
			expectedErr:      nil,
		},
		{
			name:             "all nil options",
			gwObj:            nilOptionsGateway,
			expectedClientId: "",
			expectedErr:      nil,
		},
		{
			name:             "no listeners",
			gwObj:            noListenersGateway,
			expectedClientId: "",
			expectedErr:      nil,
		},
		{
			name:             "only service accounts/no client IDs",
			gwObj:            saGateway,
			expectedClientId: "",
			expectedErr:      nil,
		},
		{
			name:             "multiple different client IDs",
			gwObj:            multiUniqueCidGateway,
			expectedClientId: "",
			expectedErr:      newUserError(fmt.Errorf("user specified multiple clientIds in one gateway resource"), "multiple unique clientIds specified. Please select one clientId to associate with the azure-app-routing-kv ServiceAccount"),
		},
	}

	for _, tc := range tcs {
		clientId, err := extractClientIdForManagedSa(tc.gwObj)
		if clientId != tc.expectedClientId {
			t.Errorf("expected clientId %s, got %s instead", tc.expectedClientId, clientId)
		}

		if tc.expectedErr != nil {
			require.Equal(t, tc.expectedErr.Error(), err.Error())
		}
		if err != nil && tc.expectedErr == nil {
			t.Errorf("did not expect an error but got %s", err)
		}

	}
}

var appRoutingSa = &corev1.ServiceAccount{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "v1",
		Kind:       "ServiceAccount",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "azure-app-routing-kv",
		Namespace: "test-ns",
		Annotations: map[string]string{
			"azure.workload.identity/client-id": "test-client-id",
		},
	},
}

func Test_KvServiceAccountReconciler(t *testing.T) {

	tcs := []struct {
		name                string
		gwObj               *gatewayv1.Gateway
		expectedError       error
		expectedSa          *corev1.ServiceAccount
		generateClientState func() client.Client
	}{
		{
			name:          "happy path single client id",
			gwObj:         singleClientIdGateway,
			expectedError: nil,
			expectedSa:    appRoutingSa,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(singleClientIdGateway).Build()
			},
		},
		{
			name:          "happy path multiple client id",
			gwObj:         multiClientIdGateway,
			expectedError: nil,
			expectedSa:    appRoutingSa,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(multiClientIdGateway).Build()
			},
		},
		{
			name:          "all nil options",
			gwObj:         nilOptionsGateway,
			expectedError: nil,
			expectedSa:    &corev1.ServiceAccount{},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(nilOptionsGateway).Build()
			},
		},
		{
			name:          "no listeners",
			gwObj:         noListenersGateway,
			expectedError: nil,
			expectedSa:    &corev1.ServiceAccount{},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(noListenersGateway).Build()
			},
		},
		{
			name:          "only service accounts/no client IDs",
			gwObj:         saGateway,
			expectedError: nil,
			expectedSa:    &corev1.ServiceAccount{},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(saGateway).Build()
			},
		},
		{
			name:          "multiple different client IDs",
			gwObj:         multiUniqueCidGateway,
			expectedError: nil,
			expectedSa:    &corev1.ServiceAccount{},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(noListenersGateway).Build()
			},
		},
		{
			name:          "existing app routing SA with same client ID",
			gwObj:         multiClientIdGateway,
			expectedError: nil,
			expectedSa:    appRoutingSa,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(multiClientIdGateway, appRoutingSa).Build()
			},
		},
		{
			name:          "existing app routing SA with different client ID",
			gwObj:         multiClientIdGateway,
			expectedError: nil,
			expectedSa:    &corev1.ServiceAccount{},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(multiClientIdGateway, &corev1.ServiceAccount{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "ServiceAccount",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "azure-app-routing-kv",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"azure.workload.identity/client-id": "test-client-id-3",
						},
					},
				}).Build()
			},
		},
	}

	for _, tc := range tcs {
		// Define preexisting state
		ctx := logr.NewContext(context.Background(), logr.Discard())
		c := tc.generateClientState()
		k := &KvServiceAccountReconciler{
			client: c,
			events: record.NewFakeRecorder(1),
		}

		// Define initial metrics
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: tc.gwObj.Namespace, Name: tc.gwObj.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, kvSaControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, kvSaControllerName, metrics.LabelSuccess)

		_, err := k.Reconcile(ctx, req)

		if tc.expectedError == nil {
			require.Equal(t, nil, err)
			// compare service accounts
			actualSa := &corev1.ServiceAccount{}
			_ = c.Get(ctx, types.NamespacedName{Name: tc.expectedSa.Name, Namespace: tc.expectedSa.Namespace}, actualSa)
			require.Equal(t, tc.expectedSa.ObjectMeta.Name, actualSa.ObjectMeta.Name)
			require.Equal(t, tc.expectedSa.ObjectMeta.Namespace, actualSa.ObjectMeta.Namespace)
			require.Equal(t, tc.expectedSa.ObjectMeta.Annotations, actualSa.ObjectMeta.Annotations)
			require.Equal(t, tc.expectedSa.TypeMeta, actualSa.TypeMeta)

			require.Equal(t, testutils.GetErrMetricCount(t, kvSaControllerName), beforeErrCount)
			require.Greater(t, testutils.GetReconcileMetricCount(t, kvSaControllerName, metrics.LabelSuccess), beforeRequestCount)

		} else {
			require.Equal(t, tc.expectedError.Error(), err.Error())
			require.Greater(t, testutils.GetErrMetricCount(t, kvSaControllerName), beforeErrCount)
		}
	}
}
