package common

import (
	"context"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type resourceReconciler struct {
	name                    string
	client                  client.Client
	logger                  logr.Logger
	interval, retryInterval time.Duration
	resources               []client.Object
}

// NewResourceReconciler creates a reconciler that continuously ensures that the provided resources are provisioned
func NewResourceReconciler(manager ctrl.Manager, name string, resources []client.Object, reconcileInterval time.Duration) error {
	rr := &resourceReconciler{
		name:          name,
		client:        manager.GetClient(),
		logger:        manager.GetLogger().WithName(name),
		interval:      reconcileInterval,
		retryInterval: time.Second,
		resources:     resources,
	}
	return manager.Add(rr)
}

func (r *resourceReconciler) Start(ctx context.Context) error {
	r.logger.Info("starting resource reconciler")
	defer r.logger.Info("stopping resource reconciler")

	interval := time.Nanosecond // run immediately when starting up
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(util.Jitter(interval, 0.3)):
		}

		if err := r.tick(ctx); err != nil {
			r.logger.Error(err, "reconciling resources")
			interval = r.retryInterval
			continue
		}

		interval = r.interval
	}
}

func (r *resourceReconciler) tick(ctx context.Context) error {
	start := time.Now()
	r.logger.Info("starting to reconcile resources")
	defer func() {
		r.logger.Info("finished reconciling resources", "latencySec", time.Since(start).Seconds())
	}()

	for _, res := range r.resources {
		copy := res.DeepCopyObject().(client.Object)
		if copy.GetDeletionTimestamp() != nil {
			if err := r.client.Delete(ctx, copy); err != nil && !k8serrors.IsNotFound(err) {
				r.logger.Error(err, "deleting unneeded resources")
			}
			continue
		}

		if err := util.Upsert(ctx, r.client, copy); err != nil {
			return err
		}
	}

	return nil
}

func (r *resourceReconciler) NeedLeaderElection() bool {
	return true
}
