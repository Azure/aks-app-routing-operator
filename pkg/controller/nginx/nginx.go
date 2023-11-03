package nginx

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/service"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	defaultIcName = "webapprouting.kubernetes.azure.com"
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
	kcs, err := kubernetes.NewForConfig(m.GetConfig()) // need to use config since manager hasn't started yet
	if err != nil {
		return nil, err
	}

	defaultControllerClass := "webapprouting.kubernetes.azure.com/nginx"
	defaultIc, err := kcs.NetworkingV1().IngressClasses().Get(context.Background(), defaultIcName, metav1.GetOptions{})
	if err == nil {
		defaultControllerClass = defaultIc.Spec.Controller
	}
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}

	defaultIngConfig := &manifests.NginxIngressConfig{
		ControllerClass: defaultControllerClass,
		ResourceName:    "nginx",
		IcName:          defaultIcName,
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

	if err := n.addIngressReconciler(); err != nil {
		return nil, err
	}

	return n.ingConfigs, nil
}

func (n *nginx) addIngressReconciler() error {
	return service.NewNginxIngressReconciler(n.manager, n.defaultIngConfig)
}
