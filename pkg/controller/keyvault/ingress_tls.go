// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
)

// IngressTLSReconciler updates ingress resources that reference Keyvault secrets such that they
// use the referenced cert (and only that cert) for TLS termination of all configured hosts.
type IngressTLSReconciler struct {
	client client.Client
}

func NewIngressTLSReconciler(manager ctrl.Manager, conf *config.Config) error {
	if conf.DisableKeyvault {
		return nil
	}
	return ctrl.
		NewControllerManagedBy(manager).
		For(&netv1.Ingress{}).
		Complete(&IngressTLSReconciler{client: manager.GetClient()})
}

func (i *IngressTLSReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = logger.WithName("tlsReconciler")

	ing := &netv1.Ingress{}
	err = i.client.Get(ctx, req.NamespacedName, ing)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = logger.WithValues("name", ing.Name, "namespace", ing.Namespace, "generation", ing.Generation)

	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != manifests.IngressClass || ing.Annotations == nil {
		// Don't remove the existing TLS rules if annotations/class are removed from ingress
		return ctrl.Result{}, nil
	}

	tlsRule := netv1.IngressTLS{SecretName: fmt.Sprintf("keyvault-%s", ing.Name)}
	for _, cur := range ing.Spec.Rules {
		tlsRule.Hosts = append(tlsRule.Hosts, cur.Host)
	}

	// If user specifies a Keyvault cert, all hosts in the ingress must use it.
	// So we remove any TLS configurations except for the one we manage.
	expected := []netv1.IngressTLS{tlsRule}
	if reflect.DeepEqual(ing.Spec.TLS, expected) {
		return ctrl.Result{}, nil
	}
	ing.Spec.TLS = expected

	logger.Info("updating ingress TLS rules")
	return ctrl.Result{}, i.client.Update(ctx, ing)
}
