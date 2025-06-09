package keyvault

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	spcpkg "github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault/spc"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	netv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

// spcOwnerType defines the struct for objects that can own a SecretProviderClass
type spcOwnerType[objectType client.Object] struct {
	// Kind is the kind of the object that owns the SecretProviderClass
	Kind string
	// OwnerNameAnnotation is the annotation key used to store the owner name in the SecretProviderClass
	OwnerNameAnnotation string
	// Namespace returns the namespace of the owner object, or "" if not cluster-scoped
	Namespace func(obj objectType) string
	// ShouldReconcile returns true if the SecretProviderClass should be reconciled for the given object
	ShouldReconcile func(spc *secv1.SecretProviderClass, obj objectType) (bool, error)
	// GetServiceAccountName returns the service account name that should be used for Workload Identity. Returns "", nil if not applicable.
	GetServiceAccountName func(ctx context.Context, cl client.Client, spc *secv1.SecretProviderClass, obj objectType) (string, error)
}

var nicSpcOwner = spcOwnerType[*v1alpha1.NginxIngressController]{
	Kind:                "NginxIngressController",
	OwnerNameAnnotation: "kubernetes.azure.com/nginx-ingress-controller-owner",
	Namespace:           func(_ *v1alpha1.NginxIngressController) string { return "" }, // NginxIngressController is cluster-scoped
	ShouldReconcile: func(spc *secv1.SecretProviderClass, obj *v1alpha1.NginxIngressController) (bool, error) {
		return spcpkg.ShouldReconcileNic(obj), nil
	},
	GetServiceAccountName: func(_ context.Context, _ client.Client, _ *secv1.SecretProviderClass, _ *v1alpha1.NginxIngressController) (string, error) {
		return "", nil // NginxIngressController does not use Workload Identity (yet)
	},
}

func getIngressSpcOwner(ingressManager util.IngressManager) spcOwnerType[*netv1.Ingress] {
	return spcOwnerType[*netv1.Ingress]{
		Kind:                "Ingress",
		OwnerNameAnnotation: "kubernetes.azure.com/ingress-owner",
		Namespace:           func(ing *netv1.Ingress) string { return ing.Namespace },
		ShouldReconcile: func(spc *secv1.SecretProviderClass, ing *netv1.Ingress) (bool, error) {
			managed, err := spcpkg.ShouldReconcileIngress(ingressManager, ing)
			if err != nil {
				return false, fmt.Errorf("determining if ingress is managed: %w", err)
			}

			return managed, nil
		},
		GetServiceAccountName: func(_ context.Context, _ client.Client, _ *secv1.SecretProviderClass, _ *netv1.Ingress) (string, error) {
			return "", nil // Ingress does not use Workload Identity (yet)
		},
	}
}

var gatewaySpcOwner = spcOwnerType[*gatewayv1.Gateway]{
	Kind:                "Gateway",
	OwnerNameAnnotation: "kubernetes.azure.com/gateway-owner",
	Namespace:           func(gw *gatewayv1.Gateway) string { return gw.Namespace },
	ShouldReconcile: func(spc *secv1.SecretProviderClass, gw *gatewayv1.Gateway) (bool, error) {
		if !spcpkg.IsManagedGateway(gw) {
			return false, nil
		}

		for _, listener := range gw.Spec.Listeners {
			if spc.Name != spcpkg.GetGatewayListenerSpcName(gw.Name, string(listener.Name)) {
				continue
			}

			return !spcpkg.ListenerIsKvEnabled(listener), nil
		}

		return false, nil
	},
	GetServiceAccountName: func(ctx context.Context, cl client.Client, spc *secv1.SecretProviderClass, gw *gatewayv1.Gateway) (string, error) {
		sa := ""
		for _, listener := range gw.Spec.Listeners {
			if spc.Name != spcpkg.GetGatewayListenerSpcName(gw.Name, string(listener.Name)) {
				continue
			}

			if listener.TLS != nil && listener.TLS.Options != nil {
				sa = spcpkg.ServiceAccountFromListener(listener)
			}
		}

		if sa == "" {
			err := fmt.Errorf("failed to locate listener for SPC %s on user's gateway resource", spc.Name)
			return "", util.NewUserError(err, fmt.Sprintf("gateway listener for spc %s doesn't exist or doesn't contain required TLS options", spc.Name))
		}

		_, err := util.GetServiceAccountWorkloadIdentityClient(ctx, cl, sa, gw.Namespace)
		if err != nil {
			return "", err
		}

		return sa, nil
	},
}
