# Tasks

## 1. Add BMCUserSet API dependency

- [x] 1.1 Add `github.com/ironcore-dev/metal-maintenance-operator` to `go.mod` (direct dependency); run `go mod tidy`; verify `go build ./...` succeeds
- [x] 1.2 Register `baseboardv1alpha1.AddToScheme(scheme)` in `cmd/main.go`; verify `go build ./cmd/...` succeeds

## 2. Implement reconcileBMCUserSet in the controller

- [x] 2.1 Add `BMCUserSetConfig` struct to `internal/controller/dhcplease_controller.go` with fields: `SetupCredentialRef corev1.SecretReference`, `FactoryUserNames []string`, `RotationStrategy baseboardv1alpha1.RotationStrategy`, `UserName string`, `RoleID string`, `RotationPeriod metav1.Duration`, `CredentialSecretNamespace string`
- [x] 2.2 Add `BMCUserSet *BMCUserSetConfig` field to `DHCPLeaseController`; add `// +kubebuilder:rbac:groups=baseboard.metal.ironcore.dev,resources=bmcusersets,verbs=get;list;watch;create` marker
- [x] 2.3 Implement `reconcileBMCUserSet(ctx, bmcName string) error`: get existing `BMCUserSet` by name; return nil if found; if `IsNotFound`, create with `bmcSelector.matchLabels["kubernetes.metal.cloud.sap/name"]: bmcName`; return nil; return error on any other get/create failure; log "Created BMCUserSet" on success
- [x] 2.4 Call `r.reconcileBMCUserSet(ctx, rec.ServerName)` at the end of `reconcileBMC`, after `patchLeaseAnnotation`; propagate error as `fmt.Errorf("reconcile BMCUserSet %s: %w", ...)`
- [x] 2.5 Run `make test` and verify all existing tests still pass

## 3. Wire flags in cmd/main.go

- [x] 3.1 Add 8 `--bmc-userset-*` flags: `setup-credential-ref-name` (default `""`), `setup-credential-ref-namespace` (default `"metal-servers"`), `factory-usernames` (default `"support"`), `rotation-strategy` (default `DualAccount`), `username` (default `"metal"`), `role-id` (default `"Administrator"`), `rotation-period` (default `720h`), `credential-secret-namespace` (default `"metal-servers"`)
- [x] 3.2 Add `buildBMCUserSetConfig` helper: returns `nil` when `credentialRefName` is empty; otherwise builds and returns `*BMCUserSetConfig`; add `splitComma` helper for factory usernames
- [x] 3.3 Log `"--bmc-userset-setup-credential-ref-name not set; BMCUserSet creation disabled"` when the flag is empty
- [x] 3.4 Wire `BMCUserSet: buildBMCUserSetConfig(...)` into `DHCPLeaseController` initialization; verify `go build ./...` and `make lint` pass

## 4. Update Helm chart

- [x] 4.1 Add `bmcUserSet` block to `charts/metal-onboarding-operator/values.yaml` with all 8 fields matching the flag defaults
- [x] 4.2 Add conditional `{{- if .Values.bmcUserSet.setupCredentialRef.name }}` block in `charts/metal-onboarding-operator/templates/deployment.yaml` emitting all 8 `--bmc-userset-*` args
- [x] 4.3 Add `baseboard.metal.ironcore.dev / bmcusersets` rule (`create;get;list;watch`) to `charts/metal-onboarding-operator/templates/clusterrole.yaml`

## 5. Regenerate RBAC and verify

- [x] 5.1 Run `make manifests` to regenerate `config/rbac/role.yaml`; verify it gains the `bmcusersets` rule matching the kubebuilder annotation
- [x] 5.2 Run `make lint` — 0 issues (fix any `lll` violations by wrapping long flag description strings)
- [x] 5.3 Run `make test` — all tests pass
