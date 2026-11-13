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

	keyCount      = "count"
	keyResults    = "results"
	keySlug       = "slug"
	keyName       = "name"
	keyAddress    = "address"
	keySite       = "site"
	keyCluster    = "cluster"
	keyDeviceType = "device_type"
	keyDeviceRole = "device_role"
	keyPlatform   = "platform"
	keyRegion     = "region"
	keyDevice     = "device"
	keyDNSName    = "dns_name"
	testIfaceName = "eth0"

	testDeviceTypeSlug = "r640"
	testRoleSlug       = "baremetal"
	testPlatformSlug   = "ubuntu"
	testClusterType    = "openstack"
)

func deviceFixture(clusterName string) map[string]any { //nolint:unparam
	return map[string]any{
		"id": 7, keyName: testDeviceName,
		"oob_ip":      map[string]any{keyAddress: testOOBIP, keyDNSName: ""},
		keySite:       map[string]any{"id": 3, keySlug: testSiteSlug, keyName: "Test Site"},
		keyCluster:    map[string]any{"id": 5, keyName: clusterName, "type": map[string]any{keySlug: testClusterType}},
		keyDeviceType: map[string]any{keySlug: testDeviceTypeSlug},
		keyDeviceRole: map[string]any{keySlug: testRoleSlug},
		keyPlatform:   map[string]any{keySlug: testPlatformSlug},
	}
}

func siteFixture() map[string]any {
	return map[string]any{
		"id": 3, keySlug: testSiteSlug, keyRegion: map[string]any{"id": 1, keySlug: testRegionSlug},
	}
}

