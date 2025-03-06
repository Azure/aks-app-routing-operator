package dns

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

func TestExternalDNSCRDController_Reconcile(t *testing.T) {
	tcs := []struct {
		name                string
		existingResources   []client.Object
		crd                 func() *v1alpha1.ExternalDNS
		expectedUserError   string
		expectedError       error
		expectedDeployment  func() *appsv1.Deployment
		expectedConfigmap   func() *corev1.ConfigMap
		expectedRole        func() *rbacv1.Role
		expectedRoleBinding func() *rbacv1.RoleBinding
	}{
		{
			name:                "happypath public zones",
			existingResources:   []client.Object{testServiceAccount},
			crd:                 func() *v1alpha1.ExternalDNS { return happyPathPublic },
			expectedDeployment:  func() *appsv1.Deployment { return happyPathPublicDeployment },
			expectedConfigmap:   func() *corev1.ConfigMap { return happyPathPublicConfigmap },
			expectedRole:        func() *rbacv1.Role { return happyPathPublicRole },
			expectedRoleBinding: func() *rbacv1.RoleBinding { return happyPathPublicRoleBinding },
		},
		{
			name:               "happypath private zones",
			existingResources:  []client.Object{testServiceAccount},
			crd:                func() *v1alpha1.ExternalDNS { return happyPathPrivate },
			expectedDeployment: func() *appsv1.Deployment { return happyPathPrivateDeployment },
			expectedConfigmap: func() *corev1.ConfigMap {
				ret := happyPathPublicConfigmap.DeepCopy()
				ret.ObjectMeta.Name = "happy-path-private-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "happy-path-private-external-dns"
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromCRD(happyPathPrivate)
				return ret
			},
			expectedRole: func() *rbacv1.Role {
				ret := happyPathPublicRole.DeepCopy()
				ret.ObjectMeta.Name = "happy-path-private-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "happy-path-private-external-dns"
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromCRD(happyPathPrivate)
				return ret
			},
			expectedRoleBinding: func() *rbacv1.RoleBinding {
				ret := happyPathPublicRoleBinding.DeepCopy()
				ret.ObjectMeta.Name = "happy-path-private-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "happy-path-private-external-dns"
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromCRD(happyPathPrivate)
				ret.RoleRef.Name = "happy-path-private-external-dns"
				return ret
			},
		},
		{
			name:              "happypath public with filters",
			existingResources: []client.Object{testServiceAccount},
			crd:               func() *v1alpha1.ExternalDNS { return happyPathPublicFilters },
			expectedDeployment: func() *appsv1.Deployment {
				ret := happyPathPublicDeployment.DeepCopy()
				ret.ObjectMeta.Name = "happy-path-public-filters-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "happy-path-public-filters-external-dns"
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromCRD(happyPathPublicFilters)
				ret.Spec.Selector.MatchLabels["app"] = "happy-path-public-filters-external-dns"
				ret.Spec.Template.ObjectMeta.Labels["app"] = "happy-path-public-filters-external-dns"
				ret.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name = "happy-path-public-filters-external-dns"
				newArgs := []string{
					"--gateway-label-filter=app==testapp",
					"--label-filter=app==testapp",
				}
				ret.Spec.Template.Spec.Containers[0].Args = slices.Insert(ret.Spec.Template.Spec.Containers[0].Args, 4, newArgs...)
				return ret
			},
			expectedConfigmap: func() *corev1.ConfigMap {
				ret := happyPathPublicConfigmap.DeepCopy()
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromCRD(happyPathPublicFilters)
				ret.ObjectMeta.Name = "happy-path-public-filters-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "happy-path-public-filters-external-dns"
				return ret
			},
			expectedRole: func() *rbacv1.Role {
				ret := happyPathPublicRole.DeepCopy()
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromCRD(happyPathPublicFilters)
				ret.ObjectMeta.Name = "happy-path-public-filters-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "happy-path-public-filters-external-dns"
				return ret
			},
			expectedRoleBinding: func() *rbacv1.RoleBinding {
				ret := happyPathPublicRoleBinding.DeepCopy()
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromCRD(happyPathPublicFilters)
				ret.ObjectMeta.Name = "happy-path-public-filters-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "happy-path-public-filters-external-dns"
				ret.RoleRef.Name = "happy-path-public-filters-external-dns"
				return ret
			},
		},
		{
			name: "nonexistent serviceaccount",
			crd: func() *v1alpha1.ExternalDNS {
				ret := happyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				ret.Spec.Identity.ServiceAccount = "fake-service-account"
				return ret
			},
			existingResources: []client.Object{testServiceAccount},
			expectedUserError: "serviceAccount fake-service-account does not exist",
		},
		{
			name: "serviceaccount without identity specified",
			crd: func() *v1alpha1.ExternalDNS {
				ret := happyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				return ret
			},
			existingResources: []client.Object{testBadServiceAccount},
			expectedUserError: "serviceAccount test-service-account was specified but does not include necessary annotation for workload identity",
		},
	}

	for _, tc := range tcs {
		t.Logf("starting test %s", tc.name)
		ctx := logr.NewContext(context.Background(), logr.Discard())
		k8sclient := testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme, v1alpha1.AddToScheme).WithObjects(
			tc.existingResources...,
		).Build()

		// check client errors
		crdObj := tc.crd()
		err = k8sclient.Create(ctx, crdObj)
		require.NoError(t, err)

		recorder := record.NewFakeRecorder(1)
		r := &ExternalDNSCRDController{
			client: k8sclient,
			events: recorder,
			config: &config.Config{
				Registry:        testRegistry,
				ClusterUid:      "test-cluster-uid",
				DnsSyncInterval: 3 * time.Minute,
			},
		}

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: crdObj.Namespace, Name: crdObj.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, ExternalDNSCRDControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, ExternalDNSCRDControllerName, metrics.LabelSuccess)

		_, err = r.Reconcile(ctx, req)

		afterErrCount := testutils.GetErrMetricCount(t, ExternalDNSCRDControllerName)
		afterRequestCount := testutils.GetReconcileMetricCount(t, ExternalDNSCRDControllerName, metrics.LabelSuccess)

		if tc.expectedError != nil {
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.expectedError.Error())
			require.Greater(t, afterErrCount, beforeErrCount)
			require.Equal(t, afterRequestCount, beforeRequestCount)
			continue
		}

		require.Nil(t, err)
		require.NoError(t, err)
		require.Equal(t, afterErrCount, beforeErrCount)
		require.Greater(t, afterRequestCount, beforeRequestCount)

		// check user errors
		if tc.expectedUserError != "" {
			require.Greater(t, len(recorder.Events), 0)
			require.Contains(t, <-recorder.Events, tc.expectedUserError)
			continue
		}

		if len(recorder.Events) > 0 {
			t.Errorf("expected no events, got %s", <-recorder.Events)
		}

		// check externaldns resources
		// check deployment
		actualDeployment := &appsv1.Deployment{}
		err = k8sclient.Get(ctx, types.NamespacedName{Name: tc.expectedDeployment().Name, Namespace: tc.expectedDeployment().Namespace}, actualDeployment)
		if err != nil {
			t.Logf("error getting deployment: %s", err.Error())
			if k8serrors.IsNotFound(err) {
				deploymentList := &appsv1.DeploymentList{}
				require.NoError(t, k8sclient.List(ctx, deploymentList))
				t.Log("deployment not found, instead found roles:")
				for _, deployment := range deploymentList.Items {
					t.Logf("name, namespace: %s, %s", deployment.Name, deployment.Namespace)
				}
			}
		}
		require.NoError(t, err)
		require.Equal(t, tc.expectedDeployment().ObjectMeta, actualDeployment.ObjectMeta)
		require.Equal(t, tc.expectedDeployment().Spec.Selector, actualDeployment.Spec.Selector)
		require.Equal(t, tc.expectedDeployment().Spec.Template.ObjectMeta, actualDeployment.Spec.Template.ObjectMeta)
		require.Equal(t, tc.expectedDeployment().Spec.Template.Spec.ServiceAccountName, actualDeployment.Spec.Template.Spec.ServiceAccountName)
		require.Equal(t, tc.expectedDeployment().Spec.Template.Spec.Containers[0].Image, actualDeployment.Spec.Template.Spec.Containers[0].Image)
		require.Equal(t, tc.expectedDeployment().Spec.Template.Spec.Containers[0].Args, actualDeployment.Spec.Template.Spec.Containers[0].Args)
		require.Equal(t, tc.expectedDeployment().Spec.Template.Spec.Containers[0].VolumeMounts, actualDeployment.Spec.Template.Spec.Containers[0].VolumeMounts)
		require.Equal(t, tc.expectedDeployment().Spec.Template.Spec.Volumes, actualDeployment.Spec.Template.Spec.Volumes)

		// check configmap
		actualConfigmap := &corev1.ConfigMap{}
		err = k8sclient.Get(ctx, types.NamespacedName{Name: tc.expectedConfigmap().Name, Namespace: tc.expectedConfigmap().Namespace}, actualConfigmap)
		require.NoError(t, err)
		require.Equal(t, tc.expectedConfigmap().ObjectMeta, actualConfigmap.ObjectMeta)
		require.Equal(t, tc.expectedConfigmap().Data, actualConfigmap.Data)

		// check role
		actualRole := &rbacv1.Role{}
		err = k8sclient.Get(ctx, types.NamespacedName{Name: tc.expectedRole().Name, Namespace: tc.expectedRole().Namespace}, actualRole)
		require.NoError(t, err)
		require.Equal(t, tc.expectedRole().ObjectMeta, actualRole.ObjectMeta)
		require.Equal(t, tc.expectedRole().Rules, actualRole.Rules)

		// check rolebinding
		actualRoleBinding := &rbacv1.RoleBinding{}
		err = k8sclient.Get(ctx, types.NamespacedName{Name: tc.expectedRoleBinding().Name, Namespace: tc.expectedRoleBinding().Namespace}, actualRoleBinding)
		require.NoError(t, err)
		require.Equal(t, tc.expectedRoleBinding().ObjectMeta, actualRoleBinding.ObjectMeta)
		require.Equal(t, tc.expectedRoleBinding().RoleRef, actualRoleBinding.RoleRef)
		require.Equal(t, tc.expectedRoleBinding().Subjects, actualRoleBinding.Subjects)
	}

}
