# Spec Delta

## MODIFIED Requirements

### Requirement: InventoryProvider resolves MAC to cluster gate, OOB IP, server name, labels, and optional hostname
The system SHALL expose an `InventoryProvider` interface with two lookup methods: `LookupByMAC` (existing) and `LookupBySerial`. Both return the same `InventoryRecord` type: cluster membership gate (belongs here / belongs elsewhere / unknown), OOB static IP, server name (used as the `BMC` object name), topology labels (applied to the `BMC` object), and an optional BMC hostname (DNS name for the remoteboard interface).

These fields mirror what argora reads from NetBox: `device.Name`, `device.OOBIp.Address`, the remoteboard DNS name from IPAM, and the 10 topology labels derived from `region`, `site.Slug`, `cluster.Name`, `cluster.Type.Slug`, `device.Name`, and `device.DeviceType.Slug`/`DeviceRole.Slug`/`Platform.Slug`.

#### Scenario: Successful MAC resolution
- **WHEN** a MAC address is provided to an `InventoryProvider` implementation
- **THEN** the provider SHALL return a populated inventory record with cluster gate, OOB IP, server name, labels, and optional hostname, or an unambiguous error

#### Scenario: Successful serial resolution
- **WHEN** a serial number is provided to `LookupBySerial`
- **THEN** the provider SHALL return a populated inventory record with cluster gate, OOB IP, server name, labels, and optional hostname, or an unambiguous error

#### Scenario: Server not in inventory
- **WHEN** the MAC address or serial number has no matching record in the backend
- **THEN** the provider SHALL return a sentinel `ErrNotFound` error (not a partial record)

### Requirement: NetBox provider implements InventoryProvider
The system SHALL include a NetBox implementation of `InventoryProvider` that looks up devices by MAC address via the NetBox interfaces API and by serial number via the NetBox devices API.

#### Scenario: NetBox lookup by MAC
- **WHEN** a MAC address is submitted to the NetBox provider
- **THEN** the provider SHALL query NetBox for the device and return cluster membership and OOB IP

#### Scenario: NetBox lookup by serial
- **WHEN** a serial number is submitted to `LookupBySerial`
- **THEN** the provider SHALL query `GET /api/dcim/devices/?serial=<serial>` and return the matching device's inventory record

#### Scenario: NetBox unreachable returns transient error
- **WHEN** the NetBox API is unreachable or returns a 5xx error
- **THEN** the provider SHALL return a retriable error (distinct from `ErrNotFound`)
