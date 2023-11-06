package manifests

import (
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewNginxIngressController(ingressClassName string) *v1alpha1.NginxIngressController {
	return &v1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nginx-ingress-controller",
		},
		Spec: v1alpha1.NginxIngressControllerSpec{
			IngressClassName:     ingressClassName,
			ControllerNamePrefix: "prefix",
		},
		Status: v1alpha1.NginxIngressControllerStatus{},
	}
}
