// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package ingress

import (
	"context"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

const reconcileInterval = time.Minute * 5

// IngressControllerReconciler manages resources required to run the ingress controller.
type IngressControllerReconciler struct {
	client                  client.Client
	logger                  logr.Logger
	resources               []client.Object
	interval, retryInterval time.Duration
}

func NewIngressControllerReconciler(manager ctrl.Manager, resources []client.Object) error {
	icr := &IngressControllerReconciler{
		client:        manager.GetClient(),
		logger:        manager.GetLogger().WithName("ingressControllerReconciler"),
		resources:     resources,
		interval:      reconcileInterval,
		retryInterval: time.Second,
	}

	if err := manager.Add(icr); err != nil {
		return err
	}

	if err := ctrl.NewControllerManagedBy(manager).For(&netv1.Ingress{}).Complete(icr); err != nil {
		return err
	}

	return nil
}

func (i *IngressControllerReconciler) Start(ctx context.Context) error {
	start := time.Now()
	i.logger.Info("starting to reconcile ingress controller resources")
	defer func() {
		i.logger.Info("finished reconciling ingress controller resources", "latencySec", time.Since(start).Seconds())
	}()

	needed, err := i.resourcesNeeded(ctx)
	if err != nil {
		return err
	}
	if !needed {
		i.logger.Info("deleting unneeded ingress controller resources")
		return i.delete(ctx)
	}

	i.logger.Info("upserting ingress controller resources")
	return i.upsert(ctx)
}

func (i *IngressControllerReconciler) upsert(ctx context.Context) error {
	for _, res := range i.resources {
		copy := res.DeepCopyObject().(client.Object)
		if copy.GetDeletionTimestamp() != nil {
			if err := i.client.Delete(ctx, copy); err != nil {
				return err
			}
			continue
		}
		if err := util.Upsert(ctx, i.client, copy); err != nil {
			return err
		}
	}
	return nil
}

func (i *IngressControllerReconciler) resourcesNeeded(ctx context.Context) (bool, error) {
	list := &netv1.IngressList{}
	if err := i.client.List(ctx, list); err != nil {
		return false, err
	}

	set := make(map[string]struct{})
	for _, i := range list.Items {
		if i.GetDeletionTimestamp() != nil {
			continue
		}

		class := i.Spec.IngressClassName
		if class != nil {
			set[*class] = struct{}{}
		}
	}

	if _, ok := set[manifests.IngressClass]; !ok {
		return false, nil
	}

	return true, nil
}

func (i *IngressControllerReconciler) delete(ctx context.Context) error {
	var result error
	for _, res := range i.resources {
		copy := res.DeepCopyObject().(client.Object)
		if err := i.client.Delete(ctx, copy); err != nil {
			if !errors.IsNotFound(err) {
				result = multierror.Append(result, err)
			}
		}
	}

	return result
}

// Reconcile is called when an Ingress is created, updated, or deleted and reconciles IngressController resources
// based on whether any Ingresses would use them
func (i *IngressControllerReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := i.logger.WithValues("name", req.Name, "namespace", req.Namespace)
	logger.Info("starting to reconcile ingress controller resources from ingress event")
	defer func() {
		logger.Info("finished reconciling ingress controller resources from ingress event", "latencySec", time.Since(start).Seconds())
	}()

	ing := &netv1.Ingress{}
	err := i.client.Get(ctx, req.NamespacedName, ing)
	if !errors.IsNotFound(err) && err != nil { // we should ignore not found errors because it means the ingress event is deletion and was deleted
		return ctrl.Result{}, err
	}
	if err == nil && *ing.Spec.IngressClassName == manifests.IngressClass && ing.GetDeletionTimestamp() != nil {
		logger.Info("upserting ingress controller resources")
		if err := i.upsert(ctx); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	needed, err := i.resourcesNeeded(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if needed {
		logger.Info("upserting ingress controller resources")
		if err := i.upsert(ctx); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	i.logger.Info("deleting unneeded ingress controller resources")
	if err := i.delete(ctx); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
