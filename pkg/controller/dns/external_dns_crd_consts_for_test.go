package dns

import (
	"crypto/sha256"
	"encoding/hex"
	"path"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/go-autorest/autorest/to"
	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var testRegistry = "testregistry.azurecr.io"

var happyPathPublic = &v1alpha1.ExternalDNS{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "happy-path-public",
		Namespace: "test-ns",
	},
	Spec: v1alpha1.ExternalDNSSpec{
		ResourceName:       "happy-path-public",
		TenantID:           "12345678-1234-1234-1234-123456789012",
		DNSZoneResourceIDs: []string{"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test.com", "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test2.com"},
		ResourceTypes:      []string{"ingress", "gateway"},
		Identity: v1alpha1.ExternalDNSIdentity{
			ServiceAccount: "test-service-account",
		},
	},
}

var happyPathPublicJSON = `{"cloud":"","location":"","resourceGroup":"test-rg","subscriptionId":"12345678-1234-1234-1234-123456789012","tenantId":"12345678-1234-1234-1234-123456789012","useWorkloadIdentityExtension":true}`
var happyPathPublicJSONHash = sha256.Sum256([]byte(happyPathPublicJSON))
var happyPathPublicConfigmap = &corev1.ConfigMap{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ConfigMap",
		APIVersion: "v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		ResourceVersion: "1",
		Name:            "happy-path-public-external-dns",
		Namespace:       "test-ns",
		Labels: map[string]string{
			"app.kubernetes.io/managed-by":   "aks-app-routing-operator",
			"app.kubernetes.io/name":         "happy-path-public-external-dns",
			"kubernetes.azure.com/managedby": "aks",
		},
		OwnerReferences: ownerReferencesFromCRD(happyPathPublic),
	},
	Data: map[string]string{
		"azure.json": happyPathPublicJSON,
	},
}

var happyPathPublicDeployment = &appsv1.Deployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		ResourceVersion: "1",
		Name:            "happy-path-public-external-dns",
		Namespace:       "test-ns",
		Labels:          map[string]string{"app.kubernetes.io/managed-by": "aks-app-routing-operator", "kubernetes.azure.com/managedby": "aks", "app.kubernetes.io/name": "happy-path-public-external-dns"},
		OwnerReferences: ownerReferencesFromCRD(happyPathPublic),
	},
	Spec: appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "happy-path-public-external-dns"}},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app.kubernetes.io/managed-by": "aks-app-routing-operator", "kubernetes.azure.com/managedby": "aks", "app": "happy-path-public-external-dns", "checksum/configmap": hex.EncodeToString(happyPathPublicJSONHash[:])[:16]},
			},
			Spec: corev1.PodSpec{
				ServiceAccountName: testServiceAccount.Name,
				Containers: []corev1.Container{
					{
						Name:  "controller",
						Image: path.Join(testRegistry, "/oss/v2/kubernetes/external-dns:v0.15.0"),
						Args: []string{
							"--provider=azure",
							"--interval=3m0s",
							"--txt-owner-id=test-cluster-uid",
							"--txt-wildcard-replacement=approutingwildcard",
							"--source=gateway-grpcroute",
							"--source=gateway-httproute",
							"--source=ingress",
							"--domain-filter=test.com",
							"--domain-filter=test2.com",
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "azure-config",
							MountPath: "/etc/kubernetes",
							ReadOnly:  true,
						}},
					},
				},
				Volumes: []corev1.Volume{{
					Name: "azure-config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "happy-path-public-external-dns",
							},
						},
					},
				}},
			},
		},
	},
}

var happyPathPublicRole = &rbacv1.Role{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Role",
		APIVersion: "rbac.authorization.k8s.io/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		ResourceVersion: "1",
		Name:            "happy-path-public-external-dns",
		Namespace:       "test-ns",
		Labels: map[string]string{
			"app.kubernetes.io/managed-by":   "aks-app-routing-operator",
			"app.kubernetes.io/name":         "happy-path-public-external-dns",
			"kubernetes.azure.com/managedby": "aks",
		},
		OwnerReferences: ownerReferencesFromCRD(happyPathPublic),
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
		{
			APIGroups: []string{"extensions", "networking.k8s.io"},
			Resources: []string{"ingresses"},
			Verbs:     []string{"get", "watch", "list"},
		},
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
	},
}

