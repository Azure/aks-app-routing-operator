package v1alpha1

import (
	"github.com/Azure/aks-app-routing-operator/api"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConditionTypeExternalDNSDeploymentReady     = "ExternalDNSDeploymentReady"
	ConditionTypeExternalDNSDeploymentAvailable = "ExternalDNSDeploymentAvailable"
	ConditionTypeExternalDns
)

func init() {
	SchemeBuilder.Register(&ExternalDNS{}, &ExternalDNSList{})
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:resource:shortName=edc

// ExternalDNS allows users to specify desired the state of a namespace-scoped ExternalDNS configuration and includes information about the state of their resources in the form of Kubernetes events.
type ExternalDNS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalDNSSpec   `json:"spec,omitempty"`
	Status ExternalDNSStatus `json:"status,omitempty"`
}

func (e *ExternalDNS) GetCondition(conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(e.Status.Conditions, conditionType)
}

func (e *ExternalDNS) GetConditions() *[]metav1.Condition {
	return &e.Status.Conditions
}

func (e *ExternalDNS) GetGeneration() int64 {
	return e.Generation
}

func (e *ExternalDNS) SetCondition(condition metav1.Condition) {
	api.VerifyAndSetCondition(e, condition)
}

// ExternalDNSSpec allows users to specify desired the state of a namespace-scoped ExternalDNS configuration.
type ExternalDNSSpec struct {
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
	// +kubebuilder:validation:items:Enum:=ingress;gateway
	// +listType:=set
	ResourceTypes []string `json:"resourceTypes"`

	// Identity contains information about the identity that ExternalDNS will use to interface with Azure resources.
	// +kubebuilder:validation:Required
	Identity ExternalDNSIdentity `json:"identity"`

	// Filters contains optional filters that the ExternalDNS controller should use to determine which resources to manage.
	// +optional
	Filters ExternalDNSFilters `json:"filters,omitempty"`
}

// ExternalDNSIdentity contains information about the identity that ExternalDNS will use to interface with Azure resources.
type ExternalDNSIdentity struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`

	// ServiceAccount is the name of the Kubernetes ServiceAccount that ExternalDNS will use to interface with Azure resources. It must be in the same namespace as the ExternalDNS.
	ServiceAccount string `json:"serviceAccount"`
}

type ExternalDNSFilters struct {
	// GatewayLabels contains key-value pairs that the ExternalDNS controller will use to filter the Gateways that it manages.
	// +optional
	GatewayLabels map[string]string `json:"gatewayLabels,omitempty"`

	// RouteAndIngressLabels contains key-value pairs that the ExternalDNS controller will use to filter the HTTPRoutes and Ingresses that it manages.
	// +optional
	RouteAndIngressLabels map[string]string `json:"routeAndIngressLabels,omitempty"`
}

// ExternalDNSStatus defines the observed state of ExternalDNS.
type ExternalDNSStatus struct {
	// Conditions is an array of current observed conditions for the ExternalDNS
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

// ExternalDNSList contains a list of ExternalDNS.
type ExternalDNSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalDNS `json:"items"`
}
