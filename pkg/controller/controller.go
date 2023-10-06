// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package controller

import (
	"context"
	"net/http"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/dns"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/ingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/nginx"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/nginxingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/osm"
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
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var scheme = runtime.NewScheme()

func init() {
	registerSchemes(scheme)
	lgr := getLogger()
	ctrl.SetLogger(lgr)
	// need to set klog logger to same logger to get consistent logging format for all logs.
	// without this things like leader election that use klog will not have the same format.
	// https://github.com/kubernetes/client-go/blob/560efb3b8995da3adcec09865ca78c1ddc917cc9/tools/leaderelection/leaderelection.go#L250
	klog.SetLogger(lgr)
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
	})
	if err != nil {
		return nil, err
	}

	m.AddHealthzCheck("liveness", func(req *http.Request) error { return nil })

	// setup non-caching clients for use before manager starts
	kcs, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, err
	}
	cl, err := client.New(rc, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	log := m.GetLogger()
	deploy, err := getSelfDeploy(kcs, conf, log)
	if err != nil {
		return nil, err
	}
	log.V(2).Info("using namespace: " + conf.NS)

	if err := loadCRDs(cl, conf, log); err != nil {
		log.Error(err, "failed to load CRDs")
		return nil, err
	}

	if err := dns.NewExternalDns(m, conf, deploy); err != nil {
		return nil, err
	}

	nginxConfigs, err := nginx.New(m, conf, deploy)
	if err != nil {
		return nil, err
	}

	watchdogTargets := make([]*ingress.WatchdogTarget, 0)
	for _, nginxConfig := range nginxConfigs {
		watchdogTargets = append(watchdogTargets, &ingress.WatchdogTarget{
			ScrapeFn:    ingress.NginxScrapeFn,
			LabelGetter: nginxConfig,
		})
	}
	if err := ingress.NewConcurrencyWatchdog(m, conf, watchdogTargets); err != nil {
		return nil, err
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
	if err := keyvault.NewIngressSecretProviderClassReconciler(m, conf, ingressManager); err != nil {
		return nil, err
	}
	if err := keyvault.NewPlaceholderPodController(m, conf, ingressManager); err != nil {
		return nil, err
	}
	if err = keyvault.NewEventMirror(m, conf); err != nil {
		return nil, err
	}

	ingressControllerNamer := osm.NewIngressControllerNamer(icToController)
	if err := osm.NewIngressBackendReconciler(m, conf, ingressControllerNamer); err != nil {
		return nil, err
	}
	if err = osm.NewIngressCertConfigReconciler(m, conf); err != nil {
		return nil, err
	}

	if err := nginxingress.NewReconciler(conf, m); err != nil {
		return nil, err
	}

	return m, nil
}

func getSelfDeploy(kcs kubernetes.Interface, conf *config.Config, log logr.Logger) (*appsv1.Deployment, error) {
	// this doesn't work today. operator ns is not the same as resource ns which means we can't set this operator
	// as the owner of any child resources. https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/#owner-references-in-object-specifications
	// dynamic provisioning through a crd will fix this and fix our garbage collection.
	return nil, nil

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
