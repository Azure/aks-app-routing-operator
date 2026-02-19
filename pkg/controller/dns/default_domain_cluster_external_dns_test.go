package dns

import (
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultDomainServiceAccount(t *testing.T) {
	testCases := []struct {
		name     string
		conf     *config.Config
		expected *corev1.ServiceAccount
	}{
		{
			name: "creates service account with correct annotations and labels",
			conf: &config.Config{
				NS:                    "test-namespace",
				DefaultDomainClientID: "test-client-id-123",
			},
			expected: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultDomainServiceAccountName,
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"azure.workload.identity/client-id": "test-client-id-123",
					},
					Labels: manifests.GetTopLevelLabels(),
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceAccount",
					APIVersion: "v1",
				},
			},
		},
		{
			name: "creates service account in different namespace",
			conf: &config.Config{
				NS:                    "another-namespace",
				DefaultDomainClientID: "different-client-id",
			},
			expected: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultDomainServiceAccountName,
					Namespace: "another-namespace",
					Annotations: map[string]string{
						"azure.workload.identity/client-id": "different-client-id",
					},
					Labels: manifests.GetTopLevelLabels(),
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceAccount",
					APIVersion: "v1",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := defaultDomainServiceAccount(tc.conf)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestDefaultDomainClusterExternalDNS(t *testing.T) {
	testCases := []struct {
		name     string
		conf     *config.Config
		expected *approutingv1alpha1.ClusterExternalDNS
	}{
		{
			name: "creates ClusterExternalDNS with WI identity when WI enabled",
			conf: &config.Config{
				NS:                         "test-namespace",
				DefaultDomainZoneID:        "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
				EnableDefaultDomainGateway: false,
				EnabledWorkloadIdentity:    true,
			},
			expected: &approutingv1alpha1.ClusterExternalDNS{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultDomainDNSResourceName,
					Namespace: "test-namespace",
					Labels:    manifests.GetTopLevelLabels(),
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterExternalDNS",
					APIVersion: approutingv1alpha1.GroupVersion.String(),
				},
				Spec: approutingv1alpha1.ClusterExternalDNSSpec{
					ResourceName:       defaultDomainDNSResourceName,
					ResourceNamespace:  "test-namespace",
					DNSZoneResourceIDs: []string{"/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com"},
					ResourceTypes:      []string{"ingress"},
					Identity: approutingv1alpha1.ExternalDNSIdentity{
						Type:           approutingv1alpha1.IdentityTypeWorkloadIdentity,
						ServiceAccount: defaultDomainServiceAccountName,
					},
				},
			},
		},
		{
			name: "creates ClusterExternalDNS with ingress and gateway when gateway enabled",
			conf: &config.Config{
				NS:                         "test-namespace",
				DefaultDomainZoneID:        "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
				EnableDefaultDomainGateway: true,
				EnabledWorkloadIdentity:    true,
			},
			expected: &approutingv1alpha1.ClusterExternalDNS{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultDomainDNSResourceName,
					Namespace: "test-namespace",
					Labels:    manifests.GetTopLevelLabels(),
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterExternalDNS",
					APIVersion: approutingv1alpha1.GroupVersion.String(),
				},
				Spec: approutingv1alpha1.ClusterExternalDNSSpec{
					ResourceName:       defaultDomainDNSResourceName,
					ResourceNamespace:  "test-namespace",
					DNSZoneResourceIDs: []string{"/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com"},
					ResourceTypes:      []string{"ingress", "gateway"},
					Identity: approutingv1alpha1.ExternalDNSIdentity{
						Type:           approutingv1alpha1.IdentityTypeWorkloadIdentity,
						ServiceAccount: defaultDomainServiceAccountName,
					},
				},
			},
		},
		{
			name: "creates ClusterExternalDNS with MSI identity when WI disabled",
			conf: &config.Config{
				NS:                         "test-namespace",
				DefaultDomainClientID:      "test-client-id",
				DefaultDomainZoneID:        "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
				EnableDefaultDomainGateway: false,
				EnabledWorkloadIdentity:    false,
			},
			expected: &approutingv1alpha1.ClusterExternalDNS{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultDomainDNSResourceName,
					Namespace: "test-namespace",
					Labels:    manifests.GetTopLevelLabels(),
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterExternalDNS",
					APIVersion: approutingv1alpha1.GroupVersion.String(),
				},
				Spec: approutingv1alpha1.ClusterExternalDNSSpec{
					ResourceName:       defaultDomainDNSResourceName,
					ResourceNamespace:  "test-namespace",
					DNSZoneResourceIDs: []string{"/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com"},
					ResourceTypes:      []string{"ingress"},
					Identity: approutingv1alpha1.ExternalDNSIdentity{
						Type:     approutingv1alpha1.IdentityTypeManagedIdentity,
						ClientID: "test-client-id",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := defaultDomainClusterExternalDNS(tc.conf)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestDefaultDomainObjectsWorkloadIdentity(t *testing.T) {
	testCases := []struct {
		name                  string
		conf                  *config.Config
		expectedObjCount      int
		expectedTypes         []string
		hasNamespace          bool
		expectedResourceTypes []string
	}{
		{
			name: "creates all objects with namespace when NS is not kube-system, gateway disabled",
			conf: &config.Config{
				NS:                         "test-namespace",
				DefaultDomainClientID:      "test-client-id",
				DefaultDomainZoneID:        "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
				EnableDefaultDomainGateway: false,
				EnabledWorkloadIdentity:    true,
			},
			expectedObjCount:      3,
			expectedTypes:         []string{"Namespace", "ServiceAccount", "ClusterExternalDNS"},
			hasNamespace:          true,
			expectedResourceTypes: []string{"ingress"},
		},
		{
			name: "creates all objects with namespace when NS is not kube-system, gateway enabled",
			conf: &config.Config{
				NS:                         "test-namespace",
				DefaultDomainClientID:      "test-client-id",
				DefaultDomainZoneID:        "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
				EnableDefaultDomainGateway: true,
				EnabledWorkloadIdentity:    true,
			},
			expectedObjCount:      3,
			expectedTypes:         []string{"Namespace", "ServiceAccount", "ClusterExternalDNS"},
			hasNamespace:          true,
			expectedResourceTypes: []string{"ingress", "gateway"},
		},
		{
			name: "creates objects without namespace when NS is kube-system",
			conf: &config.Config{
				NS:                         "kube-system",
				DefaultDomainClientID:      "test-client-id",
				DefaultDomainZoneID:        "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
				EnableDefaultDomainGateway: false,
				EnabledWorkloadIdentity:    true,
			},
			expectedObjCount:      2,
			expectedTypes:         []string{"ServiceAccount", "ClusterExternalDNS"},
			hasNamespace:          false,
			expectedResourceTypes: []string{"ingress"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := defaultDomainObjects(tc.conf)
			require.Len(t, result, tc.expectedObjCount)

			// Verify types
			actualTypes := make([]string, len(result))
			for i, obj := range result {
				switch obj.(type) {
				case *corev1.Namespace:
					actualTypes[i] = "Namespace"
				case *corev1.ServiceAccount:
					actualTypes[i] = "ServiceAccount"
				case *approutingv1alpha1.ClusterExternalDNS:
					actualTypes[i] = "ClusterExternalDNS"
				}
			}
			require.Equal(t, tc.expectedTypes, actualTypes)

			if tc.hasNamespace {
				_, ok := result[0].(*corev1.Namespace)
				require.True(t, ok, "First object should be Namespace when NS is not kube-system")
			}

			// Verify ServiceAccount properties
			var sa *corev1.ServiceAccount
			for _, obj := range result {
				if s, ok := obj.(*corev1.ServiceAccount); ok {
					sa = s
					break
				}
			}
			require.NotNil(t, sa)
			require.Equal(t, defaultDomainServiceAccountName, sa.Name)
			require.Equal(t, tc.conf.NS, sa.Namespace)
			require.Equal(t, tc.conf.DefaultDomainClientID, sa.Annotations["azure.workload.identity/client-id"])
			require.Equal(t, manifests.GetTopLevelLabels(), sa.Labels)

			// Verify ClusterExternalDNS properties
			var cedns *approutingv1alpha1.ClusterExternalDNS
			for _, obj := range result {
				if c, ok := obj.(*approutingv1alpha1.ClusterExternalDNS); ok {
					cedns = c
					break
				}
			}
			require.NotNil(t, cedns)
			require.Equal(t, defaultDomainDNSResourceName, cedns.Name)
			require.Equal(t, tc.conf.NS, cedns.Namespace)
			require.Equal(t, tc.conf.NS, cedns.Spec.ResourceNamespace)
			require.Equal(t, []string{tc.conf.DefaultDomainZoneID}, cedns.Spec.DNSZoneResourceIDs)
			require.Equal(t, tc.expectedResourceTypes, cedns.Spec.ResourceTypes)
			require.Equal(t, approutingv1alpha1.IdentityTypeWorkloadIdentity, cedns.Spec.Identity.Type)
			require.Equal(t, defaultDomainServiceAccountName, cedns.Spec.Identity.ServiceAccount)
		})
	}
}

func TestDefaultDomainObjectsMSI(t *testing.T) {
	testCases := []struct {
		name             string
		conf             *config.Config
		expectedObjCount int
		hasNamespace     bool
	}{
		{
			name: "creates ClusterExternalDNS CR with MSI identity when WI is disabled",
			conf: &config.Config{
				NS:                      "test-namespace",
				DefaultDomainClientID:   "test-client-id",
				DefaultDomainZoneID:     "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
				EnabledWorkloadIdentity: false,
			},
			expectedObjCount: 2, // Namespace + ClusterExternalDNS
			hasNamespace:     true,
		},
		{
			name: "creates ClusterExternalDNS CR without namespace when kube-system",
			conf: &config.Config{
				NS:                      "kube-system",
				DefaultDomainClientID:   "test-client-id",
				DefaultDomainZoneID:     "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
				EnabledWorkloadIdentity: false,
			},
			expectedObjCount: 1, // ClusterExternalDNS only
			hasNamespace:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := defaultDomainObjects(tc.conf)
			require.Len(t, result, tc.expectedObjCount)

			// Should not contain a ServiceAccount
			for _, obj := range result {
				_, isSA := obj.(*corev1.ServiceAccount)
				require.False(t, isSA, "MSI mode should not create a WI-annotated ServiceAccount")
			}

			// Should contain a ClusterExternalDNS CR with MSI identity
			var cedns *approutingv1alpha1.ClusterExternalDNS
			for _, obj := range result {
				if c, ok := obj.(*approutingv1alpha1.ClusterExternalDNS); ok {
					cedns = c
					break
				}
			}
			require.NotNil(t, cedns)
			require.Equal(t, defaultDomainDNSResourceName, cedns.Name)
			require.Equal(t, []string{tc.conf.DefaultDomainZoneID}, cedns.Spec.DNSZoneResourceIDs)
			require.Equal(t, approutingv1alpha1.IdentityTypeManagedIdentity, cedns.Spec.Identity.Type)
			require.Equal(t, tc.conf.DefaultDomainClientID, cedns.Spec.Identity.ClientID)
			require.Empty(t, cedns.Spec.Identity.ServiceAccount, "MSI mode should not set ServiceAccount")

			if tc.hasNamespace {
				_, ok := result[0].(*corev1.Namespace)
				require.True(t, ok, "First object should be Namespace")
			}
		})
	}
}

func TestDefaultDomainObjectsOrdering(t *testing.T) {
	t.Run("WI mode: namespace is first when not kube-system", func(t *testing.T) {
		result := defaultDomainObjects(&config.Config{
			NS:                      "test-namespace",
			DefaultDomainClientID:   "test-client-id",
			DefaultDomainZoneID:     "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
			EnabledWorkloadIdentity: true,
		})
		require.Len(t, result, 3)

		_, ok := result[0].(*corev1.Namespace)
		require.True(t, ok)
		_, ok = result[1].(*corev1.ServiceAccount)
		require.True(t, ok)
		_, ok = result[2].(*approutingv1alpha1.ClusterExternalDNS)
		require.True(t, ok)
	})

	t.Run("WI mode: no namespace when kube-system", func(t *testing.T) {
		result := defaultDomainObjects(&config.Config{
			NS:                      "kube-system",
			DefaultDomainClientID:   "test-client-id",
			DefaultDomainZoneID:     "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
			EnabledWorkloadIdentity: true,
		})
		require.Len(t, result, 2)

		_, ok := result[0].(*corev1.ServiceAccount)
		require.True(t, ok)
		_, ok = result[1].(*approutingv1alpha1.ClusterExternalDNS)
		require.True(t, ok)
	})

	t.Run("MSI mode: namespace is first when not kube-system", func(t *testing.T) {
		result := defaultDomainObjects(&config.Config{
			NS:                      "test-namespace",
			DefaultDomainClientID:   "test-client-id",
			DefaultDomainZoneID:     "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
			EnabledWorkloadIdentity: false,
		})
		require.Len(t, result, 2)

		_, ok := result[0].(*corev1.Namespace)
		require.True(t, ok)
		_, ok = result[1].(*approutingv1alpha1.ClusterExternalDNS)
		require.True(t, ok)
	})
}
