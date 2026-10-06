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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"

	onboardingv1alpha1 "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
	dhcpshim "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/dhcp"
)

// DHCPLeaseController watches DHCPLease CRs and creates one OnboardingRequest per unique MAC address.
// It does not perform any onboarding logic itself — that is handled by OnboardingRequestReconciler.
type DHCPLeaseController struct {
	client.Client
	Scheme    *runtime.Scheme
	Namespace string // namespace where OnboardingRequests are created
}

// +kubebuilder:rbac:groups=dhcp.metal.ironcore.dev,resources=dhcpleases,verbs=get;list;watch
// +kubebuilder:rbac:groups=onboarding.metal.ironcore.dev,resources=onboardingrequests,verbs=get;list;watch;create;update;patch

// Reconcile watches a DHCPLease and ensures exactly one OnboardingRequest exists for its MAC address.
// Deleting the DHCPLease does NOT delete the OnboardingRequest — onboarding is a one-way gate.
func (r *DHCPLeaseController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	lease := &dhcpshim.DHCPLease{}
	if err := r.Get(ctx, req.NamespacedName, lease); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	mac := lease.Spec.MACAddress
	if mac == "" {
		log.Info("DHCPLease has no MACAddress, skipping", "lease", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// Derive a deterministic name for the OnboardingRequest from the MAC address.
	// Colons are replaced with hyphens to satisfy Kubernetes name constraints.
	orName := macToName(mac)
	namespace := r.Namespace
	if namespace == "" {
		namespace = req.Namespace
	}

	or := &onboardingv1alpha1.OnboardingRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      orName,
			Namespace: namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, or, func() error {
		// Only populate spec on first creation; never overwrite an in-progress OnboardingRequest.
		// ResourceVersion is empty when the object does not yet exist in the API server.
		if or.ResourceVersion == "" {
			or.Spec = onboardingv1alpha1.OnboardingRequestSpec{
				MACAddress: mac,
				AssignedIP: lease.Spec.IPAddress,
				DHCPLeaseRef: &corev1.ObjectReference{
					APIVersion: dhcpshim.GroupVersion.String(),
					Kind:       "DHCPLease",
					Namespace:  req.Namespace,
					Name:       req.Name,
				},
			}
		}
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if result == controllerutil.OperationResultCreated {
		log.Info("created OnboardingRequest", "mac", mac, "name", orName)
	}

	return ctrl.Result{}, nil
}

// macToName converts a MAC address string to a valid Kubernetes resource name.
// "aa:bb:cc:dd:ee:ff" → "mac-aa-bb-cc-dd-ee-ff"
func macToName(mac string) string {
	name := "mac-"
	for i, c := range mac {
		if c == ':' {
			name += "-"
		} else {
			name += string(mac[i])
		}
	}
	return name
}

// SetupWithManager registers the DHCPLeaseController with the manager.
func (r *DHCPLeaseController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dhcpshim.DHCPLease{}).
		Named("dhcplease").
		Complete(r)
}
