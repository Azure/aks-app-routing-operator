package informer

import (
	"errors"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/informers"
	netv1informer "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/tools/cache"
)

const ingressClassNameIndex = "ingressClassNameIndex"

// Ingress gives access to an informer with shared indexes
type Ingress interface {
	netv1informer.IngressInformer
	// ByIngressClassName returns ingresses that share the given ingress classname
	ByIngressClassName(cn string) ([]*netv1.Ingress, error)
}

type ingress struct {
	netv1informer.IngressInformer
	ingressClassNameIndex string
}

var _ Ingress = &ingress{}

func (i *ingress) ByIngressClassName(cn string) ([]*netv1.Ingress, error) {
	objs, err := i.Informer().GetIndexer().ByIndex(i.ingressClassNameIndex, cn)
	if err != nil {
		return nil, err
	}

	ings := make([]*netv1.Ingress, 0, len(objs))
	for _, obj := range objs {
		ing, ok := obj.(*netv1.Ingress)
		if !ok {
			return nil, errors.New("failed to convert to Ingress type")
		}
		ings = append(ings, ing)
	}

	return ings, nil
}

// NewIngress constructs a new Ingress Informer with shared indexers
func NewIngress(factory informers.SharedInformerFactory) (Ingress, error) {
	informer := factory.Networking().V1().Ingresses()

	if err := informer.Informer().AddIndexers(cache.Indexers{
		ingressClassNameIndex: func(obj interface{}) ([]string, error) {
			ing, ok := obj.(*netv1.Ingress)
			if !ok {
				return []string{}, nil
			}

			cn := ing.Spec.IngressClassName
			if cn == nil {
				return []string{}, nil
			}

			return []string{*cn}, nil
		},
	}); err != nil {
		return nil, err
	}

	ing := &ingress{
		IngressInformer:       informer,
		ingressClassNameIndex: ingressClassNameIndex,
	}
	return ing, nil
}
