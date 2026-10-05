# Spec Delta

## Purpose

Pluggable interface for resolving a server MAC address to inventory data (hostname, cluster gate, OOB IP, NTP, AD/LDAP, syslog), with NetBox as the primary backend and a `ServerProfile` CRD as the fallback.

## ADDED Requirements

### Requirement: InventoryProvider resolves MAC to server inventory
The system SHALL expose an `InventoryProvider` interface that maps a MAC address to: cluster membership, hostname, OOB static IP, NTP server list, AD/LDAP domain, and syslog server.

#### Scenario: Successful MAC resolution
- **WHEN** a MAC address is provided to an `InventoryProvider` implementation
- **THEN** the provider SHALL return a populated inventory record or an unambiguous error

#### Scenario: Server not in inventory
- **WHEN** the MAC address has no matching record in the backend
- **THEN** the provider SHALL return a sentinel `ErrNotFound` error (not a partial record)

### Requirement: NetBox provider implements InventoryProvider
The system SHALL include a NetBox implementation of `InventoryProvider` that looks up the device by MAC address using the NetBox API.

#### Scenario: NetBox lookup by MAC
- **WHEN** a MAC address is submitted to the NetBox provider
- **THEN** the provider SHALL query NetBox for the device and return hostname, cluster, OOB IP, NTP, AD/LDAP domain, and syslog from NetBox fields

#### Scenario: NetBox unreachable returns transient error
- **WHEN** the NetBox API is unreachable or returns a 5xx error
- **THEN** the provider SHALL return a retriable error (distinct from `ErrNotFound`)

### Requirement: ServerProfile CRD provides fallback inventory
The system SHALL support a `ServerProfile` CRD (group `onboarding.metal.ironcore.dev`) as an inventory source when NetBox is not configured or unavailable.

#### Scenario: ServerProfile matched by MAC address
- **WHEN** a `ServerProfile` CR exists whose `spec.macAddress` matches the queried MAC
- **THEN** the CRD provider SHALL return that profile's inventory fields

#### Scenario: No matching ServerProfile
- **WHEN** no `ServerProfile` CR matches the MAC address
- **THEN** the CRD provider SHALL return `ErrNotFound`

### Requirement: SiteConfig provides cluster-wide defaults
The system SHALL support a `SiteConfig` CR (group `onboarding.metal.ironcore.dev`) that provides NTP, syslog, and AD/LDAP defaults referenced by `ServerProfile` when those fields are not set per-server.

#### Scenario: ServerProfile inherits from SiteConfig
- **WHEN** a `ServerProfile` omits NTP, syslog, or AD/LDAP fields and references a `SiteConfig`
- **THEN** the provider SHALL return the values from the referenced `SiteConfig`

### Requirement: Provider selection is operator configuration
The operator SHALL be configured at startup to select which `InventoryProvider` implementation to use (NetBox, CRD, or CRD-with-NetBox-fallback).

#### Scenario: Configuration selects NetBox provider
- **WHEN** the operator is started with `inventoryProvider: netbox`
- **THEN** all `OnboardingRequest` reconciliations SHALL use the NetBox provider

#### Scenario: Configuration selects CRD fallback
- **WHEN** the operator is started with `inventoryProvider: crd`
- **THEN** all reconciliations SHALL use the `ServerProfile` CRD provider
