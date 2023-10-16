package keyvault

import netv1 "k8s.io/api/networking/v1"

// IngressManager returns a boolean indicating whether the Ingress is being managed by us
type IngressManager interface {
	IsManaging(ing *netv1.Ingress) bool
}

type ingressManager struct {
	icNames map[string]struct{}
}

// NewIngressManager returns an IngressManager from a set of ingress class names that web app routing manages
func NewIngressManager(icNames map[string]struct{}) IngressManager {
	return &ingressManager{icNames: icNames}
}

func (i ingressManager) IsManaging(ing *netv1.Ingress) bool {
	if ing == nil {
		return false
	}

	cn := ing.Spec.IngressClassName
	if cn == nil {
		return false
	}

	_, ok := i.icNames[*cn]
	return ok
}
