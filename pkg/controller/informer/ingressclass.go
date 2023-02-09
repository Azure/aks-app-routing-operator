package informer

import (
	"errors"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/informers"
	netv1informer "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/tools/cache"
)

const ingressClassControllerIndex = "ingressClassControllerIndex"

// IngressClass gives access to an informer with shared indexes
type IngressClass interface {
	netv1informer.IngressClassInformer
	// ByController returns ingress classes that share the given controller
	ByController(c string) ([]*netv1.IngressClass, error)
}

type ingressClass struct {
	netv1informer.IngressClassInformer
	ingressClassControllerIndex string
}

var _ IngressClass = &ingressClass{}

func (i *ingressClass) ByController(c string) ([]*netv1.IngressClass, error) {
	objs, err := i.Informer().GetIndexer().ByIndex(i.ingressClassControllerIndex, c)
	if err != nil {
		return nil, err
	}

	ingCs := make([]*netv1.IngressClass, 0, len(objs))
	for _, obj := range objs {
		ingC, ok := obj.(*netv1.IngressClass)
		if !ok {
			return nil, errors.New("failed to convert to IngressClass")
		}
		ingCs = append(ingCs, ingC)
	}

	return ingCs, nil
}

// NewIngressClass constructs a new IngressClass Informer with shared indexers
func NewIngressClass(factory informers.SharedInformerFactory) (IngressClass, error) {
	informer := factory.Networking().V1().IngressClasses()

	if err := informer.Informer().AddIndexers(cache.Indexers{
		ingressClassControllerIndex: func(obj interface{}) ([]string, error) {
			ingC, ok := obj.(*netv1.IngressClass)
			if !ok {
				return []string{}, errors.New("converting to ingress class")
			}

			c := ingC.Spec.Controller
			return []string{c}, nil
		},
	}); err != nil {
		return nil, err
	}

	i := &ingressClass{
		IngressClassInformer:        informer,
		ingressClassControllerIndex: ingressClassControllerIndex,
	}
	return i, nil
}
