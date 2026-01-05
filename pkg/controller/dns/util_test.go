package dns

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var mockConfigWithTenantId = mockDnsConfig{
	tenantId:            to.Ptr("12345678-1234-1234-1234-123456789012"),
	inputServiceAccount: "mock-service-account",
	resourceNamespace:   "mock-namespace",
	inputResourceName:   "mock-resource-name",
	resourceTypes:       []string{"ingress", "gateway"},
	dnsZoneresourceIDs:  []string{"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test.com", "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test2.com"},
	filters: &v1alpha1.ExternalDNSFilters{
		GatewayLabelSelector:         to.Ptr("test=test"),
		RouteAndIngressLabelSelector: to.Ptr("test=othertest"),
	},
	namespaced: true,
}

var mockConfigWithoutTenantId = mockDnsConfig{
	inputServiceAccount: "mock-service-account",
	resourceNamespace:   "mock-namespace",
	inputResourceName:   "mock-resource-name",
	resourceTypes:       []string{"ingress", "gateway"},
	dnsZoneresourceIDs:  []string{"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test.com", "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test2.com"},
	filters: &v1alpha1.ExternalDNSFilters{
		GatewayLabelSelector:         to.Ptr("test=test"),
		RouteAndIngressLabelSelector: to.Ptr("test=othertest"),
	},
	namespaced: true,
}

var conf = &config.Config{
	Registry:        testRegistry,
	ClusterUid:      "test-cluster-uid",
	DnsSyncInterval: 3 * time.Minute,
	TenantID:        "12345678-1234-1234-1234-012987654321",
}

func Test_buildInputDNSConfig(t *testing.T) {
	inputConfig := buildInputDNSConfig(mockConfigWithTenantId, conf)

	require.Equal(t, inputConfig.TenantId, "12345678-1234-1234-1234-123456789012")
	require.Equal(t, inputConfig.InputServiceAccount, mockConfigWithTenantId.inputServiceAccount)
	require.Equal(t, inputConfig.Namespace, mockConfigWithTenantId.resourceNamespace)
	require.Equal(t, inputConfig.InputResourceName, mockConfigWithTenantId.inputResourceName)
	require.Equal(t, inputConfig.ResourceTypes, map[manifests.ResourceType]struct{}{
		manifests.ResourceTypeIngress: {},
		manifests.ResourceTypeGateway: {},
	})
	require.Equal(t, inputConfig.DnsZoneresourceIDs, mockConfigWithTenantId.dnsZoneresourceIDs)
	require.Equal(t, inputConfig.Filters, mockConfigWithTenantId.filters)
	require.Equal(t, inputConfig.IsNamespaced, mockConfigWithTenantId.namespaced)
	require.Equal(t, inputConfig.UID, "resourceuid")

	// Test with nil tenant ID
	inputConfig = buildInputDNSConfig(mockConfigWithoutTenantId, conf)
	require.Equal(t, inputConfig.TenantId, conf.TenantID)
	require.Equal(t, inputConfig.InputServiceAccount, mockConfigWithTenantId.inputServiceAccount)
	require.Equal(t, inputConfig.Namespace, mockConfigWithTenantId.resourceNamespace)
	require.Equal(t, inputConfig.InputResourceName, mockConfigWithTenantId.inputResourceName)
	require.Equal(t, inputConfig.ResourceTypes, map[manifests.ResourceType]struct{}{
		manifests.ResourceTypeIngress: {},
		manifests.ResourceTypeGateway: {},
	})
	require.Equal(t, inputConfig.DnsZoneresourceIDs, mockConfigWithTenantId.dnsZoneresourceIDs)
	require.Equal(t, inputConfig.Filters, mockConfigWithTenantId.filters)
	require.Equal(t, inputConfig.IsNamespaced, mockConfigWithTenantId.namespaced)
	require.Equal(t, inputConfig.UID, "resourceuid")
}

func Test_extractResourceTypes(t *testing.T) {
	for _, tc := range []struct {
		rt       []string
		expected map[manifests.ResourceType]struct{}
	}{
		{
			rt: []string{"ingress", "gateway"},
			expected: map[manifests.ResourceType]struct{}{
				manifests.ResourceTypeIngress: {},
				manifests.ResourceTypeGateway: {},
			},
		},
		{
			rt: []string{"unknown", "gateway"},
			expected: map[manifests.ResourceType]struct{}{
				manifests.ResourceTypeGateway: {},
			},
		},
		{
			rt: []string{"ingress"},
			expected: map[manifests.ResourceType]struct{}{
				manifests.ResourceTypeIngress: {},
			},
		},
	} {
		result := extractResourceTypes(tc.rt)
		require.Equal(t, tc.expected, result)
	}
}

