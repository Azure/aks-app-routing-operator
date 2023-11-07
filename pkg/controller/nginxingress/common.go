package nginxingress

import (
	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// List returns a list of NginxIngressController objects
func List(cl client.Client, ) (*approutingv1alpha1.NginxIngressControllerList, error) {
	controllers := &approutingv1alpha1.NginxIngressControllerList{}
	if err := cl.List(nil, controllers); err != nil {
		return nil, err
	}

	return controllers, nil
}
