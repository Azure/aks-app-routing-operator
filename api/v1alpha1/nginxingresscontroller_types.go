package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NginxIngressControllerSpec defines the desired state of NginxIngressController
type NginxIngressControllerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make generate" to regenerate code after modifying this file

	// Foo is an example field of NginxIngressController. Edit nginxingresscontroller_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// NginxIngressControllerStatus defines the observed state of NginxIngressController
type NginxIngressControllerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make generate" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// NginxIngressController is the Schema for the nginxingresscontrollers API
type NginxIngressController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NginxIngressControllerSpec   `json:"spec,omitempty"`
	Status NginxIngressControllerStatus `json:"status,omitempty"`
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
