// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package ingress

import (
	"context"
	"math/rand"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

const reconcileInterval = time.Minute * 2

// IngressControllerReconciler manages resources required to run the ingress controller.
type IngressControllerReconciler struct {
	client                  client.Client
	logger                  logr.Logger
	resources               []client.Object
	interval, retryInterval time.Duration
}

func NewIngressControllerReconciler(manager ctrl.Manager, resources []client.Object) error {
	return manager.Add(&IngressControllerReconciler{
		client:        manager.GetClient(),
		logger:        manager.GetLogger().WithName("ingressControllerReconciler"),
		resources:     resources,
		interval:      reconcileInterval,
		retryInterval: time.Second,
	})
}

func (i *IngressControllerReconciler) Start(ctx context.Context) error {
	interval := time.Nanosecond // run immediately when starting up
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jitter(interval, 0.3)):
		}
		if err := i.tick(ctx); err != nil {
			i.logger.Error(err, "error reconciling ingress controller resources")
			interval = i.retryInterval
			continue
		}
		interval = i.interval
	}
}

func (i *IngressControllerReconciler) tick(ctx context.Context) error {
	start := time.Now()
	i.logger.Info("starting to reconcile ingress controller resources")
	defer func() {
		i.logger.Info("finished reconciling ingress controller resources", "latencySec", time.Since(start).Seconds())
	}()

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

func jitter(base time.Duration, ratio float64) time.Duration {
	if ratio >= 1 || ratio == 0 {
		return base
	}
	jitter := (rand.Float64() * float64(base) * ratio) - (float64(base) * (ratio / 2))
	return base + time.Duration(jitter)
}
