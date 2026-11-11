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

// Package oobsubnet provides configuration types and helpers for OOBSubnet CR management.
package oobsubnet

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SubnetConfig holds the static per-subnet fields that are not sourced from NetBox.
// These map directly to the optional fields of OOBSubnetSpec.
type SubnetConfig struct {
	Gateway    string           `yaml:"gateway"`
	BootURL    string           `yaml:"bootURL"`
	RangeStart string           `yaml:"rangeStart"`
	RangeEnd   string           `yaml:"rangeEnd"`
	LeaseTime  *metav1.Duration `yaml:"leaseTime"`
}

// ConfigMap maps CIDR strings (e.g. "10.0.1.0/24") to static OOBSubnet configuration.
type ConfigMap map[string]SubnetConfig

// NameFromCIDR converts a CIDR string to a valid Kubernetes object name by replacing
// "/" with "-" (e.g. "10.0.1.0/24" → "10.0.1.0-24").
func NameFromCIDR(cidr string) string {
	return strings.ReplaceAll(cidr, "/", "-")
}
