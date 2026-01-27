package manifests

import (
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultNginxIngressController returns the default NIC that the operator creates.
// This must match the values in pkg/controller/nginxingress/default.go GetDefaultNginxIngressController
func DefaultNginxIngressController() *v1alpha1.NginxIngressController {
	return &v1alpha1.NginxIngressController{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NginxIngressController",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: v1alpha1.NginxIngressControllerSpec{
			IngressClassName:     "webapprouting.kubernetes.azure.com",
			ControllerNamePrefix: "nginx", // Must match DefaultNicResourceName in default.go
		},
		Status: v1alpha1.NginxIngressControllerStatus{},
	}
}

func NewNginxIngressController(name, ingressClassName string) *v1alpha1.NginxIngressController {
	return &v1alpha1.NginxIngressController{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NginxIngressController",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.NginxIngressControllerSpec{
			IngressClassName:     ingressClassName,
			ControllerNamePrefix: name,
		},
		Status: v1alpha1.NginxIngressControllerStatus{},
	}
}
