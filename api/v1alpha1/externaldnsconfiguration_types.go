package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&ExternalDNSConfiguration{}, &ExternalDNSConfigurationList{})
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ExternalDNSConfiguration is the Schema for the externaldnsconfigurations API.
type ExternalDNSConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalDNSConfigurationSpec   `json:"spec,omitempty"`
	Status ExternalDNSConfigurationStatus `json:"status,omitempty"`
}

// ExternalDNSConfigurationSpec defines the desired state of ExternalDNSConfiguration.
type ExternalDNSConfigurationSpec struct {
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

	// +optional
	Filters ExternalDNSConfigurationFilters `json:"filters,omitempty"`
}

type ExternalDNSConfigurationIdentity struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
	ServiceAccount string `json:"serviceAccount"`
}

type ExternalDNSConfigurationFilters struct {
	GatewayLabels map[string]string `json:"gatewayLabels,omitempty"`
	RouteLabels   map[string]string `json:"routeLabels,omitempty"`
}

// ExternalDNSConfigurationStatus defines the observed state of ExternalDNSConfiguration.
type ExternalDNSConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// Conditions is an array of current observed conditions for the ExternalDNSConfiguration
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions"`

	ExternalDNSReadyReplicas       int32 `json:"externalDNSReadyReplicas"`
	ExternalDNSUnavailableReplicas int32 `json:"externalDNSUnavailableReplicas"`

	// Count of hash collisions for the managed resources. The App Routing Operator uses this field
	// as a collision avoidance mechanism when it needs to create the name for the managed resources.
	// +optional
	// +kubebuilder:validation:Maximum=5
	CollisionCount int32 `json:"collisionCount"`

	// ManagedResourceRefs is a list of references to the managed resources
	// +optional
	ManagedResourceRefs []ManagedObjectReference `json:"managedResourceRefs,omitempty"`
}

// +kubebuilder:object:root=true

// ExternalDNSConfigurationList contains a list of ExternalDNSConfiguration.
type ExternalDNSConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalDNSConfiguration `json:"items"`
}
