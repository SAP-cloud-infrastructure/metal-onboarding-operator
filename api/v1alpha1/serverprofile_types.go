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

// ServerProfileSpec defines the desired state of ServerProfile.
// One ServerProfile entry per physical server; used by the CRD inventory provider
// as a fallback when NetBox is not configured.
type ServerProfileSpec struct {
	// macAddress is the OOB interface MAC address used to match incoming DHCPLease CRs.
	// +kubebuilder:validation:Pattern=`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`
	// +required
	MACAddress string `json:"macAddress"`

	// serverName is the canonical name for this server, used as the BMC object name.
	// Should match the naming convention used in NetBox (e.g. "r-abc12-bb01").
	// +required
	ServerName string `json:"serverName"`

	// oobIP is the static OOB IP address for this server's BMC.
	// +required
	OOBIP string `json:"oobIP"`

	// bmcHostname is the optional DNS name for the remoteboard/BMC interface.
	// When set, it is applied to BMC.spec.hostname.
	// +optional
	BMCHostname string `json:"bmcHostname,omitempty"`

	// clusterName identifies which cluster this server belongs to.
	// Used to gate onboarding: a server with a different clusterName is marked Skipped.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// siteConfigRef references the SiteConfig CR that provides cluster-wide defaults.
	// Fields from SiteConfig are used when clusterName is not set directly.
	// +optional
	SiteConfigRef *corev1.LocalObjectReference `json:"siteConfigRef,omitempty"`

	// labels are topology and cluster labels to be applied to the BMC object metadata.
	// These supplement or override defaults from SiteConfig.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="MAC",type="string",JSONPath=".spec.macAddress"
// +kubebuilder:printcolumn:name="ServerName",type="string",JSONPath=".spec.serverName"
// +kubebuilder:printcolumn:name="OOBIP",type="string",JSONPath=".spec.oobIP"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ServerProfile is the Schema for the serverprofiles API.
// It provides CRD-backed inventory data for the InventoryProvider interface,
// used when NetBox is not available.
type ServerProfile struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec ServerProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// ServerProfileList contains a list of ServerProfile
type ServerProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ServerProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerProfile{}, &ServerProfileList{})
}
