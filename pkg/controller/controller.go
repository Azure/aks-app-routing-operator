// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package controller

import (
	"context"
	"fmt"
	"net/http"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/nginxingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/service"
	"github.com/go-logr/logr"
	cfgv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	ubzap "go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	netv1 "k8s.io/api/networking/v1"
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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/dns"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/ingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/osm"
)

var scheme = runtime.NewScheme()

const (
	nicIngressClassIndex     = "spec.ingressClassName"
	gatewayListenerIndexName = "spec.listeners.tls.options.kubernetes.azure.com/tls-cert-service-account"
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
	utilruntime.Must(gatewayv1.Install(s))
}

func NewRestConfig(conf *config.Config) *rest.Config {
	rc := ctrl.GetConfigOrDie()
	if conf.ServiceAccountTokenPath != "" {
		rc.BearerTokenFile = conf.ServiceAccountTokenPath
	}

	return rc
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
	})
	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	setupLog := m.GetLogger().WithName("setup")
	if err := setupProbes(conf, m, setupLog); err != nil {
		setupLog.Error(err, "failed to set up probes")
		return nil, fmt.Errorf("setting up probes: %w", err)
	}

	// create non-caching clients, non-caching for use before manager has started
	cl, err := client.New(rc, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create non-caching client")
		return nil, fmt.Errorf("creating non-caching client: %w", err)
	}

	if err := loadCRDs(cl, conf, setupLog); err != nil {
		setupLog.Error(err, "failed to load CRDs")
		return nil, fmt.Errorf("loading CRDs: %w", err)
	}

	if err := setupIndexers(m, setupLog); err != nil {
		setupLog.Error(err, "unable to setup indexers")
		return nil, fmt.Errorf("setting up indexers: %w", err)
	}

	if err := setupControllers(m, conf, setupLog, cl); err != nil {
		setupLog.Error(err, "unable to setup controllers")
		return nil, fmt.Errorf("setting up controllers: %w", err)
	}

	return m, nil
}

func setupIndexers(mgr ctrl.Manager, lgr logr.Logger) error {
	lgr.Info("setting up indexers")

	lgr.Info("adding Nginx Ingress Controller IngressClass indexer")
	if err := nginxingress.AddIngressClassNameIndex(mgr.GetFieldIndexer(), nicIngressClassIndex); err != nil {
		lgr.Error(err, "adding Nginx Ingress Controller IngressClass indexer")
		return fmt.Errorf("adding Nginx Ingress Controller IngressClass indexer: %w", err)
	}

	if err := keyvault.AddGatewayServiceAccountIndex(mgr.GetFieldIndexer(), gatewayListenerIndexName); err != nil {
		lgr.Error(err, "adding Gateway Service Account indexer")
		return fmt.Errorf("adding Gateway Service Account indexer: %w", err)
	}

	lgr.Info("finished setting up indexers")
	return nil
}

func setupControllers(mgr ctrl.Manager, conf *config.Config, lgr logr.Logger, cl client.Client) error {
	lgr.Info("setting up controllers")

	lgr.Info("determining default IngressClass controller class")
	defaultCc, err := nginxingress.GetDefaultIngressClassControllerClass(cl)
	if err != nil {
		return fmt.Errorf("determining default IngressClass controller class: %w", err)
	}

	lgr.Info("settup up ExternalDNS controller")
	if err := dns.NewExternalDns(mgr, conf); err != nil {
		return fmt.Errorf("setting up external dns controller: %w", err)
	}

	lgr.Info("setting up Nginx Ingress Controller reconciler")
	if err := nginxingress.NewReconciler(conf, mgr, defaultCc); err != nil {
		return fmt.Errorf("setting up nginx ingress controller reconciler: %w", err)
	}

	lgr.Info("setting up ingress cert config reconciler")
	if err = osm.NewIngressCertConfigReconciler(mgr, conf); err != nil {
		return fmt.Errorf("setting up ingress cert config reconciler: %w", err)
	}

	defaultNic := nginxingress.GetDefaultNginxIngressController()
	if err := service.NewNginxIngressReconciler(mgr, nginxingress.ToNginxIngressConfig(&defaultNic, defaultCc)); err != nil {
		return fmt.Errorf("setting up nginx ingress reconciler: %w", err)
	}

	lgr.Info("setting up default Nginx Ingress Controller reconciler")
	if err := nginxingress.NewDefaultReconciler(mgr, conf); err != nil {
		return fmt.Errorf("setting up nginx ingress default controller reconciler: %w", err)
	}

	lgr.Info("setting up ingress concurrency watchdog")
	if err := ingress.NewConcurrencyWatchdog(mgr, conf, ingress.GetListNginxWatchdogTargets(mgr.GetClient(), defaultCc)); err != nil {
		return fmt.Errorf("setting up ingress concurrency watchdog: %w", err)
	}

	ingressManager := keyvault.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
		return nginxingress.IsIngressManaged(context.Background(), mgr.GetClient(), ing, nicIngressClassIndex)
	})
	lgr.Info("setting up keyvault secret provider class reconciler")
	if err := keyvault.NewIngressSecretProviderClassReconciler(mgr, conf, ingressManager); err != nil {
		return fmt.Errorf("setting up ingress secret provider class reconciler: %w", err)
	}
	lgr.Info("setting up nginx keyvault secret provider class reconciler")
	if err := keyvault.NewNginxSecretProviderClassReconciler(mgr, conf); err != nil {
		return fmt.Errorf("setting up nginx secret provider class reconciler: %w", err)
	}
	lgr.Info("setting up keyvault placeholder pod controller")
	if err := keyvault.NewPlaceholderPodController(mgr, conf, ingressManager); err != nil {
		return fmt.Errorf("setting up placeholder pod controller: %w", err)
	}
	lgr.Info("setting up ingress tls reconciler")
	if err := keyvault.NewIngressTlsReconciler(mgr, conf, ingressManager); err != nil {
		return fmt.Errorf("setting up ingress tls reconciler: %w", err)
	}
	lgr.Info("setting up keyvault event mirror")
	if err = keyvault.NewEventMirror(mgr, conf); err != nil {
		return fmt.Errorf("setting up event mirror: %w", err)
	}

	ingressSourceSpecer := osm.NewIngressControllerSourceSpecerFromFn(func(ing *netv1.Ingress) (policyv1alpha1.IngressSourceSpec, bool, error) {
		return nginxingress.IngressSource(context.Background(), mgr.GetClient(), conf, defaultCc, ing, nicIngressClassIndex)
	})
	lgr.Info("setting up ingress backend reconciler")
	if err := osm.NewIngressBackendReconciler(mgr, conf, ingressSourceSpecer); err != nil {
		return fmt.Errorf("setting up ingress backend reconciler: %w", err)
	}

	if conf.EnableGateway {
		lgr.Info("setting up gateway reconcilers")
		if err := keyvault.NewGatewaySecretClassProviderReconciler(mgr, conf, gatewayListenerIndexName); err != nil {
			return fmt.Errorf("setting up Gateway SPC reconciler: %w", err)
		}
	}

	lgr.Info("finished setting up controllers")
	return nil
}

func setupProbes(conf *config.Config, mgr ctrl.Manager, log logr.Logger) error {
	log.Info("adding probes to manager")

	check := func(req *http.Request) error { return nil }

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
