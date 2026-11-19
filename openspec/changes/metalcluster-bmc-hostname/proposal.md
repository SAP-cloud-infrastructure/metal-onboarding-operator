# Proposal

## Why

When moo creates a `BMC` from MetalCluster inventory, it leaves `spec.hostname` unset: the MetalCluster provider never populates `InventoryRecord.BMCHostname`, and `MetalCluster.ServerEntry` has no hostname field. metal-operator's DNS record template keys `metadata.name` on the BMC hostname, so an unset hostname renders as the literal `<nil>`, the `Record` is rejected, and the BMC reconcile fails on the DNS step — which runs before server discovery. The result: the `Server` is never created and onboarding stalls.

## What Changes

- **New `remoteboardHostname` field** on `MetalCluster.ServerEntry` — the DNS hostname of the remote-management interface, sourced from NetBox IPAM.
- **MetalCluster provider populates `BMCHostname`** from `remoteboardHostname`, falling back to the server `Name` when empty, so the record always carries a non-empty hostname and moo always sets `BMC.spec.hostname`.

## Capabilities

### Modified Capabilities

- `inventory-provider`: the MetalCluster provider resolves the optional BMC hostname from `ServerEntry.RemoteboardHostname`, falling back to `ServerEntry.Name` when unset.

## Impact

- `api/v1alpha1/metalcluster_types.go` — new optional `RemoteboardHostname` field on `ServerEntry`
- `internal/provider/metalcluster/provider.go` — `recordFrom` sets `BMCHostname` (hostname-or-name fallback)
- `config/crd/bases/` and `charts/metal-onboarding-operator/crds/` — regenerated MetalCluster CRD with the new field
- No metal-operator or DNS-template changes: the existing template keying on `.Hostname` becomes correct once `spec.hostname` is always populated
