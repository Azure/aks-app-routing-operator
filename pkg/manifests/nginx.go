// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	"path"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	autov1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

const (
	controllerImageTag = "v1.3.0"
	prom               = "prometheus"
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
	ControllerClass string         // controller class which is equivalent to controller field of IngressClass
	ResourceName    string         // name given to all resources
	IcName          string         // IngressClass name
	ServiceConfig   *ServiceConfig // service config that specifies details about the LB, defaults if nil
}

func (n *NginxIngressConfig) PodLabels() map[string]string {
	return map[string]string{"app": n.ResourceName}
}

// ServiceConfig defines configuration options for required resources for a Service that goes with an Ingress
type ServiceConfig struct {
	IsInternal bool
	Hostname   string
}

// NginxIngressClass returns an IngressClass for the provided configuration
func NginxIngressClass(conf *config.Config, self *appsv1.Deployment, ingressConfig *NginxIngressConfig) []client.Object {
	ing := &netv1.IngressClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IngressClass",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: ingressConfig.IcName},
		Spec: netv1.IngressClassSpec{
			Controller: ingressConfig.ControllerClass,
		},
	}
	objs := []client.Object{ing}

	owners := getOwnerRefs(self)
	for _, obj := range objs {
		obj.SetOwnerReferences(owners)
	}

	return objs
}

// NginxIngressControllerResources returns Kubernetes objects required for the controller
func NginxIngressControllerResources(conf *config.Config, self *appsv1.Deployment, ingressConfig *NginxIngressConfig) []client.Object {
	objs := []client.Object{}

	// Can safely assume the namespace exists if using kube-system
	if conf.NS != "kube-system" {
		objs = append(objs, namespace(conf))
	}

	objs = append(objs,
		newNginxIngressControllerServiceAccount(conf, ingressConfig),
		newNginxIngressControllerClusterRole(conf, ingressConfig),
		newNginxIngressControllerClusterRoleBinding(conf, ingressConfig),
		newNginxIngressControllerService(conf, ingressConfig),
		newNginxIngressControllerDeployment(conf, ingressConfig),
		newNginxIngressControllerConfigmap(conf, ingressConfig),
		newNginxIngressControllerPDB(conf, ingressConfig),
		newNginxIngressControllerHPA(conf, ingressConfig),
	)

	owners := getOwnerRefs(self)
	for _, obj := range objs {
		obj.SetOwnerReferences(owners)
	}

	return objs
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
			Labels:    addComponentLabel(topLevelLabels, "ingress-controller"),
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
			Labels: addComponentLabel(topLevelLabels, "ingress-controller"),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints", "pods", "services", "secrets", "configmaps"},
				Verbs:     []string{"get", "watch", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "events"},
				Verbs:     []string{"*"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses"},
				Verbs:     []string{"get", "watch", "list"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses/status"},
				Verbs:     []string{"*"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingressclasses"},
				Verbs:     []string{"list", "watch", "get"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"watch", "list"},
			},
			{
				// required as of v1.3.0 due to controller switch to lease api
				// https://github.com/kubernetes/ingress-nginx/releases/tag/controller-v1.3.0
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"*"},
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
			Labels: addComponentLabel(topLevelLabels, "ingress-controller"),
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

func newNginxIngressControllerService(conf *config.Config, ingressConfig *NginxIngressConfig) *corev1.Service {
	isInternal := false

	annotations := make(map[string]string)
	if isInternal {
		annotations["service.beta.kubernetes.io/azure-load-balancer-internal"] = "true"
	}

	for k, v := range promAnnotations {
		annotations[k] = v
	}

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingressConfig.ResourceName,
			Namespace:   conf.NS,
			Labels:      addComponentLabel(topLevelLabels, "ingress-controller"),
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
	ingressControllerComponentName := "ingress-controller"
	ingressControllerDeploymentLabels := addComponentLabel(topLevelLabels, ingressControllerComponentName)

	ingressControllerPodLabels := addComponentLabel(topLevelLabels, ingressControllerComponentName)
	for k, v := range ingressConfig.PodLabels() {
		ingressControllerPodLabels[k] = v
	}

	podAnnotations := map[string]string{}
	if !conf.DisableOSM {
		podAnnotations["openservicemesh.io/sidecar-injection"] = "enabled"
	}

	for k, v := range promAnnotations {
		podAnnotations[k] = v
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
			Selector:             &metav1.LabelSelector{MatchLabels: ingressConfig.PodLabels()},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      ingressControllerPodLabels,
					Annotations: podAnnotations,
				},
				Spec: *WithPreferSystemNodes(&corev1.PodSpec{
					ServiceAccountName: ingressConfig.ResourceName,
					Containers: []corev1.Container{*withPodRefEnvVars(withTypicalReadinessProbe(10254, &corev1.Container{
						Name:  "controller",
						Image: path.Join(conf.Registry, "/oss/kubernetes/ingress/nginx-ingress-controller:"+controllerImageTag),
						Args: []string{
							"/nginx-ingress-controller",
							"--ingress-class=" + ingressConfig.IcName,
							"--controller-class=" + ingressConfig.ControllerClass,
							"--election-id=" + ingressConfig.ResourceName,
							"--publish-service=$(POD_NAMESPACE)/" + ingressConfig.ResourceName,
							"--configmap=$(POD_NAMESPACE)/" + ingressConfig.ResourceName,
							"--http-port=8080",
							"--https-port=8443",
						},
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
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1500m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
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
			Labels:    addComponentLabel(topLevelLabels, "ingress-controller"),
		},
		Data: map[string]string{
			// Can't use 'allow-snippet-annotations=false' to reduce injection risk, since we require snippet functionality for OSM routing.
			// But we can still protect against leaked service account tokens.
			// See: https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/configmap/#annotation-value-word-blocklist
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
			Labels:    addComponentLabel(topLevelLabels, "ingress-controller"),
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
			Labels:    addComponentLabel(topLevelLabels, "ingress-controller"),
		},
		Spec: autov1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autov1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       ingressConfig.ResourceName,
			},
			MinReplicas:                    util.Int32Ptr(2),
			MaxReplicas:                    100,
			TargetCPUUtilizationPercentage: util.Int32Ptr(90),
		},
	}
}
