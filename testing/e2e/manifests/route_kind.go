package manifests

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// RouteKind describes a Gateway API route type (HTTPRoute, GRPCRoute, TLSRoute, ...) so the
// gateway e2e suite can exercise different route kinds with the same surrounding scaffolding
// (Gateway, ExternalDNS, KV cert, namespaces). Today only HTTPRouteKind is implemented.
type RouteKind interface {
	// Name is a short, file-system-safe identifier (e.g. "http", "grpc", "tls"), used to derive
	// per-kind namespace prefixes, CR names, hostnames, etc.
	Name() string

	// RouteObjectName returns the route object name to use given a base name.
	RouteObjectName(baseName string) string

	// Listener returns the gateway listener spec for this kind. HTTP/GRPC use HTTPS/443 with
	// TLS terminate; TLS uses TLS/443 passthrough.
	Listener(listenerName, tlsHost, keyvaultURI, serviceAccountName string) gatewayv1.Listener

	// Route builds the route object attaching to the named gateway+listener and forwarding to
	// the named backend service.
	Route(namespace, name, gatewayName, listenerName, tlsHost, backendServiceName string, backendPort int32) client.Object
}

// HTTPRouteKind is the RouteKind implementation for HTTPRoute.
type HTTPRouteKind struct{}

var _ RouteKind = HTTPRouteKind{}

func (HTTPRouteKind) Name() string { return "http" }

func (HTTPRouteKind) RouteObjectName(baseName string) string {
	return baseName + "-httproute"
}

func (HTTPRouteKind) Listener(listenerName, tlsHost, keyvaultURI, serviceAccountName string) gatewayv1.Listener {
	hostname := gatewayv1.Hostname(tlsHost)
	return gatewayv1.Listener{
		Name:     gatewayv1.SectionName(listenerName),
		Hostname: &hostname,
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
	}
}

func (HTTPRouteKind) Route(namespace, name, gatewayName, listenerName, tlsHost, backendServiceName string, backendPort int32) client.Object {
	return &gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
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
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(tlsHost)},
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
									Name: gatewayv1.ObjectName(backendServiceName),
									Port: to.Ptr(gatewayv1.PortNumber(backendPort)),
								},
							},
						},
					},
				},
			},
		},
	}
}
