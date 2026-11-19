# Tasks

## 1. API field

- [x] 1.1 Add optional `RemoteboardHostname string` with `json:"remoteboardHostname,omitempty"` to `ServerEntry` in `api/v1alpha1/metalcluster_types.go`, documented as the remoteboard DNS hostname written to `BMC spec.hostname`, falling back to `Name` when empty
- [x] 1.2 Run `make generate manifests` and verify the regenerated MetalCluster CRD under `config/crd/bases/` and `charts/metal-onboarding-operator/crds/` contains `remoteboardHostname`

## 2. Provider

- [x] 2.1 In `recordFrom` (`internal/provider/metalcluster/provider.go`), set `BMCHostname` to `RemoteboardHostname`, falling back to `Name` when empty
- [x] 2.2 Add a table-driven test in `internal/provider/metalcluster/provider_test.go`: explicit `RemoteboardHostname` → record carries it; empty → record carries `Name`

## 3. Verification

- [x] 3.1 Run `make test` and verify all tests pass
- [ ] 3.2 Run `make lint` and verify 0 issues
