package dns

import (
	"fmt"
	"github.com/Azure/go-autorest/autorest/azure"
	"strings"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	appsv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	reconcileInterval = time.Minute * 3
)

// newExternalDnsReconciler creates a reconciler that manages external dns resources
func newExternalDnsReconciler(manager ctrl.Manager, resources []client.Object) error {
	return common.NewResourceReconciler(manager, "externalDnsReconciler", resources, reconcileInterval)
}

// NewExternalDns starts all resources required for external dns
func NewExternalDns(manager ctrl.Manager, conf *config.Config, self *appsv1.Deployment) error {
	if len(conf.DNSZoneDomains) == 0 {
		return nil
	}

	privateZones, privateZoneRg, publicZones, publicZoneRg := parseZoneIds(conf.DNSZoneDomains)
	var configs []*manifests.ExternalDnsConfig
	// one config for private, one config for public
	var objs []client.Object
	if len(privateZones) > 0 {
		configs = append(configs, generateConfig(conf, privateZones, privateZoneRg, manifests.PrivateProvider))
	}

	if len(publicZones) > 0 {
		configs = append(configs, generateConfig(conf, publicZones, publicZoneRg, manifests.Provider))
	}

	objs = append(objs, manifests.ExternalDnsResources(conf, self, configs)...)

	return newExternalDnsReconciler(manager, objs)
}

func parseZoneIds(zones []string) (privateZones []string, privateZoneRg string, publicZones []string, publicZoneRg string) {
	for _, zoneId := range zones {
		parsedZone, err := azure.ParseResourceID(zoneId)
		// this should be impossible
		if err != nil {
			continue
		}

		if strings.EqualFold(parsedZone.ResourceType, config.PrivateZoneType) {
			// it's a private zone
			privateZones = append(privateZones, zoneId)
			privateZoneRg = parsedZone.ResourceGroup
		} else {
			// it's a public zone
			publicZones = append(publicZones, zoneId)
			publicZoneRg = parsedZone.ResourceGroup
		}
	}

	return privateZones, privateZoneRg, publicZones, publicZoneRg
}

func generateConfig(conf *config.Config, zones []string, resourceGroup, provider string) *manifests.ExternalDnsConfig {
	var resourceName string

	switch provider {
	case manifests.PrivateProvider:
		resourceName = fmt.Sprintf("%s%s", manifests.ExternalDnsResourceName, manifests.PrivateSuffix)
	default:
		resourceName = fmt.Sprintf("%s%s", manifests.ExternalDnsResourceName, manifests.PublicSuffix)
	}

	return &manifests.ExternalDnsConfig{
		ResourceName:       resourceName,
		TenantId:           conf.TenantID,
		Subscription:       conf.DNSZoneSub,
		ResourceGroup:      resourceGroup,
		DnsZoneResourceIDs: zones,
		Provider:           provider,
	}
}
