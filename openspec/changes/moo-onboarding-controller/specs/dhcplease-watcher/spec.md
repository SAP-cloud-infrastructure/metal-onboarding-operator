# Spec Delta

## Purpose

Watches `DHCPLease` CRs produced by metaldhcp and creates a deduplicated `OnboardingRequest` per MAC address, providing the entry point for the DHCP-triggered onboarding pipeline.

## ADDED Requirements

### Requirement: Create OnboardingRequest from DHCPLease
The system SHALL watch `DHCPLease` CRs in group `dhcp.metal.ironcore.dev` and create one `OnboardingRequest` CR per unique MAC address.

#### Scenario: New DHCPLease triggers OnboardingRequest creation
- **WHEN** a `DHCPLease` CR is created with a MAC address not seen before
- **THEN** the system creates an `OnboardingRequest` CR in the same namespace containing the MAC address and assigned IP from the lease

#### Scenario: Duplicate DHCPLease does not create duplicate OnboardingRequest
- **WHEN** a `DHCPLease` CR is updated or a new lease CR exists for a MAC address that already has an `OnboardingRequest`
- **THEN** the system SHALL NOT create a second `OnboardingRequest` for that MAC address

#### Scenario: Deleted DHCPLease does not delete OnboardingRequest
- **WHEN** a `DHCPLease` CR is deleted
- **THEN** the corresponding `OnboardingRequest` SHALL remain and continue processing

### Requirement: OnboardingRequest references its source DHCPLease
The `OnboardingRequest` CR SHALL include a reference to the `DHCPLease` that triggered its creation.

#### Scenario: OnboardingRequest carries lease reference
- **WHEN** an `OnboardingRequest` is created from a `DHCPLease`
- **THEN** the `OnboardingRequest` spec SHALL contain the name and namespace of the originating `DHCPLease` CR

### Requirement: Cross-namespace watch requires explicit permission
The system SHALL require RBAC permission to watch `DHCPLease` CRs cluster-wide when metaldhcp deploys leases in a different namespace than the operator.

#### Scenario: Operator watches leases in configured namespace
- **WHEN** the operator is configured with a `dhcpLeaseNamespace` setting
- **THEN** the controller SHALL only watch `DHCPLease` CRs in that namespace