var happyPathPublicRoleBinding = &rbacv1.RoleBinding{
	TypeMeta: metav1.TypeMeta{
		Kind:       "RoleBinding",
		APIVersion: "rbac.authorization.k8s.io/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		ResourceVersion: "1",
		Name:            "happy-path-public-external-dns",
		Namespace:       "test-ns",
		Labels: map[string]string{
			"app.kubernetes.io/managed-by":   "aks-app-routing-operator",
			"app.kubernetes.io/name":         "happy-path-public-external-dns",
			"kubernetes.azure.com/managedby": "aks",
		},
		OwnerReferences: ownerReferencesFromCRD(happyPathPublic),
	},
	RoleRef: rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     "happy-path-public-external-dns",
	},
	Subjects: []rbacv1.Subject{{
		Kind:      "ServiceAccount",
		Name:      "test-service-account",
		Namespace: "test-ns",
	}},
}

var happyPathPublicFilters = &v1alpha1.ExternalDNS{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "happy-path-public-filters",
		Namespace: "test-ns",
	},
	Spec: v1alpha1.ExternalDNSSpec{
		ResourceName:       "happy-path-public-filters",
		TenantID:           "12345678-1234-1234-1234-123456789012",
		DNSZoneResourceIDs: []string{"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test.com", "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/test2.com"},
		ResourceTypes:      []string{"ingress", "gateway"},
		Identity: v1alpha1.ExternalDNSIdentity{
			ServiceAccount: "test-service-account",
		},
		Filters: &v1alpha1.ExternalDNSFilters{
			GatewayLabelSelector:         to.StringPtr("app=testapp"),
			RouteAndIngressLabelSelector: to.StringPtr("app=testapp"),
		},
	},
}

var happyPathPrivate = &v1alpha1.ExternalDNS{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "happy-path-private",
		Namespace: "test-ns",
	},
	Spec: v1alpha1.ExternalDNSSpec{
		ResourceName:       "happy-path-private",
		TenantID:           "12345678-1234-1234-1234-123456789012",
		DNSZoneResourceIDs: []string{"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/privateDnsZones/test.com", "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.Network/privateDnsZones/test2.com"},
		ResourceTypes:      []string{"ingress", "gateway"},
		Identity: v1alpha1.ExternalDNSIdentity{
			ServiceAccount: "test-service-account",
		},
	},
}

var happyPathPrivateDeployment = &appsv1.Deployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		ResourceVersion: "1",
		Name:            "happy-path-private-external-dns",
		Namespace:       "test-ns",
		Labels:          map[string]string{"app.kubernetes.io/managed-by": "aks-app-routing-operator", "kubernetes.azure.com/managedby": "aks", "app.kubernetes.io/name": "happy-path-private-external-dns"},
		OwnerReferences: ownerReferencesFromCRD(happyPathPrivate),
	},
	Spec: appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "happy-path-private-external-dns"}},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app.kubernetes.io/managed-by": "aks-app-routing-operator", "kubernetes.azure.com/managedby": "aks", "app": "happy-path-private-external-dns", "checksum/configmap": hex.EncodeToString(happyPathPublicJSONHash[:])[:16]},
			},
			Spec: corev1.PodSpec{
				ServiceAccountName: testServiceAccount.Name,
				Containers: []corev1.Container{
					{
						Name:  "controller",
						Image: path.Join(testRegistry, "/oss/v2/kubernetes/external-dns:v0.15.0"),
						Args: []string{
							"--provider=azure-private-dns",
							"--interval=3m0s",
							"--txt-owner-id=test-cluster-uid",
							"--txt-wildcard-replacement=approutingwildcard",
							"--source=gateway-grpcroute",
							"--source=gateway-httproute",
							"--source=ingress",
							"--domain-filter=test.com",
							"--domain-filter=test2.com",
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "azure-config",
							MountPath: "/etc/kubernetes",
							ReadOnly:  true,
						}},
					},
				},
				Volumes: []corev1.Volume{{
					Name: "azure-config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "happy-path-private-external-dns",
							},
						},
					},
				}},
			},
		},
	},
}

var testServiceAccount = &corev1.ServiceAccount{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ServiceAccount",
		APIVersion: "v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-service-account",
		Namespace: "test-ns",
		Annotations: map[string]string{
			"azure.workload.identity/client-id": "test-client-id",
		},
	},
}

// note - does not contain WI annotation
var testBadServiceAccount = &corev1.ServiceAccount{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ServiceAccount",
		APIVersion: "v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-service-account",
		Namespace: "test-ns",
	},
}

func ownerReferencesFromCRD(obj *v1alpha1.ExternalDNS) []metav1.OwnerReference {
	return []metav1.OwnerReference{{
		APIVersion: obj.APIVersion,
		Controller: util.ToPtr(true),
		Kind:       obj.Kind,
		Name:       obj.Name,
		UID:        obj.UID,
	},
	}
}
