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

package metalcluster

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	onboardingv1alpha1 "github.com/SAP-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
)

func newProvider(t *testing.T, bbs ...onboardingv1alpha1.Group) *Provider {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := onboardingv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	mc := &onboardingv1alpha1.MetalCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec:       onboardingv1alpha1.MetalClusterSpec{Groups: bbs},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mc).Build()
	return New(c, "test-cluster")
}

func bb(entry onboardingv1alpha1.ServerEntry) onboardingv1alpha1.Group {
	return onboardingv1alpha1.Group{Name: "test-bb", Servers: []onboardingv1alpha1.ServerEntry{entry}}
}

func TestBMCHostnameFromServerEntry(t *testing.T) {
	const serverName = "server-0"
	tests := []struct {
		name         string
		entry        onboardingv1alpha1.ServerEntry
		wantHostname string
	}{
		{
			name: "explicit remoteboard hostname",
			entry: onboardingv1alpha1.ServerEntry{
				Name:                serverName,
				Serial:              "ABC123",
				RemoteboardHostname: "server-0-oob.example.com",
			},
			wantHostname: "server-0-oob.example.com",
		},
		{
			name: "falls back to server name when hostname empty",
			entry: onboardingv1alpha1.ServerEntry{
				Name:   serverName,
				Serial: "ABC123",
			},
			wantHostname: serverName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newProvider(t, bb(tt.entry))
			rec, err := p.LookupBySerial(context.Background(), tt.entry.Serial)
			if err != nil {
				t.Fatalf("LookupBySerial: %v", err)
			}
			if rec.BMCHostname != tt.wantHostname {
				t.Errorf("BMCHostname = %q, want %q", rec.BMCHostname, tt.wantHostname)
			}
		})
	}
}

const (
	labelCluster = "k/cluster"
	labelType    = "k/type"
	labelBB      = "k/bb"
	labelRegion  = "k/region"
	testBB       = "bb001"
)

func TestLabelMerge(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := onboardingv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	tests := []struct {
		name         string
		globalLabels map[string]string
		bbLabels     map[string]string
		serverLabels map[string]string
		wantLabels   map[string]string
	}{
		{
			name:         "global only",
			globalLabels: map[string]string{labelCluster: "c1", labelRegion: "r1"},
			wantLabels:   map[string]string{labelCluster: "c1", labelRegion: "r1"},
		},
		{
			name:         "global and bb",
			globalLabels: map[string]string{labelCluster: "c1", labelRegion: "r1"},
			bbLabels:     map[string]string{labelBB: testBB, "k/zone": "z1"},
			wantLabels:   map[string]string{labelCluster: "c1", labelRegion: "r1", labelBB: testBB, "k/zone": "z1"},
		},
		{
			name:         "server overrides bb overrides global",
			globalLabels: map[string]string{labelType: "generic", labelCluster: "c1"},
			bbLabels:     map[string]string{labelType: "type-a", labelBB: testBB},
			serverLabels: map[string]string{labelType: "type-a-v2", "k/name": "node001-bb001"},
			wantLabels:   map[string]string{labelType: "type-a-v2", labelCluster: "c1", labelBB: testBB, "k/name": "node001-bb001"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &onboardingv1alpha1.MetalCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: onboardingv1alpha1.MetalClusterSpec{
					Labels: tt.globalLabels,
					Groups: []onboardingv1alpha1.Group{
						{
							Name:   "test-bb",
							Labels: tt.bbLabels,
							Servers: []onboardingv1alpha1.ServerEntry{
								{Name: "server-0", Serial: "SER001", Labels: tt.serverLabels},
							},
						},
					},
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mc).Build()
			p := New(c, "test-cluster")

			rec, err := p.LookupBySerial(context.Background(), "SER001")
			if err != nil {
				t.Fatalf("LookupBySerial: %v", err)
			}
			for k, want := range tt.wantLabels {
				if got := rec.Labels[k]; got != want {
					t.Errorf("label %q = %q, want %q", k, got, want)
				}
			}
			if len(rec.Labels) != len(tt.wantLabels) {
				t.Errorf("got %d labels, want %d: %v", len(rec.Labels), len(tt.wantLabels), rec.Labels)
			}
		})
	}
}
