package dns

import (
	"testing"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/stretchr/testify/require"
)

type mockDnsConfig struct {
	tenantId            string
	inputServiceAccount string
	resourceNamespace   string
	inputResourceName   string
	resourceTypes       []string
	dnsZoneresourceIDs  []string
	filters             *v1alpha1.ExternalDNSFilters
	namespaced          bool
}

var sampleMockDnsConfig = mockDnsConfig{
	tenantId:            "mock-tenant-id",
	inputServiceAccount: "mock-service-account",
	resourceNamespace:   "mock-namespace",
	inputResourceName:   "mock-resource-name",
	resourceTypes:       []string{"mock-resource-type"},
	dnsZoneresourceIDs:  []string{"mock-dns-zone-id"},
	filters:             &v1alpha1.ExternalDNSFilters{},
	namespaced:          true,
}

func (m mockDnsConfig) GetTenantId() string {
	return m.tenantId
}

func (m mockDnsConfig) GetInputServiceAccount() string {
	return m.inputServiceAccount
}

func (m mockDnsConfig) GetResourceNamespace() string {
	return m.resourceNamespace
}

func (m mockDnsConfig) GetInputResourceName() string {
	return m.inputResourceName
}

func (m mockDnsConfig) GetResourceTypes() []string {
	return m.resourceTypes
}

func (m mockDnsConfig) GetDnsZoneresourceIDs() []string {
	return m.dnsZoneresourceIDs
}

func (m mockDnsConfig) GetFilters() *v1alpha1.ExternalDNSFilters {
	return m.filters
}

func (m mockDnsConfig) GetNamespaced() bool {
	return m.namespaced
}

func Test_buildInputDNSConfig(t *testing.T) {
	config := buildInputDNSConfig(sampleMockDnsConfig)

	if config.TenantId != sampleMockDnsConfig.tenantId {
		t.Errorf("Expected tenantId %s, got %s", sampleMockDnsConfig.tenantId, config.TenantId)
	}
	if config.InputServiceAccount != sampleMockDnsConfig.inputServiceAccount {
		t.Errorf("Expected inputServiceAccount %s, got %s", sampleMockDnsConfig.inputServiceAccount, config.InputServiceAccount)
	}
	if config.Namespace != sampleMockDnsConfig.resourceNamespace {
		t.Errorf("Expected resourceNamespace %s, got %s", sampleMockDnsConfig.resourceNamespace, config.Namespace)
	}
	if config.InputResourceName != sampleMockDnsConfig.inputResourceName {
		t.Errorf("Expected inputResourceName %s, got %s", sampleMockDnsConfig.inputResourceName, config.InputResourceName)
	}
	if len(config.ResourceTypes) != 1 {
		t.Errorf("Expected 1 resource type, got %d", len(config.ResourceTypes))
	}
	if config.DnsZoneresourceIDs[0] != sampleMockDnsConfig.dnsZoneresourceIDs[0] {
		t.Errorf("Expected dnsZoneresourceIDs %s, got %s", sampleMockDnsConfig.dnsZoneresourceIDs[0], config.DnsZoneresourceIDs[0])
	}
	if !config.IsNamespaced {
		t.Error("Expected IsNamespaced to be true")
	}
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
