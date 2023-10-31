package v1alpha1

import (
	"fmt"
	"unicode"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&NginxIngressController{}, &NginxIngressControllerList{})
}

const (
	maxNameLength           = 100
	maxControllerNamePrefix = 253 - 10 // 253 is the max length of resource names - 10 to account for the length of the suffix https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
)

const (
	defaultControllerNamePrefix = "nginx"
)

// Important: Run "make crd" to regenerate code after modifying this file
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NginxIngressControllerSpec defines the desired state of NginxIngressController
type NginxIngressControllerSpec struct {
	// IngressClassName is the name of the IngressClass that will be used for the NGINX Ingress Controller. Defaults to metadata.name if
	// not specified.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	IngressClassName string `json:"ingressClassName,omitempty"`

	// ControllerNamePrefix is the name to use for the managed NGINX Ingress Controller resources.
	// +optional
	// +kubebuilder:default=nginx
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	ControllerNamePrefix string `json:"controllerNamePrefix,omitempty"`

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

const (
	// ConditionTypeAvailable indicates whether the NGINX Ingress Controller is available. Its condition status is one of
	// - "True" when the NGINX Ingress Controller is available and can be used
	// - "False" when the NGINX Ingress Controller is not available and cannot offer full functionality
	// - "Unknown" when the NGINX Ingress Controller's availability cannot be determined
	ConditionTypeAvailable = "Available"

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
//+kubebuilder:printcolumn:name="IngressClass",type="string",JSONPath=`.spec.ingressClassName`
//+kubebuilder:printcolumn:name="ControllerNamePrefix",type="string",JSONPath=`.spec.controllerNamePrefix`
//+kubebuilder:printcolumn:name="Available",type="string",JSONPath=`.status.conditions[?(@.type=="Available")].status`

// NginxIngressController is the Schema for the nginxingresscontrollers API
type NginxIngressController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +required
	Spec NginxIngressControllerSpec `json:"spec,omitempty"`

	// +optional
	Status NginxIngressControllerStatus `json:"status,omitempty"`
}

func (n *NginxIngressController) GetCondition(t string) *metav1.Condition {
	return meta.FindStatusCondition(n.Status.Conditions, t)
}

func (n *NginxIngressController) SetCondition(c metav1.Condition) {
	current := n.GetCondition(c.Type)

	if current != nil && current.Status == c.Status && current.Message == c.Message && current.Reason == c.Reason {
		current.ObservedGeneration = n.Generation
		return
	}

	c.ObservedGeneration = n.Generation
	c.LastTransitionTime = metav1.Now()
	meta.SetStatusCondition(&n.Status.Conditions, c)
}

// Valid checks this NginxIngressController to see if it's valid. Returns a string describing the validation error, if any, or empty string if there is no error.
func (n *NginxIngressController) Valid() string {
	// controller name prefix must follow https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
	// we don't check for ending because this is a prefix
	if n.Spec.ControllerNamePrefix == "" {
		return "spec.controllerNamePrefix must be specified"
	}

	if !startsWithAlphaNum(n.Spec.ControllerNamePrefix) {
		return "spec.controllerNamePrefix must start with alphanumeric character"
	}

	if !onlyAlphaNumDashPeriod(n.Spec.ControllerNamePrefix) {
		return "spec.controllerNamePrefix must contain only alphanumeric characters, dashes, and periods"
	}

	if len(n.Spec.ControllerNamePrefix) > maxControllerNamePrefix {
		return fmt.Sprintf("spec.controllerNamePrefix length must be less than or equal to %d characters", maxControllerNamePrefix)

	}

	// ingress class  name must follow https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
	if n.Spec.IngressClassName == "" {
		return "spec.ingressClassName must be specified"
	}

	if !startsWithAlphaNum(n.Spec.IngressClassName) {
		return "spec.ingressClassName must start with alphanumeric character"
	}

	if !onlyAlphaNumDashPeriod(n.Spec.IngressClassName) {
		return "spec.ingressClassName must contain only alphanumeric characters, dashes, and periods"
	}

	if !endsWithAlphaNum(n.Spec.IngressClassName) {
		return "spec.ingressClassName must end with alphanumeric character"
	}

	if len(n.Name) > maxNameLength {
		return fmt.Sprintf("Name length must be less than or equal to %d characters", maxNameLength)
	}

	return ""
}

func (n *NginxIngressController) Default() {
	if n.Spec.IngressClassName == "" {
		n.Spec.IngressClassName = n.Name
	}

	if n.Spec.ControllerNamePrefix == "" {
		n.Spec.ControllerNamePrefix = defaultControllerNamePrefix
	}
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster

// NginxIngressControllerList contains a list of NginxIngressController
type NginxIngressControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NginxIngressController `json:"items"`
}

func startsWithAlphaNum(s string) bool {
	if len(s) == 0 {
		return false
	}

	return unicode.IsLetter(rune(s[0])) || unicode.IsDigit(rune(s[0]))
}

func endsWithAlphaNum(s string) bool {
	if len(s) == 0 {
		return false
	}

	return unicode.IsLetter(rune(s[len(s)-1])) || unicode.IsDigit(rune(s[len(s)-1]))
}

func onlyAlphaNumDashPeriod(s string) bool {
	for _, c := range s {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '.' {
			return false
		}
	}

	return true
}