func emptyList() map[string]any {
	return map[string]any{keyCount: 0, keyResults: []any{}}
}

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
					keyCount: 1,
					keyResults: []any{
						map[string]any{"id": 42, keyName: testIfaceName, keyDevice: map[string]any{"id": 7, keyName: testDeviceName}},
					},
				})
				return
			}
			// remoteboard lookup by device_id + name
			_ = json.NewEncoder(w).Encode(map[string]any{
				keyCount: 1,
				keyResults: []any{
					map[string]any{"id": 99, keyName: "remoteboard", keyDevice: map[string]any{"id": 7, keyName: testDeviceName}},
				},
			})
		})
		mux.HandleFunc("/api/dcim/devices/7/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceFixture(testCluster))
		})
		mux.HandleFunc("/api/dcim/sites/3/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(siteFixture())
		})
		mux.HandleFunc("/api/dcim/regions/1/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, keySlug: testRegionSlug})
		})
		mux.HandleFunc("/api/ipam/ip-addresses/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				keyCount: 1,
				keyResults: []any{
					map[string]any{"id": 55, keyAddress: "192.168.1.100/24", keyDNSName: testBMCHostname},
				},
			})
		})

		srv := httptest.NewServer(mux)
		DeferCleanup(srv.Close)

		p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, nil, srv.Client())
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
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/cluster-type", testClusterType))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/name", testDeviceName))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/nodename", "rabc12"))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/bb", "bb01"))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/type", testDeviceTypeSlug))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/role", testRoleSlug))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/platform", testPlatformSlug))
	})

	It("returns ErrNotFound when no interface matches the MAC", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/dcim/interfaces/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(emptyList())
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		DeferCleanup(srv.Close)

		p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, nil, srv.Client())
		_, err := p.LookupByMAC(ctx, "de:ad:be:ef:00:00")
		Expect(err).To(MatchError(provider.ErrNotFound))
	})

	It("returns an error on 5xx from NetBox", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal server error"))
		}))
		DeferCleanup(srv.Close)

		p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, nil, srv.Client())
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
					keyCount: 1,
					keyResults: []any{
						map[string]any{"id": 42, keyName: testIfaceName, keyDevice: map[string]any{"id": 7, keyName: testDeviceName}},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(emptyList())
		})
		mux.HandleFunc("/api/dcim/devices/7/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 7, keyName: testDeviceName,
				"oob_ip":      map[string]any{keyAddress: testOOBIP, keyDNSName: ""},
				keySite:       map[string]any{"id": 3, keySlug: testSiteSlug},
				keyCluster:    map[string]any{"id": 8, keyName: "other-cluster", "type": map[string]any{keySlug: testClusterType}},
				keyDeviceType: map[string]any{keySlug: testDeviceTypeSlug},
				keyDeviceRole: map[string]any{keySlug: testRoleSlug},
				keyPlatform:   map[string]any{keySlug: testPlatformSlug},
			})
		})
		mux.HandleFunc("/api/dcim/sites/3/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(siteFixture())
		})
		mux.HandleFunc("/api/dcim/regions/1/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, keySlug: testRegionSlug})
		})

		srv := httptest.NewServer(mux)
		DeferCleanup(srv.Close)

		p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, nil, srv.Client())
		rec, err := p.LookupByMAC(ctx, testMAC)

		Expect(err).NotTo(HaveOccurred())
		Expect(rec.ClusterGate).To(Equal(provider.ClusterGateElsewhere))
	})

	It("applies labelMap entries from raw NetBox JSON paths", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("mac_address") != "" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					keyCount: 1,
					keyResults: []any{
						map[string]any{"id": 42, keyName: testIfaceName, keyDevice: map[string]any{"id": 7, keyName: testDeviceName}},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(emptyList())
		})
		mux.HandleFunc("/api/dcim/devices/7/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			d := deviceFixture(testCluster)
			d["custom_fields"] = map[string]any{"rack": "ra-bc-12"}
			_ = json.NewEncoder(w).Encode(d)
		})
		mux.HandleFunc("/api/dcim/sites/3/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(siteFixture())
		})
		mux.HandleFunc("/api/dcim/regions/1/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, keySlug: testRegionSlug})
		})

		srv := httptest.NewServer(mux)
		DeferCleanup(srv.Close)

		labelMap := map[string]string{
			"kubernetes.metal.cloud.sap/rack": "custom_fields.rack",
		}
		p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, labelMap, srv.Client())
		rec, err := p.LookupByMAC(ctx, testMAC)

		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/rack", "ra-bc-12"))
	})

	Describe("LookupBySerial", func() {
		const testSerial = "1BNV5H4"

		It("returns an InventoryRecord for a known serial", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("serial") != "" {
					_ = json.NewEncoder(w).Encode(map[string]any{
						keyCount:   1,
						keyResults: []any{deviceFixture(testCluster)},
					})
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
			})
			mux.HandleFunc("/api/dcim/devices/7/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(deviceFixture(testCluster))
			})
			mux.HandleFunc("/api/dcim/sites/3/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(siteFixture())
			})
			mux.HandleFunc("/api/dcim/regions/1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, keySlug: testRegionSlug})
			})
			mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(emptyList())
			})

			srv := httptest.NewServer(mux)
			DeferCleanup(srv.Close)

			p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, nil, srv.Client())
			rec, err := p.LookupBySerial(ctx, testSerial)

			Expect(err).NotTo(HaveOccurred())
			Expect(rec.ServerName).To(Equal(testDeviceName))
			Expect(rec.ClusterGate).To(Equal(provider.ClusterGateBelongs))
		})

		It("returns ErrNotFound when no device matches the serial", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(emptyList())
			}))
			DeferCleanup(srv.Close)

			p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, nil, srv.Client())
			_, err := p.LookupBySerial(ctx, "NOTFOUND")
			Expect(err).To(MatchError(provider.ErrNotFound))
		})

		It("returns a transient error on 5xx from NetBox", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("internal server error"))
			}))
			DeferCleanup(srv.Close)

			p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, nil, srv.Client())
			_, err := p.LookupBySerial(ctx, testSerial)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(MatchError(provider.ErrNotFound))
		})
	})
})
