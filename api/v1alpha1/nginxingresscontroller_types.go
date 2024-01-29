package v1alpha1

import (
	"context"
	"fmt"
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
	// MaxCollisions is the maximum number of collisions allowed when generating a name for a managed resource. This corresponds to the status.CollisionCount
	MaxCollisions = 5
)

// Important: Run "make crd" to regenerate code after modifying this file
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NginxIngressControllerSpec defines the desired state of NginxIngressController
type NginxIngressControllerSpec struct {
	// IngressClassName is the name of the IngressClass that will be used for the NGINX Ingress Controller. Defaults to metadata.name if
	// not specified.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:default:=nginx.approuting.kubernetes.azure.com
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
	// +kubebuilder:validation:Required
	IngressClassName string `json:"ingressClassName"`

	// ControllerNamePrefix is the name to use for the managed NGINX Ingress Controller resources.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=60
	// +kubebuilder:default:=nginx
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9]*[a-z0-9]$`
	// +kubebuilder:validation:Required
	ControllerNamePrefix string `json:"controllerNamePrefix"`

	// LoadBalancerAnnotations is a map of annotations to apply to the NGINX Ingress Controller's Service. Common annotations
	// will be from the Azure LoadBalancer annotations here https://cloud-provider-azure.sigs.k8s.io/topics/loadbalancer/#loadbalancer-annotations
	// +optional
	LoadBalancerAnnotations map[string]string `json:"loadBalancerAnnotations,omitempty"`

	// DefaultSSLCertificate is a struct with a secret with the fields namespace and name which is used to create the ssl certificate used by the default HTTPS server
	// +optional
	DefaultSSLCertificate DefaultSSLCertificate `json:"defaultSSLCertificate,omitempty"`
}

type DefaultSSLCertificate struct {
	Secret `json:"sslSecret"`
}

type Secret struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:default:=nginx.approuting.kubernetes.azure.com
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
	Name string `json:"secretName"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:default:=nginx.approuting.kubernetes.azure.com
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9\.]*[a-z0-9]$`
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
	// +kubebuilder:default:={"ingressClassName":"nginx.approuting.kubernetes.azure.com","controllerNamePrefix":"nginx"}
	Spec NginxIngressControllerSpec `json:"spec"` // ^ for the above thing https://github.com/kubernetes-sigs/controller-tools/issues/622 defaulting doesn't cascade, so we have to define it all. Comment on this line so it's not in crd spec.

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
