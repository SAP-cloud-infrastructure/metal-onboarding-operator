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

// Package provider defines the InventoryProvider interface and its data types.
package provider

import (
	"context"
	"errors"
)

// ErrNotFound is returned when no inventory record exists for a given MAC address.
var ErrNotFound = errors.New("inventory record not found")

// ClusterGate indicates whether a server belongs to the local cluster.
type ClusterGate string

const (
	// ClusterGateBelongs means the server is assigned to this cluster and should be onboarded.
	ClusterGateBelongs ClusterGate = "belongs"
	// ClusterGateElsewhere means the server is assigned to a different cluster; skip silently.
	ClusterGateElsewhere ClusterGate = "elsewhere"
	// ClusterGateUnknown means cluster membership could not be determined.
	ClusterGateUnknown ClusterGate = "unknown"
)

// InventoryRecord holds the data moo needs to create the BMC object for a server.
// It mirrors what argora reads from NetBox:
//   - ServerName matches device.Name (used as BMC object metadata.name)
//   - OOBIP matches device.OOBIp.Address (used as BMC spec.endpoint.ip)
//   - BMCHostname matches the remoteboard interface DNS name from NetBox IPAM (optional)
//   - Labels match the 10 topology labels argora derives from region, site, cluster, device type/role/platform
type InventoryRecord struct {
	// ClusterGate indicates whether this server belongs to the local cluster.
	ClusterGate ClusterGate

	// ServerName is the canonical server name, used as the BMC CR metadata.name.
	ServerName string

	// OOBIP is the static OOB IP address for the BMC endpoint.
	OOBIP string

	// BMCHostname is the optional DNS hostname for the remoteboard/BMC interface.
	// When set, it is written to BMC.spec.hostname.
	BMCHostname string

	// Labels are topology and cluster labels to apply to the BMC CR metadata.
	// Keyed by label name (e.g. "topology.kubernetes.io/region").
	Labels map[string]string
}

// InventoryProvider resolves a MAC address to the inventory data needed for onboarding.
type InventoryProvider interface {
	// LookupByMAC returns the InventoryRecord for the given MAC address.
	// Returns ErrNotFound if no record exists for the MAC.
	// Returns a retriable error for transient failures (network errors, 5xx responses).
	LookupByMAC(ctx context.Context, mac string) (*InventoryRecord, error)
}
