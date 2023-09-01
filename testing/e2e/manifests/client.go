package manifests

import (
	_ "embed"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed embedded/client.go
var clientContents string

//go:embed embedded/server.go
var serverContents string

func ClientAndServer(namespace, name string) []client.Object {
	var host, nameserver, keyvaultURI string

	clientDeployment := newGoDeployment(clientContents, namespace, name+"-client")
	clientDeployment.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"
	clientDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{{
		Name:  "URL",
		Value: "https://" + host,
	},
		{
			Name:  "NAMESERVER",
			Value: nameserver,
		},
		{
			Name:      "POD_IP",
			ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}},
		},
	}
	clientDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
		FailureThreshold:    1,
		InitialDelaySeconds: 1,
		PeriodSeconds:       1,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/",
				Port:   intstr.FromInt(8080),
				Scheme: corev1.URISchemeHTTP,
			},
		},
	}

	serverDeployment := newGoDeployment(serverContents, namespace, name+"-server")

	ret := []client.Object{
		clientDeployment,
		serverDeployment,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name + "server",
				Namespace: namespace,
				Annotations: map[string]string{
					"kubernetes.azure.com/ingress-host":          host,
					"kubernetes.azure.com/tls-cert-keyvault-uri": keyvaultURI,
				},
			},
		},
		// TODO: more to add here
	}
	for _, obj := range ret {
		setGroupKindVersion(obj)
	}

	return ret
}
