# Proposal

## Why

argora polls NetBox on a timer and AWX orchestrates BMC/BIOS setup and user provisioning for new and rebuilt servers — neither is event-driven and both require external tooling with no Kubernetes-native replacement. This change introduces metal-onboarding-operator (moo) as the event-driven successor: it watches `DHCPLease` CRs produced by metaldhcp (issue #1945) and drives the full onboarding pipeline through to a managed, Available server with no AWX or argora dependency.

## What Changes

- **New operator binary** (`cmd/manager/`) — kubebuilder-based operator, `onboarding.metal.ironcore.dev` API group.
- **New CRDs** in group `onboarding.metal.ironcore.dev`:
  - `OnboardingRequest` — created per `DHCPLease`, tracks each server through the onboarding pipeline state machine.
  - `ServerProfile` — CRD-backed inventory fallback (one entry per MAC/server), provides hostname, NTP, AD/LDAP, syslog when NetBox is unavailable.
  - `SiteConfig` — cluster/site-level defaults (NTP, syslog, AD domain) referenced by `ServerProfile`.
- **DHCPLease watcher** — watches `DHCPLease` CRs from metaldhcp (`dhcp.metal.ironcore.dev`); creates one `OnboardingRequest` per lease.
- **OnboardingRequest reconciler** — inventory gate (NetBox or `ServerProfile`), unauthenticated Redfish probe, `BMC`/`BMCSecret`/`ServerWiring` creation with `bmc-user-pending` + `bmc-setup-pending` annotations (consumed by metal-maintenance-operator).
- **InventoryProvider interface** — pluggable; two implementations: NetBox (primary) and `ServerProfile` CRD (fallback).
- **Previous memory assumption invalidated**: moo no longer ships its own CoreDHCP binary or `DHCPLease` CRD — metaldhcp (issue #1945) owns those. moo only watches `DHCPLease` CRs as a consumer.

## Capabilities

### New Capabilities

- `dhcplease-watcher`: Watch `DHCPLease` CRs from metaldhcp and create an `OnboardingRequest` per lease, deduplicating by MAC address.
- `onboarding-request`: Lifecycle state machine that gates a server through inventory lookup, Redfish probe, and BMC object creation, emitting annotations that hand off to metal-maintenance-operator.
- `inventory-provider`: Pluggable inventory lookup (NetBox primary, `ServerProfile` CRD fallback) that resolves MAC → hostname, cluster gate, OOB IP, NTP, AD/LDAP, syslog.

### Modified Capabilities

*(none — this is a greenfield project)*

## Impact

- **New dependency**: metaldhcp (`dhcp.metal.ironcore.dev`) must be deployed alongside moo; moo watches its `DHCPLease` CRs but does not own them.
- **Downstream handoff to metal-maintenance-operator (mmo)**: moo creates `BMC` with `bmc-user-pending` and `bmc-setup-pending` annotations; mmo acts on those.
- **metal-operator**: must propagate BMC annotations as taints on newly created `Server` objects (tracked in issue #912, separate task).
- **NetBox**: remains the source of truth for inventory; `ServerProfile` + `SiteConfig` CRDs enable offline or NetBox-free operation.
- **argora**: replaced entirely by moo for BMC/ServerWiring creation.
- **AWX**: replaced by the moo + mmo pipeline for day-0 BMC configuration.
