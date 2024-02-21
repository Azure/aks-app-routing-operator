//go:build !ignore_autogenerated

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DefaultSSLCertificate) DeepCopyInto(out *DefaultSSLCertificate) {
	*out = *in
	if in.Secret != nil {
		in, out := &in.Secret, &out.Secret
		*out = new(Secret)
		**out = **in
	}
	if in.KeyVaultURI != nil {
		in, out := &in.KeyVaultURI, &out.KeyVaultURI
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DefaultSSLCertificate.
func (in *DefaultSSLCertificate) DeepCopy() *DefaultSSLCertificate {
	if in == nil {
		return nil
	}
	out := new(DefaultSSLCertificate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ManagedObjectReference) DeepCopyInto(out *ManagedObjectReference) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ManagedObjectReference.
func (in *ManagedObjectReference) DeepCopy() *ManagedObjectReference {
	if in == nil {
		return nil
	}
	out := new(ManagedObjectReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NginxIngressController) DeepCopyInto(out *NginxIngressController) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NginxIngressController.
func (in *NginxIngressController) DeepCopy() *NginxIngressController {
	if in == nil {
		return nil
	}
	out := new(NginxIngressController)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NginxIngressController) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NginxIngressControllerList) DeepCopyInto(out *NginxIngressControllerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]NginxIngressController, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NginxIngressControllerList.
func (in *NginxIngressControllerList) DeepCopy() *NginxIngressControllerList {
	if in == nil {
		return nil
	}
	out := new(NginxIngressControllerList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NginxIngressControllerList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NginxIngressControllerSpec) DeepCopyInto(out *NginxIngressControllerSpec) {
	*out = *in
	if in.LoadBalancerAnnotations != nil {
		in, out := &in.LoadBalancerAnnotations, &out.LoadBalancerAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.DefaultSSLCertificate != nil {
		in, out := &in.DefaultSSLCertificate, &out.DefaultSSLCertificate
		*out = new(DefaultSSLCertificate)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NginxIngressControllerSpec.
func (in *NginxIngressControllerSpec) DeepCopy() *NginxIngressControllerSpec {
	if in == nil {
		return nil
	}
	out := new(NginxIngressControllerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NginxIngressControllerStatus) DeepCopyInto(out *NginxIngressControllerStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ManagedResourceRefs != nil {
		in, out := &in.ManagedResourceRefs, &out.ManagedResourceRefs
		*out = make([]ManagedObjectReference, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NginxIngressControllerStatus.
func (in *NginxIngressControllerStatus) DeepCopy() *NginxIngressControllerStatus {
	if in == nil {
		return nil
	}
	out := new(NginxIngressControllerStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Secret) DeepCopyInto(out *Secret) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Secret.
func (in *Secret) DeepCopy() *Secret {
	if in == nil {
		return nil
	}
	out := new(Secret)
	in.DeepCopyInto(out)
	return out
}
