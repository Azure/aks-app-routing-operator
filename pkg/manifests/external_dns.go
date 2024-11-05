package manifests

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
	"k8s.io/apimachinery/pkg/runtime/schema"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	replicas                = 1 // this must stay at 1 unless external-dns adds support for multiple replicas https://github.com/kubernetes-sigs/external-dns/issues/2430
	k8sNameKey              = "app.kubernetes.io/name"
	externalDnsResourceName = "external-dns"
	txtWildcardReplacement  = "approutingwildcard"
)

type IdentityType int

const (
	IdentityTypeMSI IdentityType = iota
	IdentityTypeWorkloadIdentity
)

func (i IdentityType) externalDNSIdentityConfiguration() string {
	switch i {
	case IdentityTypeWorkloadIdentity:
		return "useWorkloadIdentityExtension"
	default:
		return "useManagedIdentityExtension"
	}
}

type ResourceType int

const (
	ResourceTypeIngress ResourceType = iota
	ResourceTypeGateway
)

func (r ResourceType) string() string {
	switch r {
	case ResourceTypeGateway:
		return "Gateway"
	default:
		return "Ingress"
	}
}

// OldExternalDnsGks is a slice of GroupKinds that were previously used by ExternalDns.
// If the manifests used by app routing's external dns removes a GroupKind be sure to add
// it here to clean it up
var OldExternalDnsGks []schema.GroupKind

type Provider int

var Providers = []Provider{PublicProvider, PrivateProvider}

const (
	PublicProvider Provider = iota
	PrivateProvider
)

func (p Provider) string() string {
	switch p {
	case PublicProvider:
		return "azure"
	case PrivateProvider:
		return "azure-private-dns"
	default:
		return ""
	}
}

// ExternalDnsConfig defines configuration options for required resources for external dns
type ExternalDnsConfig struct {
	TenantId, Subscription, ResourceGroup,
	ClientId, ServiceAccountName, Namespace string
	IdentityType       IdentityType
	ResourceType       ResourceType
	Provider           Provider
	DnsZoneResourceIDs []string
}

func (e *ExternalDnsConfig) resourceName() string {
	var resourcePrefix string
	var suffix string

	switch e.ResourceType {
	case ResourceTypeGateway:
		resourcePrefix = strings.ToLower(e.ResourceType.string()) + "-"
	}

	switch e.Provider {
	case PublicProvider:
		suffix = externalDnsResourceName
	case PrivateProvider:
		suffix = externalDnsResourceName + "-private"
	}

	return resourcePrefix + suffix
}

func ExternalDNSLabels(e *ExternalDnsConfig) map[string]string {
	labels := map[string]string{
		k8sNameKey: e.resourceName(),
	}
	return labels
}

// ExternalDnsResources returns Kubernetes objects required for external dns
func ExternalDnsResources(conf *config.Config, externalDnsConfigs []*ExternalDnsConfig) []client.Object {
	var objs []client.Object
	for _, dnsConfig := range externalDnsConfigs {
		// Can safely assume the namespace exists if using kube-system
		if dnsConfig.Namespace != "" && dnsConfig.Namespace != "kube-system" {
			objs = append(objs, Namespace(conf, dnsConfig.Namespace))
		}
		objs = append(objs, externalDnsResourcesFromConfig(conf, dnsConfig)...)
	}

	return objs
}

func externalDnsResourcesFromConfig(conf *config.Config, externalDnsConfig *ExternalDnsConfig) []client.Object {
	var objs []client.Object
	if externalDnsConfig.IdentityType == IdentityTypeMSI {
		objs = append(objs, newExternalDNSServiceAccount(conf, externalDnsConfig))
	}
	objs = append(objs, newExternalDNSClusterRole(conf, externalDnsConfig))
	objs = append(objs, newExternalDNSClusterRoleBinding(conf, externalDnsConfig))

	dnsCm, dnsCmHash := newExternalDNSConfigMap(conf, externalDnsConfig)
	objs = append(objs, dnsCm)
	objs = append(objs, newExternalDNSDeployment(conf, externalDnsConfig, dnsCmHash))

	for _, obj := range objs {
		l := util.MergeMaps(obj.GetLabels(), ExternalDNSLabels(externalDnsConfig))
		obj.SetLabels(l)
	}

	return objs
}

func newExternalDNSServiceAccount(conf *config.Config, externalDnsConfig *ExternalDnsConfig) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDnsConfig.resourceName(),
			Namespace: externalDnsConfig.Namespace,
			Labels:    GetTopLevelLabels(),
		},
	}
}

func newExternalDNSClusterRole(conf *config.Config, externalDnsConfig *ExternalDnsConfig) *rbacv1.ClusterRole {
	role := &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   externalDnsConfig.resourceName(),
			Labels: GetTopLevelLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints", "pods", "services", "configmaps"},
				Verbs:     []string{"get", "watch", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}

	switch externalDnsConfig.ResourceType {
	case ResourceTypeGateway:
		role.Rules = append(role.Rules,
			[]rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"namespaces"},
					Verbs:     []string{"get", "watch", "list"},
				},
				{
					APIGroups: []string{"gateway.networking.k8s.io"},
					Resources: []string{"gateways", "httproutes", "grpcroutes"},
					Verbs:     []string{"get", "watch", "list"},
				},
			}...,
		)
	default:
		role.Rules = append(role.Rules,
			rbacv1.PolicyRule{
				APIGroups: []string{"extensions", "networking.k8s.io"},
				Resources: []string{"ingresses"},
				Verbs:     []string{"get", "watch", "list"},
			})
	}

	return role
}

