# Spec Delta

## Purpose

State machine that drives a server from initial DHCP discovery through inventory lookup, Redfish probe, and BMC object creation. The `OnboardingRequest` CR is the durable record of all resolved inventory settings; metal-maintenance-operator reads these settings from it to create `BMCUser` and `BMCSettings` objects without re-querying inventory.

## ADDED Requirements

### Requirement: OnboardingRequest follows a defined phase sequence
The system SHALL process each `OnboardingRequest` through phases in order: `InventoryLookup` → `RedfishProbe` → `BMCCreation` → `Done`.

#### Scenario: Successful end-to-end onboarding
- **WHEN** an `OnboardingRequest` is created with a valid MAC address
- **THEN** the system SHALL progress through all phases and set the final status to `Done`

#### Scenario: Phase failure sets error status and retries
- **WHEN** a phase fails transiently (e.g., network error, resource conflict)
- **THEN** the `OnboardingRequest` status SHALL reflect the failed phase and reason, and the system SHALL retry with exponential backoff

### Requirement: Inventory gate rejects unknown servers
The system SHALL reject an `OnboardingRequest` if the MAC address is not found in the configured inventory provider after all retries are exhausted.

#### Scenario: Unknown MAC address rejected
- **WHEN** the inventory provider returns no record for the MAC address after configured retries
- **THEN** the `OnboardingRequest` SHALL transition to `Failed` phase with reason `InventoryNotFound`

#### Scenario: Wrong cluster rejects silently
- **WHEN** the inventory provider indicates the server belongs to a different cluster
- **THEN** the `OnboardingRequest` SHALL transition to `Skipped` phase and take no further action

### Requirement: Redfish probe verifies BMC reachability
The system SHALL perform an unauthenticated GET to `/redfish/v1` on the IP from the inventory lookup before creating BMC objects.

#### Scenario: Successful Redfish probe
- **WHEN** `/redfish/v1` returns HTTP 200 with a valid ManagerType field
- **THEN** the system SHALL extract the ManagerType and proceed to BMC creation

#### Scenario: Unreachable BMC defers onboarding
- **WHEN** the Redfish probe fails (connection refused, timeout, non-200 response)
- **THEN** the system SHALL set phase to `RedfishProbe` with reason `Unreachable` and retry

### Requirement: OnboardingRequest stores resolved inventory settings in spec
After a successful inventory lookup the system SHALL write the resolved settings into `OnboardingRequest.spec.onboardingSettings`: the desired `BMCUser` fields (username, roleID) and the desired `BMCSettings` payload (hostname, NTP servers, AD/LDAP domain, syslog server).

#### Scenario: Settings written before BMC creation
- **WHEN** the inventory lookup phase succeeds
- **THEN** `spec.onboardingSettings` SHALL be populated with all resolved fields before the reconciler advances to the `RedfishProbe` phase

#### Scenario: Settings are immutable after being written
- **WHEN** `spec.onboardingSettings` is already populated
- **THEN** the reconciler SHALL NOT overwrite it on subsequent reconcile loops

### Requirement: BMC creation carries handoff annotations
The system SHALL create a `BMC` CR with annotation `onboarding.metal.ironcore.dev/bootstrap=true` and `onboarding.metal.ironcore.dev/onboarding-request=<OnboardingRequest-name>` to allow metal-maintenance-operator to locate the `OnboardingRequest`.

#### Scenario: BMC CR created with annotations
- **WHEN** inventory lookup and Redfish probe both succeed
- **THEN** the system SHALL create a `BMC` CR with both annotations set

#### Scenario: Idempotent BMC creation
- **WHEN** a BMC CR with the same name already exists (e.g., reconciliation after restart)
- **THEN** the system SHALL NOT create a duplicate; it SHALL verify annotations are present and continue

### Requirement: OnboardingRequest carries full status
The `OnboardingRequest` status SHALL expose the current phase, reason, and last-transition time.

#### Scenario: Status reflects current phase
- **WHEN** the reconciler advances or halts at any phase
- **THEN** `status.phase`, `status.reason`, and `status.lastTransitionTime` SHALL be updated accordingly
