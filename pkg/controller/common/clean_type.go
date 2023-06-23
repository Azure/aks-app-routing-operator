package common

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type cleanType struct {
	labels map[string]string
	gvr    schema.GroupVersionResource
}

// CleanTypeRetriever returns types and labels for the cleaner to remove
type CleanTypeRetriever func(mapper meta.RESTMapper) ([]cleanType, error) // getter function because manager client isn't usable until manager starts

type RemoveOpt struct {
	CompareStrat CompareStrategy
}

type CompareStrategy int

const (
	Everything   CompareStrategy = iota // compare Everything
	IgnoreLabels                        // ignore labels when comparing
)

func gvrsFromGk(mapper meta.RESTMapper, gk schema.GroupKind) ([]schema.GroupVersionResource, error) {
	// get potential group version resource for group kind
	mappings, err := mapper.RESTMappings(gk) // retrieve all mappings because versions might be auto updated (by conversion webhooks)
	if err != nil {
		return nil, fmt.Errorf("getting rest mappings for %s: %w", gk.String(), err)
	}

	var gvrs []schema.GroupVersionResource
	for _, mapping := range mappings {
		gvrs = append(gvrs, mapping.Resource)
	}

	return gvrs, nil
}

func addLabels(gvrs []schema.GroupVersionResource, labels map[string]string) []cleanType {
	var ret []cleanType
	for _, gvr := range gvrs {
		ret = append(ret, cleanType{
			labels: labels,
			gvr:    gvr,
		})
	}
	return ret
}

// RetrieverFromObjs retrieves a list of group version resources based on supplied object types
func RetrieverFromObjs(objs []client.Object, labels map[string]string) CleanTypeRetriever {
	return func(mapper meta.RESTMapper) ([]cleanType, error) {
		var ret []schema.GroupVersionResource
		for _, obj := range objs {
			gvrs, err := gvrsFromGk(mapper, obj.GetObjectKind().GroupVersionKind().GroupKind())
			if err != nil {
				return nil, err
			}

			ret = append(ret, gvrs...)
		}

		return addLabels(ret, labels), nil
	}
}

// RetrieverFromGk retrieves a list of group version resources based on group kinds
func RetrieverFromGk(labels map[string]string, gks ...schema.GroupKind) CleanTypeRetriever {
	return func(mapper meta.RESTMapper) ([]cleanType, error) {
		var gvrs []schema.GroupVersionResource
		for _, gk := range gks {
			new, err := gvrsFromGk(mapper, gk)
			if err != nil {
				return nil, fmt.Errorf("getting gvrs for %s: %w", gk.String(), err)
			}

			gvrs = append(gvrs, new...)
		}

		return addLabels(gvrs, labels), nil
	}
}

// RetrieverEmpty is commonly used to start a chain of retriever functions
func RetrieverEmpty() CleanTypeRetriever {
	return func(_ meta.RESTMapper) ([]cleanType, error) {
		return make([]cleanType, 0), nil
	}
}

func (g CleanTypeRetriever) Add(retriever CleanTypeRetriever) CleanTypeRetriever {
	return func(mapper meta.RESTMapper) ([]cleanType, error) {
		old, err := g(mapper)
		if err != nil {
			return nil, err
		}

		add, err := retriever(mapper)
		if err != nil {
			return nil, err
		}

		return append(old, add...), nil
	}
}

func (g CleanTypeRetriever) Remove(retriever CleanTypeRetriever, opt RemoveOpt) CleanTypeRetriever {
	return func(mapper meta.RESTMapper) ([]cleanType, error) {
		existing, err := g(mapper)
		if err != nil {
			return nil, err
		}

		filters, err := retriever(mapper)
		if err != nil {
			return nil, err
		}

		var ok []cleanType
		for _, t := range existing {
			filtered := false
			for _, filter := range filters {
				if equal(t, filter, opt.CompareStrat) {
					filtered = true
					break
				}
			}

			if !filtered {
				ok = append(ok, t)
			}
		}

		return ok, nil
	}
}

func equal(c1, c2 cleanType, cs CompareStrategy) bool {
	switch cs {
	case Everything:
		return reflect.DeepEqual(c1, c2)
	case IgnoreLabels:
		return reflect.DeepEqual(c1.gvr, c2.gvr)
	default:
		return false
	}
}
