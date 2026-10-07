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

// Package netbox implements the InventoryProvider interface using the NetBox REST API.
package netbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/provider"
)

const remoteboardInterfaceName = "remoteboard"

// Provider implements InventoryProvider against the NetBox REST API.
type Provider struct {
	baseURL     string
	tokenFile   string
	clusterName string
	httpClient  *http.Client
}

// New returns a NetBox-backed InventoryProvider.
// baseURL is the NetBox API base (e.g. "https://netbox.example.com").
// tokenFile is the path to a file containing the NetBox API token.
// clusterName is this operator's cluster name, used to gate ClusterGate decisions.
func New(baseURL, tokenFile, clusterName string) *Provider {
	return &Provider{
		baseURL:     strings.TrimRight(baseURL, "/"),
		tokenFile:   tokenFile,
		clusterName: clusterName,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// NewWithHTTPClient constructs a Provider with a custom HTTP client (for testing).
func NewWithHTTPClient(baseURL, token, clusterName string, hc *http.Client) *Provider {
	return &Provider{
		baseURL:     strings.TrimRight(baseURL, "/"),
		tokenFile:   token, // treated as literal token value when no path separator is present
		clusterName: clusterName,
		httpClient:  hc,
	}
}

// LookupByMAC queries NetBox to resolve a MAC address into an InventoryRecord.
//
// Steps:
//  1. GET /api/dcim/interfaces/?mac_address=<mac> — find the device
//  2. GET /api/dcim/devices/<id>/ — full device data (OOB IP, site, type, role, platform, cluster)
//  3. GET /api/dcim/sites/<id>/ + regions/<id>/ — resolve region slug
//  4. GET /api/dcim/interfaces/?device_id=<id>&name=remoteboard — optional BMC hostname source
//  5. GET /api/ipam/ip-addresses/?interface_id=<id> — DNS name from IPAM
func (p *Provider) LookupByMAC(ctx context.Context, mac string) (*provider.InventoryRecord, error) {
	token, err := p.readToken()
	if err != nil {
		return nil, fmt.Errorf("netbox: read token: %w", err)
	}

	ifaces, err := p.listInterfaces(ctx, token, url.Values{"mac_address": {mac}})
	if err != nil {
		return nil, fmt.Errorf("netbox: list interfaces by MAC %s: %w", mac, err)
	}
	if len(ifaces) == 0 {
		return nil, provider.ErrNotFound
	}
	if len(ifaces) > 1 {
		return nil, fmt.Errorf("netbox: MAC %s matched %d interfaces (expected 1)", mac, len(ifaces))
	}

	deviceID := ifaces[0].Device.ID

	device, err := p.getDevice(ctx, token, deviceID)
	if err != nil {
		return nil, fmt.Errorf("netbox: get device %d: %w", deviceID, err)
	}

	oobIP, err := parseIPFromCIDR(device.OOBIp.Address)
	if err != nil {
		return nil, fmt.Errorf("netbox: device %s OOB IP: %w", device.Name, err)
	}

	region, err := p.resolveRegion(ctx, token, device.Site.ID)
	if err != nil {
		return nil, fmt.Errorf("netbox: resolve region for device %s: %w", device.Name, err)
	}

	nameParts := strings.SplitN(device.Name, "-", 2)
	nodeName := device.Name
	bb := ""
	if len(nameParts) == 2 {
		nodeName = nameParts[0]
		bb = nameParts[1]
	}

	labels := map[string]string{
		"topology.kubernetes.io/region":           region,
		"topology.kubernetes.io/zone":             device.Site.Slug,
		"kubernetes.metal.cloud.sap/cluster":      device.Cluster.Name,
		"kubernetes.metal.cloud.sap/cluster-type": device.Cluster.Type.Slug,
		"kubernetes.metal.cloud.sap/name":         device.Name,
		"kubernetes.metal.cloud.sap/nodename":     nodeName,
		"kubernetes.metal.cloud.sap/bb":           bb,
		"kubernetes.metal.cloud.sap/type":         device.DeviceType.Slug,
		"kubernetes.metal.cloud.sap/role":         device.DeviceRole.Slug,
		"kubernetes.metal.cloud.sap/platform":     device.Platform.Slug,
	}

	clusterGate := provider.ClusterGateUnknown
	if device.Cluster.Name != "" {
		if device.Cluster.Name == p.clusterName {
			clusterGate = provider.ClusterGateBelongs
		} else {
			clusterGate = provider.ClusterGateElsewhere
		}
	}

	bmcHostname, err := p.resolveRemoteboardHostname(ctx, token, deviceID)
	if err != nil {
		// Non-fatal: BMC hostname is optional.
		bmcHostname = ""
	}

	return &provider.InventoryRecord{
		ClusterGate: clusterGate,
		ServerName:  device.Name,
		OOBIP:       oobIP,
		BMCHostname: bmcHostname,
		Labels:      labels,
	}, nil
}

// resolveRegion fetches the site and then its region to get the region slug.
func (p *Provider) resolveRegion(ctx context.Context, token string, siteID int) (string, error) {
	site, err := p.getSite(ctx, token, siteID)
	if err != nil {
		return "", err
	}
	if site.Region.ID == 0 {
		return "", fmt.Errorf("site %d has no region", siteID)
	}
	region, err := p.getRegion(ctx, token, site.Region.ID)
	if err != nil {
		return "", err
	}
	return region.Slug, nil
}

// resolveRemoteboardHostname looks up the remoteboard interface on a device and
// returns the DNS name of its first IPAM IP address. Returns an error if not found.
func (p *Provider) resolveRemoteboardHostname(ctx context.Context, token string, deviceID int) (string, error) {
	ifaces, err := p.listInterfaces(ctx, token, url.Values{
		"device_id": {fmt.Sprintf("%d", deviceID)},
		"name":      {remoteboardInterfaceName},
	})
	if err != nil {
		return "", err
	}
	if len(ifaces) == 0 {
		return "", fmt.Errorf("no remoteboard interface on device %d", deviceID)
	}

	ips, err := p.listIPAddresses(ctx, token, url.Values{
		"interface_id": {fmt.Sprintf("%d", ifaces[0].ID)},
	})
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no IPAM IP for remoteboard interface %d", ifaces[0].ID)
	}
	if ips[0].DNSName == "" {
		return "", fmt.Errorf("remoteboard IP has no DNS name")
	}
	return ips[0].DNSName, nil
}

// --- low-level HTTP helpers ---

func (p *Provider) readToken() (string, error) {
	// If the tokenFile field contains no path separator (used in tests), treat it as the literal token.
	if !strings.ContainsAny(p.tokenFile, "/\\") {
		return p.tokenFile, nil
	}
	raw, err := os.ReadFile(p.tokenFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func (p *Provider) authHeader(token string) string {
	if strings.HasPrefix(token, "nbt_") {
		return "Bearer " + token
	}
	return "Token " + token
}

func (p *Provider) get(ctx context.Context, token, path string, query url.Values, out any) error {
	u := p.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", p.authHeader(token))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("GET %s: read body: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("GET %s: decode: %w", path, err)
	}
	return nil
}

// --- NetBox API response types ---

type nbNestedRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type nbClusterRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type struct {
		Slug string `json:"slug"`
	} `json:"type"`
}

type nbIPField struct {
	Address string `json:"address"`
	DNSName string `json:"dns_name"`
}

type nbDevice struct {
	ID         int          `json:"id"`
	Name       string       `json:"name"`
	OOBIp      nbIPField    `json:"oob_ip"`
	Site       nbNestedRef  `json:"site"`
	Cluster    nbClusterRef `json:"cluster"`
	DeviceType struct {
		Slug string `json:"slug"`
	} `json:"device_type"`
	DeviceRole struct {
		Slug string `json:"slug"`
	} `json:"device_role"`
	Platform struct {
		Slug string `json:"slug"`
	} `json:"platform"`
}

type nbInterfaceDevice struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type nbInterface struct {
	ID     int               `json:"id"`
	Name   string            `json:"name"`
	Device nbInterfaceDevice `json:"device"`
}

type nbIPAddress struct {
	ID      int    `json:"id"`
	Address string `json:"address"`
	DNSName string `json:"dns_name"`
}

type nbSite struct {
	ID     int         `json:"id"`
	Slug   string      `json:"slug"`
	Region nbNestedRef `json:"region"`
}

type nbRegion struct {
	ID   int    `json:"id"`
	Slug string `json:"slug"`
}

type nbListResult[T any] struct {
	Count   int `json:"count"`
	Results []T `json:"results"`
}

// --- API call wrappers ---

func (p *Provider) listInterfaces(ctx context.Context, token string, q url.Values) ([]nbInterface, error) {
	var page nbListResult[nbInterface]
	if err := p.get(ctx, token, "/api/dcim/interfaces/", q, &page); err != nil {
		return nil, err
	}
	return page.Results, nil
}

func (p *Provider) getDevice(ctx context.Context, token string, id int) (*nbDevice, error) {
	var d nbDevice
	if err := p.get(ctx, token, fmt.Sprintf("/api/dcim/devices/%d/", id), nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (p *Provider) getSite(ctx context.Context, token string, id int) (*nbSite, error) {
	var s nbSite
	if err := p.get(ctx, token, fmt.Sprintf("/api/dcim/sites/%d/", id), nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (p *Provider) getRegion(ctx context.Context, token string, id int) (*nbRegion, error) {
	var r nbRegion
	if err := p.get(ctx, token, fmt.Sprintf("/api/dcim/regions/%d/", id), nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (p *Provider) listIPAddresses(ctx context.Context, token string, q url.Values) ([]nbIPAddress, error) {
	var page nbListResult[nbIPAddress]
	if err := p.get(ctx, token, "/api/ipam/ip-addresses/", q, &page); err != nil {
		return nil, err
	}
	return page.Results, nil
}

// parseIPFromCIDR strips the prefix-length from a NetBox IP (e.g. "10.0.0.1/24" → "10.0.0.1").
func parseIPFromCIDR(cidr string) (string, error) {
	if cidr == "" {
		return "", fmt.Errorf("empty OOB IP")
	}
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		// Maybe it was already a bare IP.
		if net.ParseIP(cidr) != nil {
			return cidr, nil
		}
		return "", fmt.Errorf("parse %q: %w", cidr, err)
	}
	return ip.String(), nil
}

// compile-time check that Provider satisfies the interface.
var _ provider.InventoryProvider = (*Provider)(nil)
