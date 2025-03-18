package dns

import (
	"context"
	"encoding/hex"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
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

func TestClusterExternalDNSCRDController_Reconcile(t *testing.T) {
	tcs := []dnsTestCase{
		{
			name:                       "happypath public zones",
			existingResources:          []client.Object{testServiceAccountInResourceNs},
			crd:                        func() ExternalDNSCRDConfiguration { return clusterHappyPathPublic },
			expectedDeployment:         func() *appsv1.Deployment { return clusterHappyPathPublicDeployment },
			expectedConfigmap:          func() *corev1.ConfigMap { return clusterHappyPathPublicConfigmap },
			expectedClusterRole:        func() *rbacv1.ClusterRole { return happyPathClusterRole },
			expectedClusterRoleBinding: func() *rbacv1.ClusterRoleBinding { return happyPathClusterRoleBinding },
		},
		{
			name:               "happypath private zones",
			existingResources:  []client.Object{testServiceAccountInResourceNs},
			crd:                func() ExternalDNSCRDConfiguration { return clusterHappyPathPrivate },
			expectedDeployment: func() *appsv1.Deployment { return clusterHappyPathPrivateDeployment },
			expectedConfigmap: func() *corev1.ConfigMap {
				ret := clusterHappyPathPublicConfigmap.DeepCopy()
				ret.ObjectMeta.Name = "cluster-happy-path-private-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-private-external-dns"
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPrivate)
				return ret
			},
			expectedClusterRole: func() *rbacv1.ClusterRole {
				ret := happyPathClusterRole.DeepCopy()
				ret.ObjectMeta.Name = "cluster-happy-path-private-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-private-external-dns"
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPrivate)
				return ret
			},
			expectedClusterRoleBinding: func() *rbacv1.ClusterRoleBinding {
				ret := happyPathClusterRoleBinding.DeepCopy()
				ret.ObjectMeta.Name = "cluster-happy-path-private-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-private-external-dns"
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPrivate)
				ret.RoleRef.Name = "cluster-happy-path-private-external-dns"
				return ret
			},
		},
		{
			name:              "happypath public zones no tenant ID",
			existingResources: []client.Object{testServiceAccountInResourceNs},
			crd:               func() ExternalDNSCRDConfiguration { return clusterHappyPathPublicNoTenantID },
			expectedDeployment: func() *appsv1.Deployment {
				ret := clusterHappyPathPublicDeployment.DeepCopy()
				ret.ObjectMeta.Name = "cluster-happy-path-public-no-tenant-id-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-public-no-tenant-id-external-dns"
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPublicNoTenantID)
				ret.Spec.Selector.MatchLabels["app"] = "cluster-happy-path-public-no-tenant-id-external-dns"
				ret.Spec.Template.ObjectMeta.Labels["app"] = "cluster-happy-path-public-no-tenant-id-external-dns"
				ret.Spec.Template.ObjectMeta.Labels["checksum/configmap"] = hex.EncodeToString(happyPathPublicNoTenantIDJSONHash[:])[:16]
				ret.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name = "cluster-happy-path-public-no-tenant-id-external-dns"
				return ret
			},
			expectedConfigmap: func() *corev1.ConfigMap {
				ret := clusterHappyPathPublicConfigmap.DeepCopy()
				ret.Data["azure.json"] = happyPathPublicNoTenantIDJSON
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPublicNoTenantID)
				ret.ObjectMeta.Name = "cluster-happy-path-public-no-tenant-id-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-public-no-tenant-id-external-dns"
				return ret
			},
			expectedClusterRole: func() *rbacv1.ClusterRole {
				ret := happyPathClusterRole.DeepCopy()
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPublicNoTenantID)
				ret.ObjectMeta.Name = "cluster-happy-path-public-no-tenant-id-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-public-no-tenant-id-external-dns"
				return ret
			},
			expectedClusterRoleBinding: func() *rbacv1.ClusterRoleBinding {
				ret := happyPathClusterRoleBinding.DeepCopy()
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPublicNoTenantID)
				ret.ObjectMeta.Name = "cluster-happy-path-public-no-tenant-id-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-public-no-tenant-id-external-dns"
				ret.RoleRef.Name = "cluster-happy-path-public-no-tenant-id-external-dns"
				return ret
			},
		},
		{
			name:              "happypath public with filters",
			existingResources: []client.Object{testServiceAccountInResourceNs},
			crd:               func() ExternalDNSCRDConfiguration { return clusterHappyPathPublicFilters },
			expectedDeployment: func() *appsv1.Deployment {
				ret := clusterHappyPathPublicDeployment.DeepCopy()
				ret.ObjectMeta.Name = "cluster-happy-path-public-filters-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-public-filters-external-dns"
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPublicFilters)
				ret.Spec.Selector.MatchLabels["app"] = "cluster-happy-path-public-filters-external-dns"
				ret.Spec.Template.ObjectMeta.Labels["app"] = "cluster-happy-path-public-filters-external-dns"
				ret.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name = "cluster-happy-path-public-filters-external-dns"
				newArgs := []string{
					"--gateway-label-filter=app==testapp",
					"--label-filter=app==testapp",
				}
				ret.Spec.Template.Spec.Containers[0].Args = slices.Insert(ret.Spec.Template.Spec.Containers[0].Args, 4, newArgs...)
				return ret
			},
			expectedConfigmap: func() *corev1.ConfigMap {
				ret := clusterHappyPathPublicConfigmap.DeepCopy()
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPublicFilters)
				ret.ObjectMeta.Name = "cluster-happy-path-public-filters-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-public-filters-external-dns"
				return ret
			},
			expectedClusterRole: func() *rbacv1.ClusterRole {
				ret := happyPathClusterRole.DeepCopy()
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPublicFilters)
				ret.ObjectMeta.Name = "cluster-happy-path-public-filters-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-public-filters-external-dns"
				return ret
			},
			expectedClusterRoleBinding: func() *rbacv1.ClusterRoleBinding {
				ret := happyPathClusterRoleBinding.DeepCopy()
				ret.ObjectMeta.OwnerReferences = ownerReferencesFromClusterCRD(clusterHappyPathPublicFilters)
				ret.ObjectMeta.Name = "cluster-happy-path-public-filters-external-dns"
				ret.ObjectMeta.Labels["app.kubernetes.io/name"] = "cluster-happy-path-public-filters-external-dns"
				ret.RoleRef.Name = "cluster-happy-path-public-filters-external-dns"
				return ret
			},
		},
		{
			name: "nonexistent serviceaccount",
			crd: func() ExternalDNSCRDConfiguration {
				ret := clusterHappyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				ret.Spec.Identity.ServiceAccount = "fake-service-account"
				return ret
			},
			existingResources: []client.Object{testServiceAccountInResourceNs},
			expectedUserError: "serviceAccount fake-service-account does not exist",
		},
		{
			name: "serviceaccount without identity specified",
			crd: func() ExternalDNSCRDConfiguration {
				ret := clusterHappyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				return ret
			},
			existingResources: []client.Object{testBadServiceAccountInResourceNs},
			expectedUserError: "serviceAccount test-service-account was specified but does not include necessary annotation for workload identity",
		},
		{
			name: "invalid dns zone resource id",
			crd: func() ExternalDNSCRDConfiguration {
				ret := clusterHappyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				ret.Spec.DNSZoneResourceIDs = []string{"invalid-dns-zone-id"}
				return ret
			},
			existingResources: []client.Object{testServiceAccountInResourceNs},
			expectedUserError: "failed to generate ExternalDNS resources: invalid dns zone resource id: invalid-dns-zone-id",
		},
		{
			name: "multierror failure to create deployment and configmap",
			crd: func() ExternalDNSCRDConfiguration {
				ret := clusterHappyPathPublic.DeepCopy()
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
			existingResources: []client.Object{testServiceAccountInResourceNs},
			expectedError:     errors.New("\n\t* failed to create configmap\n\t* failed to create deployment\n\n"),
		},
		{
			name: "failure to get clusterexternaldns crd",
			crd: func() ExternalDNSCRDConfiguration {
				ret := clusterHappyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				return ret
			},
			transformClient: func(builder *fake.ClientBuilder) *fake.ClientBuilder {
				return builder.WithInterceptorFuncs(interceptor.Funcs{
					Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
						switch obj.(type) {
						case *v1alpha1.ClusterExternalDNS:
							return errors.New("failed to get clusterexternaldns crd")
						}
						return nil
					},
				})
			},
			existingResources: []client.Object{testServiceAccountInResourceNs},
			expectedError:     errors.New("failed to get clusterexternaldns crd"),
		},
		{
			name: "clusterexternaldns crd not found",
			crd: func() ExternalDNSCRDConfiguration {
				ret := clusterHappyPathPublic.DeepCopy()
				ret.ResourceVersion = ""
				return ret
			},
			transformClient: func(builder *fake.ClientBuilder) *fake.ClientBuilder {
				return builder.WithInterceptorFuncs(interceptor.Funcs{
					Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
						switch obj.(type) {
						case *v1alpha1.ClusterExternalDNS:
							return k8serrors.NewNotFound(schema.GroupResource{Group: "approuting.kubernetes.azure.com", Resource: "externaldnses"}, "happy-path-public")
						}
						return nil
					},
				})
			},
			existingResources: []client.Object{testServiceAccountInResourceNs},
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

			// check client errors
			crdObj := tc.crd()
			err = k8sclient.Create(ctx, crdObj)
			require.NoError(t, err)

			recorder := record.NewFakeRecorder(1)
			c := &ClusterExternalDNSController{
				client: k8sclient,
				events: recorder,
				config: &config.Config{
					Registry:        testRegistry,
					ClusterUid:      "test-cluster-uid",
					DnsSyncInterval: 3 * time.Minute,
					TenantID:        "12345678-1234-1234-1234-012987654321",
				},
			}

			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: crdObj.GetNamespace(), Name: crdObj.GetName()}}
			beforeErrCount := testutils.GetErrMetricCount(t, ClusterExternalDNSControllerName)
			beforeRequestCount := testutils.GetReconcileMetricCount(t, ClusterExternalDNSControllerName, metrics.LabelSuccess)

			_, err = c.Reconcile(ctx, req)

			afterErrCount := testutils.GetErrMetricCount(t, ClusterExternalDNSControllerName)
			afterRequestCount := testutils.GetReconcileMetricCount(t, ClusterExternalDNSControllerName, metrics.LabelSuccess)

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
