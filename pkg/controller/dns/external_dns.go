package dns

import (
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

func addExternalDnsCleaner(manager ctrl.Manager, objs []cleanObj) error {
	return nil // holding off on actually using this until e2e added in future pr

	retriever := common.RetrieverEmpty()
	for _, obj := range objs {
		retriever = retriever.Add(common.RetrieverFromObjs(obj.resources, obj.labels)) // clean up entire unused external dns applications
	}
	for _, provider := range manifests.Providers {
		labels := util.MergeMaps(provider.Labels(), manifests.TopLevelLabels)
		retriever = retriever.Add(common.RetrieverFromGk(labels, manifests.OldExternalDnsGks...)) // clean up unused types from previous versions of app routing
	}

	retriever = retriever.Remove(common.RetrieverFromGk(
		nil, // our compare strat is ignore labels
		schema.GroupKind{
			Group: corev1.GroupName,
			Kind:  "Namespace",
		}),
		common.RemoveOpt{
			CompareStrat: common.IgnoreLabels, // ignore labels, we never want to clean namespaces
		})

	return common.NewCleaner(manager, "externalDnsCleaner", retriever)
}

// NewExternalDns starts all resources required for external dns
func NewExternalDns(manager ctrl.Manager, conf *config.Config, self *appsv1.Deployment) error {
	instances := instances(conf, self)

	deployInstances := filterAction(instances, deploy)
	deployRes := getResources(deployInstances)
	if err := addExternalDnsReconciler(manager, deployRes); err != nil {
		return err
	}

	cleanObjs := cleanObjs(instances)
	if err := addExternalDnsCleaner(manager, cleanObjs); err != nil {
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

func getLabels(instances ...instance) map[string]string {
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

func cleanObjs(instances []instance) []cleanObj {
	var cleanObjs []cleanObj
	for _, instance := range instances {
		if instance.action == clean {
			obj := cleanObj{
				resources: instance.resources,
				labels:    getLabels(instance),
			}
			cleanObjs = append(cleanObjs, obj)
		}
	}

	return cleanObjs
}
