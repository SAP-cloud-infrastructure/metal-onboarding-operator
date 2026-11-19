# Spec Delta

## MODIFIED Requirements

### Requirement: InventoryProvider resolves MAC to cluster gate, OOB IP, server name, labels, and optional hostname
The system SHALL expose an `InventoryProvider` interface that maps a MAC address to: cluster membership gate (belongs here / belongs elsewhere / unknown), OOB static IP, server name (used as the `BMC` object name), topology labels (applied to the `BMC` object), and an optional BMC hostname (DNS name for the remoteboard interface, written to `BMC spec.hostname`).

These fields mirror what argora reads from NetBox: `device.Name`, `device.OOBIp.Address`, the remoteboard DNS name from IPAM, and the 10 topology labels derived from `region`, `site.Slug`, `cluster.Name`, `cluster.Type.Slug`, `device.Name`, and `device.DeviceType.Slug`/`DeviceRole.Slug`/`Platform.Slug`.

The MetalCluster-backed provider SHALL resolve the optional BMC hostname from `ServerEntry.RemoteboardHostname`, falling back to `ServerEntry.Name` when it is empty, so the returned record always carries a non-empty hostname.

#### Scenario: Successful MAC resolution
- **WHEN** a MAC address is provided to an `InventoryProvider` implementation
- **THEN** the provider SHALL return a populated inventory record with cluster gate, OOB IP, server name, labels, and optional hostname, or an unambiguous error

#### Scenario: Server not in inventory
- **WHEN** the MAC address has no matching record in the backend
- **THEN** the provider SHALL return a sentinel `ErrNotFound` error (not a partial record)

#### Scenario: MetalCluster entry with explicit remoteboard hostname
- **WHEN** a `ServerEntry` sets `RemoteboardHostname`
- **THEN** the MetalCluster provider SHALL return an inventory record whose BMC hostname equals `RemoteboardHostname`

#### Scenario: MetalCluster entry without a remoteboard hostname
- **WHEN** a `ServerEntry` leaves `RemoteboardHostname` empty
- **THEN** the MetalCluster provider SHALL return an inventory record whose BMC hostname equals `ServerEntry.Name`
