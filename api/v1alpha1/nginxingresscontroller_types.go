package v1alpha1

import (
	"context"
	"fmt"
	"unicode"

	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	SchemeBuilder.Register(&NginxIngressController{}, &NginxIngressControllerList{})
}

const (
	maxNameLength = 100
	// MaxCollisions is the maximum number of collisions allowed when generating a name for a managed resource. This corresponds to the status.CollisionCount
	MaxCollisions           = 5
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

	// DefaultSSLCertificate is a string in the form of namespace/name or a keyvault uri that is used to create the default ssl certificate used by the default HTTPS server
	DefaultSSLCertificate struct {
		Secret SSLSecret `json:"sslSecret"`
	}
}

type SSLSecret struct {
	Name      string `json:"secretName"`
	Namespace string `json:"secretNamespace"`
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
	// +kubebuilder:validation:Maximum=5
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

	if !isLowercaseRfc1123Subdomain(n.Spec.ControllerNamePrefix) {
		return "spec.controllerNamePrefix " + lowercaseRfc1123SubdomainValidationFailReason
	}

	if len(n.Spec.ControllerNamePrefix) > maxControllerNamePrefix {
		return fmt.Sprintf("spec.controllerNamePrefix length must be less than or equal to %d characters", maxControllerNamePrefix)

	}

	if n.Spec.IngressClassName == "" {
		return "spec.ingressClassName must be specified"
	}

	if !isLowercaseRfc1123Subdomain(n.Spec.IngressClassName) {
		return "spec.ingressClassName " + lowercaseRfc1123SubdomainValidationFailReason
	}

	if len(n.Name) > maxNameLength {
		return fmt.Sprintf("Name length must be less than or equal to %d characters", maxNameLength)
	}

	return ""
}

// Default sets default spec values for this NginxIngressController
func (n *NginxIngressController) Default() {
	if n.Spec.IngressClassName == "" {
		n.Spec.IngressClassName = n.Name
	}

	if n.Spec.ControllerNamePrefix == "" {
		n.Spec.ControllerNamePrefix = defaultControllerNamePrefix
	}
}

// Collides returns whether the fields in this NginxIngressController would collide with an existing resources making it
// impossible for this NginxIngressController to become available. This should be run before an NginxIngressController is created.
// Returns whether there's a collision, the collision reason, and an error if one occurred. The collision reason is something that
// the user can use to understand and resolve.
func (n *NginxIngressController) Collides(ctx context.Context, cl client.Client) (bool, string, error) {
	lgr := logr.FromContextOrDiscard(ctx).WithValues("name", n.Name, "ingressClassName", n.Spec.IngressClassName)
	lgr.Info("checking for NginxIngressController collisions")

	// check for NginxIngressController collisions
	lgr.Info("checking for NginxIngressController collision")
	var nginxIngressControllerList NginxIngressControllerList
	if err := cl.List(ctx, &nginxIngressControllerList); err != nil {
		lgr.Error(err, "listing NginxIngressControllers")
		return false, "", fmt.Errorf("listing NginxIngressControllers: %w", err)
	}

	for _, nic := range nginxIngressControllerList.Items {
		if nic.Spec.IngressClassName == n.Spec.IngressClassName && nic.Name != n.Name {
			lgr.Info("NginxIngressController collision found")
			return true, fmt.Sprintf("spec.ingressClassName \"%s\" is invalid because NginxIngressController \"%s\" already uses IngressClass \"%[1]s\"", n.Spec.IngressClassName, nic.Name), nil
		}
	}

	// Check for an IngressClass collision.
	// This is purposefully after the NginxIngressController check because if the collision is through an NginxIngressController
	// that's the one we want to report as the reason since the user action for fixing that would involve working with the NginxIngressController
	// resource rather than the IngressClass resource.
	lgr.Info("checking for IngressClass collision")
	ic := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: n.Spec.IngressClassName,
		},
	}
	err := cl.Get(ctx, types.NamespacedName{Name: ic.Name}, ic)
	if err == nil {
		lgr.Info("IngressClass collision found")
		return true, fmt.Sprintf("spec.ingressClassName \"%s\" is invalid because IngressClass \"%[1]s\" already exists", n.Spec.IngressClassName), nil
	}
	if !k8serrors.IsNotFound(err) {
		lgr.Error(err, "checking for IngressClass collisions")
		return false, "", fmt.Errorf("checking for IngressClass collisions: %w", err)
	}

	return false, "", nil
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster

// NginxIngressControllerList contains a list of NginxIngressController
type NginxIngressControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NginxIngressController `json:"items"`
}

var lowercaseRfc1123SubdomainValidationFailReason = "must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"

func isLowercaseRfc1123Subdomain(s string) bool {
	if !startsWithAlphaNum(s) {
		return false
	}

	if !endsWithAlphaNum(s) {
		return false
	}

	if !onlyAlphaNumDashPeriod(s) {
		return false
	}

	if !isLower(s) {
		return false
	}

	return true
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

func isLower(s string) bool {
	for _, c := range s {
		if unicode.IsUpper(c) && unicode.IsLetter(c) {
			return false
		}
	}

	return true
}
