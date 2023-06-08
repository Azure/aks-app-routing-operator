package nginx

import (
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/ingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/service"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type nginx struct {
	name             string
	manager          manager.Manager
	conf             *config.Config
	self             *appsv1.Deployment
	ingConfigs       []*manifests.NginxIngressConfig
	defaultIngConfig *manifests.NginxIngressConfig
}

// New starts all resources required for provisioning Nginx ingresses and the configs used for those ingresses
func New(m manager.Manager, conf *config.Config, self *appsv1.Deployment) ([]*manifests.NginxIngressConfig, error) {
	defaultIngConfig := &manifests.NginxIngressConfig{
		ControllerClass: "webapprouting.kubernetes.azure.com/nginx",
		ResourceName:    "nginx",
		IcName:          "webapprouting.kubernetes.azure.com",
	}

	// TODO: re-add for dynamic provisioning, until then serviceConfig is nil
	//if conf.DNSZoneDomain != "" && conf.DNSZonePrivate {
	//	defaultIngConfig.ServiceConfig = &manifests.ServiceConfig{
	//		IsInternal: true,
	//		Hostname:   conf.DNSZoneDomain,
	//	}
	//}

	ingConfigs := []*manifests.NginxIngressConfig{defaultIngConfig}
	n := &nginx{
		name:             "nginx",
		manager:          m,
		conf:             conf,
		self:             self,
		ingConfigs:       ingConfigs,
		defaultIngConfig: defaultIngConfig,
	}

	if err := n.addIngressClassReconciler(); err != nil {
		return nil, err
	}

	if err := n.addIngressControllerReconciler(); err != nil {
		return nil, err
	}

	if err := n.addIngressReconciler(); err != nil {
		return nil, err
	}

	return n.ingConfigs, nil
}

func (n *nginx) addIngressClassReconciler() error {
	objs := []client.Object{}
	for _, config := range n.ingConfigs {
		objs = append(objs, manifests.NginxIngressClass(n.conf, n.self, config)...)
	}

	return ingress.NewIngressClassReconciler(n.manager, objs, n.name)
}

func (n *nginx) addIngressControllerReconciler() error {
	objs := []client.Object{}
	for _, config := range n.ingConfigs {
		objs = append(objs, manifests.NginxIngressControllerResources(n.conf, n.self, config)...)
	}

	return ingress.NewIngressControllerReconciler(n.manager, objs, n.name)
}

func (n *nginx) addIngressReconciler() error {
	return service.NewNginxIngressReconciler(n.manager, n.defaultIngConfig)
}
