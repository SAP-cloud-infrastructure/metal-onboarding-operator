# Design

## Context

See proposal.md — Why.

moo sets `BMC.spec.hostname` only when the inventory record carries a hostname (`dhcplease_controller.go`, `if rec.BMCHostname != ""`). The MetalCluster provider's `recordFrom` (`internal/provider/metalcluster/provider.go`) never sets `BMCHostname`, and `MetalCluster.ServerEntry` has no hostname field — so `spec.hostname` is always nil for MetalCluster-sourced BMCs. metal-operator's deployed DNS template (`helm-charts/system/metal-operator-remote`) uses `metadata.name: {{ .Hostname }}`, and a nil `*string` renders as `<nil>`, which fails RFC 1123 validation on the `disco.stable.sap.cc/Record`.

The NetBox provider already returns `BMCHostname` (remoteboard DNS name from IPAM), and the `inventory-provider` spec already defines it as an optional field. This change closes the equivalent gap for the MetalCluster provider.

## Goals / Non-Goals

**Goals:**
- MetalCluster can carry a remoteboard hostname per server
- `BMC.spec.hostname` is always populated for MetalCluster-sourced BMCs, so the DNS record name is always valid

**Non-Goals:**
- Changing metal-operator or the DNS record template
- Populating `remoteboardHostname` from argora/NetBox sync (separate inventory-sync work; the field is optional and the fallback covers its absence)

## Decisions

### 1. Fallback to server name lives in `recordFrom`

**Decision:** Compute `hostname := s.RemoteboardHostname; if hostname == "" { hostname = s.Name }` in `recordFrom` and set `BMCHostname: hostname`.

**Rationale:** Keeping the fallback in the provider means `InventoryRecord.BMCHostname` is always non-empty, so the existing `if rec.BMCHostname != ""` guard in the controller stays correct and unchanged. The server `Name` is a valid RFC 1123 name (it is already used as the BMC `metadata.name`), so it is always a safe DNS record name.

**Alternative considered:** Default in the controller instead. Rejected — it would duplicate the fallback across inventory providers and weaken the `InventoryRecord` contract.

### 2. New field is optional, additive

**Decision:** `RemoteboardHostname string json:"remoteboardHostname,omitempty"` with `+optional`.

**Rationale:** Existing MetalCluster CRs remain valid; the name fallback covers servers that omit it. No migration needed.

## Risks / Trade-offs

- A server `Name` that is not the real DNS hostname becomes the DNS record name when `remoteboardHostname` is unset. Acceptable: the name is already the BMC object name and a valid RFC 1123 subdomain, and a valid record is strictly better than a failed reconcile that blocks server discovery.

## Migration Plan

No migration. CRD change is additive and optional; rebuild the operator image and apply the regenerated CRD.
