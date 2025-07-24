package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DefaultDomainCertificateSpec defines the desired state of DefaultDomainCertificate.
type DefaultDomainCertificateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of DefaultDomainCertificate. Edit defaultdomaincertificate_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// DefaultDomainCertificateStatus defines the observed state of DefaultDomainCertificate.
type DefaultDomainCertificateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DefaultDomainCertificate is the Schema for the defaultdomaincertificates API.
type DefaultDomainCertificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultDomainCertificateSpec   `json:"spec,omitempty"`
	Status DefaultDomainCertificateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DefaultDomainCertificateList contains a list of DefaultDomainCertificate.
type DefaultDomainCertificateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultDomainCertificate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DefaultDomainCertificate{}, &DefaultDomainCertificateList{})
}
