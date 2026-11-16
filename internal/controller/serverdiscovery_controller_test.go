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
	newServer := func(name string, labels map[string]string) *metalv1alpha1.Server {
		return &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
		}
	}

	newController := func(server *metalv1alpha1.Server) *ServerDiscoveryController {
		s := newFakeScheme()
		_ = metalv1alpha1.AddToScheme(s)
		c := fake.NewClientBuilder().WithScheme(s).WithObjects(server).Build()
		return &ServerDiscoveryController{Client: c, Scheme: s}
	}

	It("creates a ServerReadinessRule with the correct spec and owner reference", func() {
		server := newServer("node001", map[string]string{labelManagedBy: labelManagedByValue})
		r := newController(server)

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "node001"}})
		Expect(err).NotTo(HaveOccurred())

		rule := &metalv1alpha1.ServerReadinessRule{}
		Expect(r.Get(ctx, types.NamespacedName{Name: "node001"}, rule)).To(Succeed())

		Expect(rule.Spec.EnforcementMode).To(Equal(metalv1alpha1.EnforcementModeBootstrapOnly))
		Expect(rule.Spec.Taint.Key).To(Equal(undiscoveredTaintKey))
		Expect(rule.Spec.Taint.Effect).To(Equal(metalv1alpha1.TaintEffectNoBind))
		Expect(rule.Spec.Conditions).To(HaveLen(1))
		Expect(rule.Spec.Conditions[0].Type).To(Equal("Discovered"))
		Expect(rule.Spec.Conditions[0].RequiredStatus).To(Equal(metav1.ConditionTrue))

		Expect(rule.OwnerReferences).To(HaveLen(1))
		Expect(rule.OwnerReferences[0].Name).To(Equal("node001"))
	})

	It("is idempotent when the rule already exists with the correct spec", func() {
		server := newServer("node002", map[string]string{labelManagedBy: labelManagedByValue})
		r := newController(server)

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "node002"}})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "node002"}})
		Expect(err).NotTo(HaveOccurred())

		ruleList := &metalv1alpha1.ServerReadinessRuleList{}
		Expect(r.List(ctx, ruleList)).To(Succeed())
		Expect(ruleList.Items).To(HaveLen(1))
	})
})
