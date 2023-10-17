package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make generate" to regenerate code after modifying this file
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NginxIngressControllerSpec defines the desired state of NginxIngressController
type NginxIngressControllerSpec struct {
	// ControllerNamespace is the namespace where the NGINX Ingress Controller's required resources are deployed
	// +optional
	// +kubebuilder:default=app-routing-system
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	ControllerNamespace string `json:"controllerNamespace,omitempty"`

	// IngressClassName is the name of the IngressClass that will be used for the NGINX Ingress Controller. Defaults to metadata.name if
	// not specified.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"

	IngressClassName string `json:"ingressClassName,omitempty"`

	// ControllerName is the name to use for the managed NGINX Ingress Controller resources. This will be used as the name unless
	// there's a collision in which case it will be used as a prefix.
	// +optional
	// +kubebuilder:default=nginx
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	ControllerName string `json:"controllerName,omitempty"`

	// LoadBalancerAnnotations is a map of annotations to apply to the NGINX Ingress Controller's Service. Common annotations
	// will be from the Azure LoadBalancer annotations here https://cloud-provider-azure.sigs.k8s.io/topics/loadbalancer/#loadbalancer-annotations
	// +optional
	LoadBalancerAnnotations map[string]string `json:"loadBalancerAnnotations,omitempty"`
}

// NginxIngressControllerStatus defines the observed state of NginxIngressController
type NginxIngressControllerStatus struct {
	// Conditions is an array of current observed conditions for the NGINX Ingress Controller
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions"`

	// ControllerReplicas is the desired number of replicas of the NGINX Ingress Controller
	// +optional
	ControllerReplicas int32 `json:"controllerReplicas"`

	// ControllerReadyReplicas is the number of ready replicas of the NGINX Ingress Controller deployment
	// +optional
	ControllerReadyReplicas int32 `json:"controllerReadyReplicas"`

	// ControllerAvailableReplicas is the number of available replicas of the NGINX Ingress Controller deployment
	// +optional
	ControllerAvailableReplicas int32 `json:"controllerAvailableReplicas"`

	// ControllerUnavailableReplicas is the number of unavailable replicas of the NGINX Ingress Controller deployment
	// +optional
	ControllerUnavailableReplicas int32 `json:"controllerUnavailableReplicas"`

	// Count of hash collisions for the managed resources. The App Routing Operator uses this field
	// as a collision avoidance mechanism when it needs to create the name for the managed resources.
	// +optional
	CollisionCount int32 `json:"collisionCount"`

	// ManagedResourceRefs is a list of references to the managed resources
	// +optional
	ManagedResourceRefs []ManagedObjectReference `json:"managedResourceRefs,omitempty"`
}

// nginxIngressControllerConditionType defines a specific condition of a NginxIngressController
type nginxIngressControllerConditionType string

const (
	// ConditionTypeAvailable indicates whether the NGINX Ingress Controller is available. Its condition status is one of
	// - "True" when the NGINX Ingress Controller is available and can be used
	// - "False" when the NGINX Ingress Controller is not available and cannot offer full functionality
	// - "Unknown" when the NGINX Ingress Controller's availability cannot be determined
	ConditionTypeAvailable nginxIngressControllerConditionType = "Available"

	// ConditionTypeIngressClassReady indicates whether the IngressClass exists. Its condition status is one of
	// - "True" when the IngressClass exists
	// - "False" when the IngressClass does not exist
	// - "Collision" when the IngressClass exists, but it's not owned by the NginxIngressController.
	// - "Unknown" when the IngressClass's existence cannot be determined
	ConditionTypeIngressClassReady = "IngressClassReady"

	// ConditionTypeControllerAvailable indicates whether the NGINX Ingress Controller deployment is available. Its condition status is one of
	// - "True" when the NGINX Ingress Controller deployment is available
	// - "False" when the NGINX Ingress Controller deployment is not available
	// - "Unknown" when the NGINX Ingress Controller deployment's availability cannot be determined
	ConditionTypeControllerAvailable = "ControllerAvailable"

	// ConditionTypeProgressing indicates whether the NGINX Ingress Controller availability is progressing. Its condition status is one of
	// - "True" when the NGINX Ingress Controller availability is progressing
	// - "False" when the NGINX Ingress Controller availability is not progressing
	// - "Unknown" when the NGINX Ingress Controller availability's progress cannot be determined
	ConditionTypeProgressing = "Progressing"
)

// ManagedObjectReference is a reference to an object
type ManagedObjectReference struct {
	// Name is the name of the managed object
	Name string `json:"name"`

	// Namespace is the namespace of the managed object. If not specified, the resource is cluster-scoped
	// +optional
	Namespace string `json:"namespace"`

	// Kind is the kind of the managed object
	Kind string `json:"kind"`

	// APIGroup is the API group of the managed object. If not specified, the resource is in the core API group
	// +optional
	APIGroup string `json:"apiGroup"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName=nic

// NginxIngressController is the Schema for the nginxingresscontrollers API
type NginxIngressController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +required
	Spec NginxIngressControllerSpec `json:"spec,omitempty"`

	// +optional
	Status NginxIngressControllerStatus `json:"status,omitempty"`
}

func (n *NginxIngressController) GetCondition(t nginxIngressControllerConditionType) *metav1.Condition {
	return meta.FindStatusCondition(n.Status.Conditions, string(t))
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster

// NginxIngressControllerList contains a list of NginxIngressController
type NginxIngressControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NginxIngressController `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NginxIngressController{}, &NginxIngressControllerList{})
}