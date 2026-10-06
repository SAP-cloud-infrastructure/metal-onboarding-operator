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

// Package crd implements the InventoryProvider interface using Kubernetes CRs
// (ServerProfile and SiteConfig) as the inventory backend.
package crd

import (
	"context"
	"maps"

	"sigs.k8s.io/controller-runtime/pkg/client"

	onboardingv1alpha1 "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
	"github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/provider"
)

// Provider implements InventoryProvider by reading ServerProfile and SiteConfig CRs.
type Provider struct {
	client    client.Client
	namespace string
}

// New returns a CRD-backed InventoryProvider that searches the given namespace for ServerProfiles.
func New(c client.Client, namespace string) *Provider {
	return &Provider{client: c, namespace: namespace}
}

// LookupByMAC finds a ServerProfile whose spec.macAddress matches mac, then merges
// SiteConfig defaults and returns an InventoryRecord.
func (p *Provider) LookupByMAC(ctx context.Context, mac string) (*provider.InventoryRecord, error) {
	list := &onboardingv1alpha1.ServerProfileList{}
	if err := p.client.List(ctx, list, client.InNamespace(p.namespace)); err != nil {
		return nil, err
	}

	var matched *onboardingv1alpha1.ServerProfile
	for i := range list.Items {
		if list.Items[i].Spec.MACAddress == mac {
			matched = &list.Items[i]
			break
		}
	}
	if matched == nil {
		return nil, provider.ErrNotFound
	}

	record := &provider.InventoryRecord{
		ServerName:  matched.Spec.ServerName,
		OOBIP:       matched.Spec.OOBIP,
		BMCHostname: matched.Spec.BMCHostname,
		Labels:      make(map[string]string),
	}

	// Merge SiteConfig defaults first (lower precedence).
	if matched.Spec.SiteConfigRef != nil {
		siteConfig := &onboardingv1alpha1.SiteConfig{}
		if err := p.client.Get(ctx, client.ObjectKey{Name: matched.Spec.SiteConfigRef.Name}, siteConfig); err == nil {
			maps.Copy(record.Labels, siteConfig.Spec.DefaultLabels)
			// Use SiteConfig clusterName when ServerProfile doesn't override it.
			if matched.Spec.ClusterName == "" {
				record.ClusterGate = clusterGate(siteConfig.Spec.ClusterName)
			}
		}
	}

	// ServerProfile labels override SiteConfig defaults.
	maps.Copy(record.Labels, matched.Spec.Labels)

	if matched.Spec.ClusterName != "" {
		record.ClusterGate = clusterGate(matched.Spec.ClusterName)
	}

	if record.ClusterGate == "" {
		record.ClusterGate = provider.ClusterGateUnknown
	}

	return record, nil
}

// clusterGate returns ClusterGateBelongs when clusterName is non-empty (the ServerProfile
// explicitly names a cluster, which means it belongs). The CRD provider does not have
// visibility into what the local cluster name is — callers should compare against the
// operator's own cluster identity if they need to distinguish belongs/elsewhere.
// For now, any named cluster in a ServerProfile is treated as "belongs".
func clusterGate(clusterName string) provider.ClusterGate {
	if clusterName == "" {
		return provider.ClusterGateUnknown
	}
	return provider.ClusterGateBelongs
}
