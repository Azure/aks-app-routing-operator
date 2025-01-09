package v1alpha1

import (
	"github.com/Azure/aks-app-routing-operator/api"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterExternalDNS allows users to specify desired the state of a cluster-scoped ExternalDNS deployment and includes information about the state of their resources in the form of Kubernetes events.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cedns
type ClusterExternalDNS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterExternalDNSSpec   `json:"spec,omitempty"`
	Status ClusterExternalDNSStatus `json:"status,omitempty"`
}

func (c *ClusterExternalDNS) GetCondition(conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(c.Status.Conditions, conditionType)
}

func (c *ClusterExternalDNS) GetConditions() *[]metav1.Condition {
	return &c.Status.Conditions
}

func (c *ClusterExternalDNS) GetGeneration() int64 {
	return c.Generation
}

func (c *ClusterExternalDNS) SetCondition(condition metav1.Condition) {
	api.VerifyAndSetCondition(c, condition)
}

// ClusterExternalDNSSpec allows users to specify desired the state of a cluster-scoped ExternalDNS deployment.
type ClusterExternalDNSSpec struct {
	// TenantID is the ID of the Azure tenant where the DNS zones are located.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format:=uuid
	// +kubebuilder:validation:Pattern=`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`
	TenantID string `json:"tenantID"`

	// DNSZoneResourceIDs is a list of Azure Resource IDs of the DNS zones that the ExternalDNS controller should manage. These must be in the same resource group and be of the same type (public or private).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems:=1
	// +kubebuilder:validation:MaxItems:=20
	// +kubebuilder:validation:items:Pattern:=`(?i)\/subscriptions\/(.{36})\/resourcegroups\/(.+?)\/providers\/Microsoft.network\/(dnszones|privatednszones)\/(.+)`
	// +listType:=set
	DNSZoneResourceIDs []string `json:"dnsZoneResourceIDs"`

	// ResourceTypes is a list of Kubernetes resource types that the ExternalDNS controller should manage. The supported resource types are 'ingress' and 'gateway'.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems:=1
	// +kubebuilder:validation:items:Enum:=ingress;gateway
	// +listType:=set
	ResourceTypes []string `json:"resourceTypes"`

	// Identity contains information about the identity that ExternalDNS will use to interface with Azure resources.
	// +kubebuilder:validation:Required
	Identity ExternalDNSIdentity `json:"identity"`

	// ResourceNamespace is the namespace where the ExternalDNS resources will be deployed by app routing.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
	ResourceNamespace string `json:"resourceNamespace"`

	// Filters contains optional filters that the ExternalDNS controller should use to determine which resources to manage.
	// +optional
	Filters ExternalDNSFilters `json:"filters,omitempty"`
}

// ClusterExternalDNSStatus contains information about the state of the managed ExternalDNS resources.
type ClusterExternalDNSStatus struct { // keeping these two separate for now in case cluster-wide needs to be different
	ExternalDNSStatus `json:",inline"`
}

// ClusterExternalDNSList contains a list of ClusterExternalDNS.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type ClusterExternalDNSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterExternalDNS `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterExternalDNS{}, &ClusterExternalDNSList{})
}
