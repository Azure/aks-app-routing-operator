// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	"path"
	"strconv"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	autov1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	controllerImageTag             = "v1.10.0"
	prom                           = "prometheus"
	IngressControllerComponentName = "ingress-controller"
)

var (
	// NginxResourceTypes is a list of resource types used to deploy the Nginx Ingress Controller
	NginxResourceTypes = []resourceType{
		{
			Group:   netv1.GroupName,
			Version: netv1.SchemeGroupVersion.Version,
			Name:    "IngressClass",
		},
		{
			Group:   corev1.GroupName,
			Version: corev1.SchemeGroupVersion.Version,
			Name:    "ServiceAccount",
		},
		{
			Group:   rbacv1.GroupName,
			Version: rbacv1.SchemeGroupVersion.Version,
			Name:    "ClusterRole",
		},
		{
			Group:   rbacv1.GroupName,
			Version: rbacv1.SchemeGroupVersion.Version,
			Name:    "Role",
		},
		{
			Group:   rbacv1.GroupName,
			Version: rbacv1.SchemeGroupVersion.Version,
			Name:    "ClusterRoleBinding",
		},
		{
			Group:   rbacv1.GroupName,
			Version: rbacv1.SchemeGroupVersion.Version,
			Name:    "RoleBinding",
		},
		{
			Group:   corev1.GroupName,
			Version: corev1.SchemeGroupVersion.Version,
			Name:    "Service",
		},
		{
			Group:   appsv1.GroupName,
			Version: appsv1.SchemeGroupVersion.Version,
			Name:    "Deployment",
		},
		{
			Group:   corev1.GroupName,
			Version: corev1.SchemeGroupVersion.Version,
			Name:    "ConfigMap",
		},
		{
			Group:   policyv1.GroupName,
			Version: policyv1.SchemeGroupVersion.Version,
			Name:    "PodDisruptionBudget",
		},
		{
			Group:   autov1.GroupName,
			Version: autov1.SchemeGroupVersion.Version,
			Name:    "HorizontalPodAutoscaler",
		},
	}
)

var nginxLabels = util.MergeMaps(
	map[string]string{
		k8sNameKey: "nginx",
	},
	GetTopLevelLabels(),
)

var (
	promServicePort = corev1.ServicePort{
		Name:       prom,
		Port:       10254,
		TargetPort: intstr.FromString(prom),
	}
	promPodPort = corev1.ContainerPort{
		Name:          prom,
		ContainerPort: promServicePort.Port,
	}
	promAnnotations = map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/port":   strconv.Itoa(int(promServicePort.Port)),
	}
)

// NginxIngressConfig defines configuration options for required resources for an Ingress
type NginxIngressConfig struct {
	ControllerClass       string         // controller class which is equivalent to controller field of IngressClass
	ResourceName          string         // name given to all resources
	IcName                string         // IngressClass name
	ServiceConfig         *ServiceConfig // service config that specifies details about the LB, defaults if nil
	DefaultSSLCertificate string         // namespace/name used to create SSL certificate for the default HTTPS server (catch-all)
	MinReplicas           int32
	MaxReplicas           int32
	// TargetCPUUtilizationPercentage is the target average CPU utilization of the Ingress Controller
	TargetCPUUtilizationPercentage int32
}

func (n *NginxIngressConfig) PodLabels() map[string]string {
	return map[string]string{"app": n.ResourceName}
}

// ServiceConfig defines configuration options for required resources for a Service that goes with an Ingress
type ServiceConfig struct {
	Annotations map[string]string
}

func GetNginxResources(conf *config.Config, ingressConfig *NginxIngressConfig) *NginxResources {
	res := &NginxResources{
		IngressClass:            newNginxIngressControllerIngressClass(conf, ingressConfig),
		ServiceAccount:          newNginxIngressControllerServiceAccount(conf, ingressConfig),
		ClusterRole:             newNginxIngressControllerClusterRole(conf, ingressConfig),
		Role:                    newNginxIngressControllerRole(conf, ingressConfig),
		ClusterRoleBinding:      newNginxIngressControllerClusterRoleBinding(conf, ingressConfig),
		RoleBinding:             newNginxIngressControllerRoleBinding(conf, ingressConfig),
		Service:                 newNginxIngressControllerService(conf, ingressConfig),
		Deployment:              newNginxIngressControllerDeployment(conf, ingressConfig),
		ConfigMap:               newNginxIngressControllerConfigmap(conf, ingressConfig),
		HorizontalPodAutoscaler: newNginxIngressControllerHPA(conf, ingressConfig),
		PodDisruptionBudget:     newNginxIngressControllerPDB(conf, ingressConfig),
	}

	for _, obj := range res.Objects() {
		l := util.MergeMaps(obj.GetLabels(), nginxLabels)
		obj.SetLabels(l)
	}

	// Can safely assume the namespace exists if using kube-system.
	// Purposefully do this after applying the labels, namespace isn't an Nginx-specific resource
	if conf.NS != "kube-system" {
		res.Namespace = Namespace(conf)
	}

	return res
}

