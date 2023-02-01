package manifests

import (
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	topLevelLabels = map[string]string{"app.kubernetes.io/managed-by": "aks-app-routing-operator"}
)

func getOwnerRefs(deploy *appsv1.Deployment) []metav1.OwnerReference {
	if deploy == nil {
		return nil
	}
	return []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       deploy.Name,
		UID:        deploy.UID,
	}}
}

func newNamespace(conf *config.Config) *corev1.Namespace {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        conf.NS,
			Labels:      topLevelLabels,
			Annotations: map[string]string{},
		},
	}

	return ns
}
