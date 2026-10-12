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

package netbox_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/provider"
	netboxprovider "github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/provider/netbox"
)

func TestNetboxProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NetBox Provider Suite")
}

const (
	testMAC         = "aa:bb:cc:dd:ee:ff"
	testCluster     = "prod-cluster"
	testDeviceName  = "rabc12-bb01"
	testOOBIP       = "10.10.0.1/24"
	testRegionSlug  = "eu-de-1"
	testSiteSlug    = "qa-test"
	testBMCHostname = "rabc12r.example.com"
)

var _ = Describe("NetBox Provider", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns a full InventoryRecord for a known MAC", func() {
		// Use a mux that distinguishes the two /api/dcim/interfaces/ calls by query parameter.
		mux := http.NewServeMux()
		mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("mac_address") != "" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"count": 1,
					"results": []any{
						map[string]any{"id": 42, "name": "eth0", "device": map[string]any{"id": 7, "name": testDeviceName}},
					},
				})
				return
			}
			// remoteboard lookup by device_id + name
			_ = json.NewEncoder(w).Encode(map[string]any{
				"count": 1,
				"results": []any{
					map[string]any{"id": 99, "name": "remoteboard", "device": map[string]any{"id": 7, "name": testDeviceName}},
				},
			})
		})
		mux.HandleFunc("/api/dcim/devices/7/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 7, "name": testDeviceName,
				"oob_ip":      map[string]any{"address": testOOBIP, "dns_name": ""},
				"site":        map[string]any{"id": 3, "slug": testSiteSlug, "name": "Test Site"},
				"cluster":     map[string]any{"id": 5, "name": testCluster, "type": map[string]any{"slug": "openstack"}},
				"device_type": map[string]any{"slug": "r640"},
				"device_role": map[string]any{"slug": "baremetal"},
				"platform":    map[string]any{"slug": "ubuntu"},
			})
		})
		mux.HandleFunc("/api/dcim/sites/3/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 3, "slug": testSiteSlug, "region": map[string]any{"id": 1, "slug": testRegionSlug},
			})
		})
		mux.HandleFunc("/api/dcim/regions/1/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "slug": testRegionSlug})
		})
		mux.HandleFunc("/api/ipam/ip-addresses/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"count": 1,
				"results": []any{
					map[string]any{"id": 55, "address": "192.168.1.100/24", "dns_name": testBMCHostname},
				},
			})
		})

		srv := httptest.NewServer(mux)
		DeferCleanup(srv.Close)

		p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, srv.Client())
		rec, err := p.LookupByMAC(ctx, testMAC)

		Expect(err).NotTo(HaveOccurred())
		Expect(rec.ServerName).To(Equal(testDeviceName))
		Expect(rec.OOBIP).To(Equal("10.10.0.1"))
		Expect(rec.BMCHostname).To(Equal(testBMCHostname))
		Expect(rec.ClusterGate).To(Equal(provider.ClusterGateBelongs))

		// Verify all 10 topology labels.
		Expect(rec.Labels).To(HaveKeyWithValue("topology.kubernetes.io/region", testRegionSlug))
		Expect(rec.Labels).To(HaveKeyWithValue("topology.kubernetes.io/zone", testSiteSlug))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/cluster", testCluster))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/cluster-type", "openstack"))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/name", testDeviceName))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/nodename", "rabc12"))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/bb", "bb01"))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/type", "r640"))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/role", "baremetal"))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/platform", "ubuntu"))
	})

	It("returns ErrNotFound when no interface matches the MAC", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/dcim/interfaces/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"count": 0, "results": []any{}})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		DeferCleanup(srv.Close)

		p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, srv.Client())
		_, err := p.LookupByMAC(ctx, "de:ad:be:ef:00:00")
		Expect(err).To(MatchError(provider.ErrNotFound))
	})

	It("returns an error on 5xx from NetBox", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal server error"))
		}))
		DeferCleanup(srv.Close)

		p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, srv.Client())
		_, err := p.LookupByMAC(ctx, testMAC)
		Expect(err).To(HaveOccurred())
		Expect(err).NotTo(MatchError(provider.ErrNotFound))
	})

	It("returns ClusterGateElsewhere when device belongs to a different cluster", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("mac_address") != "" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"count": 1,
					"results": []any{
						map[string]any{"id": 42, "name": "eth0", "device": map[string]any{"id": 7, "name": testDeviceName}},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 0, "results": []any{}})
		})
		mux.HandleFunc("/api/dcim/devices/7/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 7, "name": testDeviceName,
				"oob_ip":      map[string]any{"address": testOOBIP, "dns_name": ""},
				"site":        map[string]any{"id": 3, "slug": testSiteSlug},
				"cluster":     map[string]any{"id": 8, "name": "other-cluster", "type": map[string]any{"slug": "openstack"}},
				"device_type": map[string]any{"slug": "r640"},
				"device_role": map[string]any{"slug": "baremetal"},
				"platform":    map[string]any{"slug": "ubuntu"},
			})
		})
		mux.HandleFunc("/api/dcim/sites/3/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 3, "slug": testSiteSlug, "region": map[string]any{"id": 1, "slug": testRegionSlug},
			})
		})
		mux.HandleFunc("/api/dcim/regions/1/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "slug": testRegionSlug})
		})

		srv := httptest.NewServer(mux)
		DeferCleanup(srv.Close)

		p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, srv.Client())
		rec, err := p.LookupByMAC(ctx, testMAC)

		Expect(err).NotTo(HaveOccurred())
		Expect(rec.ClusterGate).To(Equal(provider.ClusterGateElsewhere))
	})
})
