# Proposal

## Why

When a `DHCPLease` arrives, moo looks up the server in NetBox by MAC address. In practice many servers have their MAC addresses missing from NetBox interfaces, causing continuous `ErrNotFound` requeue loops with no visibility into why onboarding is stuck. Reaching the BMC via Redfish first and matching by serial number (ServiceTag) is more robust — serial numbers are reliably populated in NetBox and Redfish exposes them without authentication.

## What Changes

- **New serial-first lookup path**: before the MAC lookup, moo performs an unauthenticated `GET https://<lease-IP>/redfish/v1`. If the BMC responds, the `ServiceTag` (or `UUID` as fallback) is extracted and used to query NetBox by `serial=`.
- **MAC as fallback**: the existing `LookupByMAC` path runs unchanged when Redfish is unreachable or the serial is not found in NetBox.
- **New `LookupBySerial` on `InventoryProvider`**: adds a second lookup method to the interface and the NetBox implementation.
- **DHCPLease annotations on failure**: when both serial and MAC lookups fail, the reconciler patches an `onboarding-state` annotation onto the DHCPLease and emits a Kubernetes Event so the failure is observable without log scraping.
- **RBAC**: the controller gains `patch` permission on `DHCPLease` resources to write annotations.

## Capabilities

### New Capabilities

- `dhcplease-watcher/serial-lookup`: Redfish-first serial lookup with MAC fallback and DHCPLease annotation on terminal failure

### Modified Capabilities

- `inventory-provider`: `InventoryProvider` interface gains `LookupBySerial(ctx, serial)` method; NetBox provider implements it via `GET /api/dcim/devices/?serial=<serial>`

## Impact

- `internal/provider/provider.go` — `InventoryProvider` interface gains `LookupBySerial`
- `internal/provider/netbox/provider.go` — new `LookupBySerial` method and NetBox device serial query
- `internal/controller/dhcplease_controller.go` — serial-first lookup logic, DHCPLease annotation patch, Event emission; gains `SubnetProvider` field (already present from OOBSubnet work), Event recorder
- `cmd/main.go` — wire Event recorder into controller
- `config/rbac/role.yaml` — regenerated to include `dhcpleases` patch verb
- Any other `InventoryProvider` implementations (CRD-based, fakes in tests) must add `LookupBySerial`
