package spc

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var gatewaySecretProviderControllerName = controllername.New("gateway", "keyvault", "secret", "provider")

func NewGatewaySecretClassProviderReconciler(manager ctrl.Manager, conf *config.Config, serviceAccountIndexName string) error {
	metrics.InitControllerMetrics(gatewaySecretProviderControllerName)
	if conf.DisableKeyvault {
		return nil
	}

	spcReconciler := &secretProviderClassReconciler[*gatewayv1.Gateway]{
		name: gatewaySecretProviderControllerName,

		client: manager.GetClient(),
		events: manager.GetEventRecorderFor("aks-app-routing-operator"),
		config: conf,
	}

	return gatewaySecretProviderControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&gatewayv1.Gateway{}).
			Owns(&secv1.SecretProviderClass{}).
			Watches(&corev1.ServiceAccount{}, handler.EnqueueRequestsFromMapFunc(generateGatewayGetter(manager, serviceAccountIndexName))),
		manager.GetLogger(),
	).Complete(spcReconciler)
}

func gatewayToSpcOpts(ctx context.Context, conf *config.Config, gw *gatewayv1.Gateway, cl client.Client) iter.Seq2[spcOpts, error] {
	return func(yield func(spcOpts, error) bool) {
		if conf == nil {
			yield(spcOpts{}, errors.New("config is nil"))
			return
		}

		if gw == nil {
			yield(spcOpts{}, errors.New("gateway is nil"))
			return
		}

		if gw.Spec.GatewayClassName != istioGatewayClassName {
			// todo: test this and make sure it returns no values instead of hangs
			return
		}

		for index, listener := range gw.Spec.Listeners {
			name := getGatewaySpcName(gw.Name, string(listener.Name))
			opts := spcOpts{
				action:     actionReconcile,
				name:       name,
				namespace:  gw.Namespace,
				tenantId:   conf.TenantID,
				secretName: name,
				cloud:      conf.Cloud,
			}

			if !listenerIsKvEnabled(listener) {
				opts.action = actionCleanup
				if !yield(opts, nil) {
					return
				}
				continue
			}

			clientId, err := clientIdFromListener(ctx, cl, gw.Namespace, listener)
			if err != nil {
				if !yield(opts, err) {
					return
				}
				continue
			}
			opts.clientId = clientId

			uri := string(listener.TLS.Options[certUriTLSOption])
			certRef, err := parseKeyVaultCertURI(uri)
			if err != nil {
				if !yield(opts, fmt.Errorf("parsing KeyVault cert URI %s: %w", uri, err)) {
					return
				}
				continue
			}

			opts.vaultName = certRef.vaultName
			opts.certName = certRef.certName
			opts.objectVersion = certRef.objectVersion
			opts.modifyOwner = func(obj client.Object) error {
				gwObj, ok := obj.(*gatewayv1.Gateway)
				if !ok {
					return fmt.Errorf("object is not a Gateway: %T", obj)
				}

				gwObj.Spec.Listeners[index].TLS.CertificateRefs = []gatewayv1.SecretObjectReference{
					{
						Namespace: util.ToPtr(gatewayv1.Namespace(opts.namespace)),
						Name:      util.ToPtr(gatewayv1.Name(opts.secretName)),
						Group:     util.ToPtr(gatewayv1.Group(corev1.GroupName)),
						Kind:      util.ToPtr(gatewayv1.Kind("Secret")),
					},
				}

				return nil
			}
			if !yield(opts, nil) {
				return
			}
		}
	}
}

func getGatewaySpcName(gwName, listenerName string) string {
	name := fmt.Sprintf("kv-gw-cert-%s-%s", gwName, listenerName)
	if len(name) > 253 {
		name = name[:253]
	}

	return name
}

func listenerIsKvEnabled(listener gatewayv1.Listener) bool {
	return listener.TLS != nil && listener.TLS.Options != nil && listener.TLS.Options[certUriTLSOption] != ""
}

func clientIdFromListener(ctx context.Context, cl client.Client, namespace string, listener gatewayv1.Listener) (string, error) {
	certUri := string(listener.TLS.Options[certUriTLSOption])
	saName := string(listener.TLS.Options[serviceAccountTLSOption])

	// validate user input
	if certUri != "" && saName == "" {
		return "", util.NewUserError(errors.New("user specified cert URI but no ServiceAccount in a listener"), "KeyVault Cert URI provided, but the required ServiceAccount option was not. Please provide a ServiceAccount via the TLS option kubernetes.azure.com/tls-cert-service-account")
	}
	if certUri == "" && saName != "" {
		return "", util.NewUserError(errors.New("user specified ServiceAccount but no cert URI in a listener"), "ServiceAccount for WorkloadIdentity provided, but KeyVault Cert URI was not. Please provide a TLS Cert URI via the TLS option kubernetes.azure.com/tls-cert-keyvault-uri")
	}

	// this should never happen since we check for this prior to this function call but just to be safe
	if certUri == "" && saName == "" {
		return "", util.NewUserError(errors.New("none of the required TLS options were specified"), "KeyVault Cert URI and ServiceAccount must both be specified to use TLS functionality in App Routing")
	}

	clientId, err := getServiceAccountClientId(ctx, cl, saName, namespace)
	if err != nil {
		return "", err
	}

	return clientId, nil
}

func getServiceAccountClientId(ctx context.Context, cl client.Client, saName, saNamespace string) (string, error) {
	sa := &corev1.ServiceAccount{}
	if err := cl.Get(ctx, client.ObjectKey{Name: saName, Namespace: saNamespace}, sa); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return "", fmt.Errorf("failed to fetch service account to verify workload identity configuration: %w", err)
		}

		return "", util.NewUserError(err, fmt.Sprintf("serviceAccount %s does not exist in namespace %s", saName, saNamespace))
	}

	if sa.Annotations == nil || sa.Annotations[wiSaClientIdAnnotation] == "" {
		return "", util.NewUserError(errors.New("user-specified service account does not contain WI annotation"), fmt.Sprintf("serviceAccount %s was specified but does not include necessary annotation for workload identity", saName))
	}

	return sa.Annotations[wiSaClientIdAnnotation], nil
}
