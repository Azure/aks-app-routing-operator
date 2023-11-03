// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package controller

import (
	"context"
	"fmt"
	"net/http"
	"os"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/nginxingress"
	"github.com/Azure/aks-app-routing-operator/pkg/webhook"
	"github.com/go-logr/logr"
	cfgv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	ubzap "go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/dns"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/ingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/nginx"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/osm"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	registerSchemes(scheme)
	ctrl.SetLogger(getLogger())
	// need to set klog logger to same logger to get consistent logging format for all logs.
	// without this things like leader election that use klog will not have the same format.
	// https://github.com/kubernetes/client-go/blob/560efb3b8995da3adcec09865ca78c1ddc917cc9/tools/leaderelection/leaderelection.go#L250
	klog.SetLogger(getLogger())
}

func getLogger(opts ...zap.Opts) logr.Logger {
	// use raw opts to add caller info to logs
	rawOpts := zap.RawZapOpts(ubzap.AddCaller())

	// zap is the default recommended logger for controller-runtime when wanting json structured output
	return zap.New(append(opts, rawOpts)...)
}

func registerSchemes(s *runtime.Scheme) {
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(secv1.Install(s))
	utilruntime.Must(cfgv1alpha2.AddToScheme(s))
	utilruntime.Must(policyv1alpha1.AddToScheme(s))
	utilruntime.Must(approutingv1alpha1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))
}

func NewManager(conf *config.Config) (ctrl.Manager, error) {
	rc := ctrl.GetConfigOrDie()
	if conf.ServiceAccountTokenPath != "" {
		rc.BearerTokenFile = conf.ServiceAccountTokenPath
	}
	return NewManagerForRestConfig(conf, rc)
}

func NewManagerForRestConfig(conf *config.Config, rc *rest.Config) (ctrl.Manager, error) {
	m, err := ctrl.NewManager(rc, ctrl.Options{
		Metrics:                metricsserver.Options{BindAddress: conf.MetricsAddr},
		HealthProbeBindAddress: conf.ProbeAddr,
		Scheme:                 scheme,

		// we use an active-passive HA model meaning only the leader performs actions
		LeaderElection:          true,
		LeaderElectionNamespace: "kube-system",
		LeaderElectionID:        "aks-app-routing-operator-leader",

		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port:    conf.WebhookPort,
			CertDir: conf.CertDir,
		}),
	})
	if err != nil {
		return nil, err
	}

	setupLog := m.GetLogger().WithName("setup")
	webhooksReady := make(chan struct{})
	if err := setupProbes(m, webhooksReady, setupLog); err != nil {
		return nil, fmt.Errorf("setting up probes: %w", err)
	}

	// create non-caching clients, non-caching for use before manager has started
	cl, err := client.New(rc, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	if err := loadCRDs(cl, conf, setupLog); err != nil {
		return nil, fmt.Errorf("loading CRDs: %w", err)
	}

	if err := setupControllers(m, conf); err != nil {
		return nil, fmt.Errorf("setting up controllers: %w", err)
	}

	webhookCfg, err := webhook.New(conf)
	if err != nil {
		return nil, fmt.Errorf("creating webhook config: %w", err)
	}

	if err := webhookCfg.EnsureWebhookConfigurations(context.Background(), cl); err != nil {
		return nil, fmt.Errorf("ensuring webhook configurations: %w", err)
	}

	certsReady := make(chan struct{})
	if err := webhookCfg.AddCertManager(context.Background(), m, certsReady, cl); err != nil {
		return nil, fmt.Errorf("adding cert manager: %w", err)
	}

	go func() {
		// webhooks cannot be served until certificates are created and ready.
		// without this, the webhook server would attempt to start immediately and ctrl runtime
		// manager will fail because the certs don't exist yet.

		setupLog.Info("waiting for certs to be ready")
		<-certsReady
		setupLog.Info("certs are ready")

		setupLog.Info("setting up webhooks")
		if err := setupWebhooks(m, webhookCfg.AddWebhooks); err != nil {
			setupLog.Error(err, "failed to setup webhooks")
			os.Exit(1)
		}
		setupLog.Info("webhooks are ready")

		close(webhooksReady)
	}()

	return m, nil
}

