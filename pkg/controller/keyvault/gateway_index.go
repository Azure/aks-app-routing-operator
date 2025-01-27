package keyvault

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
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
	if !ok || gateway == nil {
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

	return util.Keys(saSet)
}

func generateGatewayGetter(mgr ctrl.Manager, serviceAccountIndexName string) handler.MapFunc {
	logger := mgr.GetLogger()
	return func(ctx context.Context, obj client.Object) []ctrl.Request {
		sa, ok := obj.(*corev1.ServiceAccount)
		if !ok {
			return nil
		}
		gateways := &gatewayv1.GatewayList{}
		err := mgr.GetClient().List(ctx, gateways, client.MatchingFields{serviceAccountIndexName: sa.Name}, client.InNamespace(sa.Namespace))
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
