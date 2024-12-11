package v1alpha1

import (
	"github.com/Azure/aks-app-routing-operator/api"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName=cedc

// ClusterExternalDNSConfiguration allows users to specify desired the state of a cluster-scoped ExternalDNS configuration.
type ClusterExternalDNSConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterExternalDNSConfigurationSpec   `json:"spec,omitempty"`
	Status ClusterExternalDNSConfigurationStatus `json:"status,omitempty"`
}

func (c *ClusterExternalDNSConfiguration) GetCondition(conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(c.Status.Conditions, conditionType)
}

func (c *ClusterExternalDNSConfiguration) GetConditions() *[]metav1.Condition {
	return &c.Status.Conditions
}

func (c *ClusterExternalDNSConfiguration) GetGeneration() int64 {
	return c.Generation
}

func (c *ClusterExternalDNSConfiguration) SetCondition(condition metav1.Condition) {
	api.VerifyAndSetCondition(c, condition)
}

// ClusterExternalDNSConfigurationSpec defines the desired state of ClusterExternalDNSConfiguration.
type ClusterExternalDNSConfigurationSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format:=uuid
	// +kubebuilder:validation:Pattern=`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`
	TenantID string `json:"tenantID"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems:=1
	// +kubebuilder:validation:MaxItems:=20
	// +kubebuilder:validation:items:Pattern:=`(?i)\/subscriptions\/(.{36})\/resourcegroups\/(.+?)\/providers\/Microsoft.network\/(dnszones|privatednszones)\/(.+)`
	// +listType:=set
	DNSZoneResourceIDs []string `json:"dnsZoneResourceIDs"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems:=1
	// +kubebuilder:validation:items:enum:=ingress;gateway
	// +listType:=set
	ResourceTypes []string `json:"resourceTypes"`

	// +kubebuilder:validation:Required
	Identity ExternalDNSConfigurationIdentity `json:"identity"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
	ResourceNamespace string `json:"resourceNamespace"`

	// +optional
	Filters ClusterExternalDNSConfigurationFilters `json:"filters,omitempty"`
}

type ClusterExternalDNSConfigurationFilters struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
	GatewayNamespace string `json:"gatewayNamespace,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
	RouteNamespace string `json:"routeNamespace,omitempty"`

	ExternalDNSConfigurationFilters `json:",inline"`
}

// ClusterExternalDNSConfigurationStatus defines the observed state of ClusterExternalDNSConfiguration.
type ClusterExternalDNSConfigurationStatus struct { // keeping these two separate for now in case cluster-wide needs to be different
	ExternalDNSConfigurationStatus `json:",inline"`
}

// +kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster

// ClusterExternalDNSConfigurationList contains a list of ClusterExternalDNSConfiguration.
type ClusterExternalDNSConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterExternalDNSConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterExternalDNSConfiguration{}, &ClusterExternalDNSConfigurationList{})
}