func newNginxIngressControllerIngressClass(conf *config.Config, ingressConfig *NginxIngressConfig) *netv1.IngressClass {
	return &netv1.IngressClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IngressClass",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: ingressConfig.IcName, Labels: GetTopLevelLabels()},
		Spec: netv1.IngressClassSpec{
			Controller: ingressConfig.ControllerClass,
		},
	}
}

func newNginxIngressControllerServiceAccount(conf *config.Config, ingressConfig *NginxIngressConfig) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressConfig.ResourceName,
			Namespace: conf.NS,
			Labels:    AddComponentLabel(GetTopLevelLabels(), "ingress-controller"),
		},
	}
}

func newNginxIngressControllerClusterRole(conf *config.Config, ingressConfig *NginxIngressConfig) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   ingressConfig.ResourceName,
			Labels: AddComponentLabel(GetTopLevelLabels(), "ingress-controller"),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "endpoints", "nodes", "pods", "secrets", "namespaces"},
				Verbs:     []string{"list", "watch"},
			},
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses"},
				Verbs:     []string{"get", "watch", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses/status"},
				Verbs:     []string{"update"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingressclasses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"discovery.k8s.io"},
				Resources: []string{"endpointslices"},
				Verbs:     []string{"list", "watch", "get"},
			},
		},
	}
}

func newNginxIngressControllerRole(conf *config.Config, ingressConfig *NginxIngressConfig) *rbacv1.Role {
	return &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressConfig.ResourceName,
			Labels:    AddComponentLabel(GetTopLevelLabels(), "ingress-controller"),
			Namespace: conf.NS,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
				Verbs:     []string{"get"},
			},
			// temporary permission used for update from 1.3.0->1.8.1
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"update"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "pods", "secrets", "endpoints"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses/status"},
				Verbs:     []string{"update"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingressclasses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups:     []string{"coordination.k8s.io"},
				Resources:     []string{"leases"},
				ResourceNames: []string{ingressConfig.ResourceName},
				Verbs:         []string{"get", "update"},
			},
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch"},
			},
			{
				APIGroups: []string{"discovery.k8s.io"},
				Resources: []string{"endpointslices"},
				Verbs:     []string{"list", "watch", "get"},
			},
		},
	}
}

func newNginxIngressControllerClusterRoleBinding(conf *config.Config, ingressConfig *NginxIngressConfig) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   ingressConfig.ResourceName,
			Labels: AddComponentLabel(GetTopLevelLabels(), "ingress-controller"),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     ingressConfig.ResourceName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      ingressConfig.ResourceName,
			Namespace: conf.NS,
		}},
	}
}

func newNginxIngressControllerRoleBinding(conf *config.Config, ingressConfig *NginxIngressConfig) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressConfig.ResourceName,
			Namespace: conf.NS,
			Labels:    AddComponentLabel(GetTopLevelLabels(), "ingress-controller"),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     ingressConfig.ResourceName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      ingressConfig.ResourceName,
			Namespace: conf.NS,
		}},
	}
}

func newNginxIngressControllerService(conf *config.Config, ingressConfig *NginxIngressConfig) *corev1.Service {
	annotations := make(map[string]string)
	for k, v := range promAnnotations {
		annotations[k] = v
	}

	if ingressConfig != nil && ingressConfig.ServiceConfig != nil {
		for k, v := range ingressConfig.ServiceConfig.Annotations {
			annotations[k] = v
		}
	}

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingressConfig.ResourceName,
			Namespace:   conf.NS,
			Labels:      AddComponentLabel(GetTopLevelLabels(), "ingress-controller"),
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
			Type:                  corev1.ServiceTypeLoadBalancer,
			Selector:              ingressConfig.PodLabels(),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http"),
				},
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromString("https"),
				},
				promServicePort,
			},
		},
	}
}

