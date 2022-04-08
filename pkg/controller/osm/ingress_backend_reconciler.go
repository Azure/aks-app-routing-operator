// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package osm

import (
	"context"

	"github.com/go-logr/logr"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

// IngressBackendReconciler creates an Open Service Mesh IngressBackend for every ingress resource with "aks.io/use-osm-mtls=true".
// This allows nginx to use mTLS provided by OSM when contacting upstreams.
type IngressBackendReconciler struct {
	client client.Client
	config *config.Config
}

func NewIngressBackendReconciler(manager ctrl.Manager, conf *config.Config) error {
	if conf.DisableOSM {
		return nil
	}
	return ctrl.
		NewControllerManagedBy(manager).
		For(&netv1.Ingress{}).
		Complete(&IngressBackendReconciler{client: manager.GetClient(), config: conf})
}

func (i *IngressBackendReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = logger.WithName("ingressBackendReconciler")

	ing := &netv1.Ingress{}
	err = i.client.Get(ctx, req.NamespacedName, ing)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = logger.WithValues("name", ing.Name, "namespace", ing.Namespace, "generation", ing.Generation)

	backend := &policyv1alpha1.IngressBackend{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IngressBackend",
			APIVersion: "policy.openservicemesh.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ing.Name,
			Namespace: ing.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: ing.APIVersion,
				Controller: util.BoolPtr(true),
				Kind:       ing.Kind,
				Name:       ing.Name,
				UID:        ing.UID,
			}},
		},
	}

	if ing.Annotations == nil || ing.Annotations["aks.io/use-osm-mtls"] == "" {
		err = i.client.Get(ctx, client.ObjectKeyFromObject(backend), backend)
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, i.client.Delete(ctx, backend)
	}

	backend.Spec = policyv1alpha1.IngressBackendSpec{
		Backends: []policyv1alpha1.BackendSpec{},
		Sources: []policyv1alpha1.IngressSourceSpec{
			{
				Kind:      "Service",
				Name:      manifests.IngressControllerName,
				Namespace: i.config.NS,
			},
			{
				Kind: "AuthenticatedPrincipal",
				Name: osmNginxSAN,
			},
		},
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil || path.Backend.Service.Port.Number == 0 {
				continue
			}
			backend.Spec.Backends = append(backend.Spec.Backends, policyv1alpha1.BackendSpec{
				Name: path.Backend.Service.Name,
				TLS:  policyv1alpha1.TLSSpec{SkipClientCertValidation: false},
				Port: policyv1alpha1.PortSpec{
					Number:   int(path.Backend.Service.Port.Number),
					Protocol: "https",
				},
			})
		}
	}

	logger.Info("reconciling OSM ingress backend for ingress")
	return ctrl.Result{}, util.Upsert(ctx, i.client, backend)
}
