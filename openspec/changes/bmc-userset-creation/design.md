# Design

## Context

See proposal.md — Why.

moo already creates a `BMC` CR when a DHCPLease maps to a known server in the MetalCluster inventory. The BMC is created with an `onboarding.metal.ironcore.dev/bootstrap: "true"` annotation as the handoff signal to mmo. mmo's `bmcuserset_controller` watches `BMCUserSet` CRs, selects matching BMCs by label, and drives the credential bootstrap cycle that ultimately sets `BMC.Spec.BMCSecretRef`.

The `BMCUserSet` API lives in `github.com/ironcore-dev/metal-maintenance-operator/api/baseboard/v1alpha1` (mmo). A `BMCUserSet` is cluster-scoped; one per BMC is sufficient.

## Goals / Non-Goals

**Goals:**
- Create one `BMCUserSet` per node, named after the BMC, immediately after the BMC CR exists
- Keep the feature completely opt-in (no change to existing behavior when unconfigured)
- Keep configuration uniform per cluster — no per-node config in MetalCluster

**Non-Goals:**
- Updating or deleting an existing `BMCUserSet` — lifecycle after creation is owned by mmo
- Per-node credential customization
- Waiting for mmo to confirm the credential is ready before returning from reconcile

## Decisions

### 1. moo creates the BMCUserSet; mmo owns it after creation

**Decision:** moo creates the `BMCUserSet` once (idempotent: no-op if it exists) then exits. mmo's existing `bmcuserset_controller` takes over: it selects the BMC by label, bootstraps credentials, and sets `BMC.Spec.BMCSecretRef`.

**Rationale:** moo already holds all the information needed to create the `BMCUserSet` (BMC name, labels from inventory). mmo already has the logic to act on it. No new controller is needed.

**Alternative considered:** A separate credential-bootstrap controller in moo. Rejected — duplicates mmo's existing capability and introduces another actor that writes to `BMC.Spec.BMCSecretRef`.

### 2. BMC label `kubernetes.metal.cloud.sap/name: <bmcName>` as the selector

**Decision:** `BMCUserSet.spec.bmcSelector.matchLabels` uses `kubernetes.metal.cloud.sap/name: <bmcName>`, the same label moo copies from the MetalCluster inventory onto the BMC.

**Rationale:** This label is already guaranteed to exist on the BMC at creation time (moo sets it). It is unique per BMC and follows the established labeling convention.

**Alternative considered:** Selector on `metadata.name`. Not supported by `BMCUserSet` — the selector is a `metav1.LabelSelector`.

### 3. Config from flags/Helm values, not from MetalCluster CRD

**Decision:** The `BMCUserSet` template fields (rotation settings, factory usernames, credential namespace, etc.) come from operator flags wired via Helm values, uniform across all nodes.

**Rationale:** These settings are cluster-wide policy (e.g., all nodes use the same factory account username `support`). They belong in Helm values, not in per-node inventory. The MetalCluster CRD extension for per-node config (discussed but deferred) is out of scope.

**Alternative considered:** Embed config in MetalCluster `.spec.servers[].bmcUserSet`. Deferred — not needed for the initial implementation; the Helm approach unblocks the use case immediately.

### 4. BMCUserSet creation is synchronous in reconcileBMC

**Decision:** `reconcileBMCUserSet` is called at the end of `reconcileBMC`, after the BMC is created/patched and the lease is annotated. A failure returns an error and retriggers reconciliation.

**Rationale:** The BMC is the prerequisite; creating the `BMCUserSet` before the BMC would be meaningless. Returning an error on failure ensures the controller retries rather than silently skipping.

### 5. Feature disabled when setup credential ref name is empty

**Decision:** If `--bmc-userset-setup-credential-ref-name` is empty (the default), `buildBMCUserSetConfig` returns `nil` and `reconcileBMCUserSet` is a no-op.

**Rationale:** Keeps the default deployment behavior unchanged. Operators deploy moo in environments without mmo; they should not see spurious errors or unexpected `BMCUserSet` CRs.

## Risks / Trade-offs

- **mmo not installed**: if `--bmc-userset-setup-credential-ref-name` is set but mmo's CRD is not installed, `r.Create` returns an error and reconciliation retries. The operator will log errors until mmo is deployed. Mitigation: document the dependency; the Helm chart should declare mmo as a prerequisite.
- **BMCUserSet created before mmo CRD exists**: same scenario as above. Mitigation: same.
- **Config mismatch across nodes**: since config is uniform per cluster, a change to Helm values after some `BMCUserSet` CRs are created does not update existing ones. Mitigation: existing `BMCUserSet` CRs are owned by mmo; operators who need different settings must update them manually or delete and let moo recreate.

## Migration Plan

Deployment is additive. Existing BMC CRs and clusters are unaffected:
- Deploy new moo image with `bmcUserSet.setupCredentialRef.name` empty (the default) — no `BMCUserSet` CRs are created.
- When ready to enable, set `bmcUserSet.setupCredentialRef.name` and ensure the referenced Secret and mmo are deployed. On the next reconcile of each DHCPLease, moo creates the missing `BMCUserSet` CRs idempotently.
- Rollback: remove `bmcUserSet.setupCredentialRef.name` and redeploy. Existing `BMCUserSet` CRs are unaffected (moo never deletes them).
