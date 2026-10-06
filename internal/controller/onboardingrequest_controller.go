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
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	onboardingv1alpha1 "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
	"github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/provider"
)

const (
	annotationBootstrap   = "onboarding.metal.ironcore.dev/bootstrap"
	annotationManagerType = "onboarding.metal.ironcore.dev/manager-type"

	redfishProbeTimeout = 10 * time.Second
	requeueBackoff      = 30 * time.Second
)

// OnboardingRequestReconciler reconciles a OnboardingRequest object
type OnboardingRequestReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	InventoryProvider provider.InventoryProvider
	// MaxInventoryRetries is the number of times to retry inventory lookup before failing.
	// Zero means infinite retries.
	MaxInventoryRetries int
}

// +kubebuilder:rbac:groups=onboarding.metal.ironcore.dev,resources=onboardingrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=onboarding.metal.ironcore.dev,resources=onboardingrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=onboarding.metal.ironcore.dev,resources=onboardingrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=onboarding.metal.ironcore.dev,resources=serverprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=onboarding.metal.ironcore.dev,resources=siteconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=readiness.metal.ironcore.dev,resources=serverwirings,verbs=get;list;watch;create;update;patch

// Reconcile reads the current phase of the OnboardingRequest and advances it by one step.
func (r *OnboardingRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	or := &onboardingv1alpha1.OnboardingRequest{}
	if err := r.Get(ctx, req.NamespacedName, or); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Terminal phases — no further reconciliation needed.
	switch or.Status.Phase {
	case onboardingv1alpha1.PhaseDone,
		onboardingv1alpha1.PhaseFailed,
		onboardingv1alpha1.PhaseSkipped:
		return ctrl.Result{}, nil
	}

	// Advance phase by one step.
	switch or.Status.Phase {
	case "", onboardingv1alpha1.PhaseInventoryLookup:
		return r.inventoryLookupPhase(ctx, or)
	case onboardingv1alpha1.PhaseRedfishProbe:
		return r.redfishProbePhase(ctx, or)
	case onboardingv1alpha1.PhaseBMCCreation:
		return r.bmcCreationPhase(ctx, or)
	default:
		log.Info("unknown phase, resetting to InventoryLookup", "phase", or.Status.Phase)
		return r.setPhase(ctx, or, onboardingv1alpha1.PhaseInventoryLookup, "")
	}
}

