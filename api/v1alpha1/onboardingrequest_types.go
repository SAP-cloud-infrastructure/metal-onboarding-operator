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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OnboardingPhase represents the current phase of an OnboardingRequest.
// +kubebuilder:validation:Enum=InventoryLookup;RedfishProbe;BMCCreation;Done;Failed;Skipped
type OnboardingPhase string

const (
	PhaseInventoryLookup OnboardingPhase = "InventoryLookup"
	PhaseRedfishProbe    OnboardingPhase = "RedfishProbe"
	PhaseBMCCreation     OnboardingPhase = "BMCCreation"
	PhaseDone            OnboardingPhase = "Done"
	PhaseFailed          OnboardingPhase = "Failed"
	PhaseSkipped         OnboardingPhase = "Skipped"
)

// OnboardingRequestSpec defines the desired state of OnboardingRequest.
type OnboardingRequestSpec struct {
	// macAddress is the MAC address of the server's OOB interface that triggered DHCP discovery.
	// +kubebuilder:validation:Pattern=`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`
	// +required
	MACAddress string `json:"macAddress"`

	// assignedIP is the IP address assigned to the server by the DHCP lease.
	// +optional
	AssignedIP string `json:"assignedIP,omitempty"`

	// dhcpLeaseRef references the DHCPLease CR that triggered this OnboardingRequest.
	// +optional
	DHCPLeaseRef *corev1.ObjectReference `json:"dhcpLeaseRef,omitempty"`
}

// OnboardingRequestStatus defines the observed state of OnboardingRequest.
type OnboardingRequestStatus struct {
	// phase is the current phase of the onboarding state machine.
	// +optional
	Phase OnboardingPhase `json:"phase,omitempty"`

	// reason provides a machine-readable cause for the current phase or failure.
	// +optional
	Reason string `json:"reason,omitempty"`

	// lastTransitionTime is when the phase last changed.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// managerType is the Redfish ManagerType discovered during the RedfishProbe phase
	// (e.g. "iDRAC", "iLO"). Populated after a successful probe; used in the
	// onboarding.metal.ironcore.dev/manager-type annotation on the BMC CR.
	// +optional
	ManagerType string `json:"managerType,omitempty"`

	// oobIP is the OOB IP resolved by the inventory provider during InventoryLookup.
	// +optional
	OOBIP string `json:"oobIP,omitempty"`

	// serverName is the canonical server name resolved by the inventory provider.
	// Used as the name for the BMC object.
	// +optional
	ServerName string `json:"serverName,omitempty"`

	// conditions represent the current state of the OnboardingRequest resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="ManagerType",type="string",JSONPath=".status.managerType"
// +kubebuilder:printcolumn:name="ServerName",type="string",JSONPath=".status.serverName"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OnboardingRequest is the Schema for the onboardingrequests API.
// One OnboardingRequest is created per unique MAC address discovered via DHCPLease.
// It drives the server through inventory lookup, Redfish probe, and BMC object creation.
type OnboardingRequest struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec OnboardingRequestSpec `json:"spec"`

	// +optional
	Status OnboardingRequestStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OnboardingRequestList contains a list of OnboardingRequest
type OnboardingRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OnboardingRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OnboardingRequest{}, &OnboardingRequestList{})
}
