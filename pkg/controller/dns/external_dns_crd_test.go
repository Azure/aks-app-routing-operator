package dns

import (
	"context"
	"errors"
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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// tcs - go through various CRD inputs, each error method, evaluate err vs expected resources

func TestExternalDNSCRDController_Reconcile(t *testing.T) {
	tcs := []dnsTestCase{
		{
			name:                "happypath public zones",
			existingResources:   []client.Object{testServiceAccount},
			crd:                 func() ExternalDNSCRDConfiguration { return happyPathPublic },
			expectedDeployment:  func() *appsv1.Deployment { return happyPathPublicDeployment },
			expectedConfigmap:   func() *corev1.ConfigMap { return happyPathPublicConfigmap },
			expectedRole:        func() *rbacv1.Role { return happyPathPublicRole },
			expectedRoleBinding: func() *rbacv1.RoleBinding { return happyPathPublicRoleBinding },
		},
		{
			name:               "happypath private zones",
			existingResources:  []client.Object{testServiceAccount},
			crd:                func() ExternalDNSCRDConfiguration { return happyPathPrivate },
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
			crd:               func() ExternalDNSCRDConfiguration { return happyPathPublicFilters },
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
			crd: func() ExternalDNSCRDConfiguration {
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
			crd: func() ExternalDNSCRDConfiguration {
				ret := happyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				return ret
			},
			existingResources: []client.Object{testBadServiceAccount},
			expectedUserError: "serviceAccount test-service-account was specified but does not include necessary annotation for workload identity",
		},
		{
			name: "invalid dns zone resource id",
			crd: func() ExternalDNSCRDConfiguration {
				ret := happyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				ret.Spec.DNSZoneResourceIDs = []string{"invalid-dns-zone-id"}
				return ret
			},
			existingResources: []client.Object{testServiceAccount},
			expectedUserError: "failed to generate ExternalDNS resources: invalid dns zone resource id: invalid-dns-zone-id",
		},
		{
			name: "multierror failure to create deployment and configmap",
			crd: func() ExternalDNSCRDConfiguration {
				ret := happyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				return ret
			},
			transformClient: func(builder *fake.ClientBuilder) *fake.ClientBuilder {
				return builder.WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(_ context.Context, _ client.WithWatch, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
						switch obj.(type) {
						case *appsv1.Deployment:
							return errors.New("failed to create deployment")
						case *corev1.ConfigMap:
							return errors.New("failed to create configmap")
						}
						return nil
					},
				})
			},
			existingResources: []client.Object{testServiceAccount},
			expectedError:     errors.New("\n\t* failed to create configmap\n\t* failed to create deployment\n\n"),
		},
		{
			name: "failure to get externaldns crd",
			crd: func() ExternalDNSCRDConfiguration {
				ret := happyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				return ret
			},
			transformClient: func(builder *fake.ClientBuilder) *fake.ClientBuilder {
				return builder.WithInterceptorFuncs(interceptor.Funcs{
					Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
						switch obj.(type) {
						case *v1alpha1.ExternalDNS:
							return errors.New("failed to get externaldns crd")
						}
						return nil
					},
				})
			},
			existingResources: []client.Object{testServiceAccount},
			expectedError:     errors.New("failed to get externaldns crd"),
		},
		{
			name: "externaldns crd not found",
			crd: func() ExternalDNSCRDConfiguration {
				ret := happyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				return ret
			},
			transformClient: func(builder *fake.ClientBuilder) *fake.ClientBuilder {
				return builder.WithInterceptorFuncs(interceptor.Funcs{
					Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
						switch obj.(type) {
						case *v1alpha1.ExternalDNS:
							return k8serrors.NewNotFound(schema.GroupResource{Group: "approuting.kubernetes.azure.com", Resource: "externaldnses"}, "happy-path-public")
						}
						return nil
					},
				})
			},
			existingResources: []client.Object{testServiceAccount},
			expectedError:     nil,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("starting test %s", tc.name)
			ctx := logr.NewContext(context.Background(), logr.Discard())

			k8sClientBuilder := generateDefaultClientBuilder(t, tc.existingResources)
			if tc.transformClient != nil {
				k8sClientBuilder = tc.transformClient(k8sClientBuilder)
			}
			k8sclient := k8sClientBuilder.Build()

			crdObj := tc.crd()
			var castedObj *v1alpha1.ExternalDNS
			switch temp := crdObj.(type) {
			case *v1alpha1.ExternalDNS:
				castedObj = temp
				castedObj.ObjectMeta.ResourceVersion = ""
				err = k8sclient.Create(ctx, castedObj)
				require.NoError(t, err)
			default:
				t.Fatalf("unexpected type %T", castedObj)
			}

			recorder := record.NewFakeRecorder(1)
			r := &ExternalDNSCRDController{
				client: k8sclient,
				events: recorder,
				config: &config.Config{
					Registry:        testRegistry,
					ClusterUid:      "test-cluster-uid",
					DnsSyncInterval: 3 * time.Minute,
					TenantID:        "12345678-1234-1234-1234-012987654321",
				},
			}

			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: castedObj.GetNamespace(), Name: castedObj.GetName()}}
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
				return
			}

			require.Nil(t, err)
			require.NoError(t, err)
			require.Equal(t, afterErrCount, beforeErrCount)
			require.Greater(t, afterRequestCount, beforeRequestCount)

			checkUserErrors(tc, recorder, t)
			checkTestResources(tc, k8sclient, t)
		})
	}
}

