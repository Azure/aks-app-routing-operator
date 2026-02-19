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

type ObjectsContainer interface {
	Objects() []client.Object
}

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
//   - tlsHost: hostname for DNS records + Gateway listeners
//   - serviceAccountName: name of the ServiceAccount for workload identity (must be created separately)
//   - gatewayClassName: the GatewayClass name to use (e.g., "istio")
func GatewayClientAndServer(namespace, name, nameserver, keyvaultURI, tlsHost, serviceAccountName, gatewayClassName string) GatewayClientServerResources {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")

	// Gateway and listener names (needed for TLS secret name)
	gatewayName := name + "-gateway"
	listenerName := "https"

	// The SPC controller creates a secret with this name pattern
	tlsSecretName := "kv-gw-cert-" + gatewayName + "-" + listenerName

	// Create client deployment using gateway-specific client (doesn't validate X-Forwarded-For)
	clientDeployment := newGoDeployment(gatewayClientContents, namespace, name+"-gw-client")
	clientDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{
			Name:  "URL",
			Value: "https://" + tlsHost,
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
	// Mount the TLS certificate secret as a CA certificate
	clientDeployment.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "tls-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: tlsSecretName,
				},
			},
		},
	}
	clientDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "tls-certs",
			MountPath: "/etc/ssl/certs/ca-certificates.crt",
			SubPath:   "tls.crt",
			ReadOnly:  true,
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

