package dns

import (
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const reconcileInterval = time.Minute * 3

// NewExternalDnsReconciler creates a reconciler that manages external dns resources
func NewExternalDnsReconciler(manager ctrl.Manager, resources []client.Object) error {
	return common.NewResourceReconciler(manager, "externalDnsReconciler", resources, reconcileInterval)
}
