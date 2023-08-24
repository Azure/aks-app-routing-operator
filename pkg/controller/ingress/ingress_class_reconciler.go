package ingress

import (
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewIngressClassReconciler creates a reconciler that manages ingress class resources
func NewIngressClassReconciler(manager ctrl.Manager, resources []client.Object, name string) error {
	return common.NewResourceReconciler(manager, controllername.New(name, "ingress", "class", "reconciler"), resources, reconcileInterval)
}
