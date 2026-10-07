# Tasks

## 1. Project Scaffold

- [x] 1.1 Run `kubebuilder init --domain metal.ironcore.dev --repo github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator` and verify `go.mod`, `Makefile`, `PROJECT` file are created
- [x] 1.2 Verify the generated `PROJECT` file has `domain: metal.ironcore.dev`, `repo: github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator`, and `layout: go.kubebuilder.io/v4`; confirm single-group layout (no `--multigroup` needed — moo uses one group `onboarding` under domain `metal.ironcore.dev`, matching the single-group pattern of metal-operator)
- [x] 1.3 Add `github.com/ironcore-dev/metal-operator/api` as a Go module dependency (for `BMC`, `BMCSecret`, `ServerWiring` types) and verify `go mod tidy` succeeds
- [x] 1.4 Add a metaldhcp module stub (local replace directive or shim interface `DHCPLeaseReader`) in `internal/dhcp/lease.go` to unblock compilation before metaldhcp publishes a Go module; verify `go build ./...` succeeds

## 2. API Types — onboarding.metal.ironcore.dev

- [x] 2.1 Scaffold `OnboardingRequest` CRD: `kubebuilder create api --group onboarding --version v1alpha1 --kind OnboardingRequest` with fields: `spec.macAddress`, `spec.assignedIP`, `spec.dhcpLeaseRef`; `status.phase` (enum: InventoryLookup, RedfishProbe, BMCCreation, Done, Failed, Skipped), `status.reason`, `status.lastTransitionTime`, `status.managerType`, `status.conditions`; verify `make generate manifests` produces CRD YAML with all fields
- [x] 2.2 Scaffold `ServerProfile` CRD: `kubebuilder create api --group onboarding --version v1alpha1 --kind ServerProfile` with fields: `spec.macAddress`, `spec.serverName`, `spec.oobIP`, `spec.bmcHostname` (optional), `spec.clusterName`, `spec.siteConfigRef`, `spec.labels` (map[string]string, optional topology overrides); verify generated CRD YAML
- [x] 2.3 Scaffold `SiteConfig` CRD (cluster-scoped): `kubebuilder create api --group onboarding --version v1alpha1 --kind SiteConfig --namespaced=false` with fields: `spec.clusterName`; verify generated CRD YAML
- [x] 2.4 Write unit tests for type validation (required fields, MAC address format marker) and verify `make test` passes for the types package

## 3. InventoryProvider Interface and CRD Backend

- [x] 3.1 Define `InventoryProvider` interface in `internal/provider/provider.go` with `LookupByMAC(ctx, mac) (*InventoryRecord, error)` where `InventoryRecord` contains: `ClusterGate` (belongs/elsewhere/unknown), `ServerName` (used as BMC object name), `OOBIP`, `BMCHostname` (optional), `Labels map[string]string` (topology+cluster labels applied to BMC metadata); define sentinel `ErrNotFound`; verify it compiles
- [x] 3.2 Implement `CRDProvider` in `internal/provider/crd/provider.go` that reads `ServerProfile` CRs by MAC address and returns `ServerName`, `OOBIP`, `ClusterGate`, and any `Labels` declared in the `ServerProfile`; merges cluster identity defaults from referenced `SiteConfig`; verify unit tests cover: MAC match, no match → `ErrNotFound`, SiteConfig default inheritance
- [x] 3.3 Implement `NetBoxProvider` stub in `internal/provider/netbox/provider.go` (returns `ErrNotFound` for all MACs) with a `TODO` comment marking the real HTTP implementation; verify it satisfies the interface and `make test` passes

## 4. NetBox Provider Implementation

- [x] 4.1 Implement `NetBoxProvider.LookupByMAC` using the NetBox REST API; look up MAC → interface → device → cluster; return `ServerName` (device.Name), `OOBIP` (device.OOBIp.Address), `BMCHostname` (remoteboard interface DNS name, optional), `Labels` (10 topology labels: region, site.Slug, cluster.Name, cluster.Type.Slug, device.Name, bb-suffix, device.DeviceType.Slug, DeviceRole.Slug, Platform.Slug), and `ClusterGate`; verify unit tests with an httptest server cover: found device, device-not-found → `ErrNotFound`, 5xx → retriable error, wrong-cluster → `ClusterGate: elsewhere`
- [x] 4.2 Add operator flags `--netbox-url` and `--netbox-token-file`; wire into main.go and verify the operator starts with `--inventory-provider=netbox` and the flags populated

## 5. DHCPLease Watcher

