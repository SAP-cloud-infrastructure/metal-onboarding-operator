/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

const undiscoveredTaintKey = "metal.ironcore.dev/Undiscovered"

// ServerDiscoveryController watches Server CRs created by moo (identified via
// the managed-by label) and ensures each gets the Undiscovered NoBind taint
// plus a ServerReadinessRule that removes it once MMO sets Discovered=True.
type ServerDiscoveryController struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverreadinessrules,verbs=get;list;watch;create;update;patch;delete

func (r *ServerDiscoveryController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.ensureUndiscoveredTaint(ctx, server); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ensureReadinessRule(ctx, log, server); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ServerDiscoveryController) ensureUndiscoveredTaint(ctx context.Context, server *metalv1alpha1.Server) error {
	for _, t := range server.Spec.Taints {
		if t.Key == undiscoveredTaintKey && t.Effect == metalv1alpha1.TaintEffectNoBind {
			return nil
		}
	}

	patch := client.MergeFrom(server.DeepCopy())
	server.Spec.Taints = append(server.Spec.Taints, metalv1alpha1.Taint{
		Key:    undiscoveredTaintKey,
		Effect: metalv1alpha1.TaintEffectNoBind,
	})
	return r.Patch(ctx, server, patch)
}

func (r *ServerDiscoveryController) ensureReadinessRule(ctx context.Context, log interface{ Info(string, ...any) }, server *metalv1alpha1.Server) error {
	desired := r.desiredRule(server)

	existing := &metalv1alpha1.ServerReadinessRule{}
	err := r.Get(ctx, types.NamespacedName{Name: server.Name}, existing)
	if apierrors.IsNotFound(err) {
		if createErr := r.Create(ctx, desired); createErr != nil {
			return createErr
		}
		log.Info("created ServerReadinessRule", "name", server.Name)
		return nil
	}
	if err != nil {
		return err
	}

	if reflect.DeepEqual(existing.Spec, desired.Spec) {
		return nil
	}

	// Spec changed (e.g. stale rule) — delete and recreate; spec fields are immutable.
	if err := r.Delete(ctx, existing); err != nil {
		return err
	}
	return r.Create(ctx, desired)
}

func (r *ServerDiscoveryController) desiredRule(server *metalv1alpha1.Server) *metalv1alpha1.ServerReadinessRule {
	rule := &metalv1alpha1.ServerReadinessRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: server.Name,
		},
		Spec: metalv1alpha1.ServerReadinessRuleSpec{
			ServerSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelManagedBy: labelManagedByValue,
				},
			},
			Conditions: []metalv1alpha1.ConditionRequirement{
				{
					Type:           "Discovered",
					RequiredStatus: metav1.ConditionTrue,
				},
			},
			EnforcementMode: metalv1alpha1.EnforcementModeBootstrapOnly,
			Taint: metalv1alpha1.Taint{
				Key:    undiscoveredTaintKey,
				Effect: metalv1alpha1.TaintEffectNoBind,
			},
		},
	}
	_ = controllerutil.SetControllerReference(server, rule, r.Scheme)
	return rule
}

func (r *ServerDiscoveryController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.Server{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			return obj.GetLabels()[labelManagedBy] == labelManagedByValue
		}))).
		Named("serverdiscovery").
		Complete(r)
}
