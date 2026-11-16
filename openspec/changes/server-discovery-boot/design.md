# Design

## Context

See `proposal.md — Why` for motivation. Key constraints:

- moo creates `BMC` CRs; metal-operator's `BMCReconciler` creates `Server` CRs from them (via `discoverServers`). moo never creates `Server` CRs directly.
- `metalv1alpha1.Server.Spec.Taints` is a first-class field (`[]Taint`); patching it is straightforward.
- `ServerReadinessRule` is cluster-scoped and immutable once created (`conditions`, `enforcementMode`, `taint`, `serverSelector` all have `XValidation: immutable`). If a rule needs to change, it must be deleted and recreated.
- `metalv1alpha1` is already a direct dependency in `go.mod`; no new module import needed.
- The `BMCReconciler` in metal-operator copies all BMC labels to the Server via `metautils.SetLabels(server, bmcObj.Labels)`. Propagation is synchronous on the next BMC reconcile, typically within seconds.

## Goals / Non-Goals

**Goals:**
- Patch `Undiscovered` NoBind taint onto every Server whose name matches a BMC created by moo (identified via `managed-by` label).
- Create a `ServerReadinessRule` that removes the taint once MMO sets `Discovered=True`.
- Keep the rule garbage-collected via owner reference on the Server.
- Add minimal RBAC — only what the new controller needs.

**Non-Goals:**
- Syncing `Metadata` CR fields back to NetBox (future work).
- Creating the `ServerClaim` for the discovery boot (MMO's `ServerDiscoveryReconciler` owns that).
- Modifying or interfering with the existing `DHCPLeaseController` reconcile path beyond adding the `managed-by` label.

## Decisions

### 1. New controller watching `Server`, not `BMC`

**Decision**: A new `ServerDiscoveryController` watches `metal.ironcore.dev/v1alpha1 Server` CRs filtered to those with label `onboarding.metal.ironcore.dev/managed-by=metal-onboarding-operator`.

**Why not watch BMC?** The taint lives on the `Server`, not the `BMC`. Watching BMC and then looking up the Server on each reconcile adds a second fetch and a coupling point. Watching Server directly is simpler and reacts to the exact resource being modified.

**Alternative considered**: A dedicated controller-level index on `spec.bmcRef.name` to map Server → BMC. Rejected as over-engineered for this use case; label propagation from BMC is sufficient.

### 2. `managed-by` label propagates identification

**Decision**: Add `onboarding.metal.ironcore.dev/managed-by=metal-onboarding-operator` to every BMC CR at creation/patch time in `DHCPLeaseController`. metal-operator propagates it to the Server automatically.

**Why a label and not annotation?** `ServerReadinessRule.spec.serverSelector` requires a label selector; labels are first-class for selector-based filtering. Annotations are not queryable via label selectors.

**Why this specific key?** Follows the `metal.ironcore.dev` convention used by MMO (`DiscoveryForUIDLabel`, `UndiscoveredTaintKey`) while scoping to moo's own domain.

### 3. ServerReadinessRule named after the Server, owned by it

**Decision**: `ServerReadinessRule.Name = Server.Name`. Owner reference set with `controllerutil.SetControllerReference(server, rule, scheme)`.

**Why same name?** One-to-one with the Server; avoids a lookup or index. Name collision is impossible because the Server name is unique cluster-wide and the rule is also cluster-scoped.

**Why owner reference?** Kubernetes GC handles cleanup for free. The alternative (finalizer on the Server or explicit delete in a `DeletionTimestamp` branch) adds reconcile complexity for no gain.

**Immutability caveat**: If a `ServerReadinessRule` already exists with the correct fields, no action is needed. If it exists with wrong fields (e.g., from a stale previous run), delete it and recreate. In practice this should not occur because the spec is derived deterministically from the Server.

### 4. Taint patched with strategic merge, not full replace

**Decision**: Use `client.MergeFrom` patch, appending to `spec.taints` only if the taint is absent.

**Why not replace?** Replacing `spec.taints` could clobber taints added by other controllers. The controller reads the current `spec.taints`, checks for presence, and only patches if missing.

### 5. Controller predicate: label selector

**Decision**: Use `builder.WithPredicates(predicate.NewPredicateFuncs(...))` filtering on the `managed-by` label so the controller only processes Servers moo created.

**Why not process all Servers?** Avoids interfering with Servers created by other operators (e.g., manually created Servers, or Servers from other BMCs). Defense in depth.

## Risks / Trade-offs

- **Label propagation lag**: Between `BMC` creation and label propagation to the `Server` (one metal-operator reconcile cycle, typically <1s), the `ServerDiscoveryController` may miss the first watch event. The predicate filter ensures it only processes the event once the label is present; if the event fires before the label propagates, the Server is simply skipped until the next update event (which carries the label). No polling or requeue needed.

- **Immutable ServerReadinessRule**: If the rule spec needs to change (e.g., taint key changes), the existing rule must be deleted first. The controller handles this by comparing the desired spec to the observed spec and deleting+recreating if they differ. This is a one-time migration concern, not an ongoing operational risk.

- **Race with MMO discovery**: MMO's `ServerDiscoveryReconciler` watches for the `Undiscovered` taint. If moo is slow to add the taint (e.g., on initial install with many Servers), some Servers may be claimable briefly. This is acceptable because discovery is a bootstrap-only operation and the window is sub-second in practice.

## Migration Plan

1. Deploy updated moo with the new `ServerDiscoveryController` and RBAC.
2. The controller reconciles all existing Servers with the `managed-by` label (from any previously created BMCs that already have the label after the `DHCPLeaseController` update), adding the taint and `ServerReadinessRule`.
3. Servers that are already bound (have a `ServerClaim`) are unaffected — the `NoBind` taint only blocks new claims; it does not evict existing ones.
4. No CRD changes are required in moo's own CRDs.

**Rollback**: Remove the `ServerDiscoveryController` registration from `cmd/main.go` and redeploy. Existing taints and `ServerReadinessRule` objects will remain but become unmanaged; they can be manually cleaned up or left until MMO removes the taint when `Discovered=True`.
