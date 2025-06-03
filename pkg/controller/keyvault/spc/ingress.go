package spc

import (
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	netv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var ingressSecretProviderControllerName = controllername.New("keyvault", "ingress", "secret", "provider")

func NewIngressSecretProviderClassReconciler(manager ctrl.Manager, conf *config.Config, ingressManager keyvault.IngressManager) error {
	metrics.InitControllerMetrics(ingressSecretProviderControllerName)
	if conf.DisableKeyvault {
		return nil
	}

	spcReconciler := &secretProviderClassReconciler[*netv1.Ingress]{
		name:     ingressSecretProviderControllerName,
		spcNamer: getIngressSpcName,
		shouldReconcile: func(i *netv1.Ingress) (bool, error) {
			isManaged, err := ingressManager.IsManaging(i)
			if err != nil {
				return false, fmt.Errorf("checking if ingress %s is managed: %w", i.Name, err)
			}

			return isManaged, nil
		},

		client: manager.GetClient(),
		events: manager.GetEventRecorderFor("aks-app-routing-operator"),
		config: conf,
	}

	return ingressSecretProviderControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&netv1.Ingress{}).
			Owns(&secv1.SecretProviderClass{}),
		manager.GetLogger(),
	).Complete(spcReconciler)
}

func getIngressSpcName(ing *netv1.Ingress) string {
	if ing == nil {
		return ""
	}

	return "keyvault-" + ing.Name
}
