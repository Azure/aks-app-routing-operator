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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var mockConfig = mockDnsConfig{
	tenantId:            "12345678-1234-1234-1234-123456789012",
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
}

func Test_buildInputDNSConfig(t *testing.T) {
	config := buildInputDNSConfig(mockConfig)

	require.Equal(t, config.TenantId, mockConfig.tenantId)
	require.Equal(t, config.InputServiceAccount, mockConfig.inputServiceAccount)
	require.Equal(t, config.Namespace, mockConfig.resourceNamespace)
	require.Equal(t, config.InputResourceName, mockConfig.inputResourceName)
	require.Equal(t, config.ResourceTypes, map[manifests.ResourceType]struct{}{
		manifests.ResourceTypeIngress: {},
		manifests.ResourceTypeGateway: {},
	})
	require.Equal(t, config.DnsZoneresourceIDs, mockConfig.dnsZoneresourceIDs)
	require.Equal(t, config.Filters, mockConfig.filters)
	require.Equal(t, config.IsNamespaced, mockConfig.namespaced)
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

	manifestsConf, err := generateManifestsConf(conf, mockConfig)
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
			require.Equal(t, casted.Spec.Template.Spec.ServiceAccountName, mockConfig.inputServiceAccount)
			require.Equal(t, casted.Spec.Template.Spec.Containers[0].Args, []string{
				"--provider=azure",
				"--interval=3m0s",
				"--txt-owner-id=test-cluster-uid",
				"--txt-wildcard-replacement=approutingwildcard",
				"--gateway-label-filter=test==test",
				"--label-filter=test==othertest",
				"--source=gateway-grpcroute",
				"--source=gateway-httproute",
				"--source=ingress",
				"--domain-filter=test.com",
				"--domain-filter=test2.com",
			})
		}
	}
}

func Test_deployExternalDNSResources(t *testing.T) {
	k8sClient := generateDefaultClientBuilder(t, nil).Build()
	manifestsConf, err := generateManifestsConf(conf, mockConfig)
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
