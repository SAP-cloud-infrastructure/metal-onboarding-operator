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

func newProvider(t *testing.T, servers ...onboardingv1alpha1.ServerEntry) *Provider {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := onboardingv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	mc := &onboardingv1alpha1.MetalCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec:       onboardingv1alpha1.MetalClusterSpec{Servers: servers},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mc).Build()
	return New(c, "test-cluster")
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
			p := newProvider(t, tt.entry)
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
