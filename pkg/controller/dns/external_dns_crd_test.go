package dns

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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

// tcs - go through various CRD inputs, each error method, evaluate err vs expected resources

var happyPathPublic = &v1alpha1.ExternalDNS{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "happy-path-public",
		Namespace: "test-ns",
	},
	Spec: v1alpha1.ExternalDNSSpec{
		ResourceName:       "happy-path-public",
		TenantID:           "12345678-1234-1234-1234-123456789012",
		DNSZoneResourceIDs: []string{"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test.com", "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test2.com"},
		ResourceTypes:      []string{"ingress", "gateway"},
	},
}

func TestExternalDNSCRDController_Reconcile(t *testing.T) {
	tcs := []struct {
		name                string
		existingResources   []client.Object
		crd                 func() *v1alpha1.ExternalDNS
		expectedClientError error
		expectedUserError   string
		expectedError       error
		expectedDeployment  *appsv1.Deployment
		expectedConfigmap   *corev1.ConfigMap
		expectedRole        *rbacv1.Role
		expectedRoleBinding *rbacv1.RoleBinding
	}{
		{
			name: "happypath public zones",
		},
		{
			name: "happypath private zones",
		},
		{
			name:                "mixed zones",
			expectedClientError: errors.New("all items must be of the same resource type"),
		},
		{
			name:                "no dns zones",
			expectedClientError: errors.New("spec.dnsZoneResourceIDs in body should have at least 1 items"),
		},
		{
			name:                "invalid dns zone resource id",
			expectedClientError: errors.New("??"),
		},
		{
			name:                "zones in different subs",
			expectedClientError: errors.New("all items must have the same subscription ID"),
		},
		{
			name:                "empty serviceaccount",
			expectedClientError: errors.New("serviceAccount in body should be at least 1 chars long"),
		},
		{
			name:              "nonexistent serviceaccount",
			expectedUserError: "serviceaccount fake-service-account does not exist",
		},

		{
			name:              "serviceaccount without identity specified",
			expectedUserError: "serviceaccount fake-service-account does not have an identity specified",
		},
	}

	for _, tc := range tcs {
		ctx := logr.NewContext(context.Background(), logr.Discard())
		k8sclient := testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme, v1alpha1.AddToScheme).WithObjects(
			tc.existingResources...,
		).Build()

		// check client errors
		crdObj := tc.crd()
		err = k8sclient.Create(ctx, crdObj)
		if tc.expectedClientError != nil {
			require.Error(t, err)
			require.True(t, strings.Contains(err.Error(), tc.expectedClientError.Error()))
			continue
		} else {
			require.NoError(t, err)
		}

		recorder := record.NewFakeRecorder(1)
		r := &ExternalDNSCRDController{
			client: k8sclient,
			events: recorder,
		}

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: crdObj.Namespace, Name: crdObj.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, ExternalDNSCRDControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, ExternalDNSCRDControllerName, metrics.LabelSuccess)

		_, err = r.Reconcile(ctx, req)

		afterErrCount := testutils.GetErrMetricCount(t, ExternalDNSCRDControllerName)
		afterRequestCount := testutils.GetReconcileMetricCount(t, ExternalDNSCRDControllerName, metrics.LabelSuccess)

		if tc.expectedError != nil {
			require.Error(t, err)
			require.True(t, strings.Contains(err.Error(), tc.expectedError.Error()))
			require.Greater(t, afterErrCount, beforeErrCount)
			require.Equal(t, afterRequestCount, beforeRequestCount)
			continue
		}

		require.Equal(t, afterErrCount, beforeErrCount)
		require.Greater(t, afterRequestCount, beforeRequestCount)
		require.NoError(t, err)

		// check user errors
		if tc.expectedUserError != "" {
			require.Greater(t, len(recorder.Events), 0)
			require.Equal(t, tc.expectedUserError, <-recorder.Events)
			continue
		}

		if len(recorder.Events) > 0 {
			t.Errorf("expected no events, got %s", <-recorder.Events)
		}

		// check externaldns resources
		// check deployment
		actualDeployment := &appsv1.Deployment{}
		err = k8sclient.Get(ctx, types.NamespacedName{Name: tc.expectedDeployment.Name, Namespace: tc.expectedDeployment.Namespace}, actualDeployment)
		require.NoError(t, err)
		require.Equal(t, tc.expectedDeployment.ObjectMeta, actualDeployment.ObjectMeta)
		require.Equal(t, tc.expectedDeployment.Spec, actualDeployment.Spec)

		// check configmap
		actualConfigmap := &corev1.ConfigMap{}
		err = k8sclient.Get(ctx, types.NamespacedName{Name: tc.expectedConfigmap.Name, Namespace: tc.expectedConfigmap.Namespace}, actualConfigmap)
		require.NoError(t, err)
		require.Equal(t, tc.expectedConfigmap.ObjectMeta, actualConfigmap.ObjectMeta)
		require.Equal(t, tc.expectedConfigmap.Data, actualConfigmap.Data)

		// check role
		actualRole := &rbacv1.Role{}
		err = k8sclient.Get(ctx, types.NamespacedName{Name: tc.expectedRole.Name, Namespace: tc.expectedRole.Namespace}, actualRole)
		require.NoError(t, err)
		require.Equal(t, tc.expectedRole.ObjectMeta, actualRole.ObjectMeta)
		require.Equal(t, tc.expectedRole.Rules, actualRole.Rules)

		// check rolebinding
		actualRoleBinding := &rbacv1.RoleBinding{}
		err = k8sclient.Get(ctx, types.NamespacedName{Name: tc.expectedRoleBinding.Name, Namespace: tc.expectedRoleBinding.Namespace}, actualRoleBinding)
		require.NoError(t, err)
		require.Equal(t, tc.expectedRoleBinding.ObjectMeta, actualRoleBinding.ObjectMeta)
		require.Equal(t, tc.expectedRoleBinding.RoleRef, actualRoleBinding.RoleRef)
	}

}
