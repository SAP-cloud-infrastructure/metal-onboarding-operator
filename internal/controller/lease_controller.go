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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	baseboardv1alpha1 "github.com/ironcore-dev/metal-maintenance-operator/api/baseboard/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	onboardingv1alpha1 "github.com/SAP-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
	dhcpshim "github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/dhcp"
	"github.com/SAP-cloud-infrastructure/metal-onboarding-operator/internal/provider"
)

const (
	annotationBootstrap           = "onboarding.metal.ironcore.dev/bootstrap"
	annotationManagerType         = "onboarding.metal.ironcore.dev/manager-type"
	annotationInventoryGeneration = "onboarding.metal.ironcore.dev/inventory-generation"
	annotationOnboardState        = "metal.cloud.sap/onboarding-state"

	labelManagedBy      = "onboarding.metal.ironcore.dev/managed-by"
	labelManagedByValue = "metal-onboarding-operator"

	onboardStateBMCUnreachable = "bmc-unreachable"
	onboardStateSerialNotFound = "serial-not-found"
	onboardStateComplete       = "complete"

	redfishProbeTimeout = 10 * time.Second
)

// LeaseController watches Lease CRs and drives the full onboarding workflow:
// Redfish probe → serial lookup → MAC lookup → BMC CR creation → BMCUserSet creation.
// The BMC CR with bootstrap=true annotation is the handoff signal to metal-maintenance-operator.
type LeaseController struct {
	client.Client
	Scheme            *runtime.Scheme
	InventoryProvider provider.InventoryProvider
	EventRecorder     record.EventRecorder
	// ClusterName is the metadata.name of the MetalCluster CR this controller watches.
	// Used to filter MetalCluster watch events to the relevant CR only.
	ClusterName string
	// RedfishBaseURL overrides the Redfish endpoint base URL, used in tests.
	// When empty, the controller uses https://<lease.Spec.IP> as the base URL.
	RedfishBaseURL string
	// RedfishProbeTimeout overrides the per-probe HTTP timeout (default 10s).
	RedfishProbeTimeout time.Duration
	// FactoryBMCUserRef is the name of a BMCUser CR whose BootstrapSecretRef holds
	// factory credentials. When set, the controller uses these credentials to fetch
	// SerialNumber from /redfish/v1/Systems/1 for vendors that don't expose it
	// unauthenticated (e.g. HPE, Lenovo).
	FactoryBMCUserRef string
	// BMCUserSet holds the template config for BMCUserSet objects created per node.
	// When nil, no BMCUserSet is created (e.g. in tests or when not configured).
	BMCUserSet *BMCUserSetConfig
}

// BMCUserSetConfig holds the template fields written into each BMCUserSet created by moo.
// All fields map 1:1 to the corresponding BMCUserSet spec fields.
type BMCUserSetConfig struct {
	SetupCredentialRef        corev1.SecretReference
	FactoryUserNames          []string
	RotationStrategy          baseboardv1alpha1.RotationStrategy
	UserName                  string
	RoleID                    string
	RotationPeriod            metav1.Duration
	CredentialSecretNamespace string
}

// +kubebuilder:rbac:groups=dhcp.metal.ironcore.dev,resources=leases,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=onboarding.metal.ironcore.dev,resources=metalclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=baseboard.metal.ironcore.dev,resources=bmcusersets,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=baseboard.metal.ironcore.dev,resources=bmcusers,verbs=get;list;watch
// +kubebuilder:rbac:groups=baseboard.metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch

