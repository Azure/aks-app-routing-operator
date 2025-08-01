package v1alpha1

import (
	"github.com/Azure/aks-app-routing-operator/api"
	"k8s.io/apimachinery/pkg/api/meta"
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

	// Conditions is an array of current observed conditions for the DefaultDomainCertificate.
	// Conditions can include:
	// - "Available": Indicates if the default domain certificate target is populated and ready to be used.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

const (
	// DefaultDomainCertificateConditionTypeAvailable indicates whether the default domain certificate is available.
	DefaultDomainCertificateConditionTypeAvailable = "Available"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DefaultDomainCertificate is the Schema for the defaultdomaincertificates API.
type DefaultDomainCertificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultDomainCertificateSpec   `json:"spec,omitempty"`
	Status DefaultDomainCertificateStatus `json:"status,omitempty"`
}

func (d *DefaultDomainCertificate) GetConditions() *[]metav1.Condition {
	return &d.Status.Conditions
}

func (d *DefaultDomainCertificate) GetCondition(t string) *metav1.Condition {
	return meta.FindStatusCondition(d.Status.Conditions, t)
}

func (d *DefaultDomainCertificate) SetCondition(c metav1.Condition) {
	api.VerifyAndSetCondition(d, c)
}

// +kubebuilder:object:root=true

// DefaultDomainCertificateList contains a list of DefaultDomainCertificate.
type DefaultDomainCertificateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefaultDomainCertificate `json:"items"`
}
