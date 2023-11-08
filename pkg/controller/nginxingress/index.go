package nginxingress

import (
	"context"
	"fmt"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AddIngressClassNameIndex adds an index for the ingress class name to the indexer
func AddIngressClassNameIndex(indexer client.FieldIndexer, indexName string) error {
	if err := indexer.IndexField(context.Background(), &approutingv1alpha1.NginxIngressController{}, indexName, ingressClassNameIndexFn); err != nil {
		return fmt.Errorf("adding Nginx Ingress Controller IngressClass indexer: %w", err)
	}

	return nil
}

func ingressClassNameIndexFn(object client.Object) []string {
	nic, ok := object.(*approutingv1alpha1.NginxIngressController)
	if !ok {
		return nil
	}

	return []string{nic.Spec.IngressClassName}
}

// IsIngressManaged returns true if the ingress is managed by the operator
func IsIngressManaged(ctx context.Context, cl client.Client, ing *netv1.Ingress, ingressClassNameIndex string) (bool, error) {
	ic := ing.Spec.IngressClassName
	if ic == nil {
		return false, nil
	}

	// if it's the default one we should assume we own it because there's time in the upgrade from non crd app routing to crd app routing where a crd for the default doesn't exist
	if *ic == DefaultIcName {
		return true, nil
	}

	nics := &approutingv1alpha1.NginxIngressControllerList{}
	err := cl.List(ctx, nics, client.MatchingFields{ingressClassNameIndex: *ic})
	if err == nil {
		if len(nics.Items) == 0 {
			return false, nil
		}

		return true, nil
	}
	if !k8serrors.IsNotFound(err) {
		return false, fmt.Errorf("listing nginx ingress controllers: %w", err)
	}

	return false, nil

}

// IngressSource returns the ingress source for the given ingress if it's managed by the operator. If the Ingress isn't managed by the operator, it returns false.
func IngressSource(ctx context.Context, cl client.Client, conf *config.Config, defaultControllerClass string, ing *netv1.Ingress, ingressClassNameIndex string) (policyv1alpha1.IngressSourceSpec, bool, error) {
	ic := ing.Spec.IngressClassName
	if ic == nil {
		return policyv1alpha1.IngressSourceSpec{}, false, nil
	}

	// if it's the default one we should assume we own it because there's time in the upgrade from non crd app routing to crd app routing where a crd for the default doesn't exist
	if *ic == DefaultIcName {
		return policyv1alpha1.IngressSourceSpec{
			Kind:      "Service",
			Name:      DefaultNicResourceName,
			Namespace: conf.NS,
		}, true, nil
	}

	nics := &approutingv1alpha1.NginxIngressControllerList{}
	err := cl.List(ctx, nics, client.MatchingFields{ingressClassNameIndex: *ic})
	if err == nil {
		if len(nics.Items) == 0 {
			return policyv1alpha1.IngressSourceSpec{}, false, nil
		}

		nic := nics.Items[0]
		ingressConfig := ToNginxIngressConfig(&nic, defaultControllerClass)

		return policyv1alpha1.IngressSourceSpec{
			Kind:      "Service",
			Name:      ingressConfig.ResourceName,
			Namespace: conf.NS,
		}, true, nil
	}
	if !k8serrors.IsNotFound(err) {
		return policyv1alpha1.IngressSourceSpec{}, false, fmt.Errorf("listing nginx ingress controllers: %w", err)
	}

	return policyv1alpha1.IngressSourceSpec{}, false, nil
}