// Reconcile drives the full onboarding workflow for a single Lease in one pass.
// The reconciler is fully idempotent: if a BMC CR already exists for the server, the
// CreateOrUpdate call is a no-op (spec is only written on creation).
func (r *LeaseController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	lease := &dhcpshim.Lease{}
	if err := r.Get(ctx, req.NamespacedName, lease); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	mac := lease.Spec.MACAddress
	if mac == "" {
		log.Info("Lease has no MACAddress, skipping", "lease", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	ip := lease.Spec.IP
	if ip == "" {
		log.Info("Lease has no IP, skipping", "lease", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// --- 1. Redfish probe — unauthenticated for Dell/Bull, authenticated fallback for HPE/Lenovo ---
	var creds *bmcCredentials
	if r.FactoryBMCUserRef != "" {
		if c, err := r.fetchFactoryCredentials(ctx); err != nil {
			log.Error(err, "failed to fetch factory BMC credentials, authenticated Redfish probe disabled")
		} else {
			creds = c
		}
	}
	redfishResult, redfishErr := probeRedfish(ctx, r.redfishURL(ip), r.probeTimeout(), creds)
	bmcUnreachable := redfishErr != nil
	bmcReachable := !bmcUnreachable

	// --- 2. Serial lookup (skip if Redfish unreachable or returned no ServiceTag) ---
	var rec *provider.InventoryRecord
	var serialNotFound bool

	if bmcReachable && redfishResult.ServiceTag != "" {
		rec, err := r.InventoryProvider.LookupBySerial(ctx, redfishResult.ServiceTag)
		if err == nil {
			return r.reconcileBMC(ctx, log, lease, rec, redfishResult.ManagerType)
		}
		if !errors.Is(err, provider.ErrNotFound) {
			return ctrl.Result{}, fmt.Errorf("serial lookup for %s: %w", redfishResult.ServiceTag, err)
		}
		serialNotFound = true
		_ = rec
	}

	// --- 3. MAC lookup (fallback) ---
	var err error
	rec, err = r.InventoryProvider.LookupByMAC(ctx, mac)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			// Determine annotation value based on what we learned from Redfish.
			state := onboardStateBMCUnreachable
			reason := "BMCUnreachable"
			msg := fmt.Sprintf("MAC %s not found in inventory and BMC at %s was unreachable", mac, ip)
			if bmcReachable {
				if serialNotFound {
					state = onboardStateSerialNotFound
					reason = "SerialNotFound"
					msg = fmt.Sprintf("serial %s not found in inventory and MAC %s not found either", redfishResult.ServiceTag, mac)
				} else {
					// Redfish responded but no ServiceTag — BMC reachable but incomplete data.
					state = onboardStateBMCUnreachable
					reason = "BMCUnreachable"
					msg = fmt.Sprintf("MAC %s not found in inventory and no serial number available from BMC at %s", mac, ip)
				}
			}
			r.patchLeaseAnnotation(ctx, log, lease, state)
			if r.EventRecorder != nil {
				r.EventRecorder.Event(lease, corev1.EventTypeWarning, reason, msg)
			}
			log.Info("neither serial nor MAC found in inventory, annotated lease", "mac", mac, "state", state)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("inventory lookup for MAC %s: %w", mac, err)
	}

	return r.reconcileBMC(ctx, log, lease, rec, redfishResult.ManagerType)
}

// reconcileBMC creates or updates the BMC CR and patches the lease annotation to "complete".
func (r *LeaseController) reconcileBMC(ctx context.Context, log logr.Logger, lease *dhcpshim.Lease, rec *provider.InventoryRecord, managerType string) (ctrl.Result, error) {
	_ = log // used via logf.FromContext elsewhere; keep for future structured logging

	// When the inventory source (e.g. MetalCluster) does not carry the OOB IP,
	// fall back to the lease IP — the BMC that sent the DHCP Discover is at that address.
	if rec.OOBIP == "" {
		rec.OOBIP = lease.Spec.IP
	}

	switch rec.ClusterGate {
	case provider.ClusterGateElsewhere:
		logf.FromContext(ctx).Info("server belongs to another cluster, skipping", "server", rec.ServerName)
		return ctrl.Result{}, nil
	case provider.ClusterGateUnknown:
		logf.FromContext(ctx).Info("cluster membership unknown, requeuing", "server", rec.ServerName)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	ip := lease.Spec.IP
	oobIP, err := metalv1alpha1.ParseIP(rec.OOBIP)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("parse OOB IP %q: %w", ip, err)
	}

	bmc := &metalv1alpha1.BMC{}
	err = r.Get(ctx, types.NamespacedName{Name: rec.ServerName}, bmc)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("get BMC %s: %w", rec.ServerName, err)
	}

	if apierrors.IsNotFound(err) {
		labels := make(map[string]string, len(rec.Labels)+1)
		maps.Copy(labels, rec.Labels)
		labels[labelManagedBy] = labelManagedByValue
		bmc = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name:   rec.ServerName,
				Labels: labels,
				Annotations: map[string]string{
					annotationBootstrap:           "true",
					annotationInventoryGeneration: strconv.FormatInt(rec.InventoryGeneration, 10),
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
		if managerType != "" {
			bmc.Annotations[annotationManagerType] = managerType
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
		bmc.Labels[labelManagedBy] = labelManagedByValue
		if bmc.Annotations == nil {
			bmc.Annotations = map[string]string{}
		}
		bmc.Annotations[annotationBootstrap] = "true"
		bmc.Annotations[annotationInventoryGeneration] = strconv.FormatInt(rec.InventoryGeneration, 10)
		if managerType != "" {
			bmc.Annotations[annotationManagerType] = managerType
		}
		if err := r.Patch(ctx, bmc, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("patch BMC %s: %w", rec.ServerName, err)
		}
	}

	logf.FromContext(ctx).Info("BMC ready", "name", rec.ServerName, "managerType", managerType)
	r.patchLeaseAnnotation(ctx, logf.FromContext(ctx), lease, onboardStateComplete)

	if err := r.reconcileBMCUserSet(ctx, rec.ServerName); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile BMCUserSet %s: %w", rec.ServerName, err)
	}

	return ctrl.Result{}, nil
}

// reconcileBMCUserSet creates a BMCUserSet for the given BMC name if one does not exist.
// It is a no-op when BMCUserSet config is not set on the controller.
func (r *LeaseController) reconcileBMCUserSet(ctx context.Context, bmcName string) error {
	if r.BMCUserSet == nil {
		return nil
	}
	cfg := r.BMCUserSet

	existing := &baseboardv1alpha1.BMCUserSet{}
	err := r.Get(ctx, types.NamespacedName{Name: bmcName}, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get BMCUserSet %s: %w", bmcName, err)
	}

	rotationPeriod := cfg.RotationPeriod
	historyLimit := int32(10)
	retainCredential := true
	set := &baseboardv1alpha1.BMCUserSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: bmcName,
		},
		Spec: baseboardv1alpha1.BMCUserSetSpec{
			BMCSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"kubernetes.metal.cloud.sap/name": bmcName,
				},
			},
			SetupCredentialRef: &cfg.SetupCredentialRef,
			RotationStrategy:   cfg.RotationStrategy,
			Template: baseboardv1alpha1.BMCUserTemplate{
				UserName:                     cfg.UserName,
				RoleID:                       cfg.RoleID,
				RotationPeriod:               &rotationPeriod,
				CredentialSecretNameTemplate: "bmc-metal-cred-{{ .BMCName }}",
				CredentialSecretNamespace:    cfg.CredentialSecretNamespace,
				RotationHistoryLimit:         &historyLimit,
			},
			FactoryAccount: &baseboardv1alpha1.FactoryAccount{
				Action:           baseboardv1alpha1.FactoryAccountActionRotate,
				UserNames:        cfg.FactoryUserNames,
				RetainCredential: &retainCredential,
			},
		},
	}
	if err := r.Create(ctx, set); err != nil {
		return fmt.Errorf("create BMCUserSet %s: %w", bmcName, err)
	}
	logf.FromContext(ctx).Info("Created BMCUserSet", "name", bmcName)
	return nil
}

