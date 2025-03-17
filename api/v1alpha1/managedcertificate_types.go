package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make crd" to regenerate code after modifying this file
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ManagedCertificateSpec defines the desired state of ManagedCertificate.
type ManagedCertificateSpec struct {
	// Target defines the targets that the Certificate will be bound to.
	// +kubebuilder:validation:Required
	Target ManagedCertificateTarget `json:"target,omitempty"`

	// DnsZone defines the DNS Zone that the ManagedCertificate will be applied to.
	// +kubebuilder:validation:Required
	DnsZone ManagedCertificateDnsZone `json:"dnsZone,omitempty"`

	// DomainNames is a list of domain names that the Certificate will be issued for.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	DomainNames []string `json:"domainNames,omitempty"`

	// ServiceAccount is the name of the ServiceAccount that will be used to connect to the Azure DNS Zone.
	// +kubebuilder:validation:Required
	ServiceAccount string `json:"serviceAccount,omitempty"`
}

// ManagedCertificateTarget defines the targets that a Certificate will be bound to.
// +kubebuilder:validation:MinProperties=1
// +kubebuilder:validation:MaxProperties=1
type ManagedCertificateTarget struct {
	// Secret is the name of the Secret that will contain the Certificate.
	Secret string `json:"secret,omitempty"`
}

// ManagedCertificateDnsZone defines the DNS Zone that a ManagedCertificate will be applied to.
type ManagedCertificateDnsZone struct {
	// ResourceId is the Azure Resource ID of the DNS Zone. Can be retrieved with `az network dns zone show -g <resource-group> -n <zone-name> --query id -o tsv`.
	ResourceId string `json:"resourceId,omitempty"`

	// below fields are needed for cross-tenant scenarios

	// TenantId is the Azure Tenant ID of the DNS Zone.
	// +kubebuilder:validation:Optional
	TenantId string `json:"tenantId,omitempty"`
	// ActiveDirectoryApplicationId is the base URL of the cloud's Azure Active Directory.
	// +kubebuilder:validation:Optional
	ActiveDirectoryAuthorityHost string `json:"activeDirectoryAuthorityHost,omitempty"`
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

	// +kubebuilder:validation:Required
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
