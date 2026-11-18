# Spec Delta

## Purpose

Creates a `BMCUserSet` CR per onboarded node alongside the `BMC` CR, enabling mmo to bootstrap factory credentials and populate `BMC.Spec.BMCSecretRef` so metal-operator can connect to the BMC.

## ADDED Requirements

### Requirement: Create BMCUserSet alongside BMC CR
The system SHALL create a `BMCUserSet` CR (`baseboard.metal.ironcore.dev/v1alpha1`) for each onboarded BMC when `BMCUserSet` creation is configured, using the BMC name as the `BMCUserSet` name and selecting the BMC via a label match.

#### Scenario: BMCUserSet created on first successful BMC reconciliation
- **WHEN** the controller successfully creates or updates a `BMC` CR for a node
- **AND** the controller is configured with a `--bmc-userset-setup-credential-ref-name`
- **THEN** the system SHALL create a `BMCUserSet` CR with the same name as the `BMC` CR, with `spec.bmcSelector.matchLabels` set to `kubernetes.metal.cloud.sap/name: <bmcName>`

#### Scenario: Existing BMCUserSet is not recreated
- **WHEN** the controller reconciles a `BMC` CR for a node that already has a `BMCUserSet` CR of the same name
- **THEN** the system SHALL NOT modify or replace the existing `BMCUserSet`

#### Scenario: BMCUserSet creation skipped when unconfigured
- **WHEN** the controller reconciles a `BMC` CR and `--bmc-userset-setup-credential-ref-name` is not set
- **THEN** the system SHALL NOT create a `BMCUserSet` CR

### Requirement: BMCUserSet carries cluster-uniform credential configuration
The `BMCUserSet` CR created by the controller SHALL carry rotation and account configuration sourced from operator flags, uniform across all nodes in a cluster.

#### Scenario: BMCUserSet spec populated from operator flags
- **WHEN** a `BMCUserSet` CR is created
- **THEN** its `spec.setupCredentialRef`, `spec.rotationStrategy`, `spec.template` (userName, roleID, rotationPeriod, credentialSecretNameTemplate, credentialSecretNamespace), and `spec.factoryAccount` SHALL reflect the values from the corresponding `--bmc-userset-*` flags

#### Scenario: Credential secret name follows per-BMC template
- **WHEN** mmo processes the `BMCUserSet` and promotes a credential
- **THEN** the stable credential Secret name SHALL follow the template `bmc-metal-cred-{{ .BMCName }}`
