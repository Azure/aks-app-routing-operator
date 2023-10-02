package nginxingress

import (
	"context"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NginxIngressControllerReconciler reconciles a NginxIngressController object
type NginxIngressControllerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *NginxIngressControllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lgr := log.FromContext(ctx)
	lgr.Info("Reconciling NginxIngressController")

	var nginxIngressController approutingv1alpha1.NginxIngressController
	if err := r.Get(ctx, req.NamespacedName, &nginxIngressController); err != nil {
		lgr.Error(err, "unable to fetch NginxIngressController")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NginxIngressControllerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&approutingv1alpha1.NginxIngressController{}).
		Complete(r)
}
