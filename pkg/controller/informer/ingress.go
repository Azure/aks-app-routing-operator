package informer

import (
	"errors"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/informers"
	netv1informer "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/tools/cache"
)

const ingressClassIndex = "ingressClassIndex"

// Ingress gives access to an informer with shared indexes
type Ingress interface {
	netv1informer.IngressInformer
	// ByClass returns ingresses that share the given class
	ByClass(c string) ([]*netv1.Ingress, error)
}

type ingress struct {
	netv1informer.IngressInformer
	ingressClassIndex string
}

var _ Ingress = &ingress{}

func (i *ingress) ByClass(c string) ([]*netv1.Ingress, error) {
	objs, err := i.Informer().GetIndexer().ByIndex(i.ingressClassIndex, c)
	if err != nil {
		return nil, err
	}

	ings := make([]*netv1.Ingress, 0, len(objs))
	for _, obj := range objs {
		ing, ok := obj.(*netv1.Ingress)
		if !ok {
			return nil, errors.New("converting to Ingress")
		}

		ings = append(ings, ing)
	}

	return ings, nil
}

// NewIngress constructs a new Ingress Informer with shared indexers
func NewIngress(factory informers.SharedInformerFactory) (Ingress, error) {
	informer := factory.Networking().V1().Ingresses()

	if err := informer.Informer().AddIndexers(cache.Indexers{
		ingressClassIndex: func(obj interface{}) ([]string, error) {
			ing, ok := obj.(*netv1.Ingress)
			if !ok {
				return []string{}, errors.New("converting to Ingress")
			}

			c := ing.Spec.IngressClassName
			if c == nil || *c == "" {
				return []string{}, nil
			}

			return []string{*c}, nil
		},
	}); err != nil {
		return nil, err
	}

	i := &ingress{
		IngressInformer:   informer,
		ingressClassIndex: ingressClassIndex,
	}
	return i, nil
}
