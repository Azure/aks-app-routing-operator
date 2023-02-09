package ingress

import (
	"context"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ingressClassReconciler struct {
	client                  client.Client
	logger                  logr.Logger
	interval, retryInterval time.Duration
	resources               []client.Object
}

// NewIngressClassReconciler creates a runnable that manages ingress class resources
func NewIngressClassReconciler(manager ctrl.Manager, resources []client.Object) error {
	icr := &ingressClassReconciler{
		client:        manager.GetClient(),
		logger:        manager.GetLogger().WithName("ingressClassReconciler"),
		interval:      reconcileInterval,
		retryInterval: time.Second,
		resources:     resources,
	}

	return manager.Add(icr)
}

func (i *ingressClassReconciler) Start(ctx context.Context) error {
	interval := time.Nanosecond // run immediately when starting up
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(util.Jitter(interval, 0.3)):
		}

		if err := i.tick(ctx); err != nil {
			i.logger.Error(err, "reconciling ingress class resources")
			interval = i.retryInterval
			continue
		}

		interval = i.interval
	}
}

func (i *ingressClassReconciler) tick(ctx context.Context) error {
	start := time.Now()
	i.logger.Info("starting to reconcile ingress class resources")
	defer func() {
		i.logger.Info("finished reconciling ingress class resources", "latencySec", time.Since(start))
	}()

	for _, res := range i.resources {
		copy := res.DeepCopyObject().(client.Object)
		if copy.GetDeletionTimestamp() != nil {
			if err := i.client.Delete(ctx, copy); !k8serrors.IsNotFound(err) {
				i.logger.Error(err, "deleting unneeded resources")
			}
			continue
		}

		if err := util.Upsert(ctx, i.client, copy); err != nil {
			return err
		}
	}

	return nil
}
