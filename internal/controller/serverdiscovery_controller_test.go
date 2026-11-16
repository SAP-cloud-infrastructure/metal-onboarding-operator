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
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("ServerDiscoveryController", func() {
	newServer := func(name string, labels map[string]string, taints []metalv1alpha1.Taint) *metalv1alpha1.Server {
		return &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
			Spec: metalv1alpha1.ServerSpec{
				Taints: taints,
			},
		}
	}

	newController := func(server *metalv1alpha1.Server) *ServerDiscoveryController {
		s := newFakeScheme()
		_ = metalv1alpha1.AddToScheme(s)
		c := fake.NewClientBuilder().WithScheme(s).WithObjects(server).Build()
		return &ServerDiscoveryController{Client: c, Scheme: s}
	}

	It("adds the Undiscovered taint to a Server that has none", func() {
		server := newServer("node001", map[string]string{labelManagedBy: labelManagedByValue}, nil)
		r := newController(server)

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "node001"}})
		Expect(err).NotTo(HaveOccurred())

		updated := &metalv1alpha1.Server{}
		Expect(r.Get(ctx, types.NamespacedName{Name: "node001"}, updated)).To(Succeed())
		Expect(updated.Spec.Taints).To(ContainElement(metalv1alpha1.Taint{
			Key:    undiscoveredTaintKey,
			Effect: metalv1alpha1.TaintEffectNoBind,
		}))
	})

	It("does not double-add the taint when it is already present", func() {
		server := newServer("node002", map[string]string{labelManagedBy: labelManagedByValue}, []metalv1alpha1.Taint{
			{Key: undiscoveredTaintKey, Effect: metalv1alpha1.TaintEffectNoBind},
		})
		r := newController(server)

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "node002"}})
		Expect(err).NotTo(HaveOccurred())

		updated := &metalv1alpha1.Server{}
		Expect(r.Get(ctx, types.NamespacedName{Name: "node002"}, updated)).To(Succeed())
		count := 0
		for _, t := range updated.Spec.Taints {
			if t.Key == undiscoveredTaintKey {
				count++
			}
		}
		Expect(count).To(Equal(1))
	})

	It("creates a ServerReadinessRule with the correct spec and owner reference", func() {
		server := newServer("node003", map[string]string{labelManagedBy: labelManagedByValue}, nil)
		r := newController(server)

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "node003"}})
		Expect(err).NotTo(HaveOccurred())

		rule := &metalv1alpha1.ServerReadinessRule{}
		Expect(r.Get(ctx, types.NamespacedName{Name: "node003"}, rule)).To(Succeed())

		Expect(rule.Spec.EnforcementMode).To(Equal(metalv1alpha1.EnforcementModeBootstrapOnly))
		Expect(rule.Spec.Taint.Key).To(Equal(undiscoveredTaintKey))
		Expect(rule.Spec.Taint.Effect).To(Equal(metalv1alpha1.TaintEffectNoBind))
		Expect(rule.Spec.Conditions).To(HaveLen(1))
		Expect(rule.Spec.Conditions[0].Type).To(Equal("Discovered"))
		Expect(rule.Spec.Conditions[0].RequiredStatus).To(Equal(metav1.ConditionTrue))

		Expect(rule.OwnerReferences).To(HaveLen(1))
		Expect(rule.OwnerReferences[0].Name).To(Equal("node003"))
	})

	It("is idempotent when taint and rule are already correct", func() {
		server := newServer("node004", map[string]string{labelManagedBy: labelManagedByValue}, []metalv1alpha1.Taint{
			{Key: undiscoveredTaintKey, Effect: metalv1alpha1.TaintEffectNoBind},
		})
		r := newController(server)

		// First reconcile — creates rule
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "node004"}})
		Expect(err).NotTo(HaveOccurred())

		// Second reconcile — should be a no-op
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "node004"}})
		Expect(err).NotTo(HaveOccurred())

		ruleList := &metalv1alpha1.ServerReadinessRuleList{}
		Expect(r.List(ctx, ruleList)).To(Succeed())
		Expect(ruleList.Items).To(HaveLen(1))
	})
})
