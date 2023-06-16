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
	if len(conf.PublicZoneConfig.ZoneIds) == 0 && len(conf.PrivateZoneConfig.ZoneIds) == 0 {
		return nil
	}

	configs, _ := generateZoneConfigs(conf)

	// TODO: Uncomment this to implement an externalDNS cleanup runner
	//err := newCleanupRunner(manager, namesToDelete)
	//
	//if err != nil {
	//	return fmt.Errorf("failed to start cleanup runner: %w", err)
	//}

	objs := append([]client.Object{}, manifests.ExternalDnsResources(conf, self, configs)...)

	return newExternalDnsReconciler(manager, objs)
}

func generateZoneConfigs(conf *config.Config) (configs []*manifests.ExternalDnsConfig, namesToDelete []string) {
	publicResourceName := fmt.Sprintf("%s%s", manifests.ExternalDnsResourceName, manifests.PublicSuffix)
	privateResourceName := fmt.Sprintf("%s%s", manifests.ExternalDnsResourceName, manifests.PrivateSuffix)

	if len(conf.PrivateZoneConfig.ZoneIds) > 0 {
		configs = append(configs, generateConfig(conf, conf.PrivateZoneConfig, manifests.PrivateProvider, privateResourceName))
	} else {
		namesToDelete = append(namesToDelete, privateResourceName)
	}

	if len(conf.PublicZoneConfig.ZoneIds) > 0 {
		configs = append(configs, generateConfig(conf, conf.PublicZoneConfig, manifests.PublicProvider, publicResourceName))
	} else {
		namesToDelete = append(namesToDelete, publicResourceName)
	}

	// namesToDelete will eventually be used for externalDNS cleanup
	return
}

func generateConfig(conf *config.Config, dnsZoneConfig config.DnsZoneConfig, provider manifests.Provider, resourceName string) *manifests.ExternalDnsConfig {
	return &manifests.ExternalDnsConfig{
		ResourceName:       resourceName,
		TenantId:           conf.TenantID,
		Subscription:       dnsZoneConfig.Subscription,
		ResourceGroup:      dnsZoneConfig.ResourceGroup,
		DnsZoneResourceIDs: dnsZoneConfig.ZoneIds,
		Provider:           provider,
	}
}
