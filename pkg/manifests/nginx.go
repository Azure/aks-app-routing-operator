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

var (
	nginx1_13_1 = NginxIngressVersion{
		name: "v1.13.1",
		tag:  "v1.13.1",
	}
	nginxVersionsAscending = []NginxIngressVersion{nginx1_13_1}
	LatestNginxVersion     = nginxVersionsAscending[len(nginxVersionsAscending)-1]
)

var nginxLabels = util.MergeMaps(
	map[string]string{
		k8sNameKey: "nginx",
	},
	GetTopLevelLabels(),
)

const (
	prom                           = "prometheus"
	IngressControllerComponentName = "ingress-controller"
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

const internalLogFormat = `{"remote_addr":"$remote_addr","remote_user":"$remote_user","time_local":"$time_local","request":"$request","status":"$status","body_bytes_sent":"$body_bytes_sent","http_referer":"$http_referer","http_user_agent":"$http_user_agent","request_length":"$request_length","request_time":"$request_time","proxy_upstream_name":"$proxy_upstream_name","proxy_alternative_upstream_name":"$proxy_alternative_upstream_name","upstream_addr":"$upstream_addr","upstream_response_length":"$upstream_response_length","upstream_response_time":"$upstream_response_time","upstream_status":"$upstream_status","req_id":"$req_id","http_x_forwarded_for":"$http_x_forwarded_for","http_x_ms_client_ip_address":"$http_x_ms_client_ip_address","http_x_ms_correlation_request_id":"$http_x_ms_correlation_request_id"}`

func GetNginxResources(conf *config.Config, ingressConfig *NginxIngressConfig) *NginxResources {
	if ingressConfig != nil && ingressConfig.Version == nil {
		ingressConfig.Version = &LatestNginxVersion
	}

	res := &NginxResources{
		IngressClass:            newNginxIngressControllerIngressClass(conf, ingressConfig),
		ServiceAccount:          newNginxIngressControllerServiceAccount(conf, ingressConfig),
		ClusterRole:             newNginxIngressControllerClusterRole(conf, ingressConfig),
		Role:                    newNginxIngressControllerRole(conf, ingressConfig),
		ClusterRoleBinding:      newNginxIngressControllerClusterRoleBinding(conf, ingressConfig),
		RoleBinding:             newNginxIngressControllerRoleBinding(conf, ingressConfig),
		Service:                 newNginxIngressControllerService(conf, ingressConfig),
		PromService:             newNginxIngressControllerPromService(conf, ingressConfig),
		Deployment:              newNginxIngressControllerDeployment(conf, ingressConfig),
		ConfigMap:               newNginxIngressControllerConfigmap(conf, ingressConfig),
		HorizontalPodAutoscaler: newNginxIngressControllerHPA(conf, ingressConfig),
		PodDisruptionBudget:     newNginxIngressControllerPDB(conf, ingressConfig),
	}

	switch ingressConfig.Version {
	// this doesn't do anything yet but when different versions have different resources we should change the resources here
	}

	for _, obj := range res.Objects() {
		l := util.MergeMaps(obj.GetLabels(), nginxLabels)
		obj.SetLabels(l)
	}

	// Can safely assume the namespace exists if using kube-system.
	// Purposefully do this after applying the labels, namespace isn't an Nginx-specific resource
	if conf.NS != "kube-system" {
		res.Namespace = Namespace(conf, conf.NS)
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
	sourceRanges := []string{}
	if ingressConfig != nil && ingressConfig.ServiceConfig != nil {
		for k, v := range ingressConfig.ServiceConfig.Annotations {
			annotations[k] = v
		}

		sourceRanges = ingressConfig.ServiceConfig.LoadBalancerSourceRanges
	}

	ret := &corev1.Service{
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
			ExternalTrafficPolicy:    corev1.ServiceExternalTrafficPolicyTypeLocal,
			Type:                     corev1.ServiceTypeLoadBalancer,
			Selector:                 ingressConfig.PodLabels(),
			LoadBalancerSourceRanges: sourceRanges,
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromString("https"),
				},
			},
		},
	}

	if !ingressConfig.HTTPDisabled {
		ret.Spec.Ports = append([]corev1.ServicePort{
			{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromString("http"),
			},
		}, ret.Spec.Ports...)
	}

	return ret
}

