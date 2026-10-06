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

package crd_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"

	onboardingv1alpha1 "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
	crdprovider "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/provider/crd"

	"github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/provider"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = onboardingv1alpha1.AddToScheme(s)
	return s
}

var _ = Describe("CRD Provider", func() {
	const namespace = "test-ns"
	const testMAC = "aa:bb:cc:dd:ee:ff"

	newServerProfile := func(mac, name, oobIP string, extraLabels map[string]string, siteRef *corev1.LocalObjectReference) *onboardingv1alpha1.ServerProfile {
		return &onboardingv1alpha1.ServerProfile{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: onboardingv1alpha1.ServerProfileSpec{
				MACAddress:   mac,
				ServerName:   name,
				OOBIP:        oobIP,
				Labels:       extraLabels,
				SiteConfigRef: siteRef,
			},
		}
	}

	It("returns the matching ServerProfile record", func() {
		sp := newServerProfile(testMAC, "r-abc12-bb01", "10.10.0.1", nil, nil)
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(sp).Build()
		p := crdprovider.New(c, namespace)

		rec, err := p.LookupByMAC(context.Background(), testMAC)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.ServerName).To(Equal("r-abc12-bb01"))
		Expect(rec.OOBIP).To(Equal("10.10.0.1"))
		Expect(rec.ClusterGate).To(Equal(provider.ClusterGateUnknown))
	})

	It("returns ErrNotFound when no ServerProfile matches", func() {
		sp := newServerProfile("11:22:33:44:55:66", "other-server", "10.10.0.2", nil, nil)
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(sp).Build()
		p := crdprovider.New(c, namespace)

		_, err := p.LookupByMAC(context.Background(), testMAC)
		Expect(err).To(MatchError(provider.ErrNotFound))
	})

	It("merges SiteConfig default labels", func() {
		siteConfig := &onboardingv1alpha1.SiteConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "site-a"},
			Spec: onboardingv1alpha1.SiteConfigSpec{
				ClusterName: "prod-cluster",
				DefaultLabels: map[string]string{
					"topology.kubernetes.io/region": "eu-de-1",
				},
			},
		}
		sp := newServerProfile(testMAC, "r-abc12-bb01", "10.10.0.1", nil,
			&corev1.LocalObjectReference{Name: "site-a"})
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(sp, siteConfig).Build()
		p := crdprovider.New(c, namespace)

		rec, err := p.LookupByMAC(context.Background(), testMAC)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Labels).To(HaveKeyWithValue("topology.kubernetes.io/region", "eu-de-1"))
		Expect(rec.ClusterGate).To(Equal(provider.ClusterGateBelongs))
	})

	It("ServerProfile labels override SiteConfig defaults", func() {
		siteConfig := &onboardingv1alpha1.SiteConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "site-a"},
			Spec: onboardingv1alpha1.SiteConfigSpec{
				ClusterName:   "prod-cluster",
				DefaultLabels: map[string]string{"key": "from-site"},
			},
		}
		sp := newServerProfile(testMAC, "r-abc12-bb01", "10.10.0.1",
			map[string]string{"key": "from-profile"},
			&corev1.LocalObjectReference{Name: "site-a"})
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(sp, siteConfig).Build()
		p := crdprovider.New(c, namespace)

		rec, err := p.LookupByMAC(context.Background(), testMAC)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Labels["key"]).To(Equal("from-profile"))
	})
})
