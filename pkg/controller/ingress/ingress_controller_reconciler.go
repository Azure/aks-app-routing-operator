// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package ingress

import (
	"context"
	"errors"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/informer"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const reconcileInterval = time.Minute * 2

// ProvisionFn defines the type of a function that will be in charge of deploying kubernetes resources required for the Ingress
type ProvisionFn func(ctx context.Context, client client.Client) error

// IngressControllerReconciler manages resources required to run the ingress controller.
type IngressControllerReconciler struct {
	client                  client.Client
	logger                  logr.Logger
	ingClassInformer        informer.IngressClass
	interval, retryInterval time.Duration
	provisionCh             <-chan struct{}
	provisionFn             ProvisionFn
}

func NewIngressControllerReconciler(manager ctrl.Manager, ingClassInformer informer.IngressClass, provisionFn ProvisionFn) error {
	provisionCh := make(chan struct{}, 1)
	icr := &IngressControllerReconciler{
		client:           manager.GetClient(),
		logger:           manager.GetLogger().WithName("ingressControllerReconciler"),
		interval:         reconcileInterval,
		retryInterval:    time.Second,
		ingClassInformer: ingClassInformer,
		provisionCh:      provisionCh,
		provisionFn:      provisionFn,
	}

	triggerProvision := func() {
		if len(provisionCh) != cap(provisionCh) {
			provisionCh <- struct{}{}
		}
	}
	ingClassInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ interface{}) {
			triggerProvision()
		},
		UpdateFunc: func(_, _ interface{}) {
			triggerProvision()
		},
	})

	return manager.Add(icr)
}

func (i *IngressControllerReconciler) Start(ctx context.Context) error {
	i.logger.Info("waiting for cache to sync")
	if !cache.WaitForCacheSync(ctx.Done(), i.ingClassInformer.Informer().HasSynced) {
		return errors.New("failed to sync cache")
	}

	interval := time.Nanosecond // run immediately when starting up
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-i.provisionCh:
		case <-time.After(util.Jitter(interval, 0.3)):
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

	ctx = logr.NewContext(ctx, i.logger)
	return i.provisionFn(ctx, i.client)
}