func newExternalDNSClusterRoleBinding(conf *config.Config, externalDnsConfig *ExternalDnsConfig) *rbacv1.ClusterRoleBinding {
	var serviceAccount string
	switch externalDnsConfig.IdentityType {
	case IdentityTypeWorkloadIdentity:
		serviceAccount = externalDnsConfig.ServiceAccountName
	default:
		serviceAccount = externalDnsConfig.resourceName()
	}
	ret := &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   externalDnsConfig.resourceName(),
			Labels: GetTopLevelLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     externalDnsConfig.resourceName(),
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      serviceAccount,
			Namespace: externalDnsConfig.Namespace,
		}},
	}

	return ret
}

func newExternalDNSConfigMap(conf *config.Config, externalDnsConfig *ExternalDnsConfig) (*corev1.ConfigMap, string) {

	jsMap := map[string]interface{}{
		"tenantId":       externalDnsConfig.TenantId,
		"subscriptionId": externalDnsConfig.Subscription,
		"resourceGroup":  externalDnsConfig.ResourceGroup,
		"cloud":          conf.Cloud,
		"location":       conf.Location,
	}
	jsMap[externalDnsConfig.IdentityType.externalDNSIdentityConfiguration()] = true

	if externalDnsConfig.IdentityType == IdentityTypeMSI {
		jsMap["userAssignedIdentityID"] = externalDnsConfig.ClientId
	}

	js, err := json.Marshal(&jsMap)
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
			Name:      externalDnsConfig.resourceName(),
			Namespace: externalDnsConfig.Namespace,
			Labels:    GetTopLevelLabels(),
		},
		Data: map[string]string{
			"azure.json": string(js),
		},
	}, hex.EncodeToString(hash[:])
}

func newExternalDNSDeployment(conf *config.Config, externalDnsConfig *ExternalDnsConfig, configMapHash string) *appsv1.Deployment {
	domainFilters := []string{}

	for _, zoneId := range externalDnsConfig.DnsZoneResourceIDs {
		parsedZone, err := azure.ParseResourceID(zoneId)
		if err != nil {
			continue
		}
		domainFilters = append(domainFilters, fmt.Sprintf("--domain-filter=%s", parsedZone.ResourceName))
	}

	podLabels := GetTopLevelLabels()
	podLabels["app"] = externalDnsConfig.resourceName()
	podLabels["checksum/configmap"] = configMapHash[:16]

	var serviceAccount string
	switch externalDnsConfig.IdentityType {
	case IdentityTypeWorkloadIdentity:
		serviceAccount = externalDnsConfig.ServiceAccountName
	default:
		serviceAccount = externalDnsConfig.resourceName()
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDnsConfig.resourceName(),
			Namespace: externalDnsConfig.Namespace,
			Labels:    GetTopLevelLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:             to.Int32Ptr(replicas),
			RevisionHistoryLimit: util.Int32Ptr(2),
			Selector:             &metav1.LabelSelector{MatchLabels: map[string]string{"app": externalDnsConfig.resourceName()}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: *WithPreferSystemNodes(&corev1.PodSpec{
					ServiceAccountName: serviceAccount,
					Containers: []corev1.Container{*withLivenessProbeMatchingReadiness(withTypicalReadinessProbe(7979, &corev1.Container{
						Name:  "controller",
						Image: path.Join(conf.Registry, "/oss/kubernetes/external-dns:v0.14.2"),
						Args: append([]string{
							"--provider=" + externalDnsConfig.Provider.string(),
							"--interval=" + conf.DnsSyncInterval.String(),
							"--txt-owner-id=" + conf.ClusterUid,
							"--txt-wildcard-replacement=" + txtWildcardReplacement,
						}, append(generateResourceDeploymentArgs(externalDnsConfig.ResourceType), domainFilters...)...),
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
						SecurityContext: &corev1.SecurityContext{
							Privileged:               util.ToPtr(false),
							AllowPrivilegeEscalation: util.ToPtr(false),
							ReadOnlyRootFilesystem:   util.ToPtr(true),
							RunAsNonRoot:             util.ToPtr(true),
							RunAsUser:                util.Int64Ptr(65532),
							RunAsGroup:               util.Int64Ptr(65532),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
					}))},
					Volumes: []corev1.Volume{{
						Name: "azure-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: externalDnsConfig.resourceName(),
								},
							},
						},
					}},
				}),
			},
		},
	}
}

func generateResourceDeploymentArgs(rts ...ResourceType) []string {
	var ret []string
	for _, rt := range rts {
		switch rt {
		case ResourceTypeGateway:
			ret = append(ret, []string{
				"--source=gateway-httproute",
				"--source=gateway-grpcroute",
			}...)
		case ResourceTypeIngress:
			ret = append(ret, "--source=ingress")
		}
	}

	return ret
}
