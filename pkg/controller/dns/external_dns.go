package dns

import (
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
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
	return common.NewResourceReconciler(manager, controllername.New("external", "dns", "reconciler"), resources, reconcileInterval)
}

func addExternalDnsCleaner(manager ctrl.Manager, objs []cleanObj) error {
	return nil // disable cleaner until we have better test coverage

	retriever := common.RetrieverEmpty()
	for _, obj := range objs {
		retriever = retriever.Add(common.RetrieverFromObjs(obj.resources, obj.labels)) // clean up entire unused external dns applications
	}
	for _, provider := range manifests.Providers {
		labels := util.MergeMaps(provider.Labels(), manifests.GetTopLevelLabels())
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

	return common.NewCleaner(manager, controllername.New("external", "dns", "cleaner"), retriever)
}

// NewExternalDns starts all resources required for external dns
func NewExternalDns(manager ctrl.Manager, conf *config.Config) error {
	instances := instances(conf)

	deployInstances := filterAction(instances, deploy)
	deployRes := getResources(deployInstances)
	if err := addExternalDnsReconciler(manager, deployRes); err != nil {
		return err
	}

	objs := cleanObjs(instances)
	if err := addExternalDnsCleaner(manager, objs); err != nil {
		return err
	}

	return nil
}

func instances(conf *config.Config) []instance {
	// public
	publicCfg := publicConfig(conf)
	publicAction := actionFromConfig(publicCfg)
	publicResources := manifests.ExternalDnsResources(conf, []*manifests.ExternalDnsConfig{publicCfg})

	// private
	privateCfg := privateConfig(conf)
	privateAction := actionFromConfig(privateCfg)
	privateResources := manifests.ExternalDnsResources(conf, []*manifests.ExternalDnsConfig{privateCfg})

	return []instance{
		{
			config:    publicCfg,
			resources: publicResources,
			action:    publicAction,
		},
		{
			config:    privateCfg,
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
		DnsZoneResourceIDs: util.Keys(conf.PublicZoneConfig.ZoneIds),
	}
}

func privateConfig(conf *config.Config) *manifests.ExternalDnsConfig {
	return &manifests.ExternalDnsConfig{
		TenantId:           conf.TenantID,
		Subscription:       conf.PrivateZoneConfig.Subscription,
		ResourceGroup:      conf.PrivateZoneConfig.ResourceGroup,
		Provider:           manifests.PrivateProvider,
		DnsZoneResourceIDs: util.Keys(conf.PrivateZoneConfig.ZoneIds),
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
	for k, v := range manifests.GetTopLevelLabels() {
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

func actionFromConfig(conf *manifests.ExternalDnsConfig) action {
	if len(conf.DnsZoneResourceIDs) == 0 {
		return clean
	}

	return deploy
}
