package keyvault

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	spcpkg "github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault/spc"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

const (
	ingressOwnerAnnotation = "kubernetes.azure.com/ingress-owner"
)

var spcOwnerNotFoundErr = errors.New("no SecretProviderClass owner found")

type spcOwnerType interface {
	// IsOwner checks if the given owner type is the owner of the SecretProviderClass
	IsOwner(spc *secv1.SecretProviderClass) bool
	// GetOwnerAnnotation returns the annotation key used to store the owner name in the PlaceholderPod deployment
	GetOwnerAnnotation() string
	// GetObject returns the object that owns the SecretProviderClass. Returns noSpcOwnerErr if the owner is not found
	GetObject(ctx context.Context, cl client.Client, spc *secv1.SecretProviderClass) (client.Object, error)
	// ShouldReconcile returns true if the SecretProviderClass should be reconciled for the given object
	ShouldReconcile(spc *secv1.SecretProviderClass, obj client.Object) (bool, error)
	// GetServiceAccountName returns the service account name that should be used for Workload Identity. Returns "", nil if not applicable.
	GetServiceAccountName(ctx context.Context, cl client.Client, spc *secv1.SecretProviderClass, obj client.Object) (string, error)
}

// spcOwnerStruct defines the struct for objects that can own a SecretProviderClass
type spcOwnerStruct[objectType client.Object] struct {
	// kind is the kind of the object that owns the SecretProviderClass
	kind string
	// ownerNameAnnotation is the annotation key used to store the owner name in the SecretProviderClass
	ownerNameAnnotation string
	// namespace returns the namespace of the owner object, or "" if not cluster-scoped
	namespace func(obj objectType) string
	// shouldReconcile returns true if the SecretProviderClass should be reconciled for the given object
	shouldReconcile func(spc *secv1.SecretProviderClass, obj objectType) (bool, error)
	// getServiceAccountName returns the service account name that should be used for Workload Identity. Returns "", nil if not applicable.
	getServiceAccountName func(ctx context.Context, cl client.Client, spc *secv1.SecretProviderClass, obj objectType) (string, error)
}

func (s spcOwnerStruct[objectType]) IsOwner(spc *secv1.SecretProviderClass) bool {
	if spc == nil {
		return false
	}

	owner := util.FindOwnerKind(spc.OwnerReferences, s.kind)
	return owner != ""
}

func (s spcOwnerStruct[objectType]) GetOwnerAnnotation() string {
	return s.ownerNameAnnotation
}

func (s spcOwnerStruct[objectType]) GetObject(ctx context.Context, cl client.Client, spc *secv1.SecretProviderClass) (client.Object, error) {
	obj := util.NewObject[objectType]()
	obj.SetName(util.FindOwnerKind(spc.OwnerReferences, s.kind))
	obj.SetNamespace(s.namespace(obj))

	if err := cl.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, spcOwnerNotFoundErr
		}

		return nil, fmt.Errorf("getting object: %w", err)
	}

	return obj, nil
}

func (s spcOwnerStruct[objectType]) ShouldReconcile(spc *secv1.SecretProviderClass, obj client.Object) (bool, error) {
	return s.shouldReconcile(spc, obj.(objectType))
}

func (s spcOwnerStruct[objectType]) GetServiceAccountName(ctx context.Context, cl client.Client, spc *secv1.SecretProviderClass, obj client.Object) (string, error) {
	return s.getServiceAccountName(ctx, cl, spc, obj.(objectType))
}

var nicSpcOwner = spcOwnerStruct[*v1alpha1.NginxIngressController]{
	kind:                "NginxIngressController",
	ownerNameAnnotation: "kubernetes.azure.com/nginx-ingress-controller-owner",
	namespace:           func(_ *v1alpha1.NginxIngressController) string { return "" }, // NginxIngressController is cluster-scoped
	shouldReconcile: func(spc *secv1.SecretProviderClass, obj *v1alpha1.NginxIngressController) (bool, error) {
		return spcpkg.ShouldReconcileNic(obj), nil
	},
	getServiceAccountName: func(_ context.Context, _ client.Client, _ *secv1.SecretProviderClass, _ *v1alpha1.NginxIngressController) (string, error) {
		return "", nil // NginxIngressController does not use Workload Identity (yet)
	},
}

func getIngressSpcOwner(ingressManager util.IngressManager, cfg *config.Config) spcOwnerStruct[*netv1.Ingress] {
	return spcOwnerStruct[*netv1.Ingress]{
		kind:                "Ingress",
		ownerNameAnnotation: ingressOwnerAnnotation,
		namespace:           func(ing *netv1.Ingress) string { return ing.Namespace },
		shouldReconcile: func(spc *secv1.SecretProviderClass, ing *netv1.Ingress) (bool, error) {
			managed, err := spcpkg.ShouldReconcileIngress(ingressManager, ing)
			if err != nil {
				return false, fmt.Errorf("determining if ingress is managed: %w", err)
			}

			return managed, nil
		},
		getServiceAccountName: func(ctx context.Context, cl client.Client, spc *secv1.SecretProviderClass, ing *netv1.Ingress) (string, error) {
			if cfg == nil || !cfg.EnabledWorkloadIdentity {
				return "", nil
			}

			if ing == nil || ing.Annotations == nil {
				return "", nil // Ingress does not use Workload Identity
			}

			sa := ing.Annotations[spcpkg.IngressServiceAccountTLSAnnotation]
			if sa == "" {
				return "", nil // no service account specified, doesn't use Workload Identity
			}

			// validate that the workload identity client id exists
			if _, err := util.GetServiceAccountWorkloadIdentityClientId(ctx, cl, sa, ing.Namespace); err != nil {
				return "", err
			}

			return sa, nil
		},
	}
}

var gatewaySpcOwner = spcOwnerStruct[*gatewayv1.Gateway]{
	kind:                "Gateway",
	ownerNameAnnotation: "kubernetes.azure.com/gateway-owner",
	namespace:           func(gw *gatewayv1.Gateway) string { return gw.Namespace },
	shouldReconcile: func(spc *secv1.SecretProviderClass, gw *gatewayv1.Gateway) (bool, error) {
		if !spcpkg.IsManagedGateway(gw) {
			return false, nil
		}

		for _, listener := range gw.Spec.Listeners {
			if spc.Name != spcpkg.GetGatewayListenerSpcName(gw.Name, string(listener.Name)) {
				continue
			}

			return spcpkg.ListenerIsKvEnabled(listener), nil
		}

		return false, nil
	},
	getServiceAccountName: func(ctx context.Context, cl client.Client, spc *secv1.SecretProviderClass, gw *gatewayv1.Gateway) (string, error) {
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
			err := errors.New("failed to locate listener for SPC on user's gateway resource")
			return "", util.NewUserError(err, fmt.Sprintf("gateway listener for spc %s doesn't exist or doesn't contain required TLS options", spc.Name))
		}

		_, err := util.GetServiceAccountWorkloadIdentityClientId(ctx, cl, sa, gw.Namespace)
		if err != nil {
			return "", err
		}

		return sa, nil
	},
}