// patchLeaseAnnotation sets metal.cloud.sap/onboarding-state on the Lease.
func (r *LeaseController) patchLeaseAnnotation(ctx context.Context, log logr.Logger, lease *dhcpshim.Lease, state string) {
	_ = log
	base := lease.DeepCopy()
	if lease.Annotations == nil {
		lease.Annotations = map[string]string{}
	}
	lease.Annotations[annotationOnboardState] = state
	if err := r.Patch(ctx, lease, client.MergeFrom(base)); err != nil {
		logf.FromContext(ctx).Error(err, "failed to patch lease annotation", "state", state)
	}
}

// probeTimeout returns the configured probe timeout, falling back to redfishProbeTimeout.
func (r *LeaseController) probeTimeout() time.Duration {
	if r.RedfishProbeTimeout > 0 {
		return r.RedfishProbeTimeout
	}
	return redfishProbeTimeout
}

// redfishURL returns the Redfish base URL for the given IP.
func (r *LeaseController) redfishURL(ip string) string {
	if r.RedfishBaseURL != "" {
		return r.RedfishBaseURL
	}
	return fmt.Sprintf("https://%s", ip)
}

// redfishResult holds the fields extracted from a Redfish root response.
type redfishResult struct {
	ManagerType string
	ServiceTag  string
}

// bmcCredentials holds a username/password pair for authenticated Redfish requests.
type bmcCredentials struct {
	Username string
	Password string
}

