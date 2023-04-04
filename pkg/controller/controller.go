// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package controller

import (
	"context"
	"net/http"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/dns"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/ingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/nginx"
	"github.com/go-logr/logr"
	cfgv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/osm"
)

func init() {
	ctrl.SetLogger(klogr.New())
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
		LeaderElection:          true,
		LeaderElectionNamespace: "kube-system",
		LeaderElectionID:        "aks-app-routing-operator-leader",
		MetricsBindAddress:      conf.MetricsAddr,
		HealthProbeBindAddress:  conf.ProbeAddr,
	})
	if err != nil {
		return nil, err
	}
	secv1.Install(m.GetScheme())
	cfgv1alpha2.AddToScheme(m.GetScheme())
	policyv1alpha1.AddToScheme(m.GetScheme())

	m.AddHealthzCheck("liveness", func(req *http.Request) error { return nil })

	kcs, err := kubernetes.NewForConfig(rc) // need non-caching client since manager hasn't started yet
	if err != nil {
		return nil, err
	}

	log := m.GetLogger()
	deploy, err := getSelfDeploy(kcs, conf, log)
	if err != nil {
		return nil, err
	}
	log.V(2).Info("using namespace: " + conf.NS)

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

	return m, nil
}

func getSelfDeploy(kcs kubernetes.Interface, conf *config.Config, log logr.Logger) (*appsv1.Deployment, error) {
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
