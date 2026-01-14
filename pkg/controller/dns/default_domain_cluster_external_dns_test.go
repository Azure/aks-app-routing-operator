package dns

import (
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
					Name:      defaultDomainDNSResourceName,
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
					Name:      defaultDomainDNSResourceName,
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
			name: "creates ClusterExternalDNS with ingress only when gateway disabled",
			conf: &config.Config{
				NS:                         "test-namespace",
				DefaultDomainZoneID:        "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
				EnableDefaultDomainGateway: false,
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
						ServiceAccount: defaultDomainDNSResourceName,
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
						ServiceAccount: defaultDomainDNSResourceName,
					},
				},
			},
		},
		{
			name: "creates ClusterExternalDNS with different zone ID and gateway enabled",
			conf: &config.Config{
				NS:                         "prod-namespace",
				DefaultDomainZoneID:        "/subscriptions/prod-sub/resourceGroups/prod-rg/providers/Microsoft.Network/dnszones/prod.example.com",
				EnableDefaultDomainGateway: true,
			},
			expected: &approutingv1alpha1.ClusterExternalDNS{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultDomainDNSResourceName,
					Namespace: "prod-namespace",
					Labels:    manifests.GetTopLevelLabels(),
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterExternalDNS",
					APIVersion: approutingv1alpha1.GroupVersion.String(),
				},
				Spec: approutingv1alpha1.ClusterExternalDNSSpec{
					ResourceName:       defaultDomainDNSResourceName,
					ResourceNamespace:  "prod-namespace",
					DNSZoneResourceIDs: []string{"/subscriptions/prod-sub/resourceGroups/prod-rg/providers/Microsoft.Network/dnszones/prod.example.com"},
					ResourceTypes:      []string{"ingress", "gateway"},
					Identity: approutingv1alpha1.ExternalDNSIdentity{
						ServiceAccount: defaultDomainDNSResourceName,
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

func TestDefaultDomainObjects(t *testing.T) {
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
			},
			expectedObjCount:      2,
			expectedTypes:         []string{"ServiceAccount", "ClusterExternalDNS"},
			hasNamespace:          false,
			expectedResourceTypes: []string{"ingress"},
		},
		{
			name: "creates all objects with namespace for custom namespace",
			conf: &config.Config{
				NS:                         "custom-ns",
				DefaultDomainClientID:      "custom-client-id",
				DefaultDomainZoneID:        "/subscriptions/custom-sub/resourceGroups/custom-rg/providers/Microsoft.Network/dnszones/custom.example.com",
				EnableDefaultDomainGateway: false,
			},
			expectedObjCount:      3,
			expectedTypes:         []string{"Namespace", "ServiceAccount", "ClusterExternalDNS"},
			hasNamespace:          true,
			expectedResourceTypes: []string{"ingress"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := defaultDomainObjects(tc.conf)

			// Verify object count
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

			// Verify namespace is first if present
			if tc.hasNamespace {
				_, ok := result[0].(*corev1.Namespace)
				require.True(t, ok, "First object should be Namespace when NS is not kube-system")
			}

			// Verify ServiceAccount properties
			var serviceAccount *corev1.ServiceAccount
			for _, obj := range result {
				if sa, ok := obj.(*corev1.ServiceAccount); ok {
					serviceAccount = sa
					break
				}
			}
			require.NotNil(t, serviceAccount)
			require.Equal(t, defaultDomainDNSResourceName, serviceAccount.Name)
			require.Equal(t, tc.conf.NS, serviceAccount.Namespace)
			require.Equal(t, tc.conf.DefaultDomainClientID, serviceAccount.Annotations["azure.workload.identity/client-id"])

			// Verify ClusterExternalDNS properties
			var clusterExternalDNS *approutingv1alpha1.ClusterExternalDNS
			for _, obj := range result {
				if cedns, ok := obj.(*approutingv1alpha1.ClusterExternalDNS); ok {
					clusterExternalDNS = cedns
					break
				}
			}
			require.NotNil(t, clusterExternalDNS)
			require.Equal(t, defaultDomainDNSResourceName, clusterExternalDNS.Name)
			require.Equal(t, tc.conf.NS, clusterExternalDNS.Namespace)
			require.Equal(t, tc.conf.NS, clusterExternalDNS.Spec.ResourceNamespace)
			require.Equal(t, []string{tc.conf.DefaultDomainZoneID}, clusterExternalDNS.Spec.DNSZoneResourceIDs)
			require.Equal(t, tc.expectedResourceTypes, clusterExternalDNS.Spec.ResourceTypes)
			require.Equal(t, defaultDomainDNSResourceName, clusterExternalDNS.Spec.Identity.ServiceAccount)
		})
	}
}

func TestDefaultDomainObjectsOrdering(t *testing.T) {
	t.Run("namespace is first when not kube-system", func(t *testing.T) {
		conf := &config.Config{
			NS:                    "test-namespace",
			DefaultDomainClientID: "test-client-id",
			DefaultDomainZoneID:   "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
		}

		result := defaultDomainObjects(conf)
		require.Len(t, result, 3)

		// First should be namespace
		_, ok := result[0].(*corev1.Namespace)
		require.True(t, ok)

		// Second should be service account
		_, ok = result[1].(*corev1.ServiceAccount)
		require.True(t, ok)

		// Third should be ClusterExternalDNS
		_, ok = result[2].(*approutingv1alpha1.ClusterExternalDNS)
		require.True(t, ok)
	})

	t.Run("correct ordering when kube-system", func(t *testing.T) {
		conf := &config.Config{
			NS:                    "kube-system",
			DefaultDomainClientID: "test-client-id",
			DefaultDomainZoneID:   "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
		}

		result := defaultDomainObjects(conf)
		require.Len(t, result, 2)

		// First should be service account
		_, ok := result[0].(*corev1.ServiceAccount)
		require.True(t, ok)

		// Second should be ClusterExternalDNS
		_, ok = result[1].(*approutingv1alpha1.ClusterExternalDNS)
		require.True(t, ok)
	})
}

func TestDefaultDomainObjectsImplementsClientObject(t *testing.T) {
	t.Run("all returned objects implement client.Object", func(t *testing.T) {
		conf := &config.Config{
			NS:                    "test-namespace",
			DefaultDomainClientID: "test-client-id",
			DefaultDomainZoneID:   "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Network/dnszones/example.com",
		}

		result := defaultDomainObjects(conf)

		for i, obj := range result {
			_, ok := obj.(client.Object)
			require.True(t, ok, "Object at index %d should implement client.Object", i)
		}
	})
}
