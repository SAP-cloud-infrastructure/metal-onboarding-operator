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

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dhcpshim "github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/dhcp"
	"github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/provider"
)

// fakeInventory is a trivial InventoryProvider used in unit tests.
type fakeInventory struct {
	record *provider.InventoryRecord
	err    error
}

func (f *fakeInventory) LookupByMAC(_ context.Context, _ string) (*provider.InventoryRecord, error) {
	return f.record, f.err
}

func newFakeScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = dhcpshim.AddToScheme(s)
	_ = metalv1alpha1.AddToScheme(s)
	return s
}

func fakeRedfishServer(managerType string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/redfish/v1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := json.Marshal(map[string]string{"ManagerType": managerType})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

var _ = Describe("DHCPLeaseController unit tests (fake client)", func() {
	const (
		ns     = "default"
		testIP = "10.0.0.1"
	)

	newLease := func(name, mac string) *dhcpshim.DHCPLease {
		return &dhcpshim.DHCPLease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       dhcpshim.DHCPLeaseSpec{MACAddress: mac, IP: testIP},
		}
	}

	It("skips and creates no BMC when ClusterGateElsewhere", func() {
		inv := &fakeInventory{record: &provider.InventoryRecord{
			ClusterGate: provider.ClusterGateElsewhere,
			ServerName:  "some-server",
		}}

		lease := newLease("lease-2", "aa:bb:cc:dd:ee:ff")
		s := newFakeScheme()
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lease).Build()
		r := &DHCPLeaseController{Client: cl, Scheme: s, InventoryProvider: inv}

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "lease-2", Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())

		bmcList := &metalv1alpha1.BMCList{}
		Expect(cl.List(context.Background(), bmcList)).To(Succeed())
		Expect(bmcList.Items).To(BeEmpty())
	})

	It("returns no error and requeues when provider returns ErrNotFound", func() {
		inv := &fakeInventory{err: provider.ErrNotFound}

		lease := newLease("lease-3", "aa:bb:cc:dd:ee:ff")
		s := newFakeScheme()
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lease).Build()
		r := &DHCPLeaseController{Client: cl, Scheme: s, InventoryProvider: inv}

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "lease-3", Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
	})

	It("returns an error when the Redfish probe fails", func() {
		inv := &fakeInventory{record: &provider.InventoryRecord{
			ClusterGate: provider.ClusterGateBelongs,
			ServerName:  "some-server",
			OOBIP:       testIP,
			Labels:      map[string]string{},
		}}

		lease := newLease("lease-4", "aa:bb:cc:dd:ee:ff")
		s := newFakeScheme()
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lease).Build()
		r := &DHCPLeaseController{
			Client:            cl,
			Scheme:            s,
			InventoryProvider: inv,
			RedfishBaseURL:    "http://127.0.0.1:1", // nothing listening
		}

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "lease-4", Namespace: ns},
		})
		Expect(err).To(HaveOccurred())
	})

	It("skips a DHCPLease with no MACAddress", func() {
		inv := &fakeInventory{}
		lease := newLease("lease-empty", "")
		s := newFakeScheme()
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lease).Build()
		r := &DHCPLeaseController{Client: cl, Scheme: s, InventoryProvider: inv}

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "lease-empty", Namespace: ns},
		})
		Expect(err).NotTo(HaveOccurred())

		bmcList := &metalv1alpha1.BMCList{}
		Expect(cl.List(context.Background(), bmcList)).To(Succeed())
		Expect(bmcList.Items).To(BeEmpty())
	})
})

var _ = Describe("DHCPLeaseController integration tests (envtest)", func() {
	const (
		ns         = "default"
		testMAC    = "aa:bb:cc:dd:ee:ff"
		testIP     = "10.0.0.1"
		serverName = "rabc12-bb01"
	)

	var redfishServer *httptest.Server
	BeforeEach(func() {
		redfishServer = fakeRedfishServer("iDRAC")
	})
	AfterEach(func() {
		redfishServer.Close()
	})

	newLease := func(name string) *dhcpshim.DHCPLease {
		return &dhcpshim.DHCPLease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", name, time.Now().UnixNano()),
				Namespace: ns,
			},
			Spec: dhcpshim.DHCPLeaseSpec{MACAddress: testMAC, IP: testIP},
		}
	}

	It("creates a BMC with correct annotations, labels, and spec", func() {
		inv := &fakeInventory{record: &provider.InventoryRecord{
			ClusterGate: provider.ClusterGateBelongs,
			ServerName:  serverName,
			OOBIP:       testIP,
			Labels: map[string]string{
				"topology.kubernetes.io/region": "eu-de-1",
			},
		}}

		lease := newLease("bmc-create")
		Expect(k8sClient.Create(ctx, lease)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, lease) })

		r := &DHCPLeaseController{
			Client:            k8sClient,
			Scheme:            k8sClient.Scheme(),
			InventoryProvider: inv,
			RedfishBaseURL:    redfishServer.URL,
		}

		_, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: client.ObjectKeyFromObject(lease),
		})
		Expect(err).NotTo(HaveOccurred())

		bmc := &metalv1alpha1.BMC{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, bmc)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bmc) })

		Expect(bmc.Annotations).To(HaveKeyWithValue(annotationBootstrap, "true"))
		Expect(bmc.Annotations).To(HaveKeyWithValue(annotationManagerType, "iDRAC"))
		Expect(bmc.Labels).To(HaveKeyWithValue("topology.kubernetes.io/region", "eu-de-1"))
		Expect(bmc.Spec.Protocol.Name).To(Equal(metalv1alpha1.ProtocolNameRedfish))
		Expect(bmc.Spec.Protocol.Port).To(Equal(int32(443)))
	})

	It("is idempotent: reconciling twice produces one BMC unchanged", func() {
		uniqueServer := fmt.Sprintf("server-%d", time.Now().UnixNano())
		inv := &fakeInventory{record: &provider.InventoryRecord{
			ClusterGate: provider.ClusterGateBelongs,
			ServerName:  uniqueServer,
			OOBIP:       testIP,
			Labels:      map[string]string{},
		}}

		lease := newLease("bmc-idem")
		Expect(k8sClient.Create(ctx, lease)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, lease) })

		r := &DHCPLeaseController{
			Client:            k8sClient,
			Scheme:            k8sClient.Scheme(),
			InventoryProvider: inv,
			RedfishBaseURL:    redfishServer.URL,
		}

		req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(lease)}
		for range 2 {
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
		}

		bmcList := &metalv1alpha1.BMCList{}
		Expect(k8sClient.List(ctx, bmcList)).To(Succeed())
		found := 0
		for _, b := range bmcList.Items {
			if b.Name == uniqueServer {
				found++
				_ = k8sClient.Delete(ctx, &b)
			}
		}
		Expect(found).To(Equal(1))
	})
})
