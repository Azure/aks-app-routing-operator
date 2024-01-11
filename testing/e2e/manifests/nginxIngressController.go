package manifests

import (
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
			ControllerNamePrefix: "nginx",
		},
		Status: v1alpha1.NginxIngressControllerStatus{},
	}
}
