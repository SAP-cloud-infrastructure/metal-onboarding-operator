# Proposal

## Why

When moo creates a BMC CR, metal-operator cannot connect to the BMC because `BMC.Spec.BMCSecretRef` is empty — the credential does not exist yet. The credential can only be bootstrapped by mmo, but mmo requires a `BMCUserSet` CR to exist first. moo must create the `BMCUserSet` alongside the `BMC` CR to break this chicken-and-egg dependency.

## What Changes

- **New BMCUserSet creation step**: after creating or updating the `BMC` CR, the dhcplease-watcher controller creates a `BMCUserSet` (`baseboard.metal.ironcore.dev/v1alpha1`) named after the BMC. The `BMCUserSet` selects its BMC via `kubernetes.metal.cloud.sap/name: <bmcName>`, a label already present from MetalCluster inventory.
- **Idempotent**: if the `BMCUserSet` already exists, no action is taken.
- **Opt-in**: the feature is disabled by default. Creation only happens when `--bmc-userset-setup-credential-ref-name` is configured.
- **New `--bmc-userset-*` flags in `cmd/main.go`**: 8 flags control the `BMCUserSet` template (setup credential ref name/namespace, factory usernames, rotation strategy, managed username, role ID, rotation period, credential secret namespace).
- **New `bmcUserSet` Helm values block**: all 8 fields exposed in chart values; deployment args emitted conditionally when `setupCredentialRef.name` is non-empty.
- **New RBAC**: controller gains `get;list;watch;create` on `baseboard.metal.ironcore.dev/bmcusersets`.
- **New dependency**: `github.com/ironcore-dev/metal-maintenance-operator` added to `go.mod` to import the `BMCUserSet` API types.

## Capabilities

### New Capabilities

- `dhcplease-watcher/bmc-userset`: Per-node BMCUserSet creation alongside the BMC CR, enabling mmo to bootstrap credentials and populate `BMC.Spec.BMCSecretRef`.

### Modified Capabilities

## Impact

- `internal/controller/dhcplease_controller.go` — new `BMCUserSetConfig` struct; `reconcileBMCUserSet` method; RBAC marker
- `cmd/main.go` — 8 `--bmc-userset-*` flags; `buildBMCUserSetConfig` helper; `baseboardv1alpha1` scheme registration
- `charts/metal-onboarding-operator/values.yaml` — new `bmcUserSet` block
- `charts/metal-onboarding-operator/templates/deployment.yaml` — conditional args block
- `charts/metal-onboarding-operator/templates/clusterrole.yaml` — `bmcusersets` rule
- `config/rbac/role.yaml` — regenerated with `bmcusersets` rule
- `go.mod` — new direct dependency on `github.com/ironcore-dev/metal-maintenance-operator`
