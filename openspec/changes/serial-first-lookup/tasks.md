# Tasks

## 1. Extend InventoryProvider interface and NetBox implementation

- [x] 1.1 Add `LookupBySerial(ctx context.Context, serial string) (*InventoryRecord, error)` to `InventoryProvider` in `internal/provider/provider.go` and verify the file compiles (`go build ./...`)
- [x] 1.2 Implement `LookupBySerial` on the NetBox `Provider` in `internal/provider/netbox/provider.go` using `GET /api/dcim/devices/?serial=<serial>&limit=2`; reuse the existing `getDevice` / label-building path; verify with a new Ginkgo test in `provider_test.go` covering: found, `ErrNotFound`, and 5xx transient error
- [x] 1.3 Add `LookupBySerial` stub to `fakeInventory` in `internal/controller/dhcplease_controller_test.go` (returns `f.record, f.err`) so it satisfies the updated interface; verify `make test` passes with no new failures

## 2. Expand Redfish probe to extract ServiceTag

- [x] 2.1 Extend `probeRedfish` in `internal/controller/dhcplease_controller.go` to return a `redfishResult` struct `{ManagerType, ServiceTag string}` instead of a plain string; parse `Oem.Dell.ServiceTag` and fall back to top-level `UUID`; verify existing controller unit tests still pass
- [x] 2.2 Add a `--redfish-probe-timeout` flag in `cmd/main.go` (default `3s` for the serial probe stage; the existing 10s constant becomes the post-inventory Redfish probe timeout); wire it into the controller; verify `go build ./cmd/...` succeeds

## 3. Serial-first lookup in Reconcile

- [x] 3.1 Reorder `Reconcile` in `internal/controller/dhcplease_controller.go`: (1) Redfish probe on `https://<lease-IP>/redfish/v1`, (2) if `ServiceTag` non-empty call `LookupBySerial`, (3) on `ErrNotFound` or probe failure fall through to existing `LookupByMAC`; verify with a new unit test: Redfish responds with ServiceTag → `LookupBySerial` called, `LookupByMAC` NOT called
- [x] 3.2 Add unit test: Redfish unreachable → `LookupBySerial` skipped, `LookupByMAC` called (existing MAC path unchanged); verify `make test` passes
- [x] 3.3 Add unit test: Redfish responds but serial not in NetBox (`ErrNotFound` from `LookupBySerial`) → falls through to `LookupByMAC`; verify `make test` passes

## 4. DHCPLease annotation and Event on failure

- [x] 4.1 Add `EventRecorder record.EventRecorder` field to `DHCPLeaseController`; wire it in `cmd/main.go` via `mgr.GetEventRecorderFor("dhcplease-controller")`; verify `go build ./...` succeeds
- [x] 4.2 After both lookups return `ErrNotFound`, patch `metal.cloud.sap/onboarding-state` annotation onto the DHCPLease using `client.MergeFrom` — value `bmc-unreachable` when Redfish was unreachable, `serial-not-in-netbox` when Redfish responded but serial not found; emit a Warning Event with matching reason; verify with two new unit tests (one per annotation value)
- [x] 4.3 When reconciliation succeeds (BMC created or updated), patch `metal.cloud.sap/onboarding-state: complete` onto the DHCPLease; verify with a new unit test that the annotation is present after successful reconciliation
- [x] 4.4 Add `// +kubebuilder:rbac:groups=dhcp.metal.ironcore.dev,resources=dhcpleases,verbs=get;list;watch;patch` marker (replace existing `get;list;watch`); run `make manifests` and verify `config/rbac/role.yaml` gains the `patch` verb

## 5. Verification

- [x] 5.1 Run `make lint` and verify 0 issues
- [x] 5.2 Run `make test` and verify all tests pass including new coverage for serial lookup, Redfish expansion, and DHCPLease annotation paths
