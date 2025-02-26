package dns

import (
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
)

type ExternalDNSCRDConfiguration interface {
	GetTenantId() string
	GetInputServiceAccount() string
	GetResourceNamespace() string
	GetInputResourceName() string
	GetResourceTypes() []string
	GetDnsZoneresourceIDs() []string
	GetFilters() *v1alpha1.ExternalDNSFilters
}

func buildInputDNSConfig(e ExternalDNSCRDConfiguration) manifests.InputExternalDNSConfig {
	return manifests.InputExternalDNSConfig{
		IdentityType:        manifests.IdentityTypeWorkloadIdentity,
		TenantId:            e.GetTenantId(),
		InputServiceAccount: e.GetInputServiceAccount(),
		Namespace:           e.GetResourceNamespace(),
		InputResourceName:   e.GetInputResourceName(),
		ResourceTypes:       extractResourceTypes(e.GetResourceTypes()),
		DnsZoneresourceIDs:  e.GetDnsZoneresourceIDs(),
		Filters:             e.GetFilters(),
	}
}

func extractResourceTypes(resourceTypes []string) map[manifests.ResourceType]struct{} {
	ret := map[manifests.ResourceType]struct{}{}
	for _, rt := range resourceTypes {
		if strings.EqualFold(rt, manifests.ResourceTypeIngress.String()) {
			ret[manifests.ResourceTypeIngress] = struct{}{}
		}
		if strings.EqualFold(rt, manifests.ResourceTypeGateway.String()) {
			ret[manifests.ResourceTypeGateway] = struct{}{}
		}
	}

	return ret
}