- [x] 5.1 Scaffold `DHCPLeaseController` in `internal/controller/dhcplease_controller.go` that watches `dhcp.metal.ironcore.dev/v1alpha1 DHCPLease` (or the shim from 1.4) and creates one `OnboardingRequest` per unique MAC address using `CreateOrUpdate`; verify unit tests cover: new lease → OnboardingRequest created, second lease same MAC → no duplicate, lease deletion → OnboardingRequest survives
- [x] 5.2 Wire `DHCPLeaseController` into `cmd/manager/main.go` with configurable `--dhcp-lease-namespace` flag; verify operator starts and watches the configured namespace

## 6. OnboardingRequest Reconciler

- [x] 6.1 Scaffold `OnboardingRequestReconciler` in `internal/controller/onboardingrequest_controller.go`; implement phase dispatcher that reads `status.phase` and calls the matching handler; verify it compiles and `make test` passes with a no-op reconciler
- [x] 6.2 Implement `inventoryLookupPhase`: call `InventoryProvider.LookupByMAC`; on `ErrNotFound` after `--inventory-max-retries` → set phase `Failed/InventoryNotFound`; on `ClusterGate: elsewhere` → set phase `Skipped`; on success → store OOB IP in `status` and advance to `RedfishProbe`; verify unit tests cover all three outcomes
- [x] 6.3 Implement `redfishProbePhase`: unauthenticated GET `http://<oobIP>/redfish/v1`; on 200 + valid `ManagerType` → store `ManagerType` in `status.managerType` and advance to `BMCCreation`; on error → set phase `RedfishProbe/Unreachable` and requeue with backoff; verify unit tests with httptest cover: 200 success, connection refused, non-200
- [x] 6.4 Implement `bmcCreationPhase`: `CreateOrUpdate` `BMC` CR using `ServerName` from inventory as the object name, `OOBIP` as `spec.endpoint.ip`, protocol Redfish/443, `BMCHostname` as `spec.hostname` (when set), inventory `Labels` in `metadata.labels`, annotations `onboarding.metal.ironcore.dev/bootstrap=true` and `onboarding.metal.ironcore.dev/manager-type=<ManagerType>`; `CreateOrUpdate` `BMCSecret` (bootstrap credential placeholder); `CreateOrUpdate` `ServerWiring`; on success → set phase `Done`; verify unit tests with envtest cover: idempotent re-entry, both annotations present after creation, labels applied
- [x] 6.5 Write integration test using envtest: create `OnboardingRequest` with a mock `InventoryProvider` and httptest Redfish server; assert full phase progression to `Done`, `status.managerType` populated, and `BMC` CR has both handoff annotations

## 7. Operator Wiring and Configuration

- [x] 7.1 Wire `OnboardingRequestReconciler` into `cmd/manager/main.go` with `--inventory-provider` flag (values: `netbox`, `crd`); verify operator starts without panicking
- [x] 7.2 Add RBAC markers for all required permissions: watch `DHCPLease`, create/update `OnboardingRequest`, read `ServerProfile`/`SiteConfig`, create/update `BMC`/`BMCSecret`/`ServerWiring`; verify `make manifests` generates a `ClusterRole` covering all verbs
- [x] 7.3 Add leader-election flag (enabled by default); verify `--leader-elect=false` runs in local/dev mode without needing a lease

## 8. Helm Chart and Deployment

- [x] 8.1 Scaffold a Helm chart in `charts/metal-onboarding-operator/` with Deployment, ServiceAccount, ClusterRole, ClusterRoleBinding, and CRD install; verify `helm template` renders without errors
- [x] 8.2 Add `--inventory-provider`, `--dhcp-lease-namespace`, `--netbox-url`, `--netbox-token-file` as chart values; verify `helm template --set inventoryProvider=crd` renders the correct flag in the Deployment
- [x] 8.3 Add a `make deploy` target using `helm upgrade --install` and verify it installs cleanly against a kind cluster with metaldhcp CRDs applied

## 9. Integration Verification

- [ ] 9.1 Against a kind cluster with metaldhcp CRDs applied: create a `DHCPLease` CR manually and verify an `OnboardingRequest` CR is created within 10 seconds
- [ ] 9.2 With `--inventory-provider=crd` and a matching `ServerProfile`: create an `OnboardingRequest` and verify full phase progression to `Done` with a mock httptest Redfish endpoint; assert `BMC` CR has `bootstrap=true` and `manager-type` annotations
- [x] 9.3 Run `make test` and verify all unit and envtest tests pass
