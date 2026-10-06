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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	onboardingv1alpha1 "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
	"github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/provider"
)

// mockProvider is an in-memory InventoryProvider for integration tests.
type mockProvider struct {
	record *provider.InventoryRecord
	err    error
}

func (m *mockProvider) LookupByMAC(_ context.Context, _ string) (*provider.InventoryRecord, error) {
	return m.record, m.err
}

var _ = Describe("OnboardingRequest integration tests", func() {
	const (
		ns         = "default"
		testMAC    = "aa:bb:cc:dd:ee:ff"
		serverName = "r-test12-bb01"
		oobIP      = "10.10.0.1"
	)

	// Start a fake Redfish server that responds with a ManagerType.
	var redfishServer *httptest.Server
	BeforeEach(func() {
		redfishServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/redfish/v1" {
				body, _ := json.Marshal(map[string]interface{}{
					"ManagerType": "iDRAC",
				})
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
	})
	AfterEach(func() {
		redfishServer.Close()
	})

	It("progresses OnboardingRequest to Done with BMC annotations set", func() {
		inventory := &mockProvider{
			record: &provider.InventoryRecord{
				ClusterGate: provider.ClusterGateBelongs,
				ServerName:  serverName,
				OOBIP:       oobIP,
				Labels: map[string]string{
					"topology.kubernetes.io/region": "eu-de-1",
				},
			},
		}

		or := &onboardingv1alpha1.OnboardingRequest{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("test-or-%d", time.Now().UnixNano()), Namespace: ns},
			Spec:       onboardingv1alpha1.OnboardingRequestSpec{MACAddress: testMAC},
		}
		Expect(k8sClient.Create(ctx, or)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, or) })

		reconciler := &OnboardingRequestReconciler{
			Client:            k8sClient,
			Scheme:            k8sClient.Scheme(),
			InventoryProvider: inventory,
			RedfishBaseURL:    redfishServer.URL,
		}

		// Drive the state machine through all phases by reconciling until Done.
		Eventually(func() onboardingv1alpha1.OnboardingPhase {
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: or.Name, Namespace: ns},
			})
			current := &onboardingv1alpha1.OnboardingRequest{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: or.Name, Namespace: ns}, current); err != nil {
				return ""
			}
			return current.Status.Phase
		}, 30*time.Second, 500*time.Millisecond).Should(Equal(onboardingv1alpha1.PhaseDone))

		// Verify status fields.
		current := &onboardingv1alpha1.OnboardingRequest{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: or.Name, Namespace: ns}, current)).To(Succeed())
		Expect(current.Status.ManagerType).To(Equal("iDRAC"))
		Expect(current.Status.ServerName).To(Equal(serverName))

		// Verify the BMC CR has both handoff annotations.
		bmc := &metalv1alpha1.BMC{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, bmc)).To(Succeed())
		Expect(bmc.Annotations).To(HaveKeyWithValue("onboarding.metal.ironcore.dev/bootstrap", "true"))
		Expect(bmc.Annotations).To(HaveKeyWithValue("onboarding.metal.ironcore.dev/manager-type", "iDRAC"))
		Expect(bmc.Labels).To(HaveKeyWithValue("topology.kubernetes.io/region", "eu-de-1"))

		// Cleanup BMC.
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bmc) })
	})

	It("transitions to Skipped for servers belonging to another cluster", func() {
		inventory := &mockProvider{
			record: &provider.InventoryRecord{
				ClusterGate: provider.ClusterGateElsewhere,
				ServerName:  serverName,
				OOBIP:       oobIP,
			},
		}

		or := &onboardingv1alpha1.OnboardingRequest{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("test-or-skip-%d", time.Now().UnixNano()), Namespace: ns},
			Spec:       onboardingv1alpha1.OnboardingRequestSpec{MACAddress: testMAC},
		}
		Expect(k8sClient.Create(ctx, or)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, or) })

		reconciler := &OnboardingRequestReconciler{
			Client:            k8sClient,
			Scheme:            k8sClient.Scheme(),
			InventoryProvider: inventory,
		}

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: or.Name, Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())

		current := &onboardingv1alpha1.OnboardingRequest{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: or.Name, Namespace: ns}, current)).To(Succeed())
		Expect(current.Status.Phase).To(Equal(onboardingv1alpha1.PhaseSkipped))
	})

	It("stays in InventoryLookup when provider returns ErrNotFound", func() {
		inventory := &mockProvider{err: provider.ErrNotFound}

		or := &onboardingv1alpha1.OnboardingRequest{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("test-or-notfound-%d", time.Now().UnixNano()), Namespace: ns},
			Spec:       onboardingv1alpha1.OnboardingRequestSpec{MACAddress: testMAC},
		}
		Expect(k8sClient.Create(ctx, or)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, or) })

		reconciler := &OnboardingRequestReconciler{
			Client:            k8sClient,
			Scheme:            k8sClient.Scheme(),
			InventoryProvider: inventory,
		}

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: or.Name, Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())

		current := &onboardingv1alpha1.OnboardingRequest{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: or.Name, Namespace: ns}, current)).To(Succeed())
		// Phase should not have advanced — still in initial state (empty or InventoryLookup).
		Expect(current.Status.Phase).To(BeElementOf(onboardingv1alpha1.PhaseInventoryLookup, onboardingv1alpha1.OnboardingPhase("")))
	})
})
