package manifests

import (
	_ "embed"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

//go:embed embedded/gateway_client.golang
var gatewayClientContents string

const (
	// IstioGatewayClassName is the GatewayClass name for Istio managed gateways
	IstioGatewayClassName = "istio"

	// TLSCertKeyvaultURIOption is the TLS option key for specifying the KeyVault certificate URI
	TLSCertKeyvaultURIOption = "kubernetes.azure.com/tls-cert-keyvault-uri"

	// TLSCertServiceAccountOption is the TLS option key for specifying the ServiceAccount for workload identity
	TLSCertServiceAccountOption = "kubernetes.azure.com/tls-cert-service-account"
)

// GatewayClientServerResources contains the Kubernetes resources needed for Gateway API e2e testing
type GatewayClientServerResources struct {
	Client       *appsv1.Deployment
	Server       *appsv1.Deployment
	Service      *corev1.Service
	Gateway      *gatewayv1.Gateway
	HTTPRoute    *gatewayv1.HTTPRoute
	AddedObjects []client.Object
}

// Objects returns all Kubernetes objects in this resource set
func (g GatewayClientServerResources) Objects() []client.Object {
	ret := []client.Object{}

	if g.Server != nil {
		ret = append(ret, g.Server)
	}
	if g.Service != nil {
		ret = append(ret, g.Service)
	}
	if g.Gateway != nil {
		ret = append(ret, g.Gateway)
	}
	if g.HTTPRoute != nil {
		ret = append(ret, g.HTTPRoute)
	}
	if g.Client != nil {
		ret = append(ret, g.Client)
	}

	ret = append(ret, g.AddedObjects...)

	for _, obj := range ret {
		setGroupKindVersion(obj)
	}

	return ret
}

// GatewayClientAndServer creates the resources needed for Gateway API e2e testing with TLS
// Parameters:
//   - namespace: the namespace for all resources
//   - name: base name for resources (will be sanitized)
//   - nameserver: DNS nameserver for the client to use for resolution
//   - keyvaultURI: Azure Key Vault certificate URI for TLS
//   - host: hostname for the Gateway listener and HTTPRoute
//   - tlsHost: hostname for TLS certificate (can include wildcards)
//   - serviceAccountName: name of the ServiceAccount for workload identity (must be created separately)
//   - gatewayClassName: the GatewayClass name to use (e.g., "istio")
func GatewayClientAndServer(namespace, name, nameserver, keyvaultURI, host, tlsHost, serviceAccountName, gatewayClassName string) GatewayClientServerResources {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")

	// Create client deployment using gateway-specific client (doesn't validate X-Forwarded-For)
	clientDeployment := newGoDeployment(gatewayClientContents, namespace, name+"-gw-client")
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

	// Create server deployment
	serverName := name + "-gw-server"
	serverDeployment := newGoDeployment(serverContents, namespace, serverName)

	// Create service for the server
	serviceName := name + "-gw-service"
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
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
				"app": serverName,
			},
		},
	}

	// Create Gateway with TLS configuration
	gatewayName := name + "-gateway"
	listenerName := "https"
	gateway := &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayName,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(gatewayClassName),
			Listeners: []gatewayv1.Listener{
				{
					Name:     gatewayv1.SectionName(listenerName),
					Hostname: (*gatewayv1.Hostname)(&tlsHost),
					Port:     gatewayv1.PortNumber(443),
					Protocol: gatewayv1.HTTPSProtocolType,
					TLS: &gatewayv1.GatewayTLSConfig{
						Mode: to.Ptr(gatewayv1.TLSModeTerminate),
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							TLSCertKeyvaultURIOption:    gatewayv1.AnnotationValue(keyvaultURI),
							TLSCertServiceAccountOption: gatewayv1.AnnotationValue(serviceAccountName),
						},
					},
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: to.Ptr(gatewayv1.NamespacesFromSame),
						},
					},
				},
			},
		},
	}

	// Create HTTPRoute
	httpRouteName := name + "-httproute"
	httpRoute := &gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      httpRouteName,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:        gatewayv1.ObjectName(gatewayName),
						SectionName: to.Ptr(gatewayv1.SectionName(listenerName)),
					},
				},
			},
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(host)},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  to.Ptr(gatewayv1.PathMatchPathPrefix),
								Value: to.Ptr("/"),
							},
						},
					},
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(serviceName),
									Port: to.Ptr(gatewayv1.PortNumber(8080)),
								},
							},
						},
					},
				},
			},
		},
	}

	return GatewayClientServerResources{
		Client:    clientDeployment,
		Server:    serverDeployment,
		Service:   service,
		Gateway:   gateway,
		HTTPRoute: httpRoute,
	}
}
