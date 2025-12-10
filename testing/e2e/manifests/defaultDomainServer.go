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

func DefaultDomainServer(name string) []client.Object {
	deployment := newGoDeployment(defaultDomainServerContents, operatorNs, name)
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
					SecretName: defaultDomainCertSecret,
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
			Namespace: operatorNs,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       defaultDomainPort,
				TargetPort: intstr.FromInt(defaultDomainPort),
			}},
			Selector: map[string]string{
				"app": name,
			},
		},
	}

	defaultDomainSecret := CreateDefaultDomainSecret(certPEM, keyPEM)

	return []client.Object{
		defaultDomainSecret,
		service,
		deployment,
	}
}
