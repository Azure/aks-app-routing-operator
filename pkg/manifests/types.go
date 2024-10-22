package manifests

import (
	appsv1 "k8s.io/api/apps/v1"
	autov1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NginxResources is a struct that represents the Kubernetes resources that are created for the Nginx Ingress Controller. When these resources
// are acted upon by client-go, the fields here are updated since they are pointers to the actual resources.
type NginxResources struct {
	Namespace               *corev1.Namespace
	IngressClass            *netv1.IngressClass
	ServiceAccount          *corev1.ServiceAccount
	ClusterRole             *rbacv1.ClusterRole
	Role                    *rbacv1.Role
	ClusterRoleBinding      *rbacv1.ClusterRoleBinding
	RoleBinding             *rbacv1.RoleBinding
	Service                 *corev1.Service
	PromService             *corev1.Service
	Deployment              *appsv1.Deployment
	ConfigMap               *corev1.ConfigMap
	HorizontalPodAutoscaler *autov1.HorizontalPodAutoscaler
	PodDisruptionBudget     *policyv1.PodDisruptionBudget
}

func (n *NginxResources) Objects() []client.Object {
	objs := []client.Object{
		n.IngressClass,
		n.ServiceAccount,
		n.ClusterRole,
		n.Role,
		n.ClusterRoleBinding,
		n.RoleBinding,
		n.Service,
		n.PromService,
		n.Deployment,
		n.ConfigMap,
		n.HorizontalPodAutoscaler,
		n.PodDisruptionBudget,
	}

	if n.Namespace != nil {
		objs = append([]client.Object{n.Namespace}, objs...) // put namespace at front, so we can create resources in order
	}

	return objs
}

// NginxIngressConfig defines configuration options for required resources for an Ingress
type NginxIngressConfig struct {
	Version               *NginxIngressVersion
	ControllerClass       string         // controller class which is equivalent to controller field of IngressClass
	ResourceName          string         // name given to all resources
	IcName                string         // IngressClass name
	ServiceConfig         *ServiceConfig // service config that specifies details about the LB, defaults if nil
	ForceSSLRedirect      bool           // flag to sets all redirects to HTTPS if there is a default TLS certificate (requires DefaultSSLCertificate)
	DefaultSSLCertificate string         // namespace/name used to create SSL certificate for the default HTTPS server (catch-all)
	DefaultBackendService string         // namespace/name used to determine default backend service for / and /healthz endpoints
	CustomHTTPErrors      string         // error codes passed to the configmap to configure nginx to send traffic with the specified headers to its defaultbackend service in case of error
	MinReplicas           int32
	MaxReplicas           int32
	// TargetCPUUtilizationPercentage is the target average CPU utilization of the Ingress Controller
	TargetCPUUtilizationPercentage int32
}

func (n *NginxIngressConfig) PodLabels() map[string]string {
	return map[string]string{"app": n.ResourceName}
}

type NginxIngressVersion struct {
	name, tag string
}

// ServiceConfig defines configuration options for required resources for a Service that goes with an Ingress
type ServiceConfig struct {
	Annotations              map[string]string
	LoadBalancerSourceRanges []string
}
