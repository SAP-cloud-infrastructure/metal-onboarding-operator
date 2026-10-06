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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	onboardingv1alpha1 "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
	dhcpshim "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/dhcp"
)

func newDHCPTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = onboardingv1alpha1.AddToScheme(s)
	_ = dhcpshim.AddToScheme(s)
	return s
}

var _ = Describe("DHCPLeaseController unit tests", func() {
	const ns = "default"
	const testMAC = "aa:bb:cc:dd:ee:ff"

	newLease := func(name, mac, ip string) *dhcpshim.DHCPLease {
		return &dhcpshim.DHCPLease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       dhcpshim.DHCPLeaseSpec{MACAddress: mac, IPAddress: ip},
		}
	}

	It("creates an OnboardingRequest for a new DHCPLease", func() {
		lease := newLease("lease-1", testMAC, "10.0.0.1")
		s := newDHCPTestScheme()
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lease).Build()
		r := &DHCPLeaseController{Client: cl, Scheme: s, Namespace: ns}

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "lease-1", Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())

		or := &onboardingv1alpha1.OnboardingRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: "mac-aa-bb-cc-dd-ee-ff", Namespace: ns}, or)).To(Succeed())
		Expect(or.Spec.MACAddress).To(Equal(testMAC))
		Expect(or.Spec.AssignedIP).To(Equal("10.0.0.1"))
		Expect(or.Spec.DHCPLeaseRef).NotTo(BeNil())
	})

	It("does not create a duplicate OnboardingRequest for the same MAC", func() {
		lease1 := newLease("lease-1", testMAC, "10.0.0.1")
		lease2 := newLease("lease-2", testMAC, "10.0.0.2")
		s := newDHCPTestScheme()
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lease1, lease2).Build()
		r := &DHCPLeaseController{Client: cl, Scheme: s, Namespace: ns}

		for _, name := range []string{"lease-1", "lease-2"} {
			_, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())
		}

		list := &onboardingv1alpha1.OnboardingRequestList{}
		Expect(cl.List(context.Background(), list)).To(Succeed())
		Expect(list.Items).To(HaveLen(1))
		Expect(list.Items[0].Spec.AssignedIP).To(Equal("10.0.0.1"))
	})

	It("OnboardingRequest survives when the DHCPLease is deleted", func() {
		existingOR := &onboardingv1alpha1.OnboardingRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "mac-aa-bb-cc-dd-ee-ff", Namespace: ns},
			Spec:       onboardingv1alpha1.OnboardingRequestSpec{MACAddress: testMAC},
		}
		s := newDHCPTestScheme()
		// No lease in the fake client — simulates a deleted lease triggering reconcile.
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(existingOR).Build()
		r := &DHCPLeaseController{Client: cl, Scheme: s, Namespace: ns}

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "lease-1", Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())

		or := &onboardingv1alpha1.OnboardingRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: "mac-aa-bb-cc-dd-ee-ff", Namespace: ns}, or)).To(Succeed())
		Expect(or.Spec.MACAddress).To(Equal(testMAC))
	})

	It("skips a DHCPLease with no MACAddress", func() {
		lease := newLease("lease-empty", "", "10.0.0.3")
		s := newDHCPTestScheme()
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lease).Build()
		r := &DHCPLeaseController{Client: cl, Scheme: s, Namespace: ns}

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "lease-empty", Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())

		list := &onboardingv1alpha1.OnboardingRequestList{}
		Expect(cl.List(context.Background(), list)).To(Succeed())
		Expect(list.Items).To(BeEmpty())
	})
})
