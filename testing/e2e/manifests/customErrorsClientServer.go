package manifests

import (
	_ "embed"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed embedded/customErrorsClient.golang
var ceClientContents string

//go:embed embedded/404.html
var notFoundContents string

//go:embed embedded/503.html
var unavailableContents string

func CustomErrorsClientAndServer(namespace, name, nameserver, keyvaultURI, host, tlsHost, ingressClassName string, serviceName *string) ClientServerResources {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")

	// Client deployment
	errorsClientDeployment := newGoDeployment(ceClientContents, namespace, name+"-ce-client")
	errorsClientDeployment.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"
	errorsClientDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{
			Name:  "LIVE",
			Value: "https://" + host + liveServicePath,
		},
		{
			Name:  "DEAD",
			Value: "https://" + host + deadServicePath,
		},
		{
			Name:  "NOT_FOUND",
			Value: "https://" + host + notFoundPath,
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
	errorsClientDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
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

	errorsServerName := name + "-nginx-errors-server"
	errorsServerDeployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      errorsServerName,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: to.Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": errorsServerName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": errorsServerName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "nginx-error-server",
						Image: "registry.k8s.io/ingress-nginx/nginx-errors:v20230505@sha256:3600dcd1bbd0d05959bb01af4b272714e94d22d24a64e91838e7183c80e53f7f",
						Ports: []corev1.ContainerPort{
							{
								Name:          "http",
								ContainerPort: 8080,
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "custom-error-pages",
								MountPath: "/www",
							},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "custom-error-pages",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "custom-error-pages",
									},
									Items: []corev1.KeyToPath{
										{Key: "404", Path: "404.html"},
										{Key: "503", Path: "503.html"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	errorsServiceName := name + "-nginx-errors-service"
	if serviceName != nil {
		errorsServiceName = *serviceName
	}
	errorsService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      errorsServiceName,
			Namespace: namespace,
			Annotations: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
			Selector: map[string]string{
				"app": errorsServerName,
			},
		},
	}

	liveService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "live-service",
			Namespace: namespace,
			Annotations: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       5678,
				TargetPort: intstr.FromInt(5678),
			}},
			Selector: map[string]string{
				"app": "live",
			},
		},
	}

	deadService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dead-service",
			Namespace: namespace,
			Annotations: map[string]string{
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
				"app": "dead",
			},
		},
	}

	customErrorPagesConfigMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-error-pages",
			Namespace: namespace,
		},
		Data: map[string]string{
			"404": notFoundContents,
			"503": unavailableContents,
		},
	}

	liveServicePod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "live-app",
			Labels: map[string]string{
				"app": "live",
			},
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "live-app",
					Image: "hashicorp/http-echo",
					Args:  []string{"-text=live service"},
				},
			},
		},
	}

	liveIngress := &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-live-ingress",
			Namespace: namespace,
			Annotations: map[string]string{
				ManagedByKey: ManagedByVal,
				"kubernetes.azure.com/tls-cert-keyvault-uri": keyvaultURI,
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: to.Ptr(ingressClassName),
			Rules: []netv1.IngressRule{{
				Host: host,
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: []netv1.HTTPIngressPath{
							{
								Path:     liveServicePath,
								PathType: to.Ptr(netv1.PathTypePrefix),
								Backend: netv1.IngressBackend{
									Service: &netv1.IngressServiceBackend{
										Name: "live-service",
										Port: netv1.ServiceBackendPort{
											Number: 5678,
										},
									},
								},
							},
							{
								Path:     deadServicePath,
								PathType: to.Ptr(netv1.PathTypePrefix),
								Backend: netv1.IngressBackend{
									Service: &netv1.IngressServiceBackend{
										Name: "dead-service",
										Port: netv1.ServiceBackendPort{
											Number: 8080,
										},
									},
								},
							},
						},
					},
				},
			}},
			TLS: []netv1.IngressTLS{{
				Hosts:      []string{tlsHost},
				SecretName: "keyvault-" + name + "-ingress",
			}},
		},
	}

	if tlsHost == "" {
		liveIngress.Spec.Rules[0].Host = ""
		liveIngress.Spec.TLS = nil
		delete(liveIngress.Annotations, "kubernetes.azure.com/tls-cert-keyvault-uri")
	}

	return ClientServerResources{
		Client:  errorsClientDeployment,
		Server:  errorsServerDeployment,
		Ingress: liveIngress,
		Service: errorsService,
		AddedObjects: []client.Object{
			liveService,
			deadService,
			liveServicePod,
			customErrorPagesConfigMap,
		},
	}
}
