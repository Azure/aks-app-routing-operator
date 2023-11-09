// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package osm

import (
	"context"

	"github.com/go-logr/logr"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

var (
	ingressBackendControllerName = controllername.New("osm", "ingress", "backend")
)

// IngressControllerSourceSpecer returns the IngressSourceSpec an Ingress consumes and a boolean indicating whether it's managed by web app routing.
// If an ingress is not managed by web app routing, the specer will return false and isn't guaranteed to return the IngressSourceSpec
type IngressControllerSourceSpecer interface {
	IngressSourceSpec(ing *netv1.Ingress) (policyv1alpha1.IngressSourceSpec, bool, error)
}

type ingressControllerSourceSpecer struct {
	ingressSourceSpecFn func(ing *netv1.Ingress) (policyv1alpha1.IngressSourceSpec, bool, error)
}

// NewIngressControllerNamer returns an IngressControllerSourceSpecer using a map from ingress class names to controller names
func NewIngressControllerSourceSpecerFromFn(fn func(ing *netv1.Ingress) (policyv1alpha1.IngressSourceSpec, bool, error)) IngressControllerSourceSpecer {
	return &ingressControllerSourceSpecer{ingressSourceSpecFn: fn}
}

func (i *ingressControllerSourceSpecer) IngressSourceSpec(ing *netv1.Ingress) (policyv1alpha1.IngressSourceSpec, bool, error) {
	return i.ingressSourceSpecFn(ing)
}

// IngressBackendReconciler creates an Open Service Mesh IngressBackend for every ingress resource with "kubernetes.azure.com/use-osm-mtls=true".
// This allows nginx to use mTLS provided by OSM when contacting upstreams.
type IngressBackendReconciler struct {
	client                        client.Client
	events                        record.EventRecorder
	config                        *config.Config
	ingressControllerSourceSpecer IngressControllerSourceSpecer
}

func NewIngressBackendReconciler(manager ctrl.Manager, conf *config.Config, ingressControllerNamer IngressControllerSourceSpecer) error {
	metrics.InitControllerMetrics(ingressBackendControllerName)
	if conf.DisableOSM {
		return nil
	}
	return ingressBackendControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&netv1.Ingress{}),
		manager.GetLogger(),
	).Complete(&IngressBackendReconciler{
		client:                        manager.GetClient(),
		config:                        conf,
		ingressControllerSourceSpecer: ingressControllerNamer,
		events:                        manager.GetEventRecorderFor("aks-app-routing-operator"),
	})
}

func (i *IngressBackendReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}

	// do metrics
	defer func() {
		//placing this call inside a closure allows for result and err to be bound after Reconcile executes
		//this makes sure they have the proper value
		//just calling defer metrics.HandleControllerReconcileMetrics(ingressCertConfigControllerName, result, err) would bind
		//the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(ingressBackendControllerName, result, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return result, err
	}
	logger = ingressBackendControllerName.AddToLogger(logger)

	logger.Info("getting Ingress", "name", req.Name, "namespace", req.Namespace)
	ing := &netv1.Ingress{}
	err = i.client.Get(ctx, req.NamespacedName, ing)
	if err != nil {
		return result, client.IgnoreNotFound(err)
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
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: ing.APIVersion,
				Controller: util.BoolPtr(true),
				Kind:       ing.Kind,
				Name:       ing.Name,
				UID:        ing.UID,
			}},
		},
	}
	logger = logger.WithValues("ingressBackend", backend.Name)

	sourceSpec, ok, err := i.ingressControllerSourceSpecer.IngressSourceSpec(ing)
	if err != nil {
		logger.Error(err, "failed to get ingress source spec")
		return result, err
	}

	backend.Spec = policyv1alpha1.IngressBackendSpec{
		Backends: []policyv1alpha1.BackendSpec{},
		Sources: []policyv1alpha1.IngressSourceSpec{
			sourceSpec,
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

	if ing.Annotations == nil || ing.Annotations["kubernetes.azure.com/use-osm-mtls"] == "" || !ok {
		logger.Info("Ingress does not have osm mtls annotation, cleaning up managed IngressBackend")

		logger.Info("getting IngressBackend")
		toCleanBackend := &policyv1alpha1.IngressBackend{}
		err = i.client.Get(ctx, client.ObjectKeyFromObject(backend), toCleanBackend)
		if err != nil {
			return result, client.IgnoreNotFound(err)
		}

		if manifests.HasTopLevelLabels(toCleanBackend.Labels) {
			logger.Info("deleting IngressBackend")
			err = i.client.Delete(ctx, toCleanBackend)
			return result, client.IgnoreNotFound(err)
		}
		return result, nil
	}

	logger.Info("reconciling OSM ingress backend for ingress")
	err = util.Upsert(ctx, i.client, backend)
	if err != nil {
		i.events.Eventf(ing, "Warning", "FailedUpdateOrCreateIngressBackend", "error while creating or updating IngressBackend needed for OSM integration: %s", err)
	}

	return result, err
}
