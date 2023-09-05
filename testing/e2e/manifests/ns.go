package manifests

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UncollisionedNs returns a namespace with a guaranteed unique name after creating the namespace
func UncollisionedNs() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "app-routing-e2e-",
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
	}
}
