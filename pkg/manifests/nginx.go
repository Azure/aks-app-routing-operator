// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"

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

const Controller = "webapprouting.kubernetes.azure.com/nginx"

var (
	NginxControllerName   = "nginx"
	NginxIngressPodLabels = map[string]string{"app": NginxControllerName}

	externalDNSName = "external-dns"
)

func NginxIngressControllerResources(conf *config.Config, self *appsv1.Deployment, ic *netv1.IngressClass) []client.Object {
	objs := []client.Object{}

	// Can safely assume the namespace exists if using kube-system
	if conf.NS != "kube-system" {
		objs = append(objs, newNamespace(conf))
	}

	objs = append(objs,
		newIngressControllerServiceAccount(conf),
		newIngressControllerClusterRole(conf),
		newIngressControllerClusterRoleBinding(conf),
		newIngressControllerService(conf),
		newIngressControllerDeployment(conf, ic),
		newIngressControllerConfigmap(conf),
		newIngressControllerPDB(conf),
		newIngressControllerHPA(conf),
	)

	if conf.DNSZoneDomain != "" {
		dnsCM, dnsCMHash := newExternalDNSConfigMap(conf)
		objs = append(objs, dnsCM,
			newExternalDNSDeployment(conf, dnsCMHash))
	}

	owners := getOwnerRefs(self)
	for _, obj := range objs {
		obj.SetOwnerReferences(owners)
	}

	return objs
}

func newIngressControllerServiceAccount(conf *config.Config) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      NginxControllerName,
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
			Name:   NginxControllerName,
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
			Name:   NginxControllerName,
			Labels: topLevelLabels,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     NginxControllerName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      NginxControllerName,
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
			Name:      NginxControllerName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Spec: corev1.ServiceSpec{
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
			Type:                  corev1.ServiceTypeLoadBalancer,
			Selector:              NginxIngressPodLabels,
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

func newIngressControllerDeployment(conf *config.Config, ic *netv1.IngressClass) *appsv1.Deployment {
	podAnnotations := map[string]string{}
	if !conf.DisableOSM {
		podAnnotations["openservicemesh.io/sidecar-injection"] = "enabled"
	}
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      NginxControllerName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Spec: appsv1.DeploymentSpec{
			RevisionHistoryLimit: util.Int32Ptr(2),
			Selector:             &metav1.LabelSelector{MatchLabels: NginxIngressPodLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      NginxIngressPodLabels,
					Annotations: podAnnotations,
				},
				Spec: *WithPreferSystemNodes(&corev1.PodSpec{
					ServiceAccountName: NginxControllerName,
					Containers: []corev1.Container{*withPodRefEnvVars(withTypicalReadinessProbe(10254, &corev1.Container{
						Name:  "controller",
						Image: path.Join(conf.Registry, "/oss/kubernetes/ingress/nginx-ingress-controller:v1.2.1"),
						Args: []string{
							"/nginx-ingress-controller",
							"--ingress-class=" + ic.Name,
							"--controller-class=" + Controller,
							"--publish-service=$(POD_NAMESPACE)/" + NginxControllerName,
							"--configmap=$(POD_NAMESPACE)/" + NginxControllerName,
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

func newIngressControllerConfigmap(conf *config.Config) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      NginxControllerName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Data: map[string]string{
			// Can't use 'allow-snippet-annotations=false' to reduce injection risk, since we require snippet functionality for OSM routing.
			// But we can still protect against leaked service account tokens.
			// See: https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/configmap/#annotation-value-word-blocklist
			"annotation-value-word-blocklist": "load_module,lua_package,_by_lua,location,root,proxy_pass,serviceaccount,{,},'",
		},
	}
}

func newIngressControllerPDB(conf *config.Config) *policyv1.PodDisruptionBudget {
	maxUnavailable := intstr.FromInt(1)
	return &policyv1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodDisruptionBudget",
			APIVersion: "policy/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      NginxControllerName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector:       &metav1.LabelSelector{MatchLabels: NginxIngressPodLabels},
			MaxUnavailable: &maxUnavailable,
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
			Name:      NginxControllerName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Spec: autov1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autov1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       NginxControllerName,
			},
			MinReplicas:                    util.Int32Ptr(2),
			MaxReplicas:                    100,
			TargetCPUUtilizationPercentage: util.Int32Ptr(90),
		},
	}
}

func newExternalDNSConfigMap(conf *config.Config) (*corev1.ConfigMap, string) {
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
	hash := sha256.Sum256(js)
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
	}, hex.EncodeToString(hash[:])
}

func newExternalDNSDeployment(conf *config.Config, configMapHash string) *appsv1.Deployment {
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
			Selector:             &metav1.LabelSelector{MatchLabels: map[string]string{"app": externalDNSName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                externalDNSName,
						"checksum/configmap": configMapHash[:16],
					},
				},
				Spec: *WithPreferSystemNodes(&corev1.PodSpec{
					ServiceAccountName: NginxControllerName,
					Containers: []corev1.Container{*withLivenessProbeMatchingReadiness(withTypicalReadinessProbe(7979, &corev1.Container{
						Name:  "controller",
						Image: path.Join(conf.Registry, "/oss/kubernetes/external-dns:v0.11.0.2"),
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
					}))},
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

func withTypicalReadinessProbe(port int, contain *corev1.Container) *corev1.Container {
	copy := contain.DeepCopy()

	copy.ReadinessProbe = &corev1.Probe{
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

	return copy
}

func withLivenessProbeMatchingReadiness(contain *corev1.Container) *corev1.Container {
	copy := contain.DeepCopy()
	copy.LivenessProbe = copy.ReadinessProbe.DeepCopy()
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
