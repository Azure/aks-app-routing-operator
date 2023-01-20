package informer

import (
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/informers"
	netv1informer "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/tools/cache"
)

const IngressClassNameIndex = "ingressClassNameIndex"

// NewIngress creates a new Ingress Informer with shared indexers
func NewIngress(factory informers.SharedInformerFactory) (netv1informer.IngressInformer, error) {
	ing := factory.Networking().V1().Ingresses()

	if err := ing.Informer().AddIndexers(cache.Indexers{
		IngressClassNameIndex: func(obj interface{}) ([]string, error) {
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

	return ing, nil
}
