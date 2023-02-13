package nginx

import (
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/ingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/osm"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/service"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	defaultIngConfig = &manifests.NginxIngressConfig{
		ControllerClass: "webapprouting.kubernetes.azure.com/nginx",
		ResourceName:    "nginx",
		IcName:          "webapprouting.kubernetes.azure.com",
	}
	ingConfigs = []*manifests.NginxIngressConfig{defaultIngConfig}
)

type nginx struct {
	name       string
	manager    manager.Manager
	conf       *config.Config
	self       *appsv1.Deployment
	ingConfigs []*manifests.NginxIngressConfig
}

// New starts all resources required for Nginx ingresses
func New(m manager.Manager, conf *config.Config, self *appsv1.Deployment) error {
	n := &nginx{
		name:       "nginx",
		manager:    m,
		conf:       conf,
		self:       self,
		ingConfigs: ingConfigs,
	}

	if err := n.addIngressClassReconciler(); err != nil {
		return err
	}

	if err := n.addIngressControllerReconciler(); err != nil {
		return err
	}

	if err := n.addConcurrencyWatchdog(); err != nil {
		return err
	}

	if err := n.addIngressSecretProviderClassReconciler(); err != nil {
		return err
	}

	if err := n.addPlaceholderPodController(); err != nil {
		return err
	}

	if err := n.addIngressReconciler(); err != nil {
		return err
	}

	return nil
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

func (n *nginx) addConcurrencyWatchdog() error {
	return ingress.NewNginxConcurrencyWatchdog(n.manager, n.conf, n.ingConfigs)
}

func (n *nginx) addIngressSecretProviderClassReconciler() error {
	return keyvault.NewIngressSecretProviderClassReconciler(n.manager, n.conf, n.ingConfigs)
}

func (n *nginx) addPlaceholderPodController() error {
	return keyvault.NewPlaceholderPodController(n.manager, n.conf, n.ingConfigs)
}

func (n *nginx) addIngressReconciler() error {
	return service.NewIngressReconciler(n.manager, defaultIngConfig)

}

func (n *nginx) addIngressBackendReconciler() error {
	return osm.NewIngressBackendReconciler(n.manager, n.conf, n.ingConfigs)
}
