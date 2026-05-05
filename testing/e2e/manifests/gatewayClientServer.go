package manifests

import (
	_ "embed"

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

	// AppRoutingIstioGatewayClassName is the GatewayClass name for App Routing meshless Istio gateways
	AppRoutingIstioGatewayClassName = "approuting-istio"

	// TLSCertKeyvaultURIOption is the TLS option key for specifying the KeyVault certificate URI
	TLSCertKeyvaultURIOption = "kubernetes.azure.com/tls-cert-keyvault-uri"

	// TLSCertServiceAccountOption is the TLS option key for specifying the ServiceAccount for workload identity
	TLSCertServiceAccountOption = "kubernetes.azure.com/tls-cert-service-account"

	// gatewayBackendPort is the port the test backend service exposes; the route forwards to it.
	gatewayBackendPort int32 = 8080
)

type ObjectsContainer interface {
	Objects() []client.Object
}

// GatewayClientServerResources contains the Kubernetes resources needed for Gateway API e2e testing.
//
// HTTPRoute is exposed as a typed field for backwards compatibility; new route kinds (GRPCRoute,
// TLSRoute, ...) will live behind the Route field, so suite code that wants to be route-kind
// agnostic should prefer Route() over HTTPRoute.
type GatewayClientServerResources struct {
	Client       *appsv1.Deployment
	Server       *appsv1.Deployment
	Service      *corev1.Service
	Gateway      *gatewayv1.Gateway
	RouteObject  client.Object
	AddedObjects []client.Object
}

// Route returns the route object (HTTPRoute / GRPCRoute / TLSRoute) for this resource set.
// Prefer this over the typed HTTPRoute field when writing route-kind-agnostic code.
func (g *GatewayClientServerResources) Route() client.Object {
	return g.RouteObject
}

// Objects returns all Kubernetes objects in this resource set
func (g *GatewayClientServerResources) Objects() []client.Object {
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
	if route := g.Route(); route != nil {
		ret = append(ret, route)
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

// gatewayClientServerArgs is the parameter bundle shared by GatewayClientAndServer and the
// route-kind-aware variant. Keeping it grouped here avoids the long positional argument list
// from creeping back as more kinds are added.
type gatewayClientServerArgs struct {
	Namespace          string
	Name               string
	Nameserver         string
	KeyvaultURI        string
	TLSHost            string
	ServiceAccountName string
	GatewayClassName   string
}

// GatewayClientAndServer creates the resources needed for Gateway API e2e testing with TLS,
// using HTTPRoute. Preserved for backwards compatibility — see GatewayClientAndServerFor for
// the route-kind-agnostic entrypoint.
func GatewayClientAndServer(namespace, name, nameserver, keyvaultURI, tlsHost, serviceAccountName, gatewayClassName string) GatewayClientServerResources {
	return GatewayClientAndServerFor(HTTPRouteKind{}, gatewayClientServerArgs{
		Namespace:          namespace,
		Name:               name,
		Nameserver:         nameserver,
		KeyvaultURI:        keyvaultURI,
		TLSHost:            tlsHost,
		ServiceAccountName: serviceAccountName,
		GatewayClassName:   gatewayClassName,
	})
}

// GatewayClientAndServerFor builds the gateway+route+client+server resource set for a given
// RouteKind. All callers in the suite go through this function (directly or via the wrappers).
func GatewayClientAndServerFor(kind RouteKind, args gatewayClientServerArgs) GatewayClientServerResources {
	name := nonAlphanumericRegex.ReplaceAllString(args.Name, "")

	// Gateway and listener names (needed for TLS secret name)
	gatewayName := name + "-gateway"
	listenerName := "https"

	// The SPC controller creates a secret with this name pattern
	tlsSecretName := "kv-gw-cert-" + gatewayName + "-" + listenerName

	clientDeployment := buildGatewayClient(args.Namespace, name+"-gw-client", args.TLSHost, args.Nameserver, "", tlsSecretName)

	// Create server deployment
	serverName := name + "-gw-server"
	serverDeployment := newGoDeployment(serverContents, args.Namespace, serverName)

	// Create service for the server
	serviceName := name + "-gw-service"
	service := buildBackendService(args.Namespace, serviceName, serverName)

	// Create Gateway with the kind-specific listener
	gateway := buildGateway(args.Namespace, gatewayName, args.GatewayClassName, kind.Listener(listenerName, args.TLSHost, args.KeyvaultURI, args.ServiceAccountName))

	// Create the route via the kind
	routeName := kind.RouteObjectName(name)
	route := kind.Route(args.Namespace, routeName, gatewayName, listenerName, args.TLSHost, serviceName, gatewayBackendPort)

	res := GatewayClientServerResources{
		Client:      clientDeployment,
		Server:      serverDeployment,
		Service:     service,
		Gateway:     gateway,
		RouteObject: route,
	}

	return res
}

// GatewayFilterTestResources contains resources for testing gateway/route label selectors
// It includes two gateways - one labeled (reachable) and one unlabeled (unreachable)
type GatewayFilterTestResources struct {
	Client            *appsv1.Deployment
	Server            *appsv1.Deployment
	Service           *corev1.Service
	LabeledGateway    *gatewayv1.Gateway
	UnlabeledGateway  *gatewayv1.Gateway
	LabeledRouteObj   client.Object
	UnlabeledRouteObj client.Object
	AddedObjects      []client.Object
}

// LabeledRouteObject returns the labeled route object (HTTPRoute / GRPCRoute / TLSRoute).
func (g *GatewayFilterTestResources) LabeledRouteObject() client.Object {
	if g.LabeledRouteObj != nil {
		return g.LabeledRouteObj
	}
	return nil
}

// UnlabeledRouteObject returns the unlabeled route object.
func (g *GatewayFilterTestResources) UnlabeledRouteObject() client.Object {
	if g.UnlabeledRouteObj != nil {
		return g.UnlabeledRouteObj
	}
	return nil
}

// Objects returns all Kubernetes objects in this resource set
func (g *GatewayFilterTestResources) Objects() []client.Object {
	ret := []client.Object{}

	if g.Server != nil {
		ret = append(ret, g.Server)
	}
	if g.Service != nil {
		ret = append(ret, g.Service)
	}
	if g.LabeledGateway != nil {
		ret = append(ret, g.LabeledGateway)
	}
	if g.UnlabeledGateway != nil {
		ret = append(ret, g.UnlabeledGateway)
	}
	if r := g.LabeledRouteObject(); r != nil {
		ret = append(ret, r)
	}
	if r := g.UnlabeledRouteObject(); r != nil {
		ret = append(ret, r)
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

// GatewayLabelFilterTestConfig contains configuration for gateway label filter tests
type GatewayLabelFilterTestConfig struct {
	Namespace          string
	Name               string
	Nameserver         string
	KeyvaultURI        string
	LabeledHost        string // Host for the labeled gateway (should be reachable)
	UnlabeledHost      string // Host for the unlabeled gateway (should be unreachable)
	ServiceAccountName string
	GatewayClassName   string
	FilterLabelKey     string
	FilterLabelValue   string
}

// GatewayLabelFilterResources creates resources for testing gateway label selectors using HTTPRoute.
func GatewayLabelFilterResources(cfg GatewayLabelFilterTestConfig) GatewayFilterTestResources {
	return GatewayLabelFilterResourcesFor(HTTPRouteKind{}, cfg)
}

// GatewayLabelFilterResourcesFor builds gateway label filter resources for a given RouteKind.
// Two gateways are created (labeled vs unlabeled), each with a route of the given kind. The
// labeled gateway carries the filter label so external-dns picks it up; the unlabeled one does
// not. Both routes are themselves un-labeled — this exercises the *gateway* label selector path.
func GatewayLabelFilterResourcesFor(kind RouteKind, cfg GatewayLabelFilterTestConfig) GatewayFilterTestResources {
	name := nonAlphanumericRegex.ReplaceAllString(cfg.Name, "")

	labeledGatewayName := name + "-labeled-gw"
	listenerName := "https"
	tlsSecretName := "kv-gw-cert-" + labeledGatewayName + "-" + listenerName

	clientDeployment := buildGatewayClient(cfg.Namespace, name+"-filter-client", cfg.LabeledHost, cfg.Nameserver, cfg.UnlabeledHost, tlsSecretName)

	serverName := name + "-filter-server"
	serverDeployment := newGoDeployment(serverContents, cfg.Namespace, serverName)

	serviceName := name + "-filter-service"
	service := buildBackendService(cfg.Namespace, serviceName, serverName)

	// Labeled gateway gets the filter label; unlabeled gateway does not.
	labeledGateway := buildGateway(cfg.Namespace, labeledGatewayName, cfg.GatewayClassName, kind.Listener(listenerName, cfg.LabeledHost, cfg.KeyvaultURI, cfg.ServiceAccountName))
	labeledGateway.Labels[cfg.FilterLabelKey] = cfg.FilterLabelValue

	unlabeledGatewayName := name + "-unlabeled-gw"
	unlabeledGateway := buildGateway(cfg.Namespace, unlabeledGatewayName, cfg.GatewayClassName, kind.Listener(listenerName, cfg.UnlabeledHost, cfg.KeyvaultURI, cfg.ServiceAccountName))

	labeledRouteObj := kind.Route(cfg.Namespace, name+"-labeled-route", labeledGatewayName, listenerName, cfg.LabeledHost, serviceName, gatewayBackendPort)
	unlabeledRouteObj := kind.Route(cfg.Namespace, name+"-unlabeled-route", unlabeledGatewayName, listenerName, cfg.UnlabeledHost, serviceName, gatewayBackendPort)

	res := GatewayFilterTestResources{
		Client:            clientDeployment,
		Server:            serverDeployment,
		Service:           service,
		LabeledGateway:    labeledGateway,
		UnlabeledGateway:  unlabeledGateway,
		LabeledRouteObj:   labeledRouteObj,
		UnlabeledRouteObj: unlabeledRouteObj,
	}
	return res
}

// RouteLabelFilterResources creates resources for testing route label selectors using HTTPRoute.
// Preserved for backwards compatibility — see RouteLabelFilterResourcesFor for the route-kind-agnostic
// entrypoint.
func RouteLabelFilterResources(cfg GatewayLabelFilterTestConfig) GatewayFilterTestResources {
	return RouteLabelFilterResourcesFor(HTTPRouteKind{}, cfg)
}

// RouteLabelFilterResourcesFor builds route label filter resources for a given RouteKind.
// Two gateways are created (both un-labeled), each with a route of the given kind. The labeled
// route carries the filter label so external-dns picks it up; the unlabeled route does not.
// This exercises the *route* label selector path.
func RouteLabelFilterResourcesFor(kind RouteKind, cfg GatewayLabelFilterTestConfig) GatewayFilterTestResources {
	name := nonAlphanumericRegex.ReplaceAllString(cfg.Name, "")

	labeledGatewayName := name + "-labeled-route-gw"
	listenerName := "https"
	tlsSecretName := "kv-gw-cert-" + labeledGatewayName + "-" + listenerName

	clientDeployment := buildGatewayClient(cfg.Namespace, name+"-route-filter-client", cfg.LabeledHost, cfg.Nameserver, cfg.UnlabeledHost, tlsSecretName)

	serverName := name + "-route-filter-server"
	serverDeployment := newGoDeployment(serverContents, cfg.Namespace, serverName)

	serviceName := name + "-route-filter-service"
	service := buildBackendService(cfg.Namespace, serviceName, serverName)

	// Both gateways are un-labeled here; the *routes* carry the filter label.
	labeledGateway := buildGateway(cfg.Namespace, labeledGatewayName, cfg.GatewayClassName, kind.Listener(listenerName, cfg.LabeledHost, cfg.KeyvaultURI, cfg.ServiceAccountName))

	unlabeledGatewayName := name + "-unlabeled-route-gw"
	unlabeledGateway := buildGateway(cfg.Namespace, unlabeledGatewayName, cfg.GatewayClassName, kind.Listener(listenerName, cfg.UnlabeledHost, cfg.KeyvaultURI, cfg.ServiceAccountName))

	labeledRouteObj := kind.Route(cfg.Namespace, name+"-labeled-httproute", labeledGatewayName, listenerName, cfg.LabeledHost, serviceName, gatewayBackendPort)
	unlabeledRouteObj := kind.Route(cfg.Namespace, name+"-unlabeled-httproute", unlabeledGatewayName, listenerName, cfg.UnlabeledHost, serviceName, gatewayBackendPort)

	// Apply the filter label to the labeled route's metadata. We do this generically via
	// metav1.Object so the same code works for HTTPRoute, GRPCRoute, etc.
	if metaObj, ok := labeledRouteObj.(metav1.Object); ok {
		labels := metaObj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[cfg.FilterLabelKey] = cfg.FilterLabelValue
		metaObj.SetLabels(labels)
	}

	res := GatewayFilterTestResources{
		Client:            clientDeployment,
		Server:            serverDeployment,
		Service:           service,
		LabeledGateway:    labeledGateway,
		UnlabeledGateway:  unlabeledGateway,
		LabeledRouteObj:   labeledRouteObj,
		UnlabeledRouteObj: unlabeledRouteObj,
	}

	return res
}

// buildGatewayClient creates the gateway-test client Deployment. The client is a small Go HTTP
// server on :8080 whose readiness probe is the implicit DNS+TLS+routing assertion: the probe
// only goes Ready when the configured URL responds successfully (which requires DNS resolution,
// TLS validation against the mounted KV cert, and end-to-end routing through the gateway).
//
// If unreachableURL is non-empty, the client also asserts that URL is *not* reachable (used by
// filter tests). The cert mounted at /etc/ssl/certs/ca-certificates.crt is the cert SPC writes
// for the labeled/primary gateway listener.
func buildGatewayClient(namespace, name, url, nameserver, unreachableURL, tlsSecretName string) *appsv1.Deployment {
	deployment := newGoDeployment(gatewayClientContents, namespace, name)
	env := []corev1.EnvVar{
		{Name: "URL", Value: "https://" + url},
	}
	if unreachableURL != "" {
		env = append(env, corev1.EnvVar{Name: "UNREACHABLE_URL", Value: "https://" + unreachableURL})
	}
	env = append(env,
		corev1.EnvVar{Name: "NAMESERVER", Value: nameserver},
		corev1.EnvVar{
			Name:      "POD_IP",
			ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}},
		},
	)
	deployment.Spec.Template.Spec.Containers[0].Env = env
	deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "tls-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: tlsSecretName},
			},
		},
	}
	deployment.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "tls-certs",
			MountPath: "/etc/ssl/certs/ca-certificates.crt",
			SubPath:   "tls.crt",
			ReadOnly:  true,
		},
	}
	deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
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
	return deployment
}

// buildBackendService creates the backend Service the gateway routes traffic to. Port name is
// "http" today; gRPC/TLS variants will introduce their own service builders if they need a
// different port name (Istio uses appProtocol/port-name to pick the right L7 filter chain).
func buildBackendService(namespace, serviceName, serverDeploymentName string) *corev1.Service {
	return &corev1.Service{
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
				Port:       gatewayBackendPort,
				TargetPort: intstr.FromInt(int(gatewayBackendPort)),
			}},
			Selector: map[string]string{
				"app": serverDeploymentName,
			},
		},
	}
}

// buildGateway creates a Gateway with a single listener provided by the caller (typically from a
// RouteKind.Listener call so the protocol/TLS mode varies per kind).
func buildGateway(namespace, name, gatewayClassName string, listener gatewayv1.Listener) *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(gatewayClassName),
			Listeners:        []gatewayv1.Listener{listener},
		},
	}
}
