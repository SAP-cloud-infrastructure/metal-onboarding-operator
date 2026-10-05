# Tasks

## 1. Project Scaffold

- [ ] 1.1 Run `kubebuilder init --domain metal.ironcore.dev --repo github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator` and verify `go.mod`, `Makefile`, `PROJECT` file are created
- [ ] 1.2 Run `kubebuilder edit --multigroup=true` and verify `PROJECT` file reflects multigroup layout
- [ ] 1.3 Add `github.com/ironcore-dev/metal-operator/api` as a Go module dependency (for `BMC`, `BMCSecret`, `ServerWiring` types) and verify `go mod tidy` succeeds
- [ ] 1.4 Add a metaldhcp module stub (local replace directive or shim interface `DHCPLeaseReader`) in `internal/dhcp/lease.go` to unblock compilation before metaldhcp publishes a Go module; verify `go build ./...` succeeds

## 2. API Types — onboarding.metal.ironcore.dev

- [ ] 2.1 Scaffold `OnboardingRequest` CRD: `kubebuilder create api --group onboarding --version v1alpha1 --kind OnboardingRequest` with fields: `spec.macAddress`, `spec.assignedIP`, `spec.dhcpLeaseRef`, `spec.onboardingSettings` (struct with `bmcUser.username`, `bmcUser.roleID`, `bmcSettings.hostname`, `bmcSettings.ntpServers`, `bmcSettings.adDomain`, `bmcSettings.syslogServer`); `status.phase` (enum: InventoryLookup, RedfishProbe, BMCCreation, Done, Failed, Skipped), `status.reason`, `status.lastTransitionTime`, `status.conditions`; verify `make generate manifests` produces CRD YAML with all fields
- [ ] 2.2 Scaffold `ServerProfile` CRD: `kubebuilder create api --group onboarding --version v1alpha1 --kind ServerProfile` with fields: `spec.macAddress`, `spec.hostname`, `spec.oobIP`, `spec.ntpServers`, `spec.adDomain`, `spec.syslogServer`, `spec.siteConfigRef`; verify generated CRD YAML
- [ ] 2.3 Scaffold `SiteConfig` CRD (cluster-scoped): `kubebuilder create api --group onboarding --version v1alpha1 --kind SiteConfig --namespaced=false` with fields: `spec.ntpServers`, `spec.syslogServer`, `spec.adDomain`; verify generated CRD YAML
- [ ] 2.4 Write unit tests for type validation (required fields, MAC address format marker) and verify `make test` passes for the types package

## 3. InventoryProvider Interface and CRD Backend

- [ ] 3.1 Define `InventoryProvider` interface in `internal/provider/provider.go` with `LookupByMAC(ctx, mac) (*InventoryRecord, error)` and sentinel `ErrNotFound`; verify it compiles
- [ ] 3.2 Implement `CRDProvider` in `internal/provider/crd/provider.go` that reads `ServerProfile` CRs by MAC address and merges defaults from referenced `SiteConfig`; verify unit tests cover: MAC match, no match → `ErrNotFound`, SiteConfig default inheritance
- [ ] 3.3 Implement `NetBoxProvider` stub in `internal/provider/netbox/provider.go` (returns `ErrNotFound` for all MACs) with a `TODO` comment marking the real HTTP implementation; verify it satisfies the interface and `make test` passes

## 4. NetBox Provider Implementation

- [ ] 4.1 Implement `NetBoxProvider.LookupByMAC` using the NetBox REST API (`/api/dcim/interfaces/?mac_address=<mac>&populate_inventory=true`); return hostname, cluster membership gate, OOB IP, NTP, AD/LDAP, syslog from device custom fields; verify unit tests with an httptest server cover: found device, device-not-found → `ErrNotFound`, 5xx → retriable error, wrong-cluster → `InventoryRecord.Cluster != localCluster`
- [ ] 4.2 Add operator flag `--netbox-url` and `--netbox-token-file`; wire into main.go and verify the operator starts with `--inventory-provider=netbox` and the flags populated

## 5. DHCPLease Watcher

