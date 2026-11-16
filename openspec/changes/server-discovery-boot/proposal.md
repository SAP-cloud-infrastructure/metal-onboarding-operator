# Proposal

## Why

When moo creates a `BMC` CR and metal-operator subsequently creates a `Server` CR from it, the Server is immediately claimable by workloads before any in-band hardware inventory has been collected. The `metal-maintenance-operator` PR #210 introduced a discovery boot mechanism — gated by a `NoBind` taint — that runs `metalprobe` to populate a `Metadata` CR with CPU, RAM, NICs, storage, LLDP, PCI, and DMI data. Without moo adding the taint and the readiness rule, this discovery flow never triggers and the Metadata CR is never created.

## What Changes

- moo adds a label `onboarding.metal.ironcore.dev/managed-by=metal-onboarding-operator` to every `BMC` CR it creates; metal-operator propagates this label to the `Server` CR, allowing moo to identify its servers.
- A new `ServerDiscoveryController` in moo watches `Server` CRs carrying that label and:
  1. Patches the `metal.ironcore.dev/Undiscovered` taint (`effect: NoBind`) onto the Server if not already present.
  2. Creates a `ServerReadinessRule` (same name as the Server, cluster-scoped) that removes the taint once the Server's `Discovered` condition is `True`.
- The `ServerReadinessRule` is owned by the Server (owner reference) so it is garbage-collected automatically.
- moo adds RBAC markers for `servers` (get/list/watch/patch) and `serverreadinessrules` (get/list/watch/create/update/patch/delete) in the `metal.ironcore.dev` group.

## Capabilities

### New Capabilities

- `server-discovery-gating`: moo gates Server availability by adding the `Undiscovered` NoBind taint immediately after metal-operator creates the Server from moo's BMC CR, and creates a `ServerReadinessRule` to remove the taint once MMO marks the Server as `Discovered=True`.

### Modified Capabilities

- `onboarding-request`: The `BMCCreation` step now also adds the `onboarding.metal.ironcore.dev/managed-by` label to the BMC CR so that moo can identify downstream Server CRs.

## Impact

- `internal/controller/dhcplease_controller.go`: Add `managed-by` label when creating/patching the BMC CR.
- `internal/controller/serverdiscovery_controller.go`: New controller; watches Servers with the managed-by label, patches taint, creates/deletes `ServerReadinessRule`.
- `cmd/main.go`: Register the new controller with the manager.
- `go.mod` / imports: No new dependencies; `metalv1alpha1.ServerReadinessRule` and `metalv1alpha1.Taint` are already in the `github.com/ironcore-dev/metal-operator` dependency.
- RBAC: New markers for `servers` and `serverreadinessrules`.
- e2e / unit tests: New tests for the `ServerDiscoveryController`.
