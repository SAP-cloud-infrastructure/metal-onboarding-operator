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

	// JSON field name constants used across multiple test fixtures.
	keyCount      = "count"
	keyResults    = "results"
	keyDevice     = "device"
	keyAddress    = "address"
	keySite       = "site"
	keyCluster    = "cluster"
	keyDeviceType = "device_type"
	keyBaremetal  = "baremetal"
	keyPlatform   = "platform"
	keyRegion     = "region"
	keyDNSName    = "dns_name"
	keySlug       = "slug"
	keyDeviceRole = "device_role"
	keyEth0       = "eth0"
	keyOpenstack  = "openstack"
	keyR640       = "r640"
	keyUbuntu     = "ubuntu"
	keyPrefix     = "prefix"
	keyName       = "name"
)

// deviceFixture builds the standard device JSON map used across multiple tests.
func deviceFixture(id int, clusterName string) map[string]any {
	return map[string]any{
		"id": id, keyName: testDeviceName,
		"oob_ip":      map[string]any{keyAddress: testOOBIP, keyDNSName: ""},
		keySite:       map[string]any{"id": 3, keySlug: testSiteSlug},
		keyCluster:    map[string]any{"id": 5, keyName: clusterName, "type": map[string]any{keySlug: keyOpenstack}},
		keyDeviceType: map[string]any{keySlug: keyR640},
		keyDeviceRole: map[string]any{keySlug: keyBaremetal},
		keyPlatform:   map[string]any{keySlug: keyUbuntu},
	}
}

// siteFixture returns the standard site JSON map.
func siteFixture() map[string]any {
	return map[string]any{
		"id": 3, keySlug: testSiteSlug,
		keyRegion: map[string]any{"id": 1, keySlug: testRegionSlug},
	}
}

// interfaceListFixture returns a list result with a single interface for the given device (device ID 7).
func interfaceListFixture(ifaceID int, ifaceName string) map[string]any {
	const deviceID = 7
	return map[string]any{
		keyCount: 1,
		keyResults: []any{
			map[string]any{"id": ifaceID, keyName: ifaceName, keyDevice: map[string]any{"id": deviceID, keyName: testDeviceName}},
		},
	}
}

// emptyList returns a JSON page with no results.
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
				_ = json.NewEncoder(w).Encode(interfaceListFixture(42, keyEth0))
				return
			}
			// remoteboard lookup by device_id + name
			_ = json.NewEncoder(w).Encode(interfaceListFixture(99, "remoteboard"))
		})
		mux.HandleFunc("/api/dcim/devices/7/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			d := deviceFixture(7, testCluster)
			// The first test also has a "name" field in the site sub-object.
			d[keySite] = map[string]any{"id": 3, keySlug: testSiteSlug, keyName: "Test Site"}
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
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/cluster-type", keyOpenstack))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/name", testDeviceName))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/nodename", "rabc12"))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/bb", "bb01"))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/type", keyR640))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/role", keyBaremetal))
		Expect(rec.Labels).To(HaveKeyWithValue("kubernetes.metal.cloud.sap/platform", keyUbuntu))
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
				_ = json.NewEncoder(w).Encode(interfaceListFixture(42, keyEth0))
				return
			}
			_ = json.NewEncoder(w).Encode(emptyList())
		})
		mux.HandleFunc("/api/dcim/devices/7/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceFixture(7, "other-cluster"))
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
				_ = json.NewEncoder(w).Encode(interfaceListFixture(42, keyEth0))
				return
			}
			_ = json.NewEncoder(w).Encode(emptyList())
		})
		mux.HandleFunc("/api/dcim/devices/7/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			d := deviceFixture(7, testCluster)
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

	Describe("LookupPrefixForIP", func() {
		It("returns the most-specific prefix when multiple prefixes match", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					keyCount: 2,
					keyResults: []any{
						map[string]any{"id": 1, keyPrefix: "10.0.0.0/16"},
						map[string]any{"id": 2, keyPrefix: "10.0.1.0/24"},
					},
				})
			}))
			DeferCleanup(srv.Close)

			p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, nil, srv.Client())
			cidr, err := p.LookupPrefixForIP(ctx, "10.0.1.5")

			Expect(err).NotTo(HaveOccurred())
			Expect(cidr).To(Equal("10.0.1.0/24"))
		})

		It("returns ErrNotFound when no prefix matches", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(emptyList())
			}))
			DeferCleanup(srv.Close)

			p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, nil, srv.Client())
			_, err := p.LookupPrefixForIP(ctx, "10.0.1.5")

			Expect(err).To(MatchError(provider.ErrNotFound))
		})

		It("returns a single prefix when only one matches", func() {
			const singleCIDR = "172.16.0.0/12"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					keyCount: 1,
					keyResults: []any{
						map[string]any{"id": 3, keyPrefix: singleCIDR},
					},
				})
			}))
			DeferCleanup(srv.Close)

			p := netboxprovider.NewWithHTTPClient(srv.URL, "testtoken", testCluster, nil, srv.Client())
			cidr, err := p.LookupPrefixForIP(ctx, "172.20.5.1")

			Expect(err).NotTo(HaveOccurred())
			Expect(cidr).To(Equal(singleCIDR))
		})
	})
})
