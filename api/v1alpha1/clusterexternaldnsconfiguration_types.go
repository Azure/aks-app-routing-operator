package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClusterExternalDNSConfigurationSpec defines the desired state of ClusterExternalDNSConfiguration.
type ClusterExternalDNSConfigurationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ClusterExternalDNSConfiguration. Edit clusterexternaldnsconfiguration_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ClusterExternalDNSConfigurationStatus defines the observed state of ClusterExternalDNSConfiguration.
type ClusterExternalDNSConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ClusterExternalDNSConfiguration is the Schema for the clusterexternaldnsconfigurations API.
type ClusterExternalDNSConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterExternalDNSConfigurationSpec   `json:"spec,omitempty"`
	Status ClusterExternalDNSConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterExternalDNSConfigurationList contains a list of ClusterExternalDNSConfiguration.
type ClusterExternalDNSConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterExternalDNSConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterExternalDNSConfiguration{}, &ClusterExternalDNSConfigurationList{})
}
