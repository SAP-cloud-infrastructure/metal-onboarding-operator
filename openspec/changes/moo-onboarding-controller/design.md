# Design

## Context

See proposal.md — Why.

moo is a greenfield kubebuilder operator. The only existing comparable code is in fedhcp (Go, controller-runtime patterns) and metal-operator (also controller-runtime). Neither is imported by moo. The key external constraint is that metaldhcp (issue #1945) owns the `DHCPLease` CRD; moo watches it as a consumer and must not re-declare those types.

## Goals / Non-Goals

**Goals:**
- Event-driven server onboarding triggered by metaldhcp `DHCPLease` CRs.
- Pluggable inventory via `InventoryProvider` interface; NetBox primary, `ServerProfile` CRD fallback.
- Produces `BMC`/`BMCSecret`/`ServerWiring` objects with `bootstrap=true` annotation for mmo.
- moo and mmo are fully independent: the only shared surface is metal-operator's `BMC` CR.

**Non-Goals:**
- Any authenticated BMC operation (owned by mmo).
- Day-0 BMC/BIOS configuration (owned by mmo).
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

`InventoryRecord` contains what moo needs to create the `BMC` object, matching argora's NetBox data:

```go
type InventoryRecord struct {
    ClusterGate ClusterGate        // belongs / elsewhere / unknown
    ServerName  string             // used as BMC object metadata.name (= NetBox device.Name)
    OOBIP       string             // BMC spec.endpoint.ip (= device.OOBIp.Address)
    BMCHostname string             // optional; BMC spec.hostname (= remoteboard DNS from IPAM)
    Labels      map[string]string  // applied to BMC metadata.labels (topology + cluster labels)
}
```

The 10 labels argora sets (derived from region, site slug, cluster name, cluster type, device name, bb suffix, device type/role/platform) must be populated by the NetBox provider. The CRD provider populates what is declared in `ServerProfile`; minimal labels are acceptable for the CRD case.

mmo performs its own discovery via probe image and does not consume this record.

**Alternative considered**: Runtime dynamic selection per `OnboardingRequest`. Rejected: adds complexity for a configuration that is per-cluster, not per-server.

### D5: ServerProfile + SiteConfig CRDs in group `onboarding.metal.ironcore.dev`

These are `onboarding.metal.ironcore.dev/v1alpha1` types, defined in moo's own `api/v1alpha1/`. They are not shared with metaldhcp or mmo.

`SiteConfig` is cluster-scoped; `ServerProfile` is namespace-scoped (same namespace as the operator). `ServerProfile` references a `SiteConfig` by name for defaults.

### D6: BMC creation is idempotent via CreateOrUpdate

The reconciler uses `controller-runtime`'s `CreateOrUpdate` for `BMC`, `BMCSecret`, and `ServerWiring` objects. On conflict it verifies the annotations are present rather than failing.

**Rationale**: Operator restarts or network partitions during reconcile must not leave partial state or fail on re-entry.

### D7: moo→mmo handoff is two annotations on the BMC CR

moo sets exactly two annotations on the `BMC` CR it creates:
- `onboarding.metal.ironcore.dev/bootstrap=true` — triggers mmo's `BMCOnboardingReconciler`
- `onboarding.metal.ironcore.dev/manager-type=<ManagerType>` — vendor identity from the unauthenticated Redfish probe; used by mmo to select matching `BMCBootstrapPolicy` candidates

mmo needs nothing else from moo. It does not import moo's types, does not read `OnboardingRequest`, and performs its own discovery via probe image once it has authenticated access.

**Alternative considered**: Pass resolved inventory settings (hostname, NTP, AD/LDAP, syslog) via annotations or `OnboardingRequest.spec`. Rejected: mmo has its own probe image for discovery and must not depend on moo's inventory lookup result. Passing structured data through untyped annotations is also fragile.

### D8: mmo independence — BMCOnboardingReconciler and BMCBootstrapPolicy

This decision belongs to mmo's design, but is documented here to make the boundary explicit.

mmo's new `BMCOnboardingReconciler` watches `BMC` CRs with `bootstrap=true`. It orchestrates the full authenticated pipeline in order:

```
1. Credential bootstrap (BMCBootstrapPolicy → BMCUser)
2. Probe-image-driven discovery
3. BMCSettings (hostname, NTP, syslog, AD/LDAP via Redfish)
4. BIOSSettings (boot order, TPM)
5. Clear bootstrap=true → Server taint lifted → Available
```

Each step depends on the previous completing successfully. `BMCOnboardingReconciler` is the orchestrator; it creates child CRs (`BMCUser`, `BMCSettings`, `BIOSSettings`) and waits for each to reach its terminal state before proceeding.

**BMCBootstrapPolicy** (cluster-scoped CR in mmo) defines the ordered list of credentials to try against an unmanaged BMC:

```yaml
apiVersion: baseboard.metal.ironcore.dev/v1alpha1
kind: BMCBootstrapPolicy
metadata:
  name: default
spec:
  candidates:
    - name: provisioning-user       # fleet-wide pre-created user (SAP build-up team)
      username: sap-provision
      secretRef: { name: bmc-provisioning-secret }
      postBootstrap: delete
    - name: dell-idrac-default      # vendor default, tried if managerType matches
      managerType: iDRAC
      username: root
      secretRef: { name: bmc-dell-default }
      postBootstrap: deactivate
    - name: hpe-ilo-default
      managerType: iLO
      username: Administrator
      secretRef: { name: bmc-hpe-default }
      postBootstrap: deactivate
```

`managerType` on each candidate is matched against the `manager-type` annotation on the `BMC` CR. Unfiltered candidates (no `managerType`) are always tried. Candidates are tried in order; the first to authenticate successfully is used to create the managed `BMCUser`.

`postBootstrap` policy per candidate:
- `delete` — remove the factory/provisioning account after managed user is created (for purpose-built provisioning users)
- `deactivate` — disable the account (for vendor defaults where deletion may fail on some firmware)

mmo ships built-in vendor defaults so open-source users without a provisioning user get working bootstrap out of the box. A cluster-provided `BMCBootstrapPolicy` named `default` overrides the built-ins.

**BMC reset recovery**: a BMC reset re-enables the factory/deactivated user. mmo detects loss of managed `BMCUser` via Redfish and re-runs the bootstrap flow using `BMCBootstrapPolicy`. The deactivate-not-delete policy for vendor defaults is what makes this re-entry possible.

## Risks / Trade-offs

- **metaldhcp module dependency** → moo's DHCPLease watcher cannot be fully compiled until metaldhcp publishes a stable Go module. Mitigation: scaffold the watcher with a local interface shim; replace with the real import once metaldhcp's module is available.
- **NetBox API stability** → NetBox schema changes can break the provider. Mitigation: isolate all NetBox HTTP calls in `internal/provider/netbox/` behind the `InventoryProvider` interface so the surface area is bounded.
- **Race between lease and server power-on** → A DHCPLease may arrive before the server's BMC is reachable at the probed IP. Mitigation: Redfish probe retries with backoff; `OnboardingRequest` stays in `RedfishProbe` phase until BMC responds.
- **Single operator replica** → controller-runtime leader election is required; without it a replica restart causes double-reconcile. Mitigation: standard leader-election flag in operator flags, enabled by default in production Helm chart.
- **BMC reset does not trigger new DHCPLease** → A BMC firmware reset keeps the existing IP; no new `DHCPLease` CR is produced. moo is not re-triggered. Mitigation: mmo's `BMCOnboardingReconciler` owns reset detection and re-bootstrap independently of moo.

## Migration Plan

1. Deploy metaldhcp (issue #1945) and verify `DHCPLease` CRs are produced.
2. Apply moo CRDs (`OnboardingRequest`, `ServerProfile`, `SiteConfig`).
3. Deploy moo operator with `--inventory-provider` configured for the cluster.
4. Decommission argora once moo has produced `BMC`/`BMCSecret`/`ServerWiring` for all active servers; verify no argora reconcile loops remain.
5. AWX decommission is a separate milestone, gated on mmo completing day-0 configuration via `BMCOnboardingReconciler`.

**Rollback**: moo creates no objects that argora cannot recreate; rollback is stop-moo + restart-argora.

## Open Questions

- Should `ServerProfile` be cluster-scoped or namespace-scoped? Namespace-scoped is safer for multi-tenant clusters but requires knowing which namespace to search. Current assumption: namespace-scoped, same namespace as the operator. Revisit if multi-namespace support is needed.
- Boot URL source for `OOBSubnet` CRs (tracked in issue #1945 open questions) — does not affect moo's design.