// GatewayFilterTestResources contains resources for testing gateway/route label selectors
// It includes two gateways - one labeled (reachable) and one unlabeled (unreachable)
type GatewayFilterTestResources struct {
	Client           *appsv1.Deployment
	Server           *appsv1.Deployment
	Service          *corev1.Service
	LabeledGateway   *gatewayv1.Gateway
	UnlabeledGateway *gatewayv1.Gateway
	LabeledRoute     *gatewayv1.HTTPRoute
	UnlabeledRoute   *gatewayv1.HTTPRoute
	AddedObjects     []client.Object
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
	if g.LabeledRoute != nil {
		ret = append(ret, g.LabeledRoute)
	}
	if g.UnlabeledRoute != nil {
		ret = append(ret, g.UnlabeledRoute)
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

// GatewayLabelFilterResources creates resources for testing gateway label selectors
// Creates two gateways: one with the filter label (reachable) and one without (unreachable)
func GatewayLabelFilterResources(cfg GatewayLabelFilterTestConfig) GatewayFilterTestResources {
	name := nonAlphanumericRegex.ReplaceAllString(cfg.Name, "")

	// Gateway and listener names (needed for TLS secret name)
	labeledGatewayName := name + "-labeled-gw"
	labeledListenerName := "https"

	// The SPC controller creates a secret with this name pattern for the labeled gateway
	tlsSecretName := "kv-gw-cert-" + labeledGatewayName + "-" + labeledListenerName

	// Create client deployment that connects to labeled host and verifies unlabeled is unreachable
	clientDeployment := newGoDeployment(gatewayClientContents, cfg.Namespace, name+"-filter-client")
	clientDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{
			Name:  "URL",
			Value: "https://" + cfg.LabeledHost,
		},
		{
			Name:  "UNREACHABLE_URL",
			Value: "https://" + cfg.UnlabeledHost,
		},
		{
			Name:  "NAMESERVER",
			Value: cfg.Nameserver,
		},
		{
			Name:      "POD_IP",
			ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}},
		},
	}
	// Mount the TLS certificate secret as a CA certificate
	clientDeployment.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "tls-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: tlsSecretName,
				},
			},
		},
	}
	clientDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "tls-certs",
			MountPath: "/etc/ssl/certs/ca-certificates.crt",
			SubPath:   "tls.crt",
			ReadOnly:  true,
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

	// Create server deployment (shared by both gateways)
	serverName := name + "-filter-server"
	serverDeployment := newGoDeployment(serverContents, cfg.Namespace, serverName)

	// Create service for the server
	serviceName := name + "-filter-service"
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: cfg.Namespace,
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

	// Create labeled gateway (should be picked up by external-dns)
	labeledHostname := gatewayv1.Hostname(cfg.LabeledHost)
	labeledGateway := &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      labeledGatewayName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				ManagedByKey:       ManagedByVal,
				cfg.FilterLabelKey: cfg.FilterLabelValue,
			},
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(cfg.GatewayClassName),
			Listeners: []gatewayv1.Listener{
				{
					Name:     gatewayv1.SectionName(labeledListenerName),
					Hostname: &labeledHostname,
					Port:     gatewayv1.PortNumber(443),
					Protocol: gatewayv1.HTTPSProtocolType,
					TLS: &gatewayv1.GatewayTLSConfig{
						Mode: to.Ptr(gatewayv1.TLSModeTerminate),
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							TLSCertKeyvaultURIOption:    gatewayv1.AnnotationValue(cfg.KeyvaultURI),
							TLSCertServiceAccountOption: gatewayv1.AnnotationValue(cfg.ServiceAccountName),
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

	// Create unlabeled gateway (should NOT be picked up by external-dns)
	unlabeledGatewayName := name + "-unlabeled-gw"
	unlabeledListenerName := "https"
	unlabeledHostname := gatewayv1.Hostname(cfg.UnlabeledHost)
	unlabeledGateway := &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      unlabeledGatewayName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
				// No filter label - this gateway should be ignored by external-dns
			},
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(cfg.GatewayClassName),
			Listeners: []gatewayv1.Listener{
				{
					Name:     gatewayv1.SectionName(unlabeledListenerName),
					Hostname: &unlabeledHostname,
					Port:     gatewayv1.PortNumber(443),
					Protocol: gatewayv1.HTTPSProtocolType,
					TLS: &gatewayv1.GatewayTLSConfig{
						Mode: to.Ptr(gatewayv1.TLSModeTerminate),
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							TLSCertKeyvaultURIOption:    gatewayv1.AnnotationValue(cfg.KeyvaultURI),
							TLSCertServiceAccountOption: gatewayv1.AnnotationValue(cfg.ServiceAccountName),
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

	// Create HTTPRoute for labeled gateway
	labeledRouteName := name + "-labeled-route"
	labeledRoute := &gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      labeledRouteName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:        gatewayv1.ObjectName(labeledGatewayName),
						SectionName: to.Ptr(gatewayv1.SectionName(labeledListenerName)),
					},
				},
			},
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(cfg.LabeledHost)},
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

	// Create HTTPRoute for unlabeled gateway
	unlabeledRouteName := name + "-unlabeled-route"
	unlabeledRoute := &gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      unlabeledRouteName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:        gatewayv1.ObjectName(unlabeledGatewayName),
						SectionName: to.Ptr(gatewayv1.SectionName(unlabeledListenerName)),
					},
				},
			},
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(cfg.UnlabeledHost)},
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

	return GatewayFilterTestResources{
		Client:           clientDeployment,
		Server:           serverDeployment,
		Service:          service,
		LabeledGateway:   labeledGateway,
		UnlabeledGateway: unlabeledGateway,
		LabeledRoute:     labeledRoute,
		UnlabeledRoute:   unlabeledRoute,
	}
}

