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

	privateZones := []string{}
	var privateZoneRg string

	publicZones := []string{}
	var publicZoneRg string

	for _, zoneId := range *conf.DNSZoneIDs {
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

	// one config for private, one config for public
	objs := []client.Object{}
	if len(privateZones) > 0 {
		privateZoneConfig := &manifests.ExternalDnsConfig{
			ResourceName:       fmt.Sprintf("%s%s", manifests.ResourceName, manifests.PrivateSuffix),
			TenantId:           conf.TenantID,
			Subscription:       conf.DNSZoneSub,
			ResourceGroup:      privateZoneRg,
			DnsZoneResourceIDs: privateZones,
			Provider:           manifests.PrivateProvider,
		}
		objs = append(objs, manifests.ExternalDnsResources(conf, self, privateZoneConfig)...)
	}

	if len(publicZones) > 0 {
		publicZoneConfig := &manifests.ExternalDnsConfig{
			ResourceName:       fmt.Sprintf("%s%s", manifests.ResourceName, manifests.PublicSuffix),
			TenantId:           conf.TenantID,
			Subscription:       conf.DNSZoneSub,
			ResourceGroup:      publicZoneRg,
			DnsZoneResourceIDs: publicZones,
			Provider:           manifests.PrivateProvider,
		}

		objs = append(objs, manifests.ExternalDnsResources(conf, self, publicZoneConfig)...)
	}

	return newExternalDnsReconciler(manager, objs)
}
