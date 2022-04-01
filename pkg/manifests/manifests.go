package manifests

import (
	"encoding/json"
	"path"

	appsv1 "k8s.io/api/apps/v1"
	autov1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

const IngressClass = "webapprouting.aks.io"

var (
	ingressControllerName = "app-routing-ingress-controller"
	ingressPodLabels      = map[string]string{"app": ingressControllerName}

	externalDNSName   = "app-routing-external-dns"
	externalDNSLabels = map[string]string{"app": externalDNSName}

	topLevelLabels = map[string]string{"app.kubernetes.io/managed-by": "aks-app-routing-operator"}
)

func IngressControllerResources(conf *config.Config) []client.Object {
	return []client.Object{
		newNamespace(conf),
		newIngressClass(conf),
		newIngressControllerServiceAccount(conf),
		newIngressControllerClusterRole(conf),
		newIngressControllerClusterRoleBinding(conf),
		newIngressControllerService(conf),
		newIngressControllerDeployment(conf),
		newIngressControllerHPA(conf),
		newExternalDNSConfigMap(conf),
		newExternalDNSDeployment(conf),
	}
}

func newNamespace(conf *config.Config) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   conf.NS,
			Labels: topLevelLabels,
		},
	}
}

func newIngressClass(conf *config.Config) *netv1.IngressClass {
	return &netv1.IngressClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IngressClass",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: IngressClass,
		},
		Spec: netv1.IngressClassSpec{
			Controller: "k8s.io/ingress-nginx",
		},
	}
}

func newIngressControllerServiceAccount(conf *config.Config) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
	}
}

func newIngressControllerClusterRole(conf *config.Config) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   ingressControllerName,
			Labels: topLevelLabels,
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
		},
	}
}

func newIngressControllerClusterRoleBinding(conf *config.Config) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   ingressControllerName,
			Labels: topLevelLabels,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     ingressControllerName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      ingressControllerName,
			Namespace: conf.NS,
		}},
	}
}

func newIngressControllerService(conf *config.Config) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Spec: corev1.ServiceSpec{
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
			Type:                  corev1.ServiceTypeLoadBalancer,
			Selector:              ingressPodLabels,
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
			},
		},
	}
}

func newIngressControllerDeployment(conf *config.Config) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Spec: appsv1.DeploymentSpec{
			RevisionHistoryLimit: util.Int32Ptr(2),
			Selector:             &metav1.LabelSelector{MatchLabels: ingressPodLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ingressPodLabels,
				},
				Spec: *WithPreferSystemNodes(&corev1.PodSpec{
					ServiceAccountName: ingressControllerName,
					Containers: []corev1.Container{*withPodRefEnvVars(withTypicalProbes(10254, &corev1.Container{
						Name:  "controller",
						Image: path.Join(conf.Registry, "/oss/kubernetes/ingress/nginx-ingress-controller:1.0.5"),
						Args: []string{
							"/nginx-ingress-controller",
							"--ingress-class=" + IngressClass,
							"--annotations-prefix=approuting.aks.io",
							"--publish-service=$(POD_NAMESPACE)/" + ingressControllerName,
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

func newIngressControllerHPA(conf *config.Config) *autov1.HorizontalPodAutoscaler {
	return &autov1.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HorizontalPodAutoscaler",
			APIVersion: "autoscaling/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Spec: autov1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autov1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       ingressControllerName,
			},
			MinReplicas:                    util.Int32Ptr(2),
			MaxReplicas:                    100,
			TargetCPUUtilizationPercentage: util.Int32Ptr(90),
		},
	}
}

func newExternalDNSConfigMap(conf *config.Config) *corev1.ConfigMap {
	js, err := json.Marshal(&map[string]interface{}{
		"tenantId":                    conf.TenantID,
		"subscriptionId":              conf.DNSZoneSub,
		"resourceGroup":               conf.DNSZoneRG,
		"userAssignedIdentityID":      conf.MSIClientID,
		"useManagedIdentityExtension": true,
		"cloud":                       conf.Cloud,
		"location":                    conf.Location,
	})
	if err != nil {
		panic(err)
	}
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDNSName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Data: map[string]string{
			"azure.json": string(js),
		},
	}
}

func newExternalDNSDeployment(conf *config.Config) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDNSName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:             &replicas,
			RevisionHistoryLimit: util.Int32Ptr(2),
			Selector:             &metav1.LabelSelector{MatchLabels: externalDNSLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: externalDNSLabels,
				},
				Spec: *WithPreferSystemNodes(&corev1.PodSpec{
					ServiceAccountName: ingressControllerName,
					Containers: []corev1.Container{*withTypicalProbes(7979, &corev1.Container{
						Name:  "controller",
						Image: path.Join(conf.Registry, "/oss/kubernetes/external-dns:v0.11.0"),
						Args: []string{
							"--provider=azure",
							"--source=ingress",
							"--interval=3m0s",
							"--txt-owner-id=" + conf.DNSRecordID,
							"--domain-filter=" + conf.DNSZoneDomain,
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "azure-config",
							MountPath: "/etc/kubernetes",
							ReadOnly:  true,
						}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("250Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("250Mi"),
							},
						},
					})},
					Volumes: []corev1.Volume{{
						Name: "azure-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: externalDNSName,
								},
							},
						},
					}},
				}),
			},
		},
	}
}

func withPodRefEnvVars(contain *corev1.Container) *corev1.Container {
	copy := contain.DeepCopy()
	copy.Env = append(copy.Env, corev1.EnvVar{
		Name: "POD_NAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.name",
			},
		},
	}, corev1.EnvVar{
		Name: "POD_NAMESPACE",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.namespace",
			},
		},
	})
	return copy
}

func withTypicalProbes(port int, contain *corev1.Container) *corev1.Container {
	copy := contain.DeepCopy()

	copy.LivenessProbe = &corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 10,
		PeriodSeconds:       5,
		SuccessThreshold:    1,
		TimeoutSeconds:      1,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/healthz",
				Port:   intstr.FromInt(port),
				Scheme: corev1.URISchemeHTTP,
			},
		},
	}
	copy.ReadinessProbe = copy.LivenessProbe.DeepCopy()

	return copy
}

func WithPreferSystemNodes(spec *corev1.PodSpec) *corev1.PodSpec {
	copy := spec.DeepCopy()
	copy.PriorityClassName = "system-node-critical"

	copy.Tolerations = append(copy.Tolerations, corev1.Toleration{
		Key:      "CriticalAddonsOnly",
		Operator: corev1.TolerationOpExists,
	})

	if copy.Affinity == nil {
		copy.Affinity = &corev1.Affinity{}
	}
	if copy.Affinity.NodeAffinity == nil {
		copy.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	copy.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = append(copy.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution, corev1.PreferredSchedulingTerm{
		Weight: 100,
		Preference: corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key:      "kubernetes.azure.com/mode",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"system"},
			}},
		},
	})

	if copy.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		copy.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
	}
	copy.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(copy.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      "kubernetes.azure.com/cluster",
				Operator: corev1.NodeSelectorOpExists,
			},
			{
				Key:      "type",
				Operator: corev1.NodeSelectorOpNotIn,
				Values:   []string{"virtual-kubelet"},
			},
			{
				Key:      "kubernetes.io/os",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"linux"},
			},
		},
	})

	return copy
}