func newNginxIngressControllerPromService(conf *config.Config, ingressConfig *NginxIngressConfig) *corev1.Service {
	annotations := make(map[string]string)
	for k, v := range promAnnotations {
		annotations[k] = v
	}

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingressConfig.ResourceName + "-metrics",
			Namespace:   conf.NS,
			Labels:      AddComponentLabel(GetTopLevelLabels(), "ingress-controller"),
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: ingressConfig.PodLabels(),
			Ports: []corev1.ServicePort{
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

	podAnnotations := map[string]string{
		// https://learn.microsoft.com/en-us/azure/aks/outbound-rules-control-egress#required-outbound-network-rules-and-fqdns-for-aks-clusters
		// helps with firewalls blocking communication to api server
		"kubernetes.azure.com/set-kube-service-host-fqdn": "true",
	}
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
		// https://cloud-provider-azure.sigs.k8s.io/topics/loadbalancer/#custom-load-balancer-health-probe
		// load balancer health probe checks in 5 second intervals. It requires 2 failing probes to fail so we need at least 10s of grace period.
		// we set it to 15s to be safe. Without this Nginx process exits but the LoadBalancer continues routing to the Pod until two health checks fail.
		"--shutdown-grace-period=15",
	}

	if ingressConfig.DefaultSSLCertificate != "" {
		deploymentArgs = append(deploymentArgs, "--default-ssl-certificate="+ingressConfig.DefaultSSLCertificate)
	}

	if ingressConfig.DefaultBackendService != "" {
		deploymentArgs = append(deploymentArgs, "--default-backend-service="+ingressConfig.DefaultBackendService)
	}

	if ingressConfig.EnableSSLPassthrough {
		deploymentArgs = append(deploymentArgs, "--enable-ssl-passthrough=true")
	}

	ret := &appsv1.Deployment{
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
							MatchLabelKeys: []string{
								// https://kubernetes.io/blog/2024/08/16/matchlabelkeys-podaffinity/
								// evaluate only pods of the same version (mostly applicable to rollouts)
								"pod-template-hash",
							},
						},
					},
					ServiceAccountName: ingressConfig.ResourceName,
					Containers: []corev1.Container{*withPodRefEnvVars(withLivenessProbeMatchingReadinessNewFailureThresh(withTypicalReadinessProbe(10254, &corev1.Container{
						Name:  "controller",
						Image: path.Join(conf.Registry, "/oss/kubernetes/ingress/nginx-ingress-controller:"+ingressConfig.Version.tag),
						Args:  deploymentArgs,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: util.ToPtr(false),
							Capabilities: &corev1.Capabilities{
								Add:  []corev1.Capability{"NET_BIND_SERVICE"}, // needed to bind to 80/443 ports https://github.com/kubernetes/ingress-nginx/blob/ca6d3622e5c2819a29f4a407ed272f42d10a91a9/docs/troubleshooting.md?plain=1#L369
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: util.ToPtr(true),
							RunAsUser:    util.Int64Ptr(101),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
						Ports: []corev1.ContainerPort{
							{
								Name:          "https",
								ContainerPort: 443,
							},
							promPodPort,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("127Mi"),
							},
						},
					}), 6))},
				}),
			},
		},
	}

	if !ingressConfig.HTTPDisabled {
		ret.Spec.Template.Spec.Containers[0].Ports = append([]corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 80,
			},
		}, ret.Spec.Template.Spec.Containers[0].Ports...)
	}

	return ret
}

func newNginxIngressControllerConfigmap(conf *config.Config, ingressConfig *NginxIngressConfig) *corev1.ConfigMap {
	confMap := &corev1.ConfigMap{
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

	if ingressConfig.DefaultSSLCertificate != "" && ingressConfig.ForceSSLRedirect {
		confMap.Data["force-ssl-redirect"] = "true"
	}

	if ingressConfig.CustomHTTPErrors != "" {
		confMap.Data["custom-http-errors"] = ingressConfig.CustomHTTPErrors
	}

	if conf.EnableInternalLogging {
		confMap.Data["log-format-upstream"] = internalLogFormat
	}

	if ingressConfig.LogFormat != "" {
		confMap.Data["log-format-upstream"] = ingressConfig.LogFormat
	}

	return confMap
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
