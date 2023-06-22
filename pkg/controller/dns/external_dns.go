package dns

import (
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	return common.NewCleaner(manager, "externalDnsCleaner", common.GvrRetrieverFromObjs(toClean).RemoveGk(schema.GroupKind{
		Group: "v1",
		Kind:  "Namespace",
	}), labels)
}

// NewExternalDns starts all resources required for external dns
func NewExternalDns(manager ctrl.Manager, conf *config.Config, self *appsv1.Deployment) error {
	instances := instances(conf, self)

	deployInstances := filterAction(instances, deploy)
	deployRes := getResources(deployInstances)
	if err := addExternalDnsReconciler(manager, deployRes); err != nil {
		return err
	}

	cleanInstances := filterAction(instances, clean)
	cleanRes := getResources(cleanInstances)
	cleanLabels := getLabels(cleanInstances)
	for k, v := range manifests.TopLevelLabels {
		cleanLabels[k] = v
	}
	for _, c := range cleanInstances {
		for k, v := range c.config.Provider.Labels() {
			cleanLabels[k] = v
		}
	}

	if err := addExternalDnsCleaner(manager, cleanRes, cleanLabels); err != nil {
		return err
	}

	return nil
}

func instances(conf *config.Config, self *appsv1.Deployment) []instance {
	// public
	publicConfig := publicConfig(conf)
	publicAction := deploy
	if len(conf.PublicZoneConfig.ZoneIds) == 0 {
		publicAction = clean
	}
	publicResources := manifests.ExternalDnsResources(conf, self, []*manifests.ExternalDnsConfig{publicConfig})

	// private
	privateConfig := privateConfig(conf)
	privateAction := deploy
	if len(conf.PrivateZoneConfig.ZoneIds) == 0 {
		privateAction = clean
	}
	privateResources := manifests.ExternalDnsResources(conf, self, []*manifests.ExternalDnsConfig{privateConfig})

	return []instance{
		{
			config:    publicConfig,
			resources: publicResources,
			action:    publicAction,
		},
		{
			config:    privateConfig,
			resources: privateResources,
			action:    privateAction,
		},
	}
}

func publicConfig(conf *config.Config) *manifests.ExternalDnsConfig {
	return &manifests.ExternalDnsConfig{
		TenantId:           conf.TenantID,
		Subscription:       conf.PublicZoneConfig.Subscription,
		ResourceGroup:      conf.PublicZoneConfig.ResourceGroup,
		Provider:           manifests.PublicProvider,
		DnsZoneResourceIDs: conf.PublicZoneConfig.ZoneIds,
	}
}

func privateConfig(conf *config.Config) *manifests.ExternalDnsConfig {
	return &manifests.ExternalDnsConfig{
		TenantId:           conf.TenantID,
		Subscription:       conf.PrivateZoneConfig.Subscription,
		ResourceGroup:      conf.PrivateZoneConfig.ResourceGroup,
		Provider:           manifests.PrivateProvider,
		DnsZoneResourceIDs: conf.PrivateZoneConfig.ZoneIds,
	}
}

func filterAction(instances []instance, action action) []instance {
	var ret []instance
	for _, i := range instances {
		if i.action == action {
			ret = append(ret, i)
		}
	}

	return ret
}

func getResources(instances []instance) []client.Object {
	var ret []client.Object
	for _, i := range instances {
		ret = append(ret, i.resources...)
	}
	return ret
}

func getLabels(instances []instance) map[string]string {
	l := map[string]string{}
	for k, v := range manifests.TopLevelLabels {
		l[k] = v
	}

	for _, i := range instances {
		for k, v := range i.config.Provider.Labels() {
			l[k] = v
		}
	}

	return l
}
