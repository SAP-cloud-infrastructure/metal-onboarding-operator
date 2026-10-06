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

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	onboardingv1alpha1 "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/api/v1alpha1"
	"github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/controller"
	dhcpshim "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/dhcp"
	crdprovider "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/provider/crd"
	netboxprovider "github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/provider/netbox"
	"github.wdf.sap.corp/sap-cloud-infrastructure/metal-onboarding-operator/internal/provider"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(onboardingv1alpha1.AddToScheme(scheme))
	utilruntime.Must(dhcpshim.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var inventoryProvider string
	var dhcpLeaseNamespace string
	var netboxURL string
	var netboxTokenFile string
	var clusterName string
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&inventoryProvider, "inventory-provider", "crd",
		"Inventory provider backend to use for MAC lookups. One of: netbox, crd.")
	flag.StringVar(&dhcpLeaseNamespace, "dhcp-lease-namespace", "",
		"Namespace to watch for DHCPLease CRs. Defaults to the operator's namespace.")
	flag.StringVar(&netboxURL, "netbox-url", "",
		"Base URL of the NetBox API (required when --inventory-provider=netbox).")
	flag.StringVar(&netboxTokenFile, "netbox-token-file", "",
		"Path to a file containing the NetBox API token (required when --inventory-provider=netbox).")
	flag.StringVar(&clusterName, "cluster-name", "",
		"Name of this cluster, used to gate onboarding when --inventory-provider=netbox.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "71df65af.metal.ironcore.dev",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Build the InventoryProvider based on the --inventory-provider flag.
	var invProvider provider.InventoryProvider
	switch inventoryProvider {
	case "netbox":
		if netboxURL == "" || netboxTokenFile == "" {
			setupLog.Error(fmt.Errorf("--netbox-url and --netbox-token-file are required for --inventory-provider=netbox"), "invalid configuration")
			os.Exit(1)
		}
		invProvider = netboxprovider.New(netboxURL, netboxTokenFile, clusterName)
	case "crd":
		operatorNamespace := os.Getenv("POD_NAMESPACE")
		if operatorNamespace == "" {
			operatorNamespace = "default"
		}
		invProvider = crdprovider.New(mgr.GetClient(), operatorNamespace)
	default:
		setupLog.Error(fmt.Errorf("unknown --inventory-provider %q: must be netbox or crd", inventoryProvider), "invalid configuration")
		os.Exit(1)
	}

	// Wire DHCPLeaseController.
	if err := (&controller.DHCPLeaseController{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Namespace: dhcpLeaseNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DHCPLease")
		os.Exit(1)
	}

	// Wire OnboardingRequestReconciler with the selected InventoryProvider.
	if err := (&controller.OnboardingRequestReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		InventoryProvider: invProvider,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OnboardingRequest")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager",
		"inventory-provider", inventoryProvider,
		"dhcp-lease-namespace", dhcpLeaseNamespace,
		"leader-elect", enableLeaderElection,
	)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
