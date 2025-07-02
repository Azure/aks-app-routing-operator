package util

import netv1 "k8s.io/api/networking/v1"

// IngressManager returns a boolean indicating whether the Ingress is being managed by us
type IngressManager interface {
	IsManaging(ing *netv1.Ingress) (bool, error)
}

type ingressManager struct {
	isManagingFn func(ing *netv1.Ingress) (bool, error)
}

func (i *ingressManager) IsManaging(ing *netv1.Ingress) (bool, error) {
	return i.isManagingFn(ing)
}

// NewIngressManagerFromFn returns an IngressManager from a function that determines whether the Ingress is being managed by us
func NewIngressManagerFromFn(IsManaging func(ing *netv1.Ingress) (bool, error)) IngressManager {
	return &ingressManager{isManagingFn: IsManaging}
}
