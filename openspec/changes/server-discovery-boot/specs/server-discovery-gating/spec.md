# Spec Delta

## Purpose

Gates Server availability by applying the `Undiscovered` NoBind taint to newly-discovered Servers and creating a `ServerReadinessRule` that automatically removes the taint once the metal-maintenance-operator marks the Server as `Discovered=True`.

## ADDED Requirements

### Requirement: Server receives Undiscovered taint after BMC discovery
The system SHALL patch the taint `metal.ironcore.dev/Undiscovered` with effect `NoBind` onto any `Server` CR that carries the label `onboarding.metal.ironcore.dev/managed-by=metal-onboarding-operator` and does not already have that taint, preventing workloads from claiming the server before in-band discovery completes.

#### Scenario: Taint added to newly created Server
- **WHEN** metal-operator creates a `Server` CR with the `onboarding.metal.ironcore.dev/managed-by` label
- **THEN** the system SHALL patch `spec.taints` to include `{key: metal.ironcore.dev/Undiscovered, effect: NoBind}` if not already present

#### Scenario: Taint not duplicated on re-reconcile
- **WHEN** the controller reconciles a `Server` that already has the `Undiscovered` taint
- **THEN** the system SHALL NOT add a duplicate taint entry

### Requirement: ServerReadinessRule created per managed Server
The system SHALL create a `ServerReadinessRule` CR (cluster-scoped, same name as the Server) that removes the `metal.ironcore.dev/Undiscovered` taint when the Server's `Discovered` condition status is `True`, with `enforcementMode: BootstrapOnly`.

#### Scenario: ServerReadinessRule created on first reconcile
- **WHEN** a managed Server is reconciled for the first time
- **THEN** a `ServerReadinessRule` with a `serverSelector` matching the Server by name and the condition `{type: Discovered, requiredStatus: True}` SHALL exist

#### Scenario: ServerReadinessRule is idempotent
- **WHEN** the controller reconciles a Server that already has a `ServerReadinessRule`
- **THEN** the system SHALL NOT create a duplicate; it SHALL verify the existing rule and continue

### Requirement: ServerReadinessRule is owned by its Server
The system SHALL set an owner reference on the `ServerReadinessRule` pointing to the Server, so the rule is garbage-collected automatically when the Server is deleted.

#### Scenario: Owner reference set at creation
- **WHEN** the `ServerReadinessRule` is created
- **THEN** its `metadata.ownerReferences` SHALL contain a reference to the Server with `controller: true`

#### Scenario: Rule deleted when Server is deleted
- **WHEN** the Server CR is deleted
- **THEN** the Kubernetes garbage collector SHALL delete the owned `ServerReadinessRule`
