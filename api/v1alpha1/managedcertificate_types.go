package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ManagedCertificateSpec defines the desired state of ManagedCertificate.
type ManagedCertificateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ManagedCertificate. Edit managedcertificate_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ManagedCertificateStatus defines the observed state of ManagedCertificate.
type ManagedCertificateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ManagedCertificate is the Schema for the managedcertificates API.
type ManagedCertificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedCertificateSpec   `json:"spec,omitempty"`
	Status ManagedCertificateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ManagedCertificateList contains a list of ManagedCertificate.
type ManagedCertificateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedCertificate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedCertificate{}, &ManagedCertificateList{})
}
