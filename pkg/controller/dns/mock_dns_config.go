package dns

import (
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type mockDnsConfig struct {
	tenantId            string
	inputServiceAccount string
	resourceNamespace   string
	inputResourceName   string
	resourceTypes       []string
	dnsZoneresourceIDs  []string
	filters             *v1alpha1.ExternalDNSFilters
	namespaced          bool
}

func (m mockDnsConfig) GetTenantId() string {
	return m.tenantId
}

func (m mockDnsConfig) GetInputServiceAccount() string {
	return m.inputServiceAccount
}

func (m mockDnsConfig) GetResourceNamespace() string {
	return m.resourceNamespace
}

func (m mockDnsConfig) GetInputResourceName() string {
	return m.inputResourceName
}

func (m mockDnsConfig) GetResourceTypes() []string {
	return m.resourceTypes
}

func (m mockDnsConfig) GetDnsZoneresourceIDs() []string {
	return m.dnsZoneresourceIDs
}

func (m mockDnsConfig) GetFilters() *v1alpha1.ExternalDNSFilters {
	return m.filters
}

func (m mockDnsConfig) GetNamespaced() bool {
	return m.namespaced
}

func (m mockDnsConfig) GetNamespace() string { return "" }

func (m mockDnsConfig) SetNamespace(namespace string) {}

func (m mockDnsConfig) GetName() string { return "" }

func (m mockDnsConfig) SetName(name string) {}

func (m mockDnsConfig) GetGenerateName() string { return "" }

func (m mockDnsConfig) SetGenerateName(name string) {}

func (m mockDnsConfig) GetUID() types.UID { return "" }

func (m mockDnsConfig) SetUID(uid types.UID) {}

func (m mockDnsConfig) GetResourceVersion() string { return "" }

func (m mockDnsConfig) SetResourceVersion(version string) {}

func (m mockDnsConfig) GetGeneration() int64 { return 0 }

func (m mockDnsConfig) SetGeneration(generation int64) {}

func (m mockDnsConfig) GetSelfLink() string { return "" }

func (m mockDnsConfig) SetSelfLink(selfLink string) {}

func (m mockDnsConfig) GetCreationTimestamp() metav1.Time { return metav1.Time{} }

func (m mockDnsConfig) SetCreationTimestamp(timestamp metav1.Time) {}

func (m mockDnsConfig) GetDeletionTimestamp() *metav1.Time { return nil }

func (m mockDnsConfig) SetDeletionTimestamp(timestamp *metav1.Time) {}

func (m mockDnsConfig) GetDeletionGracePeriodSeconds() *int64 { return nil }

func (m mockDnsConfig) SetDeletionGracePeriodSeconds(i *int64) {}

func (m mockDnsConfig) GetLabels() map[string]string { return nil }

func (m mockDnsConfig) SetLabels(labels map[string]string) {}

func (m mockDnsConfig) GetAnnotations() map[string]string { return nil }

func (m mockDnsConfig) SetAnnotations(annotations map[string]string) {}

func (m mockDnsConfig) GetFinalizers() []string { return nil }

func (m mockDnsConfig) SetFinalizers(finalizers []string) {}

func (m mockDnsConfig) GetOwnerReferences() []metav1.OwnerReference { return nil }

func (m mockDnsConfig) SetOwnerReferences(references []metav1.OwnerReference) {}

func (m mockDnsConfig) GetManagedFields() []metav1.ManagedFieldsEntry { return nil }

func (m mockDnsConfig) SetManagedFields(managedFields []metav1.ManagedFieldsEntry) {}

func (m mockDnsConfig) GetObjectKind() schema.ObjectKind { return nil }

func (m mockDnsConfig) DeepCopyObject() runtime.Object { return nil }
