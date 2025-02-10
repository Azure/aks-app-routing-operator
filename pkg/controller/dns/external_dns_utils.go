package dns

import (
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func generateExternalDNSConfigForManifests(conf config.Config, obj client.Object) manifests.ExternalDnsConfig {
	inputConfig := manifests.InputExternalDNSConfig{}
	resourceTypes := manifests.ResourceTypes{}
	switch t := obj.(type) {
	case *v1alpha1.ExternalDNS:
		inputConfig.InputResourceName = t.Spec.ResourceName
		inputConfig.TenantId = t.Spec.TenantID
		inputConfig.DnsZoneresourceIDs = t.Spec.DNSZoneResourceIDs
		for _, rt := range t.Spec.ResourceTypes {
			if rt == "ingress" {
				resourceTypes.Ingress = true
			}
			if rt == "gateway" {
				resourceTypes.Gateway = true
			}
		}
	}

	return manifests.NewExternalDNSConfig(conf, inputConfig)
}
