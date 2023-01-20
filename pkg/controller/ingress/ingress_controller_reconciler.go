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
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	netv1informer "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const reconcileInterval = time.Minute * 2

// IngressControllerReconciler manages resources required to run the ingress controller.
// It provisions or deletes resources based on need.
type IngressControllerReconciler struct {
	client                  client.Client
	logger                  logr.Logger
	resources               []client.Object
	interval, retryInterval time.Duration
	className               string
	ingInformer             netv1informer.IngressInformer
	provisionCh             <-chan struct{}
}

func NewIngressControllerReconciler(manager ctrl.Manager, resources []client.Object, className string, ingInformer netv1informer.IngressInformer) error {
	provisionCh := make(chan struct{}, 1)
	icr := &IngressControllerReconciler{
		client:        manager.GetClient(),
		logger:        manager.GetLogger().WithName("ingressControllerReconciler"),
		resources:     resources,
		interval:      reconcileInterval,
		retryInterval: time.Second,
		className:     className,
		ingInformer:   ingInformer,
		provisionCh:   provisionCh,
	}

	triggerProvision := func() {
		// does this work flawlessly or do I need to wrap it as an argument?? check effective go book
		if len(provisionCh) != cap(provisionCh) {
			provisionCh <- struct{}{}
		}
	}
	ingInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ interface{}) {
			triggerProvision()
		},
		UpdateFunc: func(_, _ interface{}) {
			if len(provisionCh) != cap(provisionCh) {
				triggerProvision()
			}
		},
	})

	return manager.Add(icr)
}

func (i *IngressControllerReconciler) Start(ctx context.Context) error {
	if !cache.WaitForCacheSync(ctx.Done(), i.ingInformer.Informer().HasSynced) {
		// should we return error here or what's the right way to retry?
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
	if !shouldUpsert {
		return nil
	}

	i.logger.Info("upserting resources")
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

func (i *IngressControllerReconciler) shouldUpsert() (bool, error) {
	objs, err := i.ingInformer.Informer().GetIndexer().ByIndex(informer.IngressClassNameIndex, i.className)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	ings := make([]*netv1.Ingress, 0, len(objs))
	for _, obj := range objs {
		ing, ok := obj.(*netv1.Ingress)
		if !ok {
			return false, errors.New("failed to convert to Ingress type")
		}
		ings = append(ings, ing)
	}

	for _, ing := range ings {
		if ing.GetDeletionTimestamp() == nil {
			return true, nil
		}
	}

	return false, nil
}
