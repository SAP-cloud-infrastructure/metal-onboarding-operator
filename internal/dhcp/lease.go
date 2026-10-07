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

// Package dhcp re-exports the metaldhcp API types used by moo.
// All types here are owned by github.com/SAP-cloud-infrastructure/metaldhcp.
package dhcp

import (
	metaldhcpv1alpha1 "github.com/SAP-cloud-infrastructure/metaldhcp/api/v1alpha1"
)

// Re-export types and scheme helpers so controller code can import this
// package without depending directly on metaldhcp's module path.

type (
	DHCPLease     = metaldhcpv1alpha1.DHCPLease
	DHCPLeaseList = metaldhcpv1alpha1.DHCPLeaseList
	DHCPLeaseSpec = metaldhcpv1alpha1.DHCPLeaseSpec
)

var AddToScheme = metaldhcpv1alpha1.AddToScheme
