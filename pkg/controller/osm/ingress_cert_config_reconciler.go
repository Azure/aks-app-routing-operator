// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package osm

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/go-logr/logr"
	cfgv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
)

const (
	osmNamespace          = "kube-system"
	osmMeshConfigName     = "osm-mesh-config"
	osmNginxSAN           = "ingress-nginx.ingress.cluster.local"
	osmClientCertValidity = "24h"
	osmClientCertName     = "osm-ingress-client-cert"
)

var ingressCertConfigControllerName = controllername.New("osm", "ingress", "cert", "config")

// IngressCertConfigReconciler updates the Open Service Mesh configuration to generate a client cert
// to be used by the ingress controller when contacting upstreams.
type IngressCertConfigReconciler struct {
	client client.Client
}

func NewIngressCertConfigReconciler(manager ctrl.Manager, conf *config.Config) error {
	metrics.InitControllerMetrics(ingressCertConfigControllerName)
	if conf.DisableOSM {
		return nil
	}
	return ingressCertConfigControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&cfgv1alpha2.MeshConfig{}), manager.GetLogger(),
	).Complete(&IngressCertConfigReconciler{client: manager.GetClient()})
}

func (i *IngressCertConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, retErr error) {
	defer func() {
		metrics.HandleControllerReconcileMetrics(ingressCertConfigControllerName, res, retErr)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = ingressCertConfigControllerName.AddToLogger(logger).WithValues("namespace", req.Namespace, "name", req.Name)

	if req.Name != osmMeshConfigName || req.Namespace != osmNamespace {
		logger.Info(fmt.Sprintf("ignoring mesh config, we only reconcile mesh config %s/%s", osmNamespace, osmMeshConfigName))
		return ctrl.Result{}, nil
	}

	logger.Info("getting OSM ingress mesh config")
	conf := &cfgv1alpha2.MeshConfig{}
	err = i.client.Get(ctx, req.NamespacedName, conf)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger = logger.WithValues("generation", conf.Generation)

	var dirty bool
	if conf.Spec.Certificate.IngressGateway == nil {
		conf.Spec.Certificate.IngressGateway = &cfgv1alpha2.IngressGatewayCertSpec{}
	}
	if conf.Spec.Certificate.IngressGateway.Secret.Name != osmClientCertName {
		logger.Info("updating IngressGateway client cert secret name")
		dirty = true
		conf.Spec.Certificate.IngressGateway.Secret.Name = osmClientCertName
	}
	if conf.Spec.Certificate.IngressGateway.Secret.Namespace != osmNamespace {
		logger.Info("updating IngressGateway client cert secret namespace")
		dirty = true
		conf.Spec.Certificate.IngressGateway.Secret.Namespace = osmNamespace
	}
	if conf.Spec.Certificate.IngressGateway.ValidityDuration != osmClientCertValidity {
		logger.Info("updating IngressGateway client cert validity duration")
		dirty = true
		conf.Spec.Certificate.IngressGateway.ValidityDuration = osmClientCertValidity
	}
	if l := len(conf.Spec.Certificate.IngressGateway.SubjectAltNames); l != 1 ||
		(l == 1 && conf.Spec.Certificate.IngressGateway.SubjectAltNames[0] != osmNginxSAN) {
		logger.Info("updating IngressGateway SAN")
		dirty = true
		conf.Spec.Certificate.IngressGateway.SubjectAltNames = []string{osmNginxSAN}
	}
	if !dirty {
		logger.Info("no changes required for OSM ingress client cert configuration")
		return ctrl.Result{}, nil
	}

	logger.Info("updating OSM ingress mesh config")
	if err = i.client.Update(ctx, conf); client.IgnoreNotFound(err) != nil {
		if apierrors.IsConflict(err) {
			logger.Info("OSM ingress mesh config was updated by another process, retrying")
			return ctrl.Result{Requeue: true}, nil
		}

		logger.Error(err, "failed to update OSM ingress mesh config")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
