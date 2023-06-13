package dns

import (
	"fmt"
	"strings"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/go-autorest/autorest/azure"
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
	if len(conf.DNSZoneIDs) == 0 {
		return nil
	}

	configs := parseZoneIds(conf)
	// one config for private, one config for public
	var objs []client.Object

	objs = append(objs, manifests.ExternalDnsResources(conf, self, configs)...)

	return newExternalDnsReconciler(manager, objs)
}

func parseZoneIds(conf *config.Config) (configs []*manifests.ExternalDnsConfig) {
	var ret []*manifests.ExternalDnsConfig
	var privateZones, publicZones []string
	var privateSubscription, privateZoneRg, publicSubscription, publicZoneRg string

	for _, zoneId := range conf.DNSZoneIDs {
		parsedZone, err := azure.ParseResourceID(zoneId)
		// this should be impossible
		if err != nil {
			continue
		}

		if strings.EqualFold(parsedZone.ResourceType, config.PrivateZoneType) {
			// it's a private zone
			privateZones = append(privateZones, zoneId)
			privateSubscription = parsedZone.SubscriptionID
			privateZoneRg = parsedZone.ResourceGroup
		} else {
			// it's a public zone
			publicZones = append(publicZones, zoneId)
			publicSubscription = parsedZone.SubscriptionID
			publicZoneRg = parsedZone.ResourceGroup
		}
	}

	if len(privateZones) > 0 {
		ret = append(ret, generateConfig(conf, privateZones, privateSubscription, privateZoneRg, manifests.PrivateProvider))
	}

	if len(publicZones) > 0 {
		ret = append(ret, generateConfig(conf, publicZones, publicSubscription, publicZoneRg, manifests.Provider))
	}

	return ret
}

func generateConfig(conf *config.Config, zones []string, subscription, resourceGroup, provider string) *manifests.ExternalDnsConfig {
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
		Subscription:       subscription,
		ResourceGroup:      resourceGroup,
		DnsZoneResourceIDs: zones,
		Provider:           provider,
	}
}
