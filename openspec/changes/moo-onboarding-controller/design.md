# Design

## Context

See proposal.md — Why.

moo is a greenfield kubebuilder operator. The only existing comparable code is in fedhcp (Go, controller-runtime patterns) and metal-operator (also controller-runtime). Neither is imported by moo. The key external constraint is that metaldhcp (issue #1945) owns the `DHCPLease` CRD; moo watches it as a consumer and must not re-declare those types.

## Goals / Non-Goals

**Goals:**
- Event-driven server onboarding triggered by metaldhcp `DHCPLease` CRs.
- Pluggable inventory via `InventoryProvider` interface; NetBox primary, `ServerProfile` CRD fallback.
- Produces `BMC`/`BMCSecret`/`ServerWiring` objects annotated for metal-maintenance-operator.
- `OnboardingRequest.spec.onboardingSettings` is the durable record of all resolved inventory data; mmo reads it directly without re-querying inventory.

**Non-Goals:**
- Day-0 BMC/BIOS configuration (owned by metal-maintenance-operator / mmo).
- DHCP serving or `DHCPLease` type ownership (owned by metaldhcp).
- Ongoing lifecycle configuration (NTP drift correction, AD sync — tracked in issue #1883).
- DNS reservation and vendor console registration (phase 2, issue #912).

## Decisions

### D1: Import metaldhcp types, do not vendor-copy them

moo imports `dhcp.metal.ironcore.dev` types from metaldhcp as a Go module dependency, registers them in its scheme, and watches via controller-runtime. It does not re-declare the struct.

**Alternative considered**: Copy the struct locally. Rejected: two structs for the same CR causes subtle serialization bugs and creates drift risk.

**Implication**: metaldhcp must publish a versioned Go module before moo can compile its DHCPLease watcher. The watcher implementation can be stubbed until then using a local shim interface.

### D2: DHCPLease watcher creates OnboardingRequest, not reconciles inline

A separate `DHCPLeaseController` owns only the create-OnboardingRequest step. The `OnboardingRequestReconciler` owns all downstream phases. No phase logic runs inside the lease watcher.

**Rationale**: Keeps the lease watcher simple and idempotent (one-to-one CR creation). The `OnboardingRequestReconciler` is the durable, retry-capable state machine. Separating concerns avoids making the watcher stateful.

### D3: OnboardingRequest phase machine uses status conditions

Phase is tracked via `status.phase` (string enum) and a `status.conditions` array following Kubernetes API conventions. Each reconcile loop reads the current phase from status and advances by one step maximum.

**Alternative considered**: Annotations on the `OnboardingRequest` for phase tracking. Rejected: conditions give machine-readable reason codes and last-transition time for free, and follow established controller-runtime patterns.

### D4: InventoryProvider is a Go interface, selected at operator startup

```go
type InventoryProvider interface {
    LookupByMAC(ctx context.Context, mac string) (*InventoryRecord, error)
}
```

Implementations: `NetBoxProvider` (HTTP client to NetBox REST API) and `CRDProvider` (reads `ServerProfile` + `SiteConfig` CRs). Selected by operator flag `--inventory-provider=netbox|crd`.

**Alternative considered**: Runtime dynamic selection per `OnboardingRequest`. Rejected: adds complexity for a configuration that is per-cluster, not per-server.

### D5: ServerProfile + SiteConfig CRDs in group `onboarding.metal.ironcore.dev`

These are `onboarding.metal.ironcore.dev/v1alpha1` types, defined in moo's own `api/v1alpha1/`. They are not shared with metaldhcp.

`SiteConfig` is cluster-scoped; `ServerProfile` is namespace-scoped (same namespace as the operator). `ServerProfile` references a `SiteConfig` by name for defaults.

### D6: BMC creation is idempotent via CreateOrUpdate

The reconciler uses `controller-runtime`'s `CreateOrUpdate` for `BMC`, `BMCSecret`, and `ServerWiring` objects. On conflict it verifies the annotations are present rather than failing.

**Rationale**: Operator restarts or network partitions during reconcile must not leave partial state or fail on re-entry.

### D7: Factory credentials are not stored in moo

moo creates the `BMC` CR with `spec.bmcSecretRef` pointing to a `BMCSecret` generated with a placeholder/bootstrap credential. Actual credential rotation is owned by metal-maintenance-operator's `BMCUserReconciler`. moo is not involved in credential lifecycle.

### D8: OnboardingRequest.spec.onboardingSettings is the moo→mmo handoff record

After inventory lookup succeeds, moo writes all resolved settings into `spec.onboardingSettings` on the `OnboardingRequest`:
- `bmcUser`: desired username and roleID for the `BMCUser` CR mmo will create
- `bmcSettings`: hostname, NTP servers, AD/LDAP domain, syslog server for the `BMCSettings` CR mmo will create

This field is written once and treated as immutable; mmo reads it without re-querying NetBox or `ServerProfile`.

**Alternative considered**: Store settings as annotations on the `BMC` CR. Rejected: annotations are untyped strings with no schema validation, and the `BMC` CR is owned by metal-operator — adding arbitrary payload there is invasive.

**Alternative considered**: mmo re-queries NetBox independently. Rejected: creates a second NetBox dependency in mmo, duplicates the inventory lookup, and introduces a TOCTOU window if NetBox data changes between moo and mmo runs.

### D9: mmo locates OnboardingRequest via annotation on BMC

moo sets annotation `onboarding.metal.ironcore.dev/onboarding-request=<name>` on the `BMC` CR alongside `onboarding.metal.ironcore.dev/bootstrap=true`. mmo's bootstrap controller reads this annotation to fetch the `OnboardingRequest` and its `spec.onboardingSettings`.

**Alternative considered**: Owner reference from `BMC` → `OnboardingRequest`. Rejected: owner references require same-namespace objects; if `BMC` is cluster-scoped and `OnboardingRequest` is namespace-scoped, the reference is invalid.

**Alternative considered**: mmo field-indexes `OnboardingRequest` by `spec.bmcRef.name`. Rejected: requires mmo to import moo's API types and maintain an index; the annotation approach is a simpler, explicit pointer.

## Risks / Trade-offs

- **metaldhcp module dependency** → moo's DHCPLease watcher cannot be fully compiled until metaldhcp publishes a stable Go module. Mitigation: scaffold the watcher with a local interface shim; replace with the real import once metaldhcp's module is available.
- **NetBox API stability** → NetBox schema changes can break the provider. Mitigation: isolate all NetBox HTTP calls in `internal/provider/netbox/` behind the `InventoryProvider` interface so the surface area is bounded.
- **Race between lease and server power-on** → A DHCPLease may arrive before the server's BMC is reachable at the probed IP. Mitigation: Redfish probe retries with backoff; `OnboardingRequest` stays in `RedfishProbe` phase until BMC responds.
- **Single operator replica** → controller-runtime leader election is required; without it a replica restart causes double-reconcile. Mitigation: standard leader-election flag in operator flags, enabled by default in production Helm chart.

## Migration Plan

1. Deploy metaldhcp (issue #1945) and verify `DHCPLease` CRs are produced.
2. Apply moo CRDs (`OnboardingRequest`, `ServerProfile`, `SiteConfig`).
3. Deploy moo operator with `--inventory-provider` configured for the cluster.
4. Decommission argora once moo has produced `BMC`/`BMCSecret`/`ServerWiring` for all active servers; verify no argora reconcile loops remain.
5. AWX decommission is a separate milestone, gated on mmo completing day-0 configuration.

**Rollback**: moo creates no objects that argora cannot recreate; rollback is stop-moo + restart-argora.

## Open Questions

- Should `ServerProfile` be cluster-scoped or namespace-scoped? Namespace-scoped is safer for multi-tenant clusters but requires knowing which namespace to search. Current assumption: namespace-scoped, same namespace as the operator. Revisit if multi-namespace support is needed.
- Boot URL source for `OOBSubnet` CRs (tracked in issue #1945 open questions) — does not affect moo's design.
