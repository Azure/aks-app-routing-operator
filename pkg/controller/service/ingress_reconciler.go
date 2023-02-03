// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package service

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

// NginxIngressReconciler manages an opinionated ingress resource for services that define certain annotations.
// The resulting ingress uses Keyvault for TLS, never exposes insecure (plain http) routes, and uses OSM for upstream mTLS.
// If those integrations aren't enabled, it won't work correctly.
//
// Annotations:
// - kubernetes.azure.com/ingress-host: host of the ingress resource
// - kubernetes.azure.com/tls-cert-keyvault-uri: URI of the Keyvault certificate to present
// - kubernetes.azure.com/service-account-name: name of the service account used by upstream pods (defaults to "default")
// - kubernetes.azure.com/insecure-disable-osm: don't use OSM integration. Connections between ingreses controller and app will be insecure.
//
// This functionality allows easy adoption of good ingress practices while providing an exit strategy.
// Users can remove the annotations and take ownership of the generated resources at any time.
type NginxIngressReconciler struct {
	client              client.Client
	controller, icName  string
	requiredAnnotations map[string]string
}

func NewNginxIngressReconciler(manager ctrl.Manager, controller, icName string, requiredAnnotations map[string]string) error {
	return ctrl.
		NewControllerManagedBy(manager).
		For(&corev1.Service{}).
		Complete(&NginxIngressReconciler{client: manager.GetClient(), controller: controller, icName: icName, requiredAnnotations: requiredAnnotations})
}

// TODO: decide if we want to remove this functionality completely, it's not currently documented

func (n *NginxIngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = logger.WithName("ingressReconciler").WithValues("ingressClass", n.icName, "controller", n.controller)

	svc := &corev1.Service{}
	err = n.client.Get(ctx, req.NamespacedName, svc)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = logger.WithValues("name", svc.Name, "namespace", svc.Namespace, "generation", svc.Generation)

	if svc.Annotations == nil || svc.Annotations["kubernetes.azure.com/ingress-host"] == "" || svc.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"] == "" || len(svc.Spec.Ports) == 0 {
		// Give users a migration path away from managed ingress, etc. resources by not cleaning them up if annotations are removed.
		// Users can remove the annotations, remove the owner references from managed resources, and take ownership of them.
		return ctrl.Result{}, nil
	}

	for k, v := range n.requiredAnnotations {
		val, ok := svc.Annotations[k]
		if !ok {
			return ctrl.Result{}, nil
		}

		if val != v {
			return ctrl.Result{}, nil
		}
	}

	logger.Info("reconciling ingressClass for service")
	ic := &netv1.IngressClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IngressClass",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: n.icName,
		},
		Spec: netv1.IngressClassSpec{
			Controller: n.controller,
		},
	}
	if err := util.Upsert(ctx, n.client, ic); err != nil {
		return ctrl.Result{}, err
	}

	serviceAccount := "default"
	if sa := svc.Annotations["kubernetes.azure.com/service-account-name"]; sa != "" {
		serviceAccount = sa
	}

	pt := netv1.PathTypePrefix
	ing := &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: svc.APIVersion,
				Controller: util.BoolPtr(true),
				Kind:       svc.Kind,
				Name:       svc.Name,
				UID:        svc.UID,
			}},
			Annotations: map[string]string{
				"kubernetes.azure.com/tls-cert-keyvault-uri": svc.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"],
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: util.StringPtr(ic.Name),
			Rules: []netv1.IngressRule{{
				Host: svc.Annotations["kubernetes.azure.com/ingress-host"],
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: []netv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pt,
							Backend: netv1.IngressBackend{
								Service: &netv1.IngressServiceBackend{
									Name: svc.Name,
									Port: netv1.ServiceBackendPort{Number: svc.Spec.Ports[0].TargetPort.IntVal},
								},
							},
						}},
					},
				},
			}},
			TLS: []netv1.IngressTLS{{
				Hosts:      []string{svc.Annotations["kubernetes.azure.com/ingress-host"]},
				SecretName: fmt.Sprintf("keyvault-%s", svc.Name),
			}},
		},
	}
	if svc.Annotations["kubernetes.azure.com/insecure-disable-osm"] == "" {
		ing.Annotations["kubernetes.azure.com/use-osm-mtls"] = "true"
		ing.Annotations["nginx.ingress.kubernetes.io/backend-protocol"] = "HTTPS"
		ing.Annotations["nginx.ingress.kubernetes.io/configuration-snippet"] = fmt.Sprintf("\nproxy_ssl_name \"%s.%s.cluster.local\";", serviceAccount, svc.Namespace)
		ing.Annotations["nginx.ingress.kubernetes.io/proxy-ssl-secret"] = "kube-system/osm-ingress-client-cert"
		ing.Annotations["nginx.ingress.kubernetes.io/proxy-ssl-verify"] = "on"
	}

	logger.Info("reconciling ingress for service")
	return ctrl.Result{}, util.Upsert(ctx, n.client, ing)
}