func checkUserErrors(tc dnsTestCase, recorder *record.FakeRecorder, t *testing.T) {
	// check user errors
	if tc.expectedUserError != "" {
		require.Greater(t, len(recorder.Events), 0)
		require.Contains(t, <-recorder.Events, tc.expectedUserError)
		return
	}

	if len(recorder.Events) > 0 {
		t.Errorf("expected no events, got %s", <-recorder.Events)
	}

}

func checkTestResources(tc dnsTestCase, k8sclient client.Client, t *testing.T) {
	// check deployment
	if tc.expectedDeployment != nil {
		actualDeployment := &appsv1.Deployment{}
		err = k8sclient.Get(context.Background(), types.NamespacedName{Name: tc.expectedDeployment().Name, Namespace: tc.expectedDeployment().Namespace}, actualDeployment)
		require.NoError(t, err)
		require.Equal(t, tc.expectedDeployment().ObjectMeta, actualDeployment.ObjectMeta)
		require.Equal(t, tc.expectedDeployment().Spec.Selector, actualDeployment.Spec.Selector)
		require.Equal(t, tc.expectedDeployment().Spec.Template.ObjectMeta, actualDeployment.Spec.Template.ObjectMeta)
		require.Equal(t, tc.expectedDeployment().Spec.Template.Spec.ServiceAccountName, actualDeployment.Spec.Template.Spec.ServiceAccountName)
		require.Equal(t, tc.expectedDeployment().Spec.Template.Spec.Containers[0].Image, actualDeployment.Spec.Template.Spec.Containers[0].Image)
		require.Equal(t, tc.expectedDeployment().Spec.Template.Spec.Containers[0].Args, actualDeployment.Spec.Template.Spec.Containers[0].Args)
		require.Equal(t, tc.expectedDeployment().Spec.Template.Spec.Containers[0].VolumeMounts, actualDeployment.Spec.Template.Spec.Containers[0].VolumeMounts)
		require.Equal(t, tc.expectedDeployment().Spec.Template.Spec.Volumes, actualDeployment.Spec.Template.Spec.Volumes)
	}

	// check configmap
	if tc.expectedConfigmap != nil {
		actualConfigmap := &corev1.ConfigMap{}
		err = k8sclient.Get(context.Background(), types.NamespacedName{Name: tc.expectedConfigmap().Name, Namespace: tc.expectedConfigmap().Namespace}, actualConfigmap)
		require.NoError(t, err)
		require.Equal(t, tc.expectedConfigmap().ObjectMeta, actualConfigmap.ObjectMeta)
		require.Equal(t, tc.expectedConfigmap().Data, actualConfigmap.Data)
	}

	// check role
	if tc.expectedRole != nil {
		actualRole := &rbacv1.Role{}
		err = k8sclient.Get(context.Background(), types.NamespacedName{Name: tc.expectedRole().Name, Namespace: tc.expectedRole().Namespace}, actualRole)
		require.NoError(t, err)
		require.Equal(t, tc.expectedRole().ObjectMeta, actualRole.ObjectMeta)
		require.Equal(t, tc.expectedRole().Rules, actualRole.Rules)
	}

	// check rolebinding
	if tc.expectedRoleBinding != nil {
		actualRoleBinding := &rbacv1.RoleBinding{}
		err = k8sclient.Get(context.Background(), types.NamespacedName{Name: tc.expectedRoleBinding().Name, Namespace: tc.expectedRoleBinding().Namespace}, actualRoleBinding)
		require.NoError(t, err)
		require.Equal(t, tc.expectedRoleBinding().ObjectMeta, actualRoleBinding.ObjectMeta)
		require.Equal(t, tc.expectedRoleBinding().RoleRef, actualRoleBinding.RoleRef)
		require.Equal(t, tc.expectedRoleBinding().Subjects, actualRoleBinding.Subjects)
	}

	// check clusterrole
	if tc.expectedClusterRole != nil {
		actualClusterRole := &rbacv1.ClusterRole{}
		err = k8sclient.Get(context.Background(), types.NamespacedName{Name: tc.expectedClusterRole().Name}, actualClusterRole)
		require.NoError(t, err)
		require.Equal(t, tc.expectedClusterRole().ObjectMeta, actualClusterRole.ObjectMeta)
		require.Equal(t, tc.expectedClusterRole().Rules, actualClusterRole.Rules)
	}

	// check clusterrolebinding
	if tc.expectedClusterRoleBinding != nil {
		actualClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
		err = k8sclient.Get(context.Background(), types.NamespacedName{Name: tc.expectedClusterRoleBinding().Name}, actualClusterRoleBinding)
		require.NoError(t, err)
		require.Equal(t, tc.expectedClusterRoleBinding().ObjectMeta, actualClusterRoleBinding.ObjectMeta)
		require.Equal(t, tc.expectedClusterRoleBinding().RoleRef, actualClusterRoleBinding.RoleRef)
		require.Equal(t, tc.expectedClusterRoleBinding().Subjects, actualClusterRoleBinding.Subjects)
	}
}
