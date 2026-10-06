# Spec Delta

## Purpose

State machine that drives a server from initial DHCP discovery through inventory lookup, Redfish probe, and BMC object creation. Sets `bootstrap=true` on the `BMC` CR as the sole handoff signal to metal-maintenance-operator; mmo operates independently from there using its own probe and `BMCBootstrapPolicy`.

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

### Requirement: Redfish probe verifies BMC reachability and extracts ManagerType
The system SHALL perform an unauthenticated GET to `/redfish/v1` on the OOB IP from the inventory lookup before creating BMC objects, and SHALL extract the `ManagerType` from the response.

#### Scenario: Successful Redfish probe
- **WHEN** `/redfish/v1` returns HTTP 200 with a valid `ManagerType` field
- **THEN** the system SHALL record the `ManagerType` in `OnboardingRequest.status` and proceed to BMC creation

#### Scenario: Unreachable BMC defers onboarding
- **WHEN** the Redfish probe fails (connection refused, timeout, non-200 response)
- **THEN** the system SHALL set phase to `RedfishProbe` with reason `Unreachable` and retry

### Requirement: BMC creation carries handoff annotations
The system SHALL create a `BMC` CR with annotations `onboarding.metal.ironcore.dev/bootstrap=true` and `onboarding.metal.ironcore.dev/manager-type=<ManagerType>` to allow mmo to trigger its `BMCOnboardingReconciler` and select the correct `BMCBootstrapPolicy` candidates.

#### Scenario: BMC CR created with annotations
- **WHEN** inventory lookup and Redfish probe both succeed
- **THEN** the system SHALL create a `BMC` CR with both annotations set

#### Scenario: Idempotent BMC creation
- **WHEN** a BMC CR with the same name already exists (e.g., reconciliation after restart)
- **THEN** the system SHALL NOT create a duplicate; it SHALL verify annotations are present and continue

### Requirement: OnboardingRequest carries full status
The `OnboardingRequest` status SHALL expose the current phase, reason, last-transition time, and discovered `ManagerType`.

#### Scenario: Status reflects current phase
- **WHEN** the reconciler advances or halts at any phase
- **THEN** `status.phase`, `status.reason`, `status.lastTransitionTime`, and `status.managerType` SHALL be updated accordingly
