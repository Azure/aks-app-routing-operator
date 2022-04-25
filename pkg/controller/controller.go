// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package controller

import (
	"net/http"

	cfgv1alpha1 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha1"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/ingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/osm"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/service"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
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
	secv1.AddToScheme(m.GetScheme())
	cfgv1alpha1.AddToScheme(m.GetScheme())
	policyv1alpha1.AddToScheme(m.GetScheme())

	m.AddHealthzCheck("liveness", func(req *http.Request) error { return nil })

	if err = ingress.NewIngressControllerReconciler(m, manifests.IngressControllerResources(conf)); err != nil {
		return nil, err
	}
	if err = ingress.NewConcurrencyWatchdog(m, conf); err != nil {
		return nil, err
	}
	if err = keyvault.NewIngressSecretProviderClassReconciler(m, conf); err != nil {
		return nil, err
	}
	if err = keyvault.NewPlaceholderPodController(m, conf); err != nil {
		return nil, err
	}
	if err = keyvault.NewEventMirror(m, conf); err != nil {
		return nil, err
	}
	if err = osm.NewIngressCertConfigReconciler(m, conf); err != nil {
		return nil, err
	}
	if err = osm.NewIngressBackendReconciler(m, conf); err != nil {
		return nil, err
	}
	if err = service.NewIngressReconciler(m); err != nil {
		return nil, err
	}

	return m, nil
}
