package dns

import (
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func generateExternalDNSConfigForManifests(conf *config.Config, obj client.Object) (*manifests.ExternalDnsConfig, error) {
	inputConfig := manifests.InputExternalDNSConfig{}
	resourceTypes := manifests.ResourceTypes{}
	switch t := obj.(type) {
	case *v1alpha1.ExternalDNS:
		inputConfig.InputResourceName = t.Spec.ResourceName
		inputConfig.TenantId = t.Spec.TenantID
		inputConfig.DnsZoneresourceIDs = t.Spec.DNSZoneResourceIDs
		inputConfig.InputServiceAccount = t.Spec.Identity.ServiceAccount
		inputConfig.Namespace = t.Namespace
		for _, rt := range t.Spec.ResourceTypes {
			if strings.EqualFold(rt, "ingress") {
				resourceTypes.Ingress = true
			}
			if strings.EqualFold(rt, "gateway") {
				resourceTypes.Gateway = true
			}
		}
	}

	inputConfig.IdentityType = manifests.IdentityTypeWorkloadIdentity

	return manifests.NewExternalDNSConfig(conf, inputConfig)
}