func Test_generateManifestsConf(t *testing.T) {
	// with tenant ID
	manifestsConf, err := generateManifestsConf(conf, mockConfigWithTenantId)
	require.NoError(t, err)
	require.NotNil(t, manifestsConf)
	require.Equal(t, map[string]string{
		"app.kubernetes.io/name": "mock-resource-name-external-dns",
	}, manifestsConf.Labels())
	require.Equal(t, manifestsConf.DnsZoneResourceIds(), []string{
		"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test.com",
		"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test2.com",
	})

	for _, res := range manifestsConf.Resources() {
		switch casted := res.(type) {
		case *appsv1.Deployment:
			require.Equal(t, mockConfigWithTenantId.inputServiceAccount, casted.Spec.Template.Spec.ServiceAccountName)
			require.Equal(t, []string{
				"--provider=azure",
				"--interval=3m0s",
				"--txt-owner-id=test-cluster-uid-resourceuid",
				"--txt-wildcard-replacement=approutingwildcard",
				"--gateway-label-filter=test==test",
				"--label-filter=test==othertest",
				"--source=gateway-grpcroute",
				"--source=gateway-httproute",
				"--source=ingress",
				"--domain-filter=test.com",
				"--domain-filter=test2.com",
				"--namespace=mock-namespace",
				"--gateway-namespace=mock-namespace",
			},
				casted.Spec.Template.Spec.Containers[0].Args)
		case *corev1.ConfigMap:
			require.Equal(t, casted.Data["azure.json"], `{"cloud":"","location":"","resourceGroup":"test-rg","subscriptionId":"12345678-1234-1234-1234-123456789012","tenantId":"12345678-1234-1234-1234-123456789012","useWorkloadIdentityExtension":true}`)
		}
	}

	// without tenantID
	manifestsConf, err = generateManifestsConf(conf, mockConfigWithoutTenantId)
	require.NoError(t, err)
	require.NotNil(t, manifestsConf)
	require.Equal(t, map[string]string{
		"app.kubernetes.io/name": "mock-resource-name-external-dns",
	}, manifestsConf.Labels())
	require.Equal(t, manifestsConf.DnsZoneResourceIds(), []string{
		"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test.com",
		"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test2.com",
	})

	for _, res := range manifestsConf.Resources() {
		switch casted := res.(type) {
		case *appsv1.Deployment:
			require.Equal(t, casted.Spec.Template.Spec.ServiceAccountName, mockConfigWithoutTenantId.inputServiceAccount)
			require.Equal(t, []string{
				"--provider=azure",
				"--interval=3m0s",
				"--txt-owner-id=test-cluster-uid-resourceuid",
				"--txt-wildcard-replacement=approutingwildcard",
				"--gateway-label-filter=test==test",
				"--label-filter=test==othertest",
				"--source=gateway-grpcroute",
				"--source=gateway-httproute",
				"--source=ingress",
				"--domain-filter=test.com",
				"--domain-filter=test2.com",
				"--namespace=mock-namespace",
				"--gateway-namespace=mock-namespace",
			}, casted.Spec.Template.Spec.Containers[0].Args)
		case *corev1.ConfigMap:
			require.Equal(t, casted.Data["azure.json"], `{"cloud":"","location":"","resourceGroup":"test-rg","subscriptionId":"12345678-1234-1234-1234-123456789012","tenantId":"12345678-1234-1234-1234-012987654321","useWorkloadIdentityExtension":true}`)
		}
	}
}

func Test_deployExternalDNSResources(t *testing.T) {
	k8sClient := generateDefaultClientBuilder(t, nil).Build()
	manifestsConf, err := generateManifestsConf(conf, mockConfigWithTenantId)
	require.NoError(t, err)

	ownerRef := metav1.OwnerReference{
		APIVersion: "v1alpha1",
		Controller: util.ToPtr(true),
		Kind:       "ClusterExternalDNS",
		Name:       "mock-resource-name",
		UID:        "12345678-1234-1234-1234-123456789012",
	}

	err = deployExternalDNSResources(context.Background(), k8sClient, manifestsConf, []metav1.OwnerReference{ownerRef})
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "mock-namespace", Name: "mock-resource-name-external-dns"}, deployment)
	require.NoError(t, err)
	require.Equal(t, deployment.OwnerReferences[0].Name, ownerRef.Name)
}
