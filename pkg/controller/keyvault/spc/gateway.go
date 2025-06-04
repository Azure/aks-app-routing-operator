package spc

import (
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var gatewaySecretProviderControllerName = controllername.New("gateway", "keyvault", "secret", "provider")

func NewGatewaySecretClassProviderReconciler(manager ctrl.Manager, conf *config.Config, serviceAccountIndexName string) error {
	metrics.InitControllerMetrics(gatewaySecretProviderControllerName)
	if conf.DisableKeyvault {
		return nil
	}

	spcReconciler := &secretProviderClassReconciler[*gatewayv1.Gateway]{
		name: gatewaySecretProviderControllerName,

		client: manager.GetClient(),
		events: manager.GetEventRecorderFor("aks-app-routing-operator"),
		config: conf,
	}

	return gatewaySecretProviderControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&gatewayv1.Gateway{}).
			Owns(&secv1.SecretProviderClass{}).
			Watches(&corev1.ServiceAccount{}, handler.EnqueueRequestsFromMapFunc(generateGatewayGetter(manager, serviceAccountIndexName))),
		manager.GetLogger(),
	).Complete(spcReconciler)
}

func shouldReconcileGateway(gateway *gatewayv1.Gateway) (bool, error) {
	if gateway == nil {
		return false, nil
	}

	if gateway.Spec.GatewayClassName != istioGatewayClassName {
		return false, nil
	}

	if gateway.Spec.Listeners == nil || len(gateway.Spec.Listeners) == 0 {
		return false, nil
	}

	for _, listener := range gateway.Spec.Listeners {
		if listener.TLS != nil && listener.TLS.Options != nil {
			if _, ok := listener.TLS.Options[serviceAccountTLSOption]; ok {
				return true, nil
			}
		}
	}

	return false, nil
}
