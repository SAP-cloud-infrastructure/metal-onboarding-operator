# Spec Delta

## Purpose

Redfish-first serial lookup with MAC fallback. When the DHCPLease IP is reachable via Redfish, the controller uses the server's serial number (ServiceTag) to locate it in NetBox before falling back to MAC address lookup. DHCPLeases whose server cannot be identified have an observable annotation set so operators can detect and resolve stuck onboarding without log scraping.

## ADDED Requirements

### Requirement: Serial number lookup via Redfish before MAC lookup
The system SHALL attempt an unauthenticated GET to `https://<lease-IP>/redfish/v1` before the MAC lookup. If the response contains a `ServiceTag` in the Dell OEM extension, or a `UUID` as fallback, the system SHALL use that value to query NetBox by serial number.

#### Scenario: Redfish responds with ServiceTag and NetBox knows the serial
- **WHEN** `GET https://<lease-IP>/redfish/v1` returns HTTP 200 with a non-empty `Oem.Dell.ServiceTag`
- **THEN** the system SHALL query NetBox for `serial=<ServiceTag>` and, if found, use that device record for onboarding without performing a MAC lookup

#### Scenario: Redfish responds with UUID fallback and NetBox knows the serial
- **WHEN** the Redfish response has no `Oem.Dell.ServiceTag` but has a non-empty `UUID`
- **THEN** the system SHALL query NetBox for `serial=<UUID>` and use that device record if found

#### Scenario: Redfish unreachable falls through to MAC lookup
- **WHEN** the Redfish probe fails (connection refused, timeout, TLS error, or non-200 response)
- **THEN** the system SHALL proceed with the existing MAC address lookup without error

#### Scenario: Serial not in NetBox falls through to MAC lookup
- **WHEN** the Redfish probe succeeds but NetBox returns no device for the serial number
- **THEN** the system SHALL proceed with the MAC address lookup

### Requirement: DHCPLease annotated when no lookup succeeds
When both serial lookup and MAC lookup fail to identify the server, the system SHALL patch an annotation onto the DHCPLease CR and emit a Kubernetes Event.

#### Scenario: Both lookups fail — annotation set to bmc-unreachable
- **WHEN** the Redfish probe is unreachable AND the MAC lookup returns `ErrNotFound`
- **THEN** the system SHALL patch `metal.cloud.sap/onboarding-state: bmc-unreachable` onto the DHCPLease and emit a Warning Event with reason `BMCUnreachable`

#### Scenario: Both lookups fail — serial found but not in NetBox
- **WHEN** the Redfish probe succeeds but the serial is not in NetBox AND the MAC lookup returns `ErrNotFound`
- **THEN** the system SHALL patch `metal.cloud.sap/onboarding-state: serial-not-in-netbox` onto the DHCPLease and emit a Warning Event with reason `SerialNotInNetBox`

#### Scenario: Successful onboarding clears failure annotation
- **WHEN** a DHCPLease was previously annotated with a failure state and reconciliation now succeeds
- **THEN** the system SHALL patch `metal.cloud.sap/onboarding-state: complete` onto the DHCPLease