// probeRedfish fetches serial number and manager type from a BMC's Redfish API.
// Without creds: tries unauthenticated root for Dell (ServiceTag) and Bull (Oem.SerialNumber).
// With creds: fetches real SerialNumber from /Systems/System.Embedded.1 (Dell) or /Systems/1 (others).
func probeRedfish(ctx context.Context, baseURL string, timeout time.Duration, creds *bmcCredentials) (redfishResult, error) {
	hc := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // BMC certs are self-signed
		},
	}

	doGet := func(url, username, password string) (*http.Response, error) {
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		if username != "" {
			req.SetBasicAuth(username, password)
		}
		return hc.Do(req)
	}

	getSerial := func(path string) string {
		resp, err := doGet(baseURL+path, creds.Username, creds.Password)
		if err != nil {
			return ""
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return ""
		}
		var body struct {
			SerialNumber string `json:"SerialNumber"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return ""
		}
		return body.SerialNumber
	}

	// discoverSerial fetches /redfish/v1/Systems, picks the first member, and returns its SerialNumber.
	discoverSerial := func() string {
		resp, err := doGet(baseURL+"/redfish/v1/Systems", creds.Username, creds.Password)
		if err != nil {
			return ""
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return ""
		}
		var col struct {
			Members []struct {
				OdataID string `json:"@odata.id"`
			} `json:"Members"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&col); err != nil || len(col.Members) == 0 {
			return ""
		}
		return getSerial(col.Members[0].OdataID)
	}

	// Unauthenticated root probe to detect vendor and get serial for Bull.
	resp, err := doGet(baseURL+"/redfish/v1", "", "")
	if err != nil {
		return redfishResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return redfishResult{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var root struct {
		ManagerType string `json:"ManagerType"`
		Oem         struct {
			Dell struct {
				ServiceTag string `json:"ServiceTag"`
			} `json:"Dell"`
			SerialNumber string `json:"SerialNumber"` // Bull/Atos
		} `json:"Oem"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		return redfishResult{}, err
	}

	var serial string
	if creds != nil {
		// Bull exposes serial unauthenticated at root — no need to probe further.
		if root.Oem.SerialNumber != "" {
			serial = root.Oem.SerialNumber
		} else {
			// Discover the first system member and fetch its SerialNumber.
			serial = discoverSerial()
		}
	} else {
		// Unauthenticated: Dell ServiceTag or Bull Oem.SerialNumber
		serial = root.Oem.Dell.ServiceTag
		if serial == "" {
			serial = root.Oem.SerialNumber
		}
	}

	return redfishResult{
		ManagerType: root.ManagerType,
		ServiceTag:  serial,
	}, nil
}

// fetchFactoryCredentials retrieves the bootstrap credentials from the BMCUser CR
// referenced by FactoryBMCUserRef, via its Status.BootstrapSecretRef → BMCSecret.
func (r *LeaseController) fetchFactoryCredentials(ctx context.Context) (*bmcCredentials, error) {
	bmcUser := &baseboardv1alpha1.BMCUser{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.FactoryBMCUserRef}, bmcUser); err != nil {
		return nil, fmt.Errorf("get BMCUser %s: %w", r.FactoryBMCUserRef, err)
	}
	if bmcUser.Status.BootstrapSecretRef == nil {
		return nil, nil
	}
	secret := &metalv1alpha1.BMCSecret{}
	if err := r.Get(ctx, types.NamespacedName{Name: bmcUser.Status.BootstrapSecretRef.Name}, secret); err != nil {
		return nil, fmt.Errorf("get BMCSecret %s: %w", bmcUser.Status.BootstrapSecretRef.Name, err)
	}
	return &bmcCredentials{
		Username: string(secret.Data[metalv1alpha1.BMCSecretUsernameKeyName]),
		Password: string(secret.Data[metalv1alpha1.BMCSecretPasswordKeyName]),
	}, nil
}

// SetupWithManager registers the LeaseController with the manager.
func (r *LeaseController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dhcpshim.Lease{}).
		Watches(
			&onboardingv1alpha1.MetalCluster{},
			handler.EnqueueRequestsFromMapFunc(r.metalClusterToLeases),
		).
		Named("lease").
		Complete(r)
}

// metalClusterToLeases maps a MetalCluster change event to all Leases so that
// label changes in the MetalCluster are propagated to existing BMC CRs.
// When ClusterName is set only changes to that CR trigger re-enqueuing; when empty
// any MetalCluster change does (useful when the operator manages a single cluster
// and --cluster-name is not configured).
func (r *LeaseController) metalClusterToLeases(ctx context.Context, obj client.Object) []reconcile.Request {
	if r.ClusterName != "" && obj.GetName() != r.ClusterName {
		return nil
	}
	leaseList := &dhcpshim.LeaseList{}
	if err := r.List(ctx, leaseList); err != nil {
		logf.FromContext(ctx).Error(err, "failed to list Leases for MetalCluster re-enqueue")
		return nil
	}
	reqs := make([]reconcile.Request, len(leaseList.Items))
	for i, lease := range leaseList.Items {
		reqs[i] = reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      lease.Name,
			Namespace: lease.Namespace,
		}}
	}
	return reqs
}
