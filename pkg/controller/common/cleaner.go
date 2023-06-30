package common

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type cleaner struct {
	name       string
	mapper     meta.RESTMapper
	clientset  kubernetes.Interface
	dynamic    dynamic.Interface
	logger     logr.Logger
	retriever  CleanTypeRetriever // gets the types of resources that will be cleaned
	maxRetries int
}

// NewCleaner creates a cleaner that attempts to delete resources with the labels specified and of the types returned by CleanTypeRetriever
func NewCleaner(manager ctrl.Manager, name string, gvrRetriever CleanTypeRetriever) error {
	d, err := dynamic.NewForConfig(manager.GetConfig())
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	cs, err := kubernetes.NewForConfig(manager.GetConfig())
	if err != nil {
		return fmt.Errorf("creating clientset: %w", err)
	}

	mapper, err := apiutil.NewDynamicRESTMapper(manager.GetConfig(), apiutil.WithLazyDiscovery)
	if err != nil {
		return fmt.Errorf("creating dynamic rest mapper: %w", err)
	}

	c := &cleaner{
		name:       name,
		mapper:     mapper,
		dynamic:    d,
		logger:     manager.GetLogger().WithName(name),
		clientset:  cs,
		retriever:  gvrRetriever,
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
	if c.retriever == nil {
		return errors.New("retriever is nil")
	}

	types, err := c.retriever(c.mapper)
	if err != nil {
		return fmt.Errorf("retrieving gvr types: %w", err)
	}

	var result *multierror.Error
	for _, t := range types {
		if err := c.CleanType(ctx, t); err != nil {
			result = multierror.Append(result, fmt.Errorf("cleaning type %s with labels %s: %w", t.gvr.String(), t.labels, err))
		}
	}

	return result.ErrorOrNil()
}

func (c *cleaner) CleanType(ctx context.Context, t cleanType) error {
	l := labels.Set(t.labels)
	// get an exact match selector

	selector, err := l.AsValidatedSelector()
	if err != nil {
		return fmt.Errorf("validating label selector: %w", err)
	}

	listOpt := metav1.ListOptions{
		LabelSelector: selector.String(),
	}

	c.logger.Info("cleaning type", "type", t.gvr.String(), "selector", selector.String())

	dclient := c.dynamic.Resource(t.gvr)
	err = dclient.DeleteCollection(ctx, metav1.DeleteOptions{}, listOpt)
	if err == nil {
		return nil
	}
	if !k8serrors.IsMethodNotSupported(err) {
		return fmt.Errorf("deleting collection %s", t.gvr.String())
	}

	// delete collection is not supported for some types.
	// instead we list then delete one by one
	list, err := dclient.List(ctx, listOpt)
	if err != nil {
		return fmt.Errorf("listing %s", t.gvr.String())
	}

	if err := list.EachListItem(func(obj runtime.Object) error {
		isNamespaced, err := isNamespaced(c.clientset, t.gvr)
		if err != nil {
			return fmt.Errorf("checking if namespaced: %w", err)
		}

		o, err := meta.Accessor(obj)
		if err != nil {
			return fmt.Errorf("accessing object metadata: %w", err)
		}

		var nsClient dynamic.ResourceInterface = dclient
		if isNamespaced {
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

	return nil
}

func isNamespaced(clientset kubernetes.Interface, gvr schema.GroupVersionResource) (bool, error) {
	res, err := clientset.Discovery().ServerResourcesForGroupVersion(gvr.GroupVersion().String())
	if err != nil {
		return false, fmt.Errorf("getting server resources for group version: %w", err)
	}

	namespaced := false
	for _, r := range res.APIResources {
		if r.Name == gvr.Resource {
			namespaced = r.Namespaced
			break
		}
	}

	return namespaced, nil
}