// RouteLabelFilterResources creates resources for testing route label selectors
// Creates two gateways with routes: one route with the filter label (reachable) and one without (unreachable)
func RouteLabelFilterResources(cfg GatewayLabelFilterTestConfig) GatewayFilterTestResources {
	name := nonAlphanumericRegex.ReplaceAllString(cfg.Name, "")

	// Gateway and listener names (needed for TLS secret name)
	labeledGatewayName := name + "-labeled-route-gw"
	labeledListenerName := "https"

	// The SPC controller creates a secret with this name pattern for the labeled gateway
	tlsSecretName := "kv-gw-cert-" + labeledGatewayName + "-" + labeledListenerName

	// Create client deployment that connects to labeled route's host and verifies unlabeled is unreachable
	clientDeployment := newGoDeployment(gatewayClientContents, cfg.Namespace, name+"-route-filter-client")
	clientDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{
			Name:  "URL",
			Value: "https://" + cfg.LabeledHost,
		},
		{
			Name:  "UNREACHABLE_URL",
			Value: "https://" + cfg.UnlabeledHost,
		},
		{
			Name:  "NAMESERVER",
			Value: cfg.Nameserver,
		},
		{
			Name:      "POD_IP",
			ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}},
		},
	}
	// Mount the TLS certificate secret as a CA certificate
	clientDeployment.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "tls-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: tlsSecretName,
				},
			},
		},
	}
	clientDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "tls-certs",
			MountPath: "/etc/ssl/certs/ca-certificates.crt",
			SubPath:   "tls.crt",
			ReadOnly:  true,
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

	// Create server deployment (shared by both gateways)
	serverName := name + "-route-filter-server"
	serverDeployment := newGoDeployment(serverContents, cfg.Namespace, serverName)

	// Create service for the server
	serviceName := name + "-route-filter-service"
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: cfg.Namespace,
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

	// Create gateway for labeled route
	labeledHostname := gatewayv1.Hostname(cfg.LabeledHost)
	labeledGateway := &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      labeledGatewayName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(cfg.GatewayClassName),
			Listeners: []gatewayv1.Listener{
				{
					Name:     gatewayv1.SectionName(labeledListenerName),
					Hostname: &labeledHostname,
					Port:     gatewayv1.PortNumber(443),
					Protocol: gatewayv1.HTTPSProtocolType,
					TLS: &gatewayv1.GatewayTLSConfig{
						Mode: to.Ptr(gatewayv1.TLSModeTerminate),
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							TLSCertKeyvaultURIOption:    gatewayv1.AnnotationValue(cfg.KeyvaultURI),
							TLSCertServiceAccountOption: gatewayv1.AnnotationValue(cfg.ServiceAccountName),
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

	// Create gateway for unlabeled route
	unlabeledGatewayName := name + "-unlabeled-route-gw"
	unlabeledListenerName := "https"
	unlabeledHostname := gatewayv1.Hostname(cfg.UnlabeledHost)
	unlabeledGateway := &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      unlabeledGatewayName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(cfg.GatewayClassName),
			Listeners: []gatewayv1.Listener{
				{
					Name:     gatewayv1.SectionName(unlabeledListenerName),
					Hostname: &unlabeledHostname,
					Port:     gatewayv1.PortNumber(443),
					Protocol: gatewayv1.HTTPSProtocolType,
					TLS: &gatewayv1.GatewayTLSConfig{
						Mode: to.Ptr(gatewayv1.TLSModeTerminate),
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							TLSCertKeyvaultURIOption:    gatewayv1.AnnotationValue(cfg.KeyvaultURI),
							TLSCertServiceAccountOption: gatewayv1.AnnotationValue(cfg.ServiceAccountName),
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

	// Create labeled HTTPRoute (should be picked up by external-dns)
	labeledRouteName := name + "-labeled-httproute"
	labeledRoute := &gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      labeledRouteName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				ManagedByKey:       ManagedByVal,
				cfg.FilterLabelKey: cfg.FilterLabelValue,
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:        gatewayv1.ObjectName(labeledGatewayName),
						SectionName: to.Ptr(gatewayv1.SectionName(labeledListenerName)),
					},
				},
			},
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(cfg.LabeledHost)},
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

	// Create unlabeled HTTPRoute (should NOT be picked up by external-dns)
	unlabeledRouteName := name + "-unlabeled-httproute"
	unlabeledRoute := &gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      unlabeledRouteName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
				// No filter label - this route should be ignored by external-dns
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:        gatewayv1.ObjectName(unlabeledGatewayName),
						SectionName: to.Ptr(gatewayv1.SectionName(unlabeledListenerName)),
					},
				},
			},
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(cfg.UnlabeledHost)},
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

	return GatewayFilterTestResources{
		Client:           clientDeployment,
		Server:           serverDeployment,
		Service:          service,
		LabeledGateway:   labeledGateway,
		UnlabeledGateway: unlabeledGateway,
		LabeledRoute:     labeledRoute,
		UnlabeledRoute:   unlabeledRoute,
	}
}
