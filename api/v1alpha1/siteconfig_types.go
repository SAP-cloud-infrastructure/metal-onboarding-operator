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

// SiteConfigSpec defines the desired state of SiteConfig.
// SiteConfig provides cluster/site-level defaults referenced by ServerProfile entries.
type SiteConfigSpec struct {
	// clusterName is the name of the Kubernetes cluster this site config applies to.
	// Used by CRDProvider to gate onboarding: servers not matching this cluster are Skipped.
	// +required
	ClusterName string `json:"clusterName"`

	// defaultLabels are topology labels applied to all BMC objects for servers in this site.
	// ServerProfile.spec.labels take precedence over these defaults.
	// +optional
	DefaultLabels map[string]string `json:"defaultLabels,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="ClusterName",type="string",JSONPath=".spec.clusterName"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteConfig is the Schema for the siteconfigs API.
// It is cluster-scoped and provides per-cluster defaults for the CRD inventory provider.
type SiteConfig struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec SiteConfigSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// SiteConfigList contains a list of SiteConfig
type SiteConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SiteConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteConfig{}, &SiteConfigList{})
}
