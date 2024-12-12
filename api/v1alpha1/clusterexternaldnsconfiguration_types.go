package v1alpha1

import (
	"github.com/Azure/aks-app-routing-operator/api"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName=cedc

// ClusterExternalDNSConfiguration allows users to specify desired the state of a cluster-scoped ExternalDNS configuration and includes information about the state of their resources in the form of Kubernetes events.
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

// ClusterExternalDNSConfigurationSpec allows users to specify desired the state of a cluster-scoped ExternalDNS configuration.
type ClusterExternalDNSConfigurationSpec struct {
	// TenantID is the ID of the Azure tenant where the DNS zones are located.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format:=uuid
	// +kubebuilder:validation:Pattern=`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`
	TenantID string `json:"tenantID"`

	// DNSZoneResourceIDs is a list of Azure Resource IDs of the DNS zones that the ExternalDNS controller should manage. These should be in the same resource group and be of the same type (public or private).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems:=1
	// +kubebuilder:validation:MaxItems:=20
	// +kubebuilder:validation:items:Pattern:=`(?i)\/subscriptions\/(.{36})\/resourcegroups\/(.+?)\/providers\/Microsoft.network\/(dnszones|privatednszones)\/(.+)`
	// +listType:=set
	DNSZoneResourceIDs []string `json:"dnsZoneResourceIDs"`

	// ResourceTypes is a list of Kubernetes resource types that the ExternalDNS controller should manage. The supported resource types are 'ingress' and 'gateway'.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems:=1
	// +kubebuilder:validation:items:enum:=ingress;gateway
	// +listType:=set
	ResourceTypes []string `json:"resourceTypes"`

	// Identity contains information about the identity that ExternalDNS will use to interface with Azure resources.
	// +kubebuilder:validation:Required
	Identity ExternalDNSConfigurationIdentity `json:"identity"`

	// ResourceNamespace is the namespace where the ExternalDNS resources will be deployed by app routing.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
	ResourceNamespace string `json:"resourceNamespace"`

	// Filters contains optional filters that the ExternalDNS controller should use to determine which resources to manage.
	// +optional
	Filters ClusterExternalDNSConfigurationFilters `json:"filters,omitempty"`
}

type ClusterExternalDNSConfigurationFilters struct {
	// GatewayNamespace is the namespace where ExternalDNS should look for Gateway resources to manage.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
	// +optional
	GatewayNamespace string `json:"gatewayNamespace,omitempty"`

	// RouteNamespace is the namespace where ExternalDNS should look for HTTPRoute resources to manage.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
	RouteNamespace string `json:"routeNamespace,omitempty"`

	ExternalDNSConfigurationFilters `json:",inline"`
}

// ClusterExternalDNSConfigurationStatus contains information about the state of the managed ExternalDNS resources.
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
