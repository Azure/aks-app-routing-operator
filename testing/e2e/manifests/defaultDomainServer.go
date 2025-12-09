package manifests

import (
	_ "embed"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed embedded/defaultDomainServer.golang
var defaultDomainServerContents string

func DefaultDomainServer(namespace, name string) []client.Object {
	deployment := newGoDeployment(defaultDomainServerContents, namespace, name)
	deployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{
			Name:  "CERT_PATH",
			Value: "/etc/tls/tls.crt",
		},
		{
			Name:  "KEY_PATH",
			Value: "/etc/tls/tls.key",
		},
	}

	// Mount the secret
	deployment.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "tls-secret",
			MountPath: "/etc/tls",
			ReadOnly:  true,
		},
	}
	deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "default-domain-cert",
				},
			},
		},
	}

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
			Selector: map[string]string{
				"app": name,
			},
		},
	}

	return []client.Object{
		service,
		deployment,
	}
}
