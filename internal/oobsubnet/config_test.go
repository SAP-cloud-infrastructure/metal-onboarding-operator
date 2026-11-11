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

package oobsubnet_test

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/oobsubnet"
)

func TestNameFromCIDR(t *testing.T) {
	cases := []struct{ in, want string }{
		{"10.0.1.0/24", "10.0.1.0-24"},
		{"192.168.0.0/16", "192.168.0.0-16"},
		{"10.0.0.0/8", "10.0.0.0-8"},
	}
	for _, c := range cases {
		if got := oobsubnet.NameFromCIDR(c.in); got != c.want {
			t.Errorf("NameFromCIDR(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSubnetConfigUnmarshal(t *testing.T) {
	raw := `
"10.0.1.0/24":
  gateway: "10.0.1.1"
  bootURL: "http://boot.example.com/ipxe"
  rangeStart: "10.0.1.10"
  rangeEnd: "10.0.1.200"
  leaseTime: "4h"
"192.168.0.0/16":
  gateway: "192.168.0.1"
`
	var m oobsubnet.ConfigMap
	if err := yaml.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	c1, ok := m["10.0.1.0/24"]
	if !ok {
		t.Fatal("missing entry for 10.0.1.0/24")
	}
	if c1.Gateway != "10.0.1.1" {
		t.Errorf("Gateway = %q, want 10.0.1.1", c1.Gateway)
	}
	if c1.BootURL != "http://boot.example.com/ipxe" {
		t.Errorf("BootURL = %q, want http://boot.example.com/ipxe", c1.BootURL)
	}
	if c1.RangeStart != "10.0.1.10" {
		t.Errorf("RangeStart = %q, want 10.0.1.10", c1.RangeStart)
	}
	if c1.RangeEnd != "10.0.1.200" {
		t.Errorf("RangeEnd = %q, want 10.0.1.200", c1.RangeEnd)
	}
	if c1.LeaseTime == nil {
		t.Fatal("LeaseTime is nil")
	}
	if c1.LeaseTime.Duration != 4*time.Hour {
		t.Errorf("LeaseTime = %v, want 4h", c1.LeaseTime.Duration)
	}

	c2, ok := m["192.168.0.0/16"]
	if !ok {
		t.Fatal("missing entry for 192.168.0.0/16")
	}
	if c2.Gateway != "192.168.0.1" {
		t.Errorf("Gateway = %q, want 192.168.0.1", c2.Gateway)
	}
	if c2.LeaseTime != nil {
		t.Errorf("LeaseTime = %v, want nil", c2.LeaseTime)
	}
}

// Verify that metav1.Duration zero value (nil) survives a round-trip.
func TestSubnetConfigNilLeaseTime(t *testing.T) {
	raw := `"10.0.0.0/8": {gateway: "10.0.0.1"}`
	var m oobsubnet.ConfigMap
	if err := yaml.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	c, ok := m["10.0.0.0/8"]
	if !ok {
		t.Fatal("missing entry")
	}
	if c.LeaseTime != nil {
		t.Errorf("expected nil LeaseTime, got %v", c.LeaseTime)
	}
	_ = metav1.Duration{} // ensure import is used
}
