# Spec Delta

## MODIFIED Requirements

### Requirement: BMC creation carries handoff annotations
The system SHALL create a `BMC` CR with annotations `onboarding.metal.ironcore.dev/bootstrap=true` and `onboarding.metal.ironcore.dev/manager-type=<ManagerType>` to allow mmo to trigger its `BMCOnboardingReconciler` and select the correct `BMCBootstrapPolicy` candidates. The BMC CR SHALL also carry the label `onboarding.metal.ironcore.dev/managed-by=metal-onboarding-operator` so that the `ServerDiscoveryController` can identify the downstream `Server` CR created by metal-operator.

#### Scenario: BMC CR created with annotations
- **WHEN** inventory lookup and Redfish probe both succeed
- **THEN** the system SHALL create a `BMC` CR with both annotations set

#### Scenario: Idempotent BMC creation
- **WHEN** a BMC CR with the same name already exists (e.g., reconciliation after restart)
- **THEN** the system SHALL NOT create a duplicate; it SHALL verify annotations are present and continue

#### Scenario: BMC CR carries managed-by label
- **WHEN** a new `BMC` CR is created
- **THEN** it SHALL have the label `onboarding.metal.ironcore.dev/managed-by=metal-onboarding-operator` so metal-operator propagates it to the `Server` CR
