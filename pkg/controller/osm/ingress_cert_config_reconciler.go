// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package osm

import (
	"context"

	"github.com/go-logr/logr"
	cfgv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
)

const (
	osmNamespace                    = "kube-system"
	osmMeshConfigName               = "osm-mesh-config"
	osmNginxSAN                     = "ingress-nginx.ingress.cluster.local"
	osmClientCertValidity           = "24h"
	osmClientCertName               = "osm-ingress-client-cert"
	ingressCertConfigControllerName = "ingress_cert_config"
)

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
	return ctrl.
		NewControllerManagedBy(manager).
		For(&cfgv1alpha2.MeshConfig{}).
		Complete(&IngressCertConfigReconciler{client: manager.GetClient()})
}

func (i *IngressCertConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}

	// do metrics
	defer func() {
		//placing this call inside a closure allows for result and err to be bound after Reconcile executes
		//this makes sure they have the proper value
		//just calling defer metrics.HandleControllerReconcileMetrics(ingressCertConfigControllerName, result, err) would bind
		//the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(ingressCertConfigControllerName, result, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return result, err
	}
	logger = logger.WithName("ingressCertConfigReconciler")

	if req.Name != osmMeshConfigName || req.Namespace != osmNamespace {
		return result, nil
	}

	conf := &cfgv1alpha2.MeshConfig{}
	err = i.client.Get(ctx, req.NamespacedName, conf)
	if err != nil {
		return result, client.IgnoreNotFound(err)
	}

	var dirty bool
	if conf.Spec.Certificate.IngressGateway == nil {
		conf.Spec.Certificate.IngressGateway = &cfgv1alpha2.IngressGatewayCertSpec{}
	}
	if conf.Spec.Certificate.IngressGateway.Secret.Name != osmClientCertName {
		dirty = true
		conf.Spec.Certificate.IngressGateway.Secret.Name = osmClientCertName
	}
	if conf.Spec.Certificate.IngressGateway.Secret.Namespace != osmNamespace {
		dirty = true
		conf.Spec.Certificate.IngressGateway.Secret.Namespace = osmNamespace
	}
	if conf.Spec.Certificate.IngressGateway.ValidityDuration != osmClientCertValidity {
		dirty = true
		conf.Spec.Certificate.IngressGateway.ValidityDuration = osmClientCertValidity
	}
	if l := len(conf.Spec.Certificate.IngressGateway.SubjectAltNames); l != 1 ||
		(l == 1 && conf.Spec.Certificate.IngressGateway.SubjectAltNames[0] != osmNginxSAN) {
		dirty = true
		conf.Spec.Certificate.IngressGateway.SubjectAltNames = []string{osmNginxSAN}
	}
	if !dirty {
		return result, nil
	}

	logger.Info("updating OSM ingress client cert configuration")
	err = i.client.Update(ctx, conf)
	return result, err
}
