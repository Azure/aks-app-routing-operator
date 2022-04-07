// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package osm

import (
	"context"

	"github.com/go-logr/logr"
	cfgv1alpha1 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
)

const (
	osmNamespace          = "kube-system"
	osmMeshConfigName     = "osm-mesh-config"
	osmNginxSAN           = "ingress-nginx.ingress.cluster.local"
	osmClientCertValidity = "24h"
	osmClientCertName     = "osm-ingress-client-cert"
)

// IngressCertConfigReconciler updates the Open Service Mesh configuration to generate a client cert
// to be used by the ingress controller when contacting upstreams.
type IngressCertConfigReconciler struct {
	client client.Client
}

func NewIngressCertConfigReconciler(manager ctrl.Manager, conf *config.Config) error {
	if conf.DisableOSM {
		return nil
	}
	return ctrl.
		NewControllerManagedBy(manager).
		For(&cfgv1alpha1.MeshConfig{}).
		Complete(&IngressCertConfigReconciler{client: manager.GetClient()})
}

func (i *IngressCertConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = logger.WithName("ingressCertConfigReconciler")

	if req.Name != osmMeshConfigName || req.Namespace != osmNamespace {
		return ctrl.Result{}, nil
	}

	conf := &cfgv1alpha1.MeshConfig{}
	err = i.client.Get(ctx, req.NamespacedName, conf)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	var dirty bool
	if conf.Spec.Certificate.IngressGateway == nil {
		conf.Spec.Certificate.IngressGateway = &cfgv1alpha1.IngressGatewayCertSpec{}
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
	if len(conf.Spec.Certificate.IngressGateway.SubjectAltNames) < 1 || conf.Spec.Certificate.IngressGateway.SubjectAltNames[0] != osmNginxSAN {
		dirty = true
		conf.Spec.Certificate.IngressGateway.SubjectAltNames = []string{osmNginxSAN}
	}
	if !dirty {
		return ctrl.Result{}, nil
	}

	logger.Info("updating OSM ingress client cert configuration")
	return ctrl.Result{}, i.client.Update(ctx, conf)
}
