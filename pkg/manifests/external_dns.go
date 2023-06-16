package manifests

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	replicas                = 1
	K8sNameKey              = "app.kubernetes.io/name"
	ExternalDnsResourceName = "external-dns"
	PrivateSuffix           = "-private"
	PublicSuffix            = "-public"
)

type Provider int

const (
	PublicProvider = iota
	PrivateProvider
)

func (p Provider) String() string {
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
	TenantId, Subscription, ResourceName, ResourceGroup string
	Provider                                            Provider
	DnsZoneResourceIDs                                  []string
}

// ExternalDnsResources returns Kubernetes objects required for external dns
func ExternalDnsResources(conf *config.Config, self *appsv1.Deployment, externalDnsConfigs []*ExternalDnsConfig) []client.Object {
	var objs []client.Object

	// Can safely assume the namespace exists if using kube-system
	if conf.NS != "kube-system" {
		objs = append(objs, Namespace(conf))
	}

	for _, dnsConfig := range externalDnsConfigs {
		objs = append(objs, externalDnsResourcesFromConfig(conf, dnsConfig)...)
	}

	owners := getOwnerRefs(self)
	for _, obj := range objs {
		obj.SetOwnerReferences(owners)
	}

	return objs
}

func externalDnsResourcesFromConfig(conf *config.Config, externalDnsConfig *ExternalDnsConfig) []client.Object {
	var objs []client.Object

	objs = append(objs, newExternalDNSServiceAccount(conf, externalDnsConfig))
	objs = append(objs, newExternalDNSClusterRole(conf, externalDnsConfig))
	objs = append(objs, newExternalDNSClusterRoleBinding(conf, externalDnsConfig))

	dnsCm, dnsCmHash := newExternalDNSConfigMap(conf, externalDnsConfig)
	objs = append(objs, dnsCm)

	objs = append(objs, newExternalDNSDeployment(conf, externalDnsConfig, dnsCmHash))

	return objs
}

// will be used as part of external dns cleanup
func addDnsTypeLabel(originalLabels map[string]string, resourceName string) map[string]string {
	ret := map[string]string{}
	for k, v := range originalLabels {
		ret[k] = v
	}
	ret[K8sNameKey] = resourceName

	return ret
}

func newExternalDNSServiceAccount(conf *config.Config, externalDnsConfig *ExternalDnsConfig) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDnsConfig.ResourceName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
	}
}

func newExternalDNSClusterRole(conf *config.Config, externalDnsConfig *ExternalDnsConfig) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   externalDnsConfig.ResourceName,
			Labels: topLevelLabels,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints", "pods", "services", "configmaps"},
				Verbs:     []string{"get", "watch", "list"},
			},
			{
				APIGroups: []string{"extensions", "networking.k8s.io"},
				Resources: []string{"ingresses"},
				Verbs:     []string{"get", "watch", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
}

func newExternalDNSClusterRoleBinding(conf *config.Config, externalDnsConfig *ExternalDnsConfig) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   externalDnsConfig.ResourceName,
			Labels: topLevelLabels,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     externalDnsConfig.ResourceName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      externalDnsConfig.ResourceName,
			Namespace: conf.NS,
		}},
	}
}

func newExternalDNSConfigMap(conf *config.Config, externalDnsConfig *ExternalDnsConfig) (*corev1.ConfigMap, string) {
	js, err := json.Marshal(&map[string]interface{}{
		"tenantId":                    externalDnsConfig.TenantId,
		"subscriptionId":              externalDnsConfig.Subscription,
		"resourceGroup":               externalDnsConfig.ResourceGroup,
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
			Name:      externalDnsConfig.ResourceName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
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

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDnsConfig.ResourceName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:             to.Int32Ptr(replicas),
			RevisionHistoryLimit: util.Int32Ptr(2),
			Selector:             &metav1.LabelSelector{MatchLabels: map[string]string{"app": externalDnsConfig.ResourceName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                externalDnsConfig.ResourceName,
						"checksum/configmap": configMapHash[:16],
					},
				},
				Spec: *WithPreferSystemNodes(&corev1.PodSpec{
					ServiceAccountName: externalDnsConfig.ResourceName,
					Containers: []corev1.Container{*withLivenessProbeMatchingReadiness(withTypicalReadinessProbe(7979, &corev1.Container{
						Name:  "controller",
						Image: path.Join(conf.Registry, "/oss/kubernetes/external-dns:v0.11.0.2"),
						Args: append([]string{
							"--provider=" + externalDnsConfig.Provider.String(),
							"--source=ingress",
							"--interval=3m0s",
							"--txt-owner-id=" + hash(conf.ClusterFqdn.String()),
						}, domainFilters...),
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
									Name: externalDnsConfig.ResourceName,
								},
							},
						},
					}},
				}),
			},
		},
	}
}

func hash(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
