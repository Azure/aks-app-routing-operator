package suites

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func waitForAvailable(ctx context.Context, c client.Client, deployment appsv1.Deployment) error {
	lgr := logger.FromContext(ctx).With("deployment", deployment.Name, "namespace", deployment.Namespace)
	lgr.Info("waiting for deployment to be available")
	start := time.Now()
	for {
		d := &deployment
		if err := c.Get(ctx, client.ObjectKeyFromObject(d), d); err != nil {
			return fmt.Errorf("getting deployment: %w", err)
		}

		for _, condition := range d.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == "True" {
				lgr.Info("deployment is available")
				return nil
			}
		}

		if time.Since(start) > 10*time.Minute { // 10 minutes because it takes a decent amount of time for dns to "propagate"
			return fmt.Errorf("timed out waiting for deployment to be available")
		}

		lgr.Info("deployment is not available yet, waiting 5 seconds for retry")
		time.Sleep(5 * time.Second)
	}
}

func upsert(ctx context.Context, c client.Client, obj client.Object) error {
	copy := obj.DeepCopyObject().(client.Object)
	lgr := logger.FromContext(ctx).With("object", copy.GetName(), "namespace", copy.GetNamespace())
	lgr.Info("upserting object")

	// create or update the object
	lgr.Info("attempting to create object")
	err := c.Create(ctx, copy)
	if err == nil {
		obj.SetName(copy.GetName()) // supports objects that want to use generate name
		lgr.Info("object created")
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating object: %w", err)
	}

	lgr.Info("object already exists, attempting to update")
	if err := c.Update(ctx, copy); err != nil {
		return fmt.Errorf("updating object: %w", err)
	}

	lgr.Info("object updated")
	return nil
}
