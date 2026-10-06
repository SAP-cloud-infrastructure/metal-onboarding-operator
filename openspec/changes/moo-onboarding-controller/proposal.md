# Proposal

## Why

argora polls NetBox on a timer and AWX orchestrates BMC/BIOS setup and user provisioning for new and rebuilt servers — neither is event-driven and both require external tooling with no Kubernetes-native replacement. This change introduces metal-onboarding-operator (moo) as the event-driven successor: it watches `DHCPLease` CRs produced by metaldhcp (issue #1945) and drives the discovery and BMC provisioning pipeline, handing off to metal-maintenance-operator (mmo) for all authenticated BMC operations.

## What Changes

- **New operator binary** (`cmd/manager/`) — kubebuilder-based operator, `onboarding.metal.ironcore.dev` API group.
- **New CRDs** in group `onboarding.metal.ironcore.dev`:
  - `OnboardingRequest` — created per `DHCPLease`, tracks each server through the moo pipeline (inventory gate, Redfish probe, BMC object creation).
  - `ServerProfile` — CRD-backed inventory fallback (one entry per MAC/server), provides hostname, cluster gate, and OOB IP when NetBox is unavailable.
  - `SiteConfig` — cluster/site-level defaults referenced by `ServerProfile`.
- **DHCPLease watcher** — watches `DHCPLease` CRs from metaldhcp (`dhcp.metal.ironcore.dev`); creates one `OnboardingRequest` per lease, deduplicating by MAC.
- **OnboardingRequest reconciler** — inventory gate (NetBox or `ServerProfile`), unauthenticated Redfish probe (extracts `ManagerType`), creates `BMC`/`BMCSecret`/`ServerWiring` with `bootstrap=true` annotation for mmo.
- **InventoryProvider interface** — pluggable; two implementations: NetBox (primary) and `ServerProfile` CRD (fallback).
- **moo does not depend on mmo and mmo does not depend on moo** — the only shared surface is metal-operator's `BMC` CR.

## Capabilities

### New Capabilities

- `dhcplease-watcher`: Watch `DHCPLease` CRs from metaldhcp and create an `OnboardingRequest` per lease, deduplicating by MAC address.
- `onboarding-request`: Lifecycle state machine that gates a server through inventory lookup and Redfish probe, creates BMC objects, and emits `bootstrap=true` on the `BMC` CR to hand off to mmo.
- `inventory-provider`: Pluggable inventory lookup (NetBox primary, `ServerProfile` CRD fallback) that resolves MAC → cluster gate and OOB IP.

### Modified Capabilities

*(none — this is a greenfield project)*

## Impact

- **New dependency**: metaldhcp (`dhcp.metal.ironcore.dev`) must be deployed alongside moo; moo watches its `DHCPLease` CRs but does not own them.
- **Handoff to mmo**: moo sets `onboarding.metal.ironcore.dev/bootstrap=true` and `onboarding.metal.ironcore.dev/manager-type=<ManagerType>` on the `BMC` CR. mmo reacts to these independently — no import of moo types required.
- **mmo owns the authenticated pipeline**: credential bootstrap (`BMCBootstrapPolicy` + `BMCUser`), probe-image-driven discovery, `BMCSettings`, `BIOSSettings`, AD join — all sequenced by mmo's new `BMCOnboardingReconciler`.
- **metal-operator**: must propagate `bootstrap` annotation from `BMC` as a taint on newly created `Server` objects (tracked in issue #912, separate task).
- **argora**: replaced entirely by moo for BMC/ServerWiring creation.
- **AWX**: replaced by the moo + mmo pipeline for day-0 BMC configuration.
