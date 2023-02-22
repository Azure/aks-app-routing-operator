package manifests

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	provider        = "azure"
	privateProvider = "azure-private-dns"
)

// ExternalDnsConfig defines configuration options for required resources for external dns
type ExternalDnsConfig struct {
	ResourceName                                            string
	TenantId, Subscription, ResourceGroup, Domain, RecordId string
	IsPrivate                                               bool
}

// ExternalDnsResources returns Kubernetes objects required for external dns
func ExternalDnsResources(conf *config.Config, self *appsv1.Deployment, externalDnsConfig *ExternalDnsConfig) []client.Object {
	objs := []client.Object{
		newExternalDNSServiceAccount(conf, externalDnsConfig),
		newExternalDNSClusterRole(conf, externalDnsConfig),
		newExternalDNSClusterRoleBinding(conf, externalDnsConfig),
	}

	if conf.NS != "kube-system" {
		objs = append(objs, namespace(conf))
	}

	dnsCm, dnsCmHash := newExternalDNSConfigMap(conf, externalDnsConfig)
	objs = append(objs, dnsCm)
	objs = append(objs, newExternalDNSDeployment(conf, dnsCmHash, externalDnsConfig))

	owners := getOwnerRefs(self)
	for _, obj := range objs {
		obj.SetOwnerReferences(owners)
	}

	return objs
}

func newExternalDNSServiceAccount(conf *config.Config, dnsConfig *ExternalDnsConfig) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dnsConfig.ResourceName,
			Namespace: conf.NS,
			Labels:    topLevelLabels,
		},
	}
}

func newExternalDNSClusterRole(conf *config.Config, dnsConfig *ExternalDnsConfig) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   dnsConfig.ResourceName,
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

func newExternalDNSClusterRoleBinding(conf *config.Config, dnsConfig *ExternalDnsConfig) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   dnsConfig.ResourceName,
			Labels: topLevelLabels,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     dnsConfig.ResourceName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      dnsConfig.ResourceName,
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

func newExternalDNSDeployment(conf *config.Config, configMapHash string, externalDnsConfig *ExternalDnsConfig) *appsv1.Deployment {
	replicas := int32(1)

	provider := provider
	if externalDnsConfig.IsPrivate {
		provider = privateProvider
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
			Replicas:             &replicas,
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
						Args: []string{
							"--provider=" + provider,
							"--source=ingress",
							"--interval=3m0s",
							"--txt-owner-id=" + externalDnsConfig.RecordId,
							"--domain-filter=" + externalDnsConfig.Domain,
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
