package dns

import (
	"fmt"
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
	if conf.PublicZoneConfig != nil && len(conf.PublicZoneConfig.ZoneIds) == 0 && conf.PrivateZoneConfig != nil && len(conf.PrivateZoneConfig.ZoneIds) == 0 {
		return nil
	}

	configs := generateZoneConfigs(conf)
	// one config for private, one config for public
	var objs []client.Object

	objs = append(objs, manifests.ExternalDnsResources(conf, self, configs)...)

	return newExternalDnsReconciler(manager, objs)
}

func generateZoneConfigs(conf *config.Config) (configs []*manifests.ExternalDnsConfig) {
	var ret []*manifests.ExternalDnsConfig
	if conf.PrivateZoneConfig != nil && len(conf.PrivateZoneConfig.ZoneIds) > 0 {
		ret = append(ret, generateConfig(conf, conf.PrivateZoneConfig, manifests.PrivateProvider))
	}

	if conf.PublicZoneConfig != nil && len(conf.PublicZoneConfig.ZoneIds) > 0 {
		ret = append(ret, generateConfig(conf, conf.PublicZoneConfig, manifests.PublicProvider))
	}

	return ret
}

func generateConfig(conf *config.Config, dnsZoneConfig *config.DnsZoneConfig, provider manifests.Provider) *manifests.ExternalDnsConfig {
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
		Subscription:       dnsZoneConfig.Subscription,
		ResourceGroup:      dnsZoneConfig.ResourceGroup,
		DnsZoneResourceIDs: dnsZoneConfig.ZoneIds,
		Provider:           provider,
	}
}
