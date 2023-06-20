package common

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
)

type cleaner struct {
	name       string
	dynamic    dynamic.Interface
	logger     logr.Logger
	types      []schema.GroupVersionResource // types of resources that will be cleaned
	labels     labels.Set                    // labels that the cleanup objects are required to have
	maxRetries int
}

// this should just be a generic thing that's passed information from a queue?

func NewCleaner(manager ctrl.Manager, name string, types []schema.GroupVersionResource, lbs map[string]string) error {
	// TODO: we should use the manager client for caching purposes if possible?
	d, err := dynamic.NewForConfig(manager.GetConfig())
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	c := &cleaner{
		name:       name,
		dynamic:    d,
		logger:     manager.GetLogger().WithName(name),
		types:      types,
		labels:     labels.Set(lbs),
		maxRetries: 2,
	}
	return manager.Add(c)
}

func (c *cleaner) Start(ctx context.Context) error {
	start := time.Now()
	c.logger.Info("starting to clean resources")
	defer func() {
		c.logger.Info("finished cleaning resources", "latencySec", time.Since(start).Seconds())
	}()

	for i := 0; i <= c.maxRetries; i++ {
		err := c.Clean(ctx)
		if err == nil {
			return nil
		}

		c.logger.Error(err, "failed to clean resources", "try", i, "maxTries", c.maxRetries)
		if i == c.maxRetries {
			break // failing to clean up unused resources shouldn't crash operator
		}

		timeout := time.Duration(int(math.Pow(2, float64(i)))) * time.Second
		c.logger.Info("sleeping", "time", timeout)
		time.Sleep(timeout)
	}

	return nil
}

func (c *cleaner) Clean(ctx context.Context) error {
	for _, t := range c.types {
		selector, err := c.labels.AsValidatedSelector()
		if err != nil {
			return fmt.Errorf("validating label selector: %w", err)
		}

		listOpt := metav1.ListOptions{
			LabelSelector: selector.String(),
		}

		// does this work? delete collection doesn't work on all types
		// need to detect if it can delete collection

		// don't want to delete namespaces

		client := c.dynamic.Resource(t)
		err = client.DeleteCollection(ctx, metav1.DeleteOptions{}, listOpt)
		if err == nil {
			continue
		}
		if !errors.IsMethodNotSupported(err) {
			return fmt.Errorf("deleting collection %s", t.String())
		}

		// delete collection is not supported for some types like services
		// so we list then delete one by one
		list, err := client.List(ctx, listOpt)
		if err != nil {
			return fmt.Errorf("listing %s", t.String())
		}

		if err := list.EachListItem(func(obj runtime.Object) error {
			o, err := meta.Accessor(obj)
			if err != nil {
				return fmt.Errorf("accessing object metadata: %w", err)
			}

			// what if it's not namespaceable?
			err = client.Namespace(o.GetNamespace()).Delete(ctx, o.GetName(), metav1.DeleteOptions{})
			if err != nil {
				return fmt.Errorf("deleting object %s in %s: %w", o.GetName(), o.GetNamespace(), err)
			}

			return nil
		}); err != nil {
			return fmt.Errorf("deleting each object: %w", err)
		}
	}

	return nil
}
