/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package dhcp provides a shim for the metaldhcp DHCPLease type.
//
// metaldhcp (dhcp.metal.ironcore.dev) owns the DHCPLease CRD and will publish
// a versioned Go module once the implementation is stable (issue #1945).
// Until then, this package defines the minimal struct needed to watch
// DHCPLease CRs via controller-runtime.
//
// TODO: Replace with the real import once metaldhcp publishes a Go module:
//
//	import dhcpv1alpha1 "github.com/ironcore-dev/metaldhcp/api/v1alpha1"
package dhcp

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the group/version for metaldhcp types.
var GroupVersion = schema.GroupVersion{Group: "dhcp.metal.ironcore.dev", Version: "v1alpha1"}

// SchemeBuilder is used to add functions to this group's scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme adds the metaldhcp shim types to the provided scheme.
var AddToScheme = SchemeBuilder.AddToScheme

// DHCPLeaseSpec mirrors the fields moo needs from a metaldhcp DHCPLease.
type DHCPLeaseSpec struct {
	// MACAddress is the hardware address of the interface that performed the DHCP request.
	MACAddress string `json:"macAddress"`

	// IPAddress is the IP address assigned by the DHCP server.
	// +optional
	IPAddress string `json:"ipAddress,omitempty"`
}

// DHCPLeaseStatus holds the observed state of a DHCPLease.
type DHCPLeaseStatus struct {
	// Bound is true when the lease is active and the address is in use.
	// +optional
	Bound bool `json:"bound,omitempty"`
}

// +kubebuilder:object:root=true

// DHCPLease is a shim type matching the metaldhcp DHCPLease CR.
// Replace with the real type once metaldhcp publishes a Go module.
type DHCPLease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DHCPLeaseSpec   `json:"spec,omitempty"`
	Status DHCPLeaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DHCPLeaseList contains a list of DHCPLease.
type DHCPLeaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DHCPLease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DHCPLease{}, &DHCPLeaseList{})
}

// DeepCopyInto copies all properties of this object into another object of the same type.
func (in *DHCPLease) DeepCopyInto(out *DHCPLease) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy creates a deep copy of the DHCPLease.
func (in *DHCPLease) DeepCopy() *DHCPLease {
	if in == nil {
		return nil
	}
	out := new(DHCPLease)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements runtime.Object.
func (in *DHCPLease) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopyInto copies all properties of this object into another object of the same type.
func (in *DHCPLeaseList) DeepCopyInto(out *DHCPLeaseList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DHCPLease, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy creates a deep copy of DHCPLeaseList.
func (in *DHCPLeaseList) DeepCopy() *DHCPLeaseList {
	if in == nil {
		return nil
	}
	out := new(DHCPLeaseList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements runtime.Object.
func (in *DHCPLeaseList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}
