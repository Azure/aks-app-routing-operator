package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&DefaultDomainCertificate{}, &DefaultDomainCertificateList{})
}

// DefaultDomainCertificateSpec defines the desired state of DefaultDomainCertificate.
type DefaultDomainCertificateSpec struct {
	// Target is where the default domain certificate should be applied
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	// +kubebuilder:validation:Required
	Target DefaultDomainCertificateTarget `json:"target,omitempty"`
}

// DefaultDomainCertificateTarget defines the target for the default domain certificate
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type DefaultDomainCertificateTarget struct {
	// Secret is the name of the Secret that should contain the certificate. The default domain certificate will be reconciled in this Secret in the same namespace as the DefaultDomainCertificate resource.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Secret *string `json:"secret,omitempty"`
}

// DefaultDomainCertificateStatus defines the observed state of DefaultDomainCertificate.
type DefaultDomainCertificateStatus struct {
	// ExpirationTime is the time when the default domain certificate will expire. The certificate will be autorenewed before this time.
	ExpirationTime *metav1.Time `json:"expirationTime,omitempty"`
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