// inventoryLookupPhase resolves the MAC address to an InventoryRecord.
func (r *OnboardingRequestReconciler) inventoryLookupPhase(ctx context.Context, or *onboardingv1alpha1.OnboardingRequest) (ctrl.Result, error) {
	if r.InventoryProvider == nil {
		return ctrl.Result{RequeueAfter: requeueBackoff}, nil
	}

	rec, err := r.InventoryProvider.LookupByMAC(ctx, or.Spec.MACAddress)
	if err != nil {
		if err == provider.ErrNotFound {
			// Respect MaxInventoryRetries if set.
			if r.MaxInventoryRetries > 0 {
				retries := inventoryRetryCount(or)
				if retries >= r.MaxInventoryRetries {
					return r.setPhase(ctx, or, onboardingv1alpha1.PhaseFailed, "InventoryNotFound")
				}
			}
			return ctrl.Result{RequeueAfter: requeueBackoff}, nil
		}
		// Transient error — requeue.
		return ctrl.Result{RequeueAfter: requeueBackoff}, err
	}

	switch rec.ClusterGate {
	case provider.ClusterGateElsewhere:
		return r.setPhase(ctx, or, onboardingv1alpha1.PhaseSkipped, "BelongsToOtherCluster")
	case provider.ClusterGateUnknown:
		// Unknown cluster — treat as transient; requeue and wait for inventory to be updated.
		return ctrl.Result{RequeueAfter: requeueBackoff}, nil
	}

	// Persist the resolved inventory data in status.
	patch := client.MergeFrom(or.DeepCopy())
	or.Status.OOBIP = rec.OOBIP
	or.Status.ServerName = rec.ServerName
	or.Status.Phase = onboardingv1alpha1.PhaseRedfishProbe
	or.Status.Reason = ""
	or.Status.LastTransitionTime = nowPtr()
	if err := r.Status().Patch(ctx, or, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// redfishProbePhase performs an unauthenticated GET /redfish/v1 on the OOB IP.
func (r *OnboardingRequestReconciler) redfishProbePhase(ctx context.Context, or *onboardingv1alpha1.OnboardingRequest) (ctrl.Result, error) {
	if or.Status.OOBIP == "" {
		// Status lost; restart from inventory lookup.
		return r.setPhase(ctx, or, onboardingv1alpha1.PhaseInventoryLookup, "OOBIPMissing")
	}

	managerType, err := probeRedfish(ctx, or.Status.OOBIP)
	if err != nil {
		// Transient — BMC may not be reachable yet.
		_ = r.patchReason(ctx, or, "Unreachable")
		return ctrl.Result{RequeueAfter: requeueBackoff}, nil
	}
	if managerType == "" {
		_ = r.patchReason(ctx, or, "NoManagerType")
		return ctrl.Result{RequeueAfter: requeueBackoff}, nil
	}

	patch := client.MergeFrom(or.DeepCopy())
	or.Status.ManagerType = managerType
	or.Status.Phase = onboardingv1alpha1.PhaseBMCCreation
	or.Status.Reason = ""
	or.Status.LastTransitionTime = nowPtr()
	if err := r.Status().Patch(ctx, or, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// bmcCreationPhase creates BMC, BMCSecret, and ServerWiring objects.
func (r *OnboardingRequestReconciler) bmcCreationPhase(ctx context.Context, or *onboardingv1alpha1.OnboardingRequest) (ctrl.Result, error) {
	if or.Status.ServerName == "" || or.Status.OOBIP == "" {
		return r.setPhase(ctx, or, onboardingv1alpha1.PhaseInventoryLookup, "ServerNameOrOOBIPMissing")
	}

	// Resolve the InventoryRecord for labels and BMCHostname (needed for BMC creation).
	var rec *provider.InventoryRecord
	if r.InventoryProvider != nil {
		var err error
		rec, err = r.InventoryProvider.LookupByMAC(ctx, or.Spec.MACAddress)
		if err != nil && err != provider.ErrNotFound {
			return ctrl.Result{RequeueAfter: requeueBackoff}, err
		}
	}

	bmcName := or.Status.ServerName

	// CreateOrUpdate BMCSecret (bootstrap credential placeholder).
	bmcSecret := &metalv1alpha1.BMCSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bmcName,
			Namespace: or.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, bmcSecret, func() error {
		// Placeholder — mmo will populate the real credentials via BMCBootstrapPolicy.
		if bmcSecret.Data == nil {
			bmcSecret.Data = map[string][]byte{}
		}
		return nil
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("CreateOrUpdate BMCSecret: %w", err)
	}

	// Parse OOB IP for the BMC endpoint.
	oobIP, err := metalv1alpha1.ParseIP(or.Status.OOBIP)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid OOB IP %q: %w", or.Status.OOBIP, err)
	}

	// Build BMC labels from inventory record.
	bmcLabels := map[string]string{}
	if rec != nil {
		for k, v := range rec.Labels {
			bmcLabels[k] = v
		}
	}

	// CreateOrUpdate BMC.
	bmc := &metalv1alpha1.BMC{
		ObjectMeta: metav1.ObjectMeta{
			Name: bmcName,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, bmc, func() error {
		// Labels.
		if bmc.Labels == nil {
			bmc.Labels = map[string]string{}
		}
		for k, v := range bmcLabels {
			bmc.Labels[k] = v
		}
		// Handoff annotations — always ensure both are set.
		if bmc.Annotations == nil {
			bmc.Annotations = map[string]string{}
		}
		bmc.Annotations[annotationBootstrap] = "true"
		bmc.Annotations[annotationManagerType] = or.Status.ManagerType

		// Spec — only set on creation to avoid overwriting mmo's changes.
		if bmc.ResourceVersion == "" {
			bmc.Spec = metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{IP: oobIP},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolNameRedfish,
					Port: 443,
				},
				BMCSecretRef: corev1.LocalObjectReference{Name: bmcSecret.Name},
			}
			if rec != nil && rec.BMCHostname != "" {
				bmc.Spec.Hostname = &rec.BMCHostname
			}
		}
		return nil
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("CreateOrUpdate BMC: %w", err)
	}

	// Verify both handoff annotations are present.
	if bmc.Annotations[annotationBootstrap] != "true" || bmc.Annotations[annotationManagerType] == "" {
		return ctrl.Result{RequeueAfter: requeueBackoff}, nil
	}

	return r.setPhase(ctx, or, onboardingv1alpha1.PhaseDone, "")
}

// SetupWithManager sets up the controller with the Manager.
func (r *OnboardingRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&onboardingv1alpha1.OnboardingRequest{}).
		Named("onboardingrequest").
		Complete(r)
}

// setPhase patches the OnboardingRequest status to the given phase.
func (r *OnboardingRequestReconciler) setPhase(ctx context.Context, or *onboardingv1alpha1.OnboardingRequest, phase onboardingv1alpha1.OnboardingPhase, reason string) (ctrl.Result, error) {
	patch := client.MergeFrom(or.DeepCopy())
	or.Status.Phase = phase
	or.Status.Reason = reason
	or.Status.LastTransitionTime = nowPtr()
	if err := r.Status().Patch(ctx, or, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// patchReason updates only the status.reason field without changing the phase.
func (r *OnboardingRequestReconciler) patchReason(ctx context.Context, or *onboardingv1alpha1.OnboardingRequest, reason string) error {
	patch := client.MergeFrom(or.DeepCopy())
	or.Status.Reason = reason
	return r.Status().Patch(ctx, or, patch)
}

// inventoryRetryCount returns how many times the OnboardingRequest has been requeued
// for inventory lookup. Uses a condition with type "InventoryLookupAttempts".
func inventoryRetryCount(or *onboardingv1alpha1.OnboardingRequest) int {
	for _, c := range or.Status.Conditions {
		if c.Type == "InventoryLookupAttempts" {
			var count int
			_ = json.Unmarshal([]byte(c.Message), &count)
			return count
		}
	}
	return 0
}

// probeRedfish performs an unauthenticated GET /redfish/v1 and extracts the ManagerType.
func probeRedfish(ctx context.Context, oobIP string) (string, error) {
	url := fmt.Sprintf("http://%s/redfish/v1", oobIP)
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("redfish probe returned %d", resp.StatusCode)
	}

	var body struct {
		ManagerType string `json:"ManagerType"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.ManagerType, nil
}

func nowPtr() *metav1.Time {
	t := metav1.Now()
	return &t
}
