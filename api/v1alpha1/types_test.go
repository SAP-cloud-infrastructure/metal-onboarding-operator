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

package v1alpha1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	onboardingv1alpha1 "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
)

var _ = Describe("OnboardingRequest types", func() {
	It("accepts a valid MAC address", func() {
		req := &onboardingv1alpha1.OnboardingRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec: onboardingv1alpha1.OnboardingRequestSpec{
				MACAddress: "aa:bb:cc:dd:ee:ff",
			},
		}
		Expect(req.Spec.MACAddress).To(Equal("aa:bb:cc:dd:ee:ff"))
	})

	It("has all expected phase constants", func() {
		Expect(onboardingv1alpha1.PhaseInventoryLookup).To(Equal(onboardingv1alpha1.OnboardingPhase("InventoryLookup")))
		Expect(onboardingv1alpha1.PhaseRedfishProbe).To(Equal(onboardingv1alpha1.OnboardingPhase("RedfishProbe")))
		Expect(onboardingv1alpha1.PhaseBMCCreation).To(Equal(onboardingv1alpha1.OnboardingPhase("BMCCreation")))
		Expect(onboardingv1alpha1.PhaseDone).To(Equal(onboardingv1alpha1.OnboardingPhase("Done")))
		Expect(onboardingv1alpha1.PhaseFailed).To(Equal(onboardingv1alpha1.OnboardingPhase("Failed")))
		Expect(onboardingv1alpha1.PhaseSkipped).To(Equal(onboardingv1alpha1.OnboardingPhase("Skipped")))
	})

	It("deep copies without sharing pointers", func() {
		now := metav1.Now()
		orig := &onboardingv1alpha1.OnboardingRequest{
			Spec: onboardingv1alpha1.OnboardingRequestSpec{
				MACAddress: "aa:bb:cc:dd:ee:ff",
				AssignedIP: "10.0.0.1",
			},
			Status: onboardingv1alpha1.OnboardingRequestStatus{
				Phase:              onboardingv1alpha1.PhaseRedfishProbe,
				LastTransitionTime: &now,
				ManagerType:        "iDRAC",
			},
		}
		copy := orig.DeepCopy()
		Expect(copy.Spec.MACAddress).To(Equal(orig.Spec.MACAddress))
		Expect(copy.Status.ManagerType).To(Equal(orig.Status.ManagerType))
		copy.Status.ManagerType = "iLO"
		Expect(orig.Status.ManagerType).To(Equal("iDRAC"))
	})
})

var _ = Describe("ServerProfile types", func() {
	It("holds all required fields", func() {
		sp := &onboardingv1alpha1.ServerProfile{
			Spec: onboardingv1alpha1.ServerProfileSpec{
				MACAddress: "aa:bb:cc:dd:ee:ff",
				ServerName: "r-abc12-bb01",
				OOBIP:      "10.10.0.1",
			},
		}
		Expect(sp.Spec.ServerName).To(Equal("r-abc12-bb01"))
		Expect(sp.Spec.OOBIP).To(Equal("10.10.0.1"))
	})

	It("supports optional labels map", func() {
		sp := &onboardingv1alpha1.ServerProfile{
			Spec: onboardingv1alpha1.ServerProfileSpec{
				MACAddress: "aa:bb:cc:dd:ee:ff",
				ServerName: "r-abc12-bb01",
				OOBIP:      "10.10.0.1",
				Labels: map[string]string{
					"topology.kubernetes.io/region": "eu-de-1",
				},
			},
		}
		Expect(sp.Spec.Labels).To(HaveKey("topology.kubernetes.io/region"))
	})

	It("deep copies labels without sharing the map", func() {
		orig := &onboardingv1alpha1.ServerProfile{
			Spec: onboardingv1alpha1.ServerProfileSpec{
				MACAddress: "aa:bb:cc:dd:ee:ff",
				ServerName: "r-abc12-bb01",
				OOBIP:      "10.10.0.1",
				Labels:     map[string]string{"key": "val"},
			},
		}
		copy := orig.DeepCopy()
		copy.Spec.Labels["key"] = "changed"
		Expect(orig.Spec.Labels["key"]).To(Equal("val"))
	})
})

var _ = Describe("SiteConfig types", func() {
	It("holds clusterName and defaultLabels", func() {
		sc := &onboardingv1alpha1.SiteConfig{
			Spec: onboardingv1alpha1.SiteConfigSpec{
				ClusterName: "my-cluster",
				DefaultLabels: map[string]string{
					"topology.kubernetes.io/zone": "eu-de-1a",
				},
			},
		}
		Expect(sc.Spec.ClusterName).To(Equal("my-cluster"))
		Expect(sc.Spec.DefaultLabels).To(HaveKey("topology.kubernetes.io/zone"))
	})
})
