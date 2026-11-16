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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MetalClusterSpec defines the desired state of MetalCluster.
// A MetalCluster holds the static server inventory for one cluster — the
// authoritative source of MAC address, serial number, interface topology, and
// label data used by the onboarding controller when a DHCPLease arrives.
type MetalClusterSpec struct {
	// Servers is the list of bare-metal servers that belong to this cluster.
	// +optional
	// +listType=map
	// +listMapKey=name
	Servers []ServerEntry `json:"servers,omitempty"`
}

// ServerEntry describes one server in the cluster inventory.
type ServerEntry struct {
	// Name is the canonical server name, sourced from the NetBox DCIM device name
	// (e.g. "node001-bb684"). Used as the BMC CR metadata.name.
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`

	// Serial is the chassis serial number (manufacturer-assigned), used as the
	// primary lookup key when a Redfish probe can reach the BMC.
	// +kubebuilder:validation:MinLength=1
	// +required
	Serial string `json:"serial"`

	// RemoteboardMAC is the MAC address of the remote-management interface (BMC/iDRAC/iLO).
	// Used as fallback lookup key when the serial is unknown or the BMC is unreachable.
	// +optional
	RemoteboardMAC string `json:"remoteboardMAC,omitempty"`

	// Interfaces lists the known data-plane network interfaces of the server.
	// +optional
	Interfaces []InterfaceEntry `json:"interfaces,omitempty"`

	// Labels are key/value pairs to be applied to the BMC CR and, by extension,
	// the Server CR created by metal-operator. Specific label keys are mapped to
	// NoBind taints on the Server to gate the onboarding phases driven by mmo.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// InterfaceType classifies a network interface by its role.
// +kubebuilder:validation:Enum=dpu;nic;oob
type InterfaceType string

const (
	// InterfaceTypeDPU is a Data Processing Unit (SmartNIC) interface.
	InterfaceTypeDPU InterfaceType = "dpu"
	// InterfaceTypeNIC is a standard NIC interface.
	InterfaceTypeNIC InterfaceType = "nic"
	// InterfaceTypeOOB is an out-of-band management interface (same as remoteboard).
	InterfaceTypeOOB InterfaceType = "oob"
)

// InterfaceEntry describes one network interface of a server.
type InterfaceEntry struct {
	// Type classifies the interface by its role (dpu, nic, oob).
	// +kubebuilder:default=nic
	// +optional
	Type InterfaceType `json:"type,omitempty"`

	// MAC is the MAC address of this interface.
	// +optional
	MAC string `json:"mac,omitempty"`

	// Name is the expected interface name (e.g. "eth0", "eno1").
	// +optional
	Name string `json:"name,omitempty"`
}

// MetalClusterStatus defines the observed state of MetalCluster.
type MetalClusterStatus struct {
	// conditions represent the current state of the MetalCluster resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Servers",type=integer,JSONPath=".spec.servers[*].serial",description="Number of server entries"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// MetalCluster is the static server inventory for one cluster.
// It is the authoritative source for MAC address, serial number, interface
// topology, and label data used by the onboarding controller at BMC discovery
// time. One MetalCluster CR exists per cluster, named after the cluster.
type MetalCluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of MetalCluster
	// +optional
	Spec MetalClusterSpec `json:"spec,omitempty"`

	// status defines the observed state of MetalCluster
	// +optional
	Status MetalClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MetalClusterList contains a list of MetalCluster.
type MetalClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MetalCluster `json:"items"`
}
