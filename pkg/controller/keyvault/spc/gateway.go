package spc

import (
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	ctrl "sigs.k8s.io/controller-runtime"
)

var gatewaySecretProviderControllerName = controllername.New("gateway", "keyvault", "secret", "provider")

func NewGatewaySecretClassProviderReconciler(manager ctrl.Manager, conf *config.Config, serviceAccountIndexName string) error {
	return nil
}