func setupWebhooks(mgr ctrl.Manager, addWebhooksFn func(mgr ctrl.Manager) error) error {
	if err := addWebhooksFn(mgr); err != nil {
		return fmt.Errorf("adding webhooks: %w", err)
	}

	return nil
}

func setupControllers(mgr ctrl.Manager, conf *config.Config) error {
	var selfDeploy *appsv1.Deployment = nil // self deploy doesn't work because operator isn't in same resources as child resources

	if err := dns.NewExternalDns(mgr, conf); err != nil {
		return fmt.Errorf("setting up external dns controller: %w", err)
	}

	if err := nginxingress.NewReconciler(conf, mgr); err != nil {
		return fmt.Errorf("setting up nginx ingress controller reconciler: %w", err)
	}

	nginxConfigs, err := nginx.New(mgr, conf, selfDeploy)
	if err != nil {
		return fmt.Errorf("getting nginx configs: %w", err)
	}

	watchdogTargets := make([]*ingress.WatchdogTarget, 0)
	for _, nginxConfig := range nginxConfigs {
		watchdogTargets = append(watchdogTargets, &ingress.WatchdogTarget{
			ScrapeFn:    ingress.NginxScrapeFn,
			LabelGetter: nginxConfig,
		})
	}

	if err := ingress.NewConcurrencyWatchdog(mgr, conf, watchdogTargets); err != nil {
		return fmt.Errorf("setting up ingress concurrency watchdog: %w", err)
	}

	icToController := make(map[string]string)
	for _, nginxConfig := range nginxConfigs {
		icToController[nginxConfig.IcName] = nginxConfig.ResourceName
	}
	ics := make(map[string]struct{})
	for ic := range icToController {
		ics[ic] = struct{}{}
	}

	ingressManager := keyvault.NewIngressManager(ics)
	if err := keyvault.NewIngressSecretProviderClassReconciler(mgr, conf, ingressManager); err != nil {
		return fmt.Errorf("setting up ingress secret provider class reconciler: %w", err)
	}
	if err := keyvault.NewPlaceholderPodController(mgr, conf, ingressManager); err != nil {
		return fmt.Errorf("setting up placeholder pod controller: %w", err)
	}
	if err = keyvault.NewEventMirror(mgr, conf); err != nil {
		return fmt.Errorf("setting up event mirror: %w", err)
	}

	ingressControllerNamer := osm.NewIngressControllerNamer(icToController)
	if err := osm.NewIngressBackendReconciler(mgr, conf, ingressControllerNamer); err != nil {
		return fmt.Errorf("setting up ingress backend reconciler: %w", err)
	}
	if err = osm.NewIngressCertConfigReconciler(mgr, conf); err != nil {
		return fmt.Errorf("setting up ingress cert config reconciler: %w", err)
	}

	return nil
}

func setupProbes(mgr ctrl.Manager, webhooksReady <-chan struct{}, log logr.Logger) error {
	log.Info("adding probes to manager")

	// checks if the webhooks are ready so that the service can only serve webhook
	// traffic to ready webhooks
	check := func(req *http.Request) error {
		select {
		case <-webhooksReady:
			return mgr.GetWebhookServer().StartedChecker()(req)
		default:
			return fmt.Errorf("certs aren't ready yet")
		}
	}

	if err := mgr.AddReadyzCheck("readyz", check); err != nil {
		return fmt.Errorf("adding readyz check: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", check); err != nil {
		return fmt.Errorf("adding healthz check: %w", err)
	}

	log.Info("added probes to manager")
	return nil
}

func getSelfDeploy(kcs kubernetes.Interface, conf *config.Config, log logr.Logger) (*appsv1.Deployment, error) {
	// this doesn't actually work today. operator ns is not the same as resource ns which means we can't set this operator
	// as the owner of any child resources. https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/#owner-references-in-object-specifications
	// dynamic provisioning through a crd will fix this and fix our garbage collection.

	deploy, err := kcs.AppsV1().Deployments(conf.NS).Get(context.Background(), conf.OperatorDeployment, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// It's okay if we don't find the deployment - just skip setting ownership references latter
		log.Info("self deploy not found")
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return deploy, nil
}
