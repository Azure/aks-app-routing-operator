package manifests

import (
	_ "embed"
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed embedded/defaultBackendClient.go
var dbeClientContents string

//go:embed embedded/defaultBackendServer.go
var dbeServerContents string

//go:embed embedded/404.html
var NotFoundContents string

//go:embed embedded/404.html
var UnavailableContents string

type DefaultBackendResources struct {
	Client                 *appsv1.Deployment
	Server                 *appsv1.Deployment
	Ingress                *netv1.Ingress
	DefaultBackendServer   *appsv1.Deployment
	Service                *corev1.Service
	DefaultBackendService  *corev1.Service
	NginxIngressController *v1alpha1.NginxIngressController
}

func (t DefaultBackendResources) Objects() []client.Object {
	ret := []client.Object{
		t.Client,
		t.Server,
		t.Service,
		t.Ingress,
		t.DefaultBackendServer,
		t.DefaultBackendService,
		t.NginxIngressController,
	}

	for _, obj := range ret {
		setGroupKindVersion(obj)
	}

	return ret
}

func DefaultBackendClientAndServer(namespace, name, nameserver, keyvaultURI, host, tlsHost string) DefaultBackendResources {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")

	// Client deployment
	clientDeployment := newGoDeployment(dbeClientContents, namespace, name+"-client")
	clientDeployment.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"
	clientDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{
			Name:  "URL",
			Value: "https://" + host + "/test",
		},
		{
			Name:  "TEST_URL",
			Value: "https://" + host + "/fakehost",
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

	// Main server deployment
	serverDeployment := newGoDeployment(serverContents, namespace, name)
	ingressName := name + "-ingress"

	service :=
		&corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
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
					"app": name,
				},
			},
		}

	// Default server deployment
	defaultServerName := "default-" + name + "-server"
	defaultServerDeployment := newGoDeployment(dbeServerContents, namespace, defaultServerName)
	defaultServiceName := "default-" + name + "-service"
	ingressClassName := name + ".backend.ingressclass"
	dbeService :=
		&corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultServiceName,
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
					"app": defaultServerName,
				},
			},
		}

	ingress := &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
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
						Paths: []netv1.HTTPIngressPath{{
							Path:     "/test",
							PathType: to.Ptr(netv1.PathTypePrefix),
							Backend: netv1.IngressBackend{
								Service: &netv1.IngressServiceBackend{
									Name: name,
									Port: netv1.ServiceBackendPort{
										Number: 8080,
									},
								},
							},
						}},
					},
				},
			}},
			TLS: []netv1.IngressTLS{{
				Hosts:      []string{tlsHost},
				SecretName: "keyvault-" + ingressName,
			}},
		},
	}

	if tlsHost == "" {
		ingress.Spec.Rules[0].Host = ""
		ingress.Spec.TLS = nil
		delete(ingress.Annotations, "kubernetes.azure.com/tls-cert-keyvault-uri")
	}

	nicName := name + "-dbe-nginxingress"

	defaultSSLCert := &v1alpha1.DefaultSSLCertificate{KeyVaultURI: &keyvaultURI}
	defaultBackendService := &v1alpha1.NICNamespacedName{defaultServiceName, namespace}

	nic := &v1alpha1.NginxIngressController{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NginxIngressController",
			APIVersion: "approuting.kubernetes.azure.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nicName,
			Namespace: namespace,
			Annotations: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: v1alpha1.NginxIngressControllerSpec{
			IngressClassName:      ingressClassName,
			ControllerNamePrefix:  "nginx-" + name[len(name)-7:],
			DefaultSSLCertificate: defaultSSLCert,
			DefaultBackendService: defaultBackendService,
		},
	}

	return DefaultBackendResources{
		Client:                 clientDeployment,
		Server:                 serverDeployment,
		Ingress:                ingress,
		DefaultBackendServer:   defaultServerDeployment,
		Service:                service,
		DefaultBackendService:  dbeService,
		NginxIngressController: nic,
	}
}

func AddCustomErrorsDeployments(namespace, name, host, tlsHost, ingressClassName string, nic *v1alpha1.NginxIngressController) []client.Object {
	errorsServerDeployment :=
		&appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-errors",
				Namespace: namespace,
				Labels: map[string]string{
					ManagedByKey:                ManagedByVal,
					"app.kubernetes.io/name":    "nginx-errors",
					"app.kubernetes.io/part-of": "ingress-nginx",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: to.Ptr(int32(1)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app":                       "nginx-errors",
						"app.kubernetes.io/name":    "nginx-errors",
						"app.kubernetes.io/part-of": "ingress-nginx",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app":                       "nginx-errors",
							"app.kubernetes.io/name":    "nginx-errors",
							"app.kubernetes.io/part-of": "ingress-nginx",
						},
						Annotations: map[string]string{},
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

	errorsService :=
		&corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-errors",
				Namespace: namespace,
				Annotations: map[string]string{
					ManagedByKey: ManagedByVal,
				},
				Labels: map[string]string{
					"app.kubernetes.io/name":    "nginx-errors",
					"app.kubernetes.io/part-of": "ingress-nginx",
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				}},
				Selector: map[string]string{
					"app": name,
				},
			},
		}

	liveService :=
		&corev1.Service{
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

	deadService :=
		&corev1.Service{
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

	customErrorPagesConfigMap :=
		&corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "custom-error-pages",
				Namespace: namespace,
			},
			Data: map[string]string{
				"404": NotFoundContents,
				"503": UnavailableContents,
			},
		}

	customErrorsDefaultBackendService := &v1alpha1.NICNamespacedName{"nginx-errors", namespace}
	nic.Spec.DefaultBackendService = customErrorsDefaultBackendService

	customErrorNIC :=
		&v1alpha1.NginxIngressController{
			TypeMeta: metav1.TypeMeta{
				Kind:       "NginxIngressController",
				APIVersion: "approuting.kubernetes.azure.com/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "custom-errors-nic",
				Namespace: namespace,
				Annotations: map[string]string{
					ManagedByKey: ManagedByVal,
				},
			},
			Spec: v1alpha1.NginxIngressControllerSpec{
				IngressClassName:     ingressClassName,
				ControllerNamePrefix: "nginx-" + name[len(name)-7:],
				//DefaultSSLCertificate: customErrorsDefaultSSLCert,
				DefaultBackendService: customErrorsDefaultBackendService,
				CustomHTTPErrors:      []int{404, 503},
			},
		}

	liveServicePod :=
		&corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pod",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "live-app",
				Labels: map[string]string{
					"app": "live",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "live-app",
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
			Name:      "live-ingress",
			Namespace: namespace,
			Annotations: map[string]string{
				ManagedByKey: ManagedByVal,
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
								Path:     "/live",
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
								Path:     "/dead",
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
	return []client.Object{errorsServerDeployment, errorsService, liveService, liveServicePod, liveIngress, deadService, customErrorPagesConfigMap, customErrorNIC}
}
