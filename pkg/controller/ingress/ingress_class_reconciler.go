package ingress

import (
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewIngressClassReconciler creates a reconciler that manages ingress class resources
func NewIngressClassReconciler(manager ctrl.Manager, resources []client.Object) error {
	return common.NewResourceReconciler(manager, "ingressClassReconciler", resources, reconcileInterval)
}
