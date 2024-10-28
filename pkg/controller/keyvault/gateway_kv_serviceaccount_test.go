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

func Test_extractClientId(t *testing.T) {
	tcs := []struct {
		name             string
		gwObj            *gatewayv1.Gateway
		expectedClientId string
		expectedErr      error
	}{
		{
			name:             "happy path single client id",
			gwObj:            gatewayWithOnlyClientId,
			expectedClientId: "test-client-id",
			expectedErr:      nil,
		},
		{
			name:             "happy path multiple client id",
			gwObj:            gatewayWithMultipleListenersAndOnlyOneClientId,
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
			gwObj:            gatewayWithOnlyServiceAccounts,
			expectedClientId: "",
			expectedErr:      nil,
		},
		{
			name:             "multiple different client IDs",
			gwObj:            gwWithNoCertMultipleCid,
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

func Test_KvServiceAccountReconciler(t *testing.T) {

	tcs := []struct {
		name                string
		gwObj               *gatewayv1.Gateway
		expectedError       error
		expectedUserErr     string
		expectedSa          *corev1.ServiceAccount
		generateClientState func() client.Client
	}{
		{
			name:          "happy path single client id",
			gwObj:         gatewayWithOnlyClientId,
			expectedError: nil,
			expectedSa:    appRoutingSa,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gatewayWithOnlyClientId).Build()
			},
		},
		{
			name:          "happy path multiple client id",
			gwObj:         gatewayWithMultipleListenersAndOnlyOneClientId,
			expectedError: nil,
			expectedSa:    appRoutingSa,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gatewayWithMultipleListenersAndOnlyOneClientId).Build()
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
			gwObj:         gatewayWithOnlyServiceAccounts,
			expectedError: nil,
			expectedSa:    &corev1.ServiceAccount{},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gatewayWithOnlyServiceAccounts).Build()
			},
		},
		{
			name:            "multiple different client IDs",
			gwObj:           gwWithNoCertMultipleCid,
			expectedError:   nil,
			expectedUserErr: "Warning InvalidInput multiple unique clientIds specified. Please select one clientId to associate with the azure-app-routing-kv ServiceAccount",
			expectedSa:      &corev1.ServiceAccount{},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gwWithNoCertMultipleCid).Build()
			},
		},
		{
			name:          "existing app routing SA with same client ID",
			gwObj:         gatewayWithMultipleListenersAndOnlyOneClientId,
			expectedError: nil,
			expectedSa:    appRoutingSa,
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gatewayWithMultipleListenersAndOnlyOneClientId, appRoutingSa).Build()
			},
		},
		{
			name:            "existing app routing SA with different client ID",
			gwObj:           gatewayWithMultipleListenersAndOnlyOneClientId,
			expectedError:   nil,
			expectedUserErr: "Warning InvalidInput gateway specifies clientId test-client-id but azure-app-routing-kv ServiceAccount already uses clientId test-client-id-3",
			expectedSa:      &corev1.ServiceAccount{},
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(gatewayWithMultipleListenersAndOnlyOneClientId, &corev1.ServiceAccount{
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
		fmt.Println("Starting case", tc.name)
		// Define preexisting state
		ctx := logr.NewContext(context.Background(), logr.Discard())
		c := tc.generateClientState()
		recorder := record.NewFakeRecorder(1)
		k := &KvServiceAccountReconciler{
			client: c,
			events: recorder,
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

		if tc.expectedUserErr != "" {
			require.Greater(t, len(recorder.Events), 0)
			require.Equal(t, tc.expectedUserErr, <-recorder.Events)
		} else {
			require.Equal(t, 0, len(recorder.Events))
		}

	}
}