func newNginxIngressControllerDeployment(conf *config.Config, ingressConfig *NginxIngressConfig) *appsv1.Deployment {
	ingressControllerDeploymentLabels := AddComponentLabel(GetTopLevelLabels(), IngressControllerComponentName)

	ingressControllerPodLabels := AddComponentLabel(GetTopLevelLabels(), IngressControllerComponentName)
	for k, v := range ingressConfig.PodLabels() {
		ingressControllerPodLabels[k] = v
	}

	podAnnotations := map[string]string{}
	if !conf.DisableOSM {
		podAnnotations["openservicemesh.io/sidecar-injection"] = "disabled"
	}

	for k, v := range promAnnotations {
		podAnnotations[k] = v
	}

	selector := &metav1.LabelSelector{MatchLabels: ingressConfig.PodLabels()}

	deploymentArgs := []string{
		"/nginx-ingress-controller",
		"--ingress-class=" + ingressConfig.IcName,
		"--controller-class=" + ingressConfig.ControllerClass,
		"--election-id=" + ingressConfig.ResourceName,
		"--publish-service=$(POD_NAMESPACE)/" + ingressConfig.ResourceName,
		"--configmap=$(POD_NAMESPACE)/" + ingressConfig.ResourceName,
		"--enable-annotation-validation=true",
		"--http-port=8080",
		"--https-port=8443",
	}

	if ingressConfig.DefaultSSLCertificate != "" {
		deploymentArgs = append(deploymentArgs, "--default-ssl-certificate="+ingressConfig.DefaultSSLCertificate)
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressConfig.ResourceName,
			Namespace: conf.NS,
			Labels:    ingressControllerDeploymentLabels,
		},
		Spec: appsv1.DeploymentSpec{
			RevisionHistoryLimit: util.Int32Ptr(2),
			Selector:             selector,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      ingressControllerPodLabels,
					Annotations: podAnnotations,
				},
				Spec: *WithPreferSystemNodes(&corev1.PodSpec{
					TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
						{
							MaxSkew:           1,
							TopologyKey:       "kubernetes.io/hostname", // spread across nodes
							WhenUnsatisfiable: corev1.ScheduleAnyway,
							LabelSelector:     selector,
						},
					},
					ServiceAccountName: ingressConfig.ResourceName,
					Containers: []corev1.Container{*withPodRefEnvVars(withTypicalReadinessProbe(10254, &corev1.Container{
						Name:  "controller",
						Image: path.Join(conf.Registry, "/oss/kubernetes/ingress/nginx-ingress-controller:"+controllerImageTag),
						Args:  deploymentArgs,
						SecurityContext: &corev1.SecurityContext{
							RunAsUser: util.Int64Ptr(101),
						},
						Ports: []corev1.ContainerPort{
							{
								Name:          "http",
								ContainerPort: 8080,
							},
							{
								Name:          "https",
								ContainerPort: 8443,
							},
							promPodPort,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("127Mi"),
							},
						},
					}))},
				}),
			},
		},
	}
}

func newNginxIngressControllerConfigmap(conf *config.Config, ingressConfig *NginxIngressConfig) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressConfig.ResourceName,
			Namespace: conf.NS,
			Labels:    AddComponentLabel(GetTopLevelLabels(), "ingress-controller"),
		},
		Data: map[string]string{
			// Can't use 'allow-snippet-annotations=false' to reduce injection risk, since we require snippet functionality for OSM routing.
			// But we can still protect against leaked service account tokens.
			// See: https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/configmap/#annotation-value-word-blocklist
			"allow-snippet-annotations":       "true",
			"annotation-value-word-blocklist": "load_module,lua_package,_by_lua,location,root,proxy_pass,serviceaccount,{,},'",
		},
	}
}

func newNginxIngressControllerPDB(conf *config.Config, ingressConfig *NginxIngressConfig) *policyv1.PodDisruptionBudget {
	maxUnavailable := intstr.FromInt(1)
	return &policyv1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodDisruptionBudget",
			APIVersion: "policy/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressConfig.ResourceName,
			Namespace: conf.NS,
			Labels:    AddComponentLabel(GetTopLevelLabels(), "ingress-controller"),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector:       &metav1.LabelSelector{MatchLabels: ingressConfig.PodLabels()},
			MaxUnavailable: &maxUnavailable,
		},
	}
}

func newNginxIngressControllerHPA(conf *config.Config, ingressConfig *NginxIngressConfig) *autov1.HorizontalPodAutoscaler {
	return &autov1.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HorizontalPodAutoscaler",
			APIVersion: "autoscaling/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressConfig.ResourceName,
			Namespace: conf.NS,
			Labels:    AddComponentLabel(GetTopLevelLabels(), "ingress-controller"),
		},
		Spec: autov1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autov1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       ingressConfig.ResourceName,
			},
			MinReplicas:                    util.Int32Ptr(ingressConfig.MinReplicas),
			MaxReplicas:                    ingressConfig.MaxReplicas,
			TargetCPUUtilizationPercentage: &ingressConfig.TargetCPUUtilizationPercentage,
		},
	}
}
