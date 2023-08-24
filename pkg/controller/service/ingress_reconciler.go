// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package service

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

var (
	ingressControllerName = controllername.New("service", "ingress", "reconciler")
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
	client    client.Client
	ingConfig *manifests.NginxIngressConfig
}

func NewNginxIngressReconciler(manager ctrl.Manager, ingConfig *manifests.NginxIngressConfig) error {
	metrics.InitControllerMetrics(ingressControllerName)

	return ctrl.
		NewControllerManagedBy(manager).
		For(&corev1.Service{}).
		Complete(&NginxIngressReconciler{client: manager.GetClient(), ingConfig: ingConfig})
}

func (i *NginxIngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}

	// do metrics
	defer func() {
		//placing this call inside a closure allows for result and err to be bound after Reconcile executes
		//this makes sure they have the proper value
		//just calling defer metrics.HandleControllerReconcileMetrics(controllerName, result, err) would bind
		//the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(ingressControllerName, result, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return result, err
	}
	logger = ingressControllerName.AddToLogger(logger)

	svc := &corev1.Service{}
	err = i.client.Get(ctx, req.NamespacedName, svc)
	if errors.IsNotFound(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	logger = logger.WithValues("name", svc.Name, "namespace", svc.Namespace, "generation", svc.Generation)

	if svc.Annotations == nil || svc.Annotations["kubernetes.azure.com/ingress-host"] == "" || svc.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"] == "" || len(svc.Spec.Ports) == 0 {
		// Give users a migration path away from managed ingress, etc. resources by not cleaning them up if annotations are removed.
		// Users can remove the annotations, remove the owner references from managed resources, and take ownership of them.
		return result, nil
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
			IngressClassName: util.StringPtr(i.ingConfig.IcName),
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
	err = util.Upsert(ctx, i.client, ing)
	return result, err
}
