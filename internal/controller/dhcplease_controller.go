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
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	dhcpshim "github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/dhcp"
	"github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/provider"
)

const (
	annotationBootstrap   = "onboarding.metal.ironcore.dev/bootstrap"
	annotationManagerType = "onboarding.metal.ironcore.dev/manager-type"

	redfishProbeTimeout = 10 * time.Second
)

// DHCPLeaseController watches DHCPLease CRs and drives the full onboarding workflow:
// inventory lookup → Redfish probe → BMC CR creation.
// The BMC CR with bootstrap=true annotation is the handoff signal to metal-maintenance-operator.
type DHCPLeaseController struct {
	client.Client
	Scheme            *runtime.Scheme
	InventoryProvider provider.InventoryProvider
	// RedfishBaseURL overrides the Redfish endpoint base URL, used in tests.
	// When empty, the controller uses http://<lease.Spec.IP> as the base URL.
	RedfishBaseURL string
}

// +kubebuilder:rbac:groups=dhcp.metal.ironcore.dev,resources=dhcpleases,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch;create;update;patch

// Reconcile drives the full onboarding workflow for a single DHCPLease in one pass.
// The reconciler is fully idempotent: if a BMC CR already exists for the server, the
// CreateOrUpdate call is a no-op (spec is only written on creation).
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

	ip := lease.Spec.IP
	if ip == "" {
		log.Info("DHCPLease has no IP, skipping", "lease", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// --- 1. Inventory lookup ---
	rec, err := r.InventoryProvider.LookupByMAC(ctx, mac)
	if err != nil {
		if err == provider.ErrNotFound {
			log.Info("MAC not found in inventory, requeuing", "mac", mac)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("inventory lookup for MAC %s: %w", mac, err)
	}

	switch rec.ClusterGate {
	case provider.ClusterGateElsewhere:
		log.Info("server belongs to another cluster, skipping", "mac", mac, "server", rec.ServerName)
		return ctrl.Result{}, nil
	case provider.ClusterGateUnknown:
		log.Info("cluster membership unknown, requeuing", "mac", mac)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// --- 2. Redfish probe ---
	managerType, err := probeRedfish(ctx, r.redfishURL(ip))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("redfish probe for %s: %w", ip, err)
	}
	if managerType == "" {
		return ctrl.Result{}, fmt.Errorf("redfish probe for %s: no ManagerType in response", ip)
	}

	// --- 3. CreateOrUpdate BMC ---
	oobIP, err := metalv1alpha1.ParseIP(ip)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("parse lease IP %q: %w", ip, err)
	}

	bmc := &metalv1alpha1.BMC{}
	err = r.Get(ctx, types.NamespacedName{Name: rec.ServerName}, bmc)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("get BMC %s: %w", rec.ServerName, err)
	}

	if apierrors.IsNotFound(err) {
		bmc = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name:   rec.ServerName,
				Labels: rec.Labels,
				Annotations: map[string]string{
					annotationBootstrap:   "true",
					annotationManagerType: managerType,
				},
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{IP: oobIP},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolNameRedfish,
					Port: 443,
				},
			},
		}
		if rec.BMCHostname != "" {
			bmc.Spec.Hostname = &rec.BMCHostname
		}
		if err := r.Create(ctx, bmc); err != nil {
			return ctrl.Result{}, fmt.Errorf("create BMC %s: %w", rec.ServerName, err)
		}
	} else {
		// BMC already exists — only update labels and handoff annotations.
		patch := client.MergeFrom(bmc.DeepCopy())
		if bmc.Labels == nil {
			bmc.Labels = map[string]string{}
		}
		maps.Copy(bmc.Labels, rec.Labels)
		if bmc.Annotations == nil {
			bmc.Annotations = map[string]string{}
		}
		bmc.Annotations[annotationBootstrap] = "true"
		bmc.Annotations[annotationManagerType] = managerType
		if err := r.Patch(ctx, bmc, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("patch BMC %s: %w", rec.ServerName, err)
		}
	}

	log.Info("BMC ready", "name", rec.ServerName, "managerType", managerType)
	return ctrl.Result{}, nil
}

// redfishURL returns the Redfish base URL for the given IP.
func (r *DHCPLeaseController) redfishURL(ip string) string {
	if r.RedfishBaseURL != "" {
		return r.RedfishBaseURL
	}
	return fmt.Sprintf("http://%s", ip)
}

// probeRedfish performs an unauthenticated GET /redfish/v1 and extracts the ManagerType.
func probeRedfish(ctx context.Context, baseURL string) (string, error) {
	url := baseURL + "/redfish/v1"
	reqCtx, cancel := context.WithTimeout(ctx, redfishProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var body struct {
		ManagerType string `json:"ManagerType"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.ManagerType, nil
}

// SetupWithManager registers the DHCPLeaseController with the manager.
func (r *DHCPLeaseController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dhcpshim.DHCPLease{}).
		Named("dhcplease").
		Complete(r)
}
