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
var defaultBackendClientContents string

//go:embed embedded/server.go
var dbeServerContents string

type defaultBackendResources struct {
	Client                 *appsv1.Deployment
	Server                 *appsv1.Deployment
	Service                *corev1.Service
	NginxIngressController *v1alpha1.NginxIngressController
}

func (t defaultBackendResources) Objects() []client.Object {
	ret := []client.Object{
		t.Client,
		t.Server,
		t.Service,
		t.NginxIngressController,
	}

	for _, obj := range ret {
		setGroupKindVersion(obj)
	}

	return ret
}

func DefaultBackendClientAndServer(namespace, name, nameserver, keyvaultURI, host, tlsHost string) defaultBackendResources {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")
	clientDeployment := newGoDeployment(defaultBackendClientContents, namespace, name+"-client")
	clientDeployment.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"
	clientDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{
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

	serverName := name + "-server"
	serverDeployment := newGoDeployment(dbeServerContents, namespace, serverName)
	serviceName := name + "service"
	nicName := name + "-nginxingress"

	service :=
		&corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
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
					"app": serverName,
				},
			},
		}

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
				"kubernetes.azure.com/tls-cert-keyvault-uri": keyvaultURI,
			},
		},
		Spec: v1alpha1.NginxIngressControllerSpec{
			IngressClassName: "webapprouting.kubernetes.azure.com",
			Rules: []netv1.IngressRule{{
				Host: host,
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: []netv1.HTTPIngressPath{{
							Path:     "/",
							PathType: to.Ptr(netv1.PathTypePrefix),
							Backend: netv1.IngressBackend{
								Service: &netv1.IngressServiceBackend{
									Name: serviceName,
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
				SecretName: "keyvault-" + nicName,
			}},
		},
	}

	if tlsHost == "" {
		nic.Spec.Rules[0].Host = ""
		nic.Spec.TLS = nil
		delete(nic.Annotations, "kubernetes.azure.com/tls-cert-keyvault-uri")
	}

	return defaultBackendResources{
		Client:                 clientDeployment,
		Server:                 serverDeployment,
		Service:                service,
		NginxIngressController: nic,
	}
}
