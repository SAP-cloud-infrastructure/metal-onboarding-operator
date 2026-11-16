# Tasks

## 1. Add managed-by label to BMC CRs

- [x] 1.1 In `internal/controller/dhcplease_controller.go`, add constant `labelManagedBy = "onboarding.metal.ironcore.dev/managed-by"` and value `"metal-onboarding-operator"`. In `reconcileBMC`, add this label to `bmc.Labels` at both the creation path (`apierrors.IsNotFound`) and the patch path (alongside the existing `maps.Copy`). Verify by running `make test` — the existing `TestDHCPLeaseController` tests that check BMC creation should still pass, and the new label should appear on the BMC object.

## 2. New ServerDiscoveryController

- [x] 2.1 Create `internal/controller/serverdiscovery_controller.go`. Define `ServerDiscoveryController` with fields `client.Client` and `Scheme *runtime.Scheme`. Add RBAC markers: `// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;patch` and `// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverreadinessrules,verbs=get;list;watch;create;update;patch;delete`. Verify the file compiles with `go build ./internal/controller/...`.

- [x] 2.2 Implement `Reconcile`: fetch the `Server` by `req.NamespacedName`; skip if not found. Call `ensureUndiscoveredTaint` (see 2.3) and `ensureReadinessRule` (see 2.4). Return any error. Verify by running `make test` after tasks 2.3–2.4 are complete.

- [x] 2.3 Implement `ensureUndiscoveredTaint(ctx, server)`: check if `server.Spec.Taints` already contains `{Key: "metal.ironcore.dev/Undiscovered", Effect: metalv1alpha1.TaintEffectNoBind}`; if not, patch it in using `client.MergeFrom` with the taint appended. Verify by unit test in `serverdiscovery_controller_test.go` (see task 3): a Server without the taint gets it patched; a Server with it is not double-tainted.

- [x] 2.4 Implement `ensureReadinessRule(ctx, server)`: try to get `ServerReadinessRule` by `server.Name`; if not found, create one with `serverSelector` matching `metadata.name=<server.Name>`, `conditions: [{type: "Discovered", requiredStatus: "True"}]`, `enforcementMode: BootstrapOnly`, `taint: {key: "metal.ironcore.dev/Undiscovered", effect: NoBind}`, and owner reference set via `controllerutil.SetControllerReference(server, rule, r.Scheme)`. If found with different spec, delete and recreate. Verify by unit test (see task 3): rule is created with correct spec and owner reference.

- [x] 2.5 Implement `SetupWithManager`: use `ctrl.NewControllerManagedBy(mgr).For(&metalv1alpha1.Server{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool { return obj.GetLabels()[labelManagedBy] == "metal-onboarding-operator" }))).Named("serverdiscovery").Complete(r)`. Verify by running `make manifests` and confirming no new CRD files are generated (no new CRD owned by moo).

## 3. Unit tests for ServerDiscoveryController

- [x] 3.1 Create `internal/controller/serverdiscovery_controller_test.go` in the existing `controller` package (same Ginkgo suite as `dhcplease_controller_test.go`). Add tests:
  - A Server without the taint gets it added.
  - A Server already with the taint is not double-tainted (idempotent).
  - A `ServerReadinessRule` is created with the correct spec and owner reference.
  - Re-reconciling a Server that already has both the taint and the rule is a no-op (no extra API calls).
  Verify with `make test`.

## 4. Wire up in cmd/main.go and regenerate RBAC

- [x] 4.1 In `cmd/main.go`, after the `DHCPLeaseController` setup block, add setup for `ServerDiscoveryController`: `(&controller.ServerDiscoveryController{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr)`. Verify the binary compiles with `go build ./cmd/...`.

- [x] 4.2 Run `make manifests` to regenerate `config/rbac/role.yaml` from the new RBAC markers. Verify `config/rbac/role.yaml` contains entries for `servers` (get/list/watch/patch) and `serverreadinessrules` (get/list/watch/create/update/patch/delete) under `metal.ironcore.dev`.

- [x] 4.3 Run `make lint-fix` and `make test` to confirm no lint errors and all tests pass.
