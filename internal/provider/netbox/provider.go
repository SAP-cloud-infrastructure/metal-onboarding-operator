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

// Package netbox implements the InventoryProvider interface using the NetBox REST API.
package netbox

import (
	"context"

	"github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/provider"
)

// Provider implements InventoryProvider against the NetBox REST API.
//
// TODO: Implement real NetBox HTTP client in LookupByMAC.
// The implementation should:
//  1. Query NetBox DCIM interfaces by MAC address to find the device
//  2. Resolve the device's cluster membership (DCIM → Virtualization)
//  3. Read device.OOBIp.Address for OOBIP
//  4. Read the remoteboard interface DNS name from IPAM for BMCHostname (optional)
//  5. Build 10 topology labels from region, site.Slug, cluster.Name, cluster.Type.Slug,
//     device.Name, bb-suffix (device.Name split), device.DeviceType.Slug,
//     device.DeviceRole.Slug, device.Platform.Slug
//  6. Compare cluster against the operator's --cluster-name flag to set ClusterGate
type Provider struct {
	baseURL     string
	tokenFile   string
	clusterName string
}

// New returns a NetBox-backed InventoryProvider.
// baseURL is the NetBox API base (e.g. "https://netbox.example.com").
// tokenFile is the path to a file containing the NetBox API token.
// clusterName is this operator's cluster name, used to gate ClusterGate decisions.
func New(baseURL, tokenFile, clusterName string) *Provider {
	return &Provider{
		baseURL:     baseURL,
		tokenFile:   tokenFile,
		clusterName: clusterName,
	}
}

// LookupByMAC returns ErrNotFound for all MACs until the real implementation is added.
func (p *Provider) LookupByMAC(_ context.Context, _ string) (*provider.InventoryRecord, error) {
	return nil, provider.ErrNotFound
}

// compile-time check that Provider satisfies the interface.
var _ provider.InventoryProvider = (*Provider)(nil)
