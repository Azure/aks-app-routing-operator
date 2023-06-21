package common

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type cleaner struct {
	name         string
	client       client.Client
	clientset    kubernetes.Interface
	dynamic      dynamic.Interface
	logger       logr.Logger
	gvrRetriever gvrRetriever // gets the types of resources that will be cleaned
	labels       labels.Set   // labels that the cleanup objects are required to have
	maxRetries   int
}

type gvrRetriever func(client client.Client) ([]schema.GroupVersionResource, error) // we use a getter function because manager client isn't usable until manager starts

func gvrs(client client.Client, obj client.Object) ([]schema.GroupVersionResource, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()

	mappings, err := client.RESTMapper().RESTMappings(gvk.GroupKind()) // retrieve all mappings because versions might be auto updated (by conversion webhooks)
	if err != nil {
		return nil, fmt.Errorf("getting rest mappings for %s: %w", gvk.String(), err)
	}

	var gvrs []schema.GroupVersionResource
	for _, mapping := range mappings {
		gvrs = append(gvrs, mapping.Resource)
	}

	return gvrs, nil
}

// GvrRetrieverFromObjs retrieves a list of group version resources based on supplied object types
func GvrRetrieverFromObjs(objs []client.Object) gvrRetriever {
	return func(client client.Client) ([]schema.GroupVersionResource, error) {
		var ret []schema.GroupVersionResource
		for _, obj := range objs {
			gvrs, err := gvrs(client, obj)
			if err != nil {
				return nil, err
			}

			ret = append(ret, gvrs...)
		}

		return ret, nil
	}
}

// RemoveGk removes group kinds from retrieved types
func (g gvrRetriever) RemoveGk(gks ...schema.GroupKind) gvrRetriever {
	return func(client client.Client) ([]schema.GroupVersionResource, error) {
		gvrs, err := g(client)
		if err != nil {
			return nil, err
		}

		var filters []schema.GroupVersionResource
		for _, gk := range gks {
			mappings, err := client.RESTMapper().RESTMappings(gk)
			if err != nil {
				return nil, fmt.Errorf("getting rest mappings for %s: %w", gk.String(), err)
			}

			for _, mapping := range mappings {
				filters = append(filters, mapping.Resource)
			}
		}

		var ok []schema.GroupVersionResource
		for _, gvr := range gvrs {
			filtered := false
			for _, filter := range filters {
				if reflect.DeepEqual(filter, gvr) {
					filtered = true
					break
				}
			}

			if !filtered {
				ok = append(ok, gvr)
			}
		}

		return ok, nil
	}
}

// NewCleaner creates a cleaner that attempts to delete resources with the labels specified and of the types returned by gvrRetriever
func NewCleaner(manager ctrl.Manager, name string, gvrRetriever gvrRetriever, lbs map[string]string) error {
	// TODO: we should use the manager client for caching purposes if possible?
	d, err := dynamic.NewForConfig(manager.GetConfig())
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	cs, err := kubernetes.NewForConfig(manager.GetConfig())
	if err != nil {
		return fmt.Errorf("creating clientset: %w", err)
	}

	c := &cleaner{
		name:         name,
		client:       manager.GetClient(),
		dynamic:      d,
		logger:       manager.GetLogger().WithName(name),
		clientset:    cs,
		gvrRetriever: gvrRetriever,
		labels:       labels.Set(lbs),
		maxRetries:   2,
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
	if c.gvrRetriever == nil {
		return errors.New("gvrRetriever is nil")
	}

	types, err := c.gvrRetriever(c.client)
	if err != nil {
		return fmt.Errorf("retrieving gvr types: %w", err)
	}

	for _, t := range types {
		selector, err := c.labels.AsValidatedSelector()
		if err != nil {
			return fmt.Errorf("validating label selector: %w", err)
		}

		listOpt := metav1.ListOptions{
			LabelSelector: selector.String(),
		}

		// does this work? delete collection doesn't work on all types
		// need to detect if it can delete collection

		// TODO: don't want to delete namespaces
		c.logger.Info("cleaning type", "type", t.String(), "labels", c.labels.AsSelectorPreValidated().String())

		dclient := c.dynamic.Resource(t)
		err = dclient.DeleteCollection(ctx, metav1.DeleteOptions{}, listOpt)
		if err == nil {
			continue
		}
		if !k8serrors.IsMethodNotSupported(err) {
			return fmt.Errorf("deleting collection %s", t.String())
		}

		// delete collection is not supported for some types.
		// instead we list then delete one by one
		list, err := dclient.List(ctx, listOpt)
		if err != nil {
			return fmt.Errorf("listing %s", t.String())
		}

		if err := list.EachListItem(func(obj runtime.Object) error {
			o, err := meta.Accessor(obj)
			if err != nil {
				return fmt.Errorf("accessing object metadata: %w", err)
			}

			// what if it's not namespaceable?
			res, err := c.clientset.Discovery().ServerResourcesForGroupVersion(t.String())
			if err != nil {
				return fmt.Errorf("getting server resources for group version")
			}

			namespaced := false
			for _, r := range res.APIResources {
				if r.Name == t.Resource {
					namespaced = r.Namespaced
					break
				}
			}

			var nsClient dynamic.ResourceInterface = dclient
			if namespaced {
				nsClient = dclient.Namespace(o.GetNamespace())
			}

			err = nsClient.Delete(ctx, o.GetName(), metav1.DeleteOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				return fmt.Errorf("deleting object %s in %s: %w", o.GetName(), o.GetNamespace(), err)
			}

			return nil
		}); err != nil {
			return fmt.Errorf("deleting each object: %w", err)
		}
	}

	return nil
}