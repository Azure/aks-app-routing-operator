package manifests

import (
	_ "embed"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed embedded/defaultBackendClient.go
var dbClientContents string

//go:embed embedded/defaultBackendServer.go
var dbServerContents string

var (
	validUrlPath    = "/test"
	invalidUrlPath  = "/fakehost"
	liveServicePath = "/live"
	deadServicePath = "/dead"
	notFoundPath    = "/notfound"
)

func DefaultBackendClientAndServer(namespace, name, nameserver, keyvaultURI, ingressClassName, host, tlsHost string) ClientServerResources {
	// Client deployment
	clientDeployment := newGoDeployment(dbClientContents, namespace, name+"-db-client")
	clientDeployment.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"
	clientDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{
			Name:  "VALID_URL",
			Value: "https://" + host + validUrlPath,
		},
		{
			Name:  "INVALID_URL",
			Value: "https://" + host + invalidUrlPath,
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
	serverName := name + "dbtest-server"
	serviceName := name + "dbtest-service"

	serverDeployment := newGoDeployment(serverContents, namespace, serverName)
	ingressName := name + "-ingress"

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

	// Default server deployment
	defaultServerName := "default-" + name + "-server"
	defaultServiceName := "default-" + name + "-service"

	defaultServerDeployment := newGoDeployment(dbServerContents, namespace, defaultServerName)
	dbService :=
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
							Path:     validUrlPath,
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
				SecretName: "keyvault-" + ingressName,
			}},
		},
	}

	if tlsHost == "" {
		ingress.Spec.Rules[0].Host = ""
		ingress.Spec.TLS = nil
		delete(ingress.Annotations, "kubernetes.azure.com/tls-cert-keyvault-uri")
	}

	return ClientServerResources{
		Client:  clientDeployment,
		Server:  serverDeployment,
		Ingress: ingress,
		Service: service,
		AddedObjects: []client.Object{
			defaultServerDeployment,
			dbService,
		},
	}
}
