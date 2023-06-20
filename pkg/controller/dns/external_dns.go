package dns

import (
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

// addExternalDnsReconciler creates a reconciler that manages external dns resources
func addExternalDnsReconciler(manager ctrl.Manager, resources []client.Object) error {
	return common.NewResourceReconciler(manager, "externalDnsReconciler", resources, reconcileInterval)
}

func addExternalDnsCleaner(manager ctrl.Manager, toClean []client.Object, labels map[string]string) error {
	return common.NewCleaner(manager, "externalDnsCleaner", common.GvrRetrieverFromObjs(toClean), labels)
}

// NewExternalDns starts all resources required for external dns
func NewExternalDns(manager ctrl.Manager, conf *config.Config, self *appsv1.Deployment) error {
	if len(conf.PublicZoneConfig.ZoneIds) == 0 && len(conf.PrivateZoneConfig.ZoneIds) == 0 {
		return nil
	}

	var needed []*manifests.ExternalDnsConfig
	var toClean []*manifests.ExternalDnsConfig

	publicConfig := &manifests.ExternalDnsConfig{
		TenantId:           conf.TenantID,
		Subscription:       conf.PublicZoneConfig.Subscription,
		ResourceGroup:      conf.PublicZoneConfig.ResourceGroup,
		Provider:           manifests.PublicProvider,
		DnsZoneResourceIDs: conf.PublicZoneConfig.ZoneIds,
	}
	if len(conf.PublicZoneConfig.ZoneIds) > 0 {
		needed = append(needed, publicConfig)
	} else {
		toClean = append(toClean, publicConfig)
	}

	privateConfig := &manifests.ExternalDnsConfig{
		TenantId:           conf.TenantID,
		Subscription:       conf.PrivateZoneConfig.Subscription,
		ResourceGroup:      conf.PrivateZoneConfig.ResourceGroup,
		Provider:           manifests.PrivateProvider,
		DnsZoneResourceIDs: conf.PrivateZoneConfig.ZoneIds,
	}
	if len(conf.PrivateZoneConfig.ZoneIds) > 0 {
		needed = append(needed, privateConfig)
	} else {
		toClean = append(toClean, privateConfig)
	}

	res := manifests.ExternalDnsResources(conf, self, needed)
	if err := addExternalDnsReconciler(manager, res); err != nil {
		return err
	}

	cleanRes := manifests.ExternalDnsResources(conf, self, toClean)
	cleanLabels := map[string]string{}
	for k, v := range manifests.TopLevelLabels {
		cleanLabels[k] = v
	}
	for _, c := range toClean {
		for k, v := range c.Provider.Labels() {
			cleanLabels[k] = v
		}
	}

	if err := addExternalDnsCleaner(manager, cleanRes, cleanLabels); err != nil {
		return err
	}

	return nil
}
