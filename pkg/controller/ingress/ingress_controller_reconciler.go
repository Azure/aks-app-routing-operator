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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const reconcileInterval = time.Minute * 2

// IngressControllerReconciler manages resources required to run the ingress controller.
type IngressControllerReconciler struct {
	client                  client.Client
	logger                  logr.Logger
	resources               []client.Object
	interval, retryInterval time.Duration
	controller              string
	ingClassInformer        informer.IngressClass
	provisionCh             <-chan struct{}
}

func NewIngressControllerReconciler(manager ctrl.Manager, resources []client.Object, controller string, ingClassInformer informer.IngressClass) error {
	provisionCh := make(chan struct{}, 1)
	icr := &IngressControllerReconciler{
		client:           manager.GetClient(),
		logger:           manager.GetLogger().WithName("ingressControllerReconciler"),
		resources:        resources,
		interval:         reconcileInterval,
		retryInterval:    time.Second,
		controller:       controller,
		ingClassInformer: ingClassInformer,
		provisionCh:      provisionCh,
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

	return i.provision(ctx)
}

func (i *IngressControllerReconciler) provision(ctx context.Context) error {
	shouldUpsert, err := i.shouldUpsert()
	if err != nil {
		return err
	}

	i.logger.Info("upserting resources")
	for _, res := range i.resources {
		copy := res.DeepCopyObject().(client.Object)
		if copy.GetDeletionTimestamp() != nil {
			if err := i.client.Delete(ctx, copy); !k8serrors.IsNotFound(err) {
				i.logger.Error(err, "deleting unneeded resources")
			}
			continue
		}

		if !shouldUpsert {
			continue
		}

		if err := util.Upsert(ctx, i.client, copy); err != nil {
			return err
		}
	}
	return nil
}

func (i *IngressControllerReconciler) shouldUpsert() (bool, error) {
	if i.ingClassInformer == nil {
		return false, errors.New("ingressClassInformer is nil")
	}

	ingCs, err := i.ingClassInformer.ByController(i.controller)
	if err != nil {
		return false, err
	}

	for _, ingC := range ingCs {
		if ingC.GetDeletionTimestamp() == nil {
			return true, nil
		}
	}

	return false, nil
}
