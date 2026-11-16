# Design

## Context

See proposal.md — Why.

The controller today does: MAC lookup → Redfish probe → BMC create. The Redfish probe already hits `https://<lease-IP>/redfish/v1` unauthenticated; the root response on Dell iDRAC already contains `Oem.Dell.ServiceTag` and `Oem.Dell.ManagerMACAddress` at no extra cost. The root response is also what `probeRedfish()` already parses for `ManagerType`.

The `InventoryProvider` interface currently has one method: `LookupByMAC`. All existing implementations (NetBox provider, test fakes) satisfy it.

## Goals / Non-Goals

**Goals:**
- Use the serial number from Redfish to look up the device in NetBox before falling back to MAC
- Annotate DHCPLeases with an observable failure state when neither lookup succeeds
- Keep the existing `LookupByMAC` path and its behavior unchanged

**Non-Goals:**
- Writing MACs back to NetBox
- Handling non-Dell BMC vendors differently (UUID fallback covers them)
- Retrying with escalating timeouts (existing 30s requeue is sufficient)

## Decisions

### 1. Extend `InventoryProvider` with `LookupBySerial` rather than a new interface

**Decision:** Add `LookupBySerial(ctx, serial string) (*InventoryRecord, error)` to the existing `InventoryProvider` interface.

**Rationale:** Serial and MAC lookups return identical data (`InventoryRecord`). Keeping one interface avoids threading a second interface through the controller struct and `cmd/main.go`. The cost is that all existing `InventoryProvider` implementations (CRD provider, test fakes) must add the method — but there are only two fakes in tests and the CRD provider is not yet implemented.

**Alternative considered:** A separate `SerialProvider` interface with a nil-check guard in the controller (opt-in, like `SubnetProvider`). Rejected because serial lookup is not optional — it should always run when possible, not only when a flag is set.

### 2. Redfish probe moves before the MAC lookup and extracts ServiceTag in the same call

**Decision:** Expand `probeRedfish` to return both `ManagerType` and `ServiceTag` (and `UUID` as fallback). The serial-first path shares the same HTTP call as the existing Redfish probe.

**Rationale:** The Redfish root already contains all three fields. Making a second HTTP call would be wasteful and increase latency. The probe result carries both the ServiceTag (for NetBox lookup) and the ManagerType (for BMC annotation).

**Alternative considered:** Separate Redfish call only for serial discovery, keeping `probeRedfish` unchanged. Rejected — two round-trips to the same endpoint for data already in the first response.

### 3. Redfish probe runs first, even before MAC lookup

**Decision:** The lookup order is: Redfish probe → serial lookup → MAC lookup. The Redfish probe moves from step 2 to step 1.

**Rationale:** If the lease IP is the BMC IP (OOB network), Redfish will respond and the serial lookup avoids the MAC lookup entirely. If it's a data-plane NIC, Redfish won't respond (short timeout) and the MAC fallback runs. The net effect is one extra network probe on the fast path, justified by higher reliability.

**Alternative considered:** Serial lookup only as fallback after MAC fails. Rejected — this doesn't help for servers whose MAC is missing from NetBox, which is the primary problem being solved.

### 4. DHCPLease annotation via patch, Event via controller-runtime recorder

**Decision:** Use `client.MergeFrom` patch to write the `metal.cloud.sap/onboarding-state` annotation on the DHCPLease. Emit a `corev1.EventTypeWarning` Event using the controller-runtime `record.EventRecorder`.

**Rationale:** Patch avoids optimistic lock conflicts on a frequently-reconciled object. The Event recorder is already the idiomatic way to emit Events in controller-runtime; it handles deduplication.

**Wiring:** `DHCPLeaseController` gains an `EventRecorder record.EventRecorder` field. `cmd/main.go` sets it from `mgr.GetEventRecorderFor("dhcplease-controller")`. The RBAC marker gains `patch` on `dhcpleases`.

### 5. Redfish probe timeout stays at 10s; failure is non-fatal

**Decision:** Keep `redfishProbeTimeout = 10s`. A failed probe (timeout, TLS error, non-200) is not an error — it falls through silently to MAC lookup.

**Rationale:** The probe may be hitting a data-plane NIC that has nothing listening on 443. Treating this as an error would break the existing MAC-based path for servers that were previously working.

## Risks / Trade-offs

- **Extra latency on the Redfish-unreachable path**: every MAC-only lease now waits 10s for the Redfish timeout before the MAC lookup. Mitigation: make `redfishProbeTimeout` configurable via flag (`--redfish-probe-timeout`), defaulting to 3s for the serial probe (shorter than the existing 10s post-inventory probe).
- **`InventoryProvider` interface is now wider**: any future implementation must add `LookupBySerial`. Mitigation: the interface is internal; there are only two implementations (NetBox + test fake). A `NotImplemented` stub is acceptable for the CRD provider until it is built.
- **UUID collisions**: using `UUID` as serial fallback is vendor-specific and may not match NetBox `serial` field for non-Dell hardware. Mitigation: log a warning when falling back to UUID; treat a non-match as `ErrNotFound` and continue to MAC lookup.

## Migration Plan

No API or CRD changes. The new annotations are additive. Existing BMC CRs are unaffected. Deploy by rebuilding the operator image — no migration steps required.
