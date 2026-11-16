// Package metalcluster implements InventoryProvider against MetalCluster CRs.
// It replaces the NetBox-backed provider for clusters where argora populates
// MetalCluster objects instead of querying NetBox directly.
package metalcluster

import (
	"context"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	onboardingv1alpha1 "github.com/SAP-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
	"github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/provider"
)

var _ provider.InventoryProvider = (*Provider)(nil)

// Provider implements InventoryProvider using MetalCluster CRs as the data source.
// The OOB IP is not stored in MetalCluster (it comes from the DHCP lease);
// callers must set it separately via WithOOBIP before using the returned InventoryRecord.
type Provider struct {
	client      client.Client
	clusterName string // name of the MetalCluster CR to look up
}

// New returns a MetalCluster-backed InventoryProvider.
// clusterName is the metadata.name of the MetalCluster CR in the cluster.
func New(c client.Client, clusterName string) *Provider {
	return &Provider{client: c, clusterName: clusterName}
}

// LookupBySerial finds the ServerEntry whose Serial matches (case-insensitive).
// Returns ErrNotFound when no entry matches.
func (p *Provider) LookupBySerial(ctx context.Context, serial string) (*provider.InventoryRecord, error) {
	mc, err := p.getMetalCluster(ctx)
	if err != nil {
		return nil, err
	}
	serial = strings.ToUpper(serial)
	for _, s := range mc.Spec.Servers {
		if strings.ToUpper(s.Serial) == serial {
			return recordFrom(s), nil
		}
	}
	return nil, provider.ErrNotFound
}

// LookupByMAC finds the ServerEntry whose RemoteboardMAC matches (case-insensitive).
// Returns ErrNotFound when no entry matches.
func (p *Provider) LookupByMAC(ctx context.Context, mac string) (*provider.InventoryRecord, error) {
	mc, err := p.getMetalCluster(ctx)
	if err != nil {
		return nil, err
	}
	mac = normMAC(mac)
	for _, s := range mc.Spec.Servers {
		if normMAC(s.RemoteboardMAC) == mac {
			return recordFrom(s), nil
		}
	}
	return nil, provider.ErrNotFound
}

func (p *Provider) getMetalCluster(ctx context.Context) (*onboardingv1alpha1.MetalCluster, error) {
	mc := &onboardingv1alpha1.MetalCluster{}
	if err := p.client.Get(ctx, client.ObjectKey{Name: p.clusterName}, mc); err != nil {
		return nil, err
	}
	return mc, nil
}

func recordFrom(s onboardingv1alpha1.ServerEntry) *provider.InventoryRecord {
	return &provider.InventoryRecord{
		ClusterGate: provider.ClusterGateBelongs,
		ServerName:  s.Name,
		// OOBIP is not in MetalCluster — the caller provides the lease IP directly.
		OOBIP:  "",
		Labels: s.Labels,
	}
}

// normMAC normalises a MAC address to lowercase with colons removed for comparison.
func normMAC(mac string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(mac, ":", ""), "-", ""))
}
