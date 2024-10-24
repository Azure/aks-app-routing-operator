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

// resourceType is a struct that represents a Kubernetes resource type
type resourceType struct {
	Group   string
	Version string
	// Name is the name of the resource type
	Name string
}

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