- [ ] 5.1 Scaffold `DHCPLeaseController` in `internal/controller/dhcplease_controller.go` that watches `dhcp.metal.ironcore.dev/v1alpha1 DHCPLease` (or the shim from 1.4) and creates one `OnboardingRequest` per unique MAC address using `CreateOrUpdate`; verify unit tests cover: new lease → OnboardingRequest created, second lease same MAC → no duplicate, lease deletion → OnboardingRequest survives
- [ ] 5.2 Wire `DHCPLeaseController` into `cmd/manager/main.go` with configurable `--dhcp-lease-namespace` flag; verify operator starts and watches the configured namespace

## 6. OnboardingRequest Reconciler

- [ ] 6.1 Scaffold `OnboardingRequestReconciler` in `internal/controller/onboardingrequest_controller.go`; implement phase dispatcher that reads `status.phase` and calls the matching handler; verify it compiles and `make test` passes with a no-op reconciler
- [ ] 6.2 Implement `inventoryLookupPhase`: call `InventoryProvider.LookupByMAC`; on `ErrNotFound` after `--inventory-max-retries` → set phase `Failed/InventoryNotFound`; on wrong-cluster → set phase `Skipped`; on success → write resolved settings into `spec.onboardingSettings` (patch once, skip if already set) and advance to `RedfishProbe`; verify unit tests cover all three outcomes plus idempotent re-entry when settings already written
- [ ] 6.3 Implement `redfishProbePhase`: GET `http://<oobIP>/redfish/v1` unauthenticated; on 200 + valid ManagerType → advance to `BMCCreation`; on error → set phase `RedfishProbe/Unreachable` and requeue with backoff; verify unit tests with httptest cover: 200 success, connection refused, non-200
- [ ] 6.4 Implement `bmcCreationPhase`: `CreateOrUpdate` `BMC` CR with annotations `onboarding.metal.ironcore.dev/bootstrap=true` and `onboarding.metal.ironcore.dev/onboarding-request=<name>`; `CreateOrUpdate` `BMCSecret` (bootstrap credential placeholder); `CreateOrUpdate` `ServerWiring`; on success → set phase `Done`; verify unit tests with envtest cover: idempotent re-entry, both annotations present after creation
- [ ] 6.5 Write integration test using envtest: create `OnboardingRequest` with a mock `InventoryProvider` and httptest Redfish server; assert full phase progression to `Done`, `spec.onboardingSettings` populated with inventory data, and `BMC` CR has both handoff annotations

## 7. Operator Wiring and Configuration

- [ ] 7.1 Wire `OnboardingRequestReconciler` into `cmd/manager/main.go` with `--inventory-provider` flag (values: `netbox`, `crd`); verify operator starts without panicking
- [ ] 7.2 Add RBAC markers for all required permissions: watch `DHCPLease`, create/update `OnboardingRequest`, read `ServerProfile`/`SiteConfig`, create/update `BMC`/`BMCSecret`/`ServerWiring`; verify `make manifests` generates a `ClusterRole` covering all verbs
- [ ] 7.3 Add leader-election flag (enabled by default); verify `--leader-elect=false` runs in local/dev mode without needing a lease

## 8. Helm Chart and Deployment

- [ ] 8.1 Scaffold a Helm chart in `charts/metal-onboarding-operator/` with Deployment, ServiceAccount, ClusterRole, ClusterRoleBinding, and CRD install; verify `helm template` renders without errors
- [ ] 8.2 Add `--inventory-provider`, `--dhcp-lease-namespace`, `--netbox-url`, `--netbox-token-file` as chart values; verify `helm template --set inventoryProvider=crd` renders the correct flag in the Deployment
- [ ] 8.3 Add a `make deploy` target using `helm upgrade --install` and verify it installs cleanly against a kind cluster with metaldhcp CRDs applied

## 9. Integration Verification

- [ ] 9.1 Against a kind cluster with metaldhcp CRDs applied: create a `DHCPLease` CR manually and verify an `OnboardingRequest` CR is created within 10 seconds
- [ ] 9.2 With `--inventory-provider=crd` and a matching `ServerProfile`: create an `OnboardingRequest` and verify full phase progression to `Done` with a mock httptest Redfish endpoint; assert `BMC` CR has both handoff annotations
- [ ] 9.3 Run `make test` and verify all unit and envtest tests pass
