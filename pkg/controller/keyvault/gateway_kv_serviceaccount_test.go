package keyvault

import (
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func test_extractClientId(t *testing.T) {
	tcs := []struct {
		name             string
		gwObj            *gatewayv1.Gateway
		expectedClientId string
		expectedErr      error
	}{
		{
			name: "happy path single client id",
		},
		{
			name: "happy path multiple client id",
		},
		{
			name: "all nil options",
		},
		{
			name: "no listeners",
		},
		{
			name: "only service accounts/no client IDs",
		},
		{
			name: "multiple different client IDs",
		},
	}

	for _, tc := range tcs {
		clientId, err := extractClientId(tc.gwObj)
		if clientId != tc.expectedClientId {
			t.Errorf("expected clientId %s, got %s instead", tc.expectedClientId, clientId)
		}

		if err == nil && tc.expectedErr != nil {
			t.Errorf("expected error %s but got nil instead", tc.expectedErr)
		}
		if err != nil && tc.expectedErr == nil {
			t.Errorf("did not expect an error but got %s", err)
		}

		if err.Error() != tc.expectedErr.Error() {
			t.Errorf("expected error %s but got %s", tc.expectedErr.Error(), err.Error())
		}
	}
}

func test_Reconcile(t *testing.T) {

	tcs := []struct {
		gwObj                  *gatewayv1.Gateway
		expectedError          error
		expectedServiceAccount *corev1.ServiceAccount
	}{
		{},
	}

	for _, tc := range tcs {
		c := fake.NewClientBuilder().WithObjects(tc.gwObj).Build()
		k := &KvServiceAccountReconciler{
			client: c,
			events: record.NewFakeRecorder(1),
		}

		// Create the secret provider class
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: tc.gwObj.Namespace, Name: tc.gwObj.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, kvSaControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, kvSaControllerName, metrics.LabelSuccess)

		_, err := i.Reconcile(ctx, req)
		require.NoError(t, err)

		require.Equal(t, testutils.GetErrMetricCount(t, ingressSecretProviderControllerName), beforeErrCount)
		require.Greater(t, testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)

		clientId, err := extractClientId(tc.gwObj)
		if clientId != tc.expectedClientId {
			t.Errorf("expected clientId %s, got %s instead", tc.expectedClientId, clientId)
		}

		if err == nil && tc.expectedErr != nil {
			t.Errorf("expected error %s but got nil instead", tc.expectedErr)
		}
		if err != nil && tc.expectedErr == nil {
			t.Errorf("did not expect an error but got %s", err)
		}

		if err.Error() != tc.expectedErr.Error() {
			t.Errorf("expected error %s but got %s", tc.expectedErr.Error(), err.Error())
		}
	}
}
