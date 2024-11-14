package keyvault

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func AddGatewayServiceAccountIndex(indexer client.FieldIndexer, indexName string) error {
	if err := indexer.IndexField(context.Background(), &gatewayv1.Gateway{}, indexName, gatewayServiceAccountIndexFn); err != nil {
		return fmt.Errorf("adding Gateway Service Account indexer: %w", err)
	}

	return nil
}

func gatewayServiceAccountIndexFn(object client.Object) []string {
	gateway, ok := object.(*gatewayv1.Gateway)
	if !ok {
		return nil
	}

	saSet := map[string]struct{}{}

	for _, listener := range gateway.Spec.Listeners {
		if listener.TLS != nil && listener.TLS.Options != nil {
			serviceAccountName, ok := listener.TLS.Options[serviceAccountTLSOption]
			if !ok {
				continue
			}
			saSet[string(serviceAccountName)] = struct{}{}
		}
	}

	saSlice := make([]string, 0, len(saSet))
	for sa := range saSet {
		saSlice = append(saSlice, sa)
	}

	return saSlice
}

func generateGatewayGetter(mgr ctrl.Manager, serviceAccountIndexName string) handler.MapFunc {
	logger := mgr.GetLogger()
	fmt.Println("calling generate gateway getter")
	return func(ctx context.Context, obj client.Object) []ctrl.Request {
		fmt.Println("generateGatewayGetter")
		sa, ok := obj.(*corev1.ServiceAccount)
		if !ok {
			return nil
		}
		gateways := &gatewayv1.GatewayList{}
		err := mgr.GetClient().List(context.TODO(), gateways, client.MatchingFields{serviceAccountIndexName: sa.Name})
		if err != nil {
			logger.Error(err, "failed to list gateways for service account", "name", sa.Name, "namespace", sa.Namespace)
			return nil
		}
		ret := make([]ctrl.Request, 0)
		for _, gateway := range gateways.Items {
			ret = append(ret, ctrl.Request{NamespacedName: client.ObjectKey{Name: gateway.Name, Namespace: gateway.Namespace}})
		}

		return ret
	}

}
