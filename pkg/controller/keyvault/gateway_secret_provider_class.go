package keyvault

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllerutils"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	gatewaySecretProviderControllerName = controllername.New("gateway", "keyvault", "secret", "provider")
)

// GatewaySecretProviderClassReconciler manages a SecretProviderClass for Gateway resource that specifies a ServiceAccount
// and Keyvault URI in its TLS options field. The SPC is used to mirror the Keyvault values into
// a k8s secret so that it can be used by the CRD controller.
type GatewaySecretProviderClassReconciler struct {
	client client.Client
	events record.EventRecorder
	config *config.Config
}

func NewGatewaySecretClassProviderReconciler(manager ctrl.Manager, conf *config.Config, serviceAccountIndexName string) error {
	metrics.InitControllerMetrics(gatewaySecretProviderControllerName)

	return gatewaySecretProviderControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&gatewayv1.Gateway{}).
			Owns(&secv1.SecretProviderClass{}).
			Watches(&corev1.ServiceAccount{}, handler.EnqueueRequestsFromMapFunc(generateGatewayGetter(manager, serviceAccountIndexName))), manager.GetLogger(),
	).Complete(&GatewaySecretProviderClassReconciler{
		client: manager.GetClient(),
		events: manager.GetEventRecorderFor("aks-app-routing-operator"),
		config: conf,
	})
}

func (g *GatewaySecretProviderClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, retErr error) {
	// set up metrics given result/error
	defer func() {
		metrics.HandleControllerReconcileMetrics(gatewaySecretProviderControllerName, res, retErr)
	}()

	// set up logger
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("creating logger: %w", err)
	}
	logger = gatewaySecretProviderControllerName.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	// retrieve gateway resource from request + log the get attempt, but ignore not found
	gwObj := &gatewayv1.Gateway{}
	err = g.client.Get(ctx, req.NamespacedName, gwObj)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to fetch Gateway")
			return ctrl.Result{}, fmt.Errorf("fetching gateway: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if !shouldReconcileGateway(gwObj) {
		return ctrl.Result{}, nil
	}

	// check its TLS options - needs to have both cert uri and either serviceaccount name or clientid
	for index, listener := range gwObj.Spec.Listeners {
		spc := &secv1.SecretProviderClass{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "secrets-store.csi.x-k8s.io/v1",
				Kind:       "SecretProviderClass",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      generateGwListenerCertName(gwObj.Name, listener.Name),
				Namespace: req.Namespace,
				Labels:    manifests.GetTopLevelLabels(),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: gwObj.APIVersion,
					Controller: util.ToPtr(true),
					Kind:       gwObj.Kind,
					Name:       gwObj.Name,
					UID:        gwObj.UID,
				}},
			},
		}
		logger = logger.WithValues("spc", spc.Name)

		if listenerIsKvEnabled(listener) {
			var clientId string
			clientId, err = retrieveClientIdForListener(ctx, g.client, req.Namespace, listener.TLS.Options)
			if err != nil {
				var userErr controllerutils.UserError
				if errors.As(err, &userErr) {
					logger.Info(fmt.Sprintf("failed to fetch clientId for SPC for listener %s due to user error: %s, sending warning event", listener.Name, userErr.UserMessage))
					g.events.Eventf(gwObj, corev1.EventTypeWarning, "InvalidInput", "invalid TLS configuration: %s", userErr.UserMessage)
					return ctrl.Result{}, nil
				}
				logger.Error(err, fmt.Sprintf("failed to fetch clientId for listener %s: %s", listener.Name, err.Error()))
				return ctrl.Result{}, fmt.Errorf("fetching clientId for listener: %w", err)
			}

			// otherwise it's active + valid - build SPC
			certUri := string(listener.TLS.Options[certUriTLSOption])

			logger.Info("building spc for listener and upserting")
			spcConf := spcConfig{
				ClientId:        clientId,
				TenantId:        g.config.TenantID,
				KeyvaultCertUri: certUri,
				Name:            generateGwListenerCertName(gwObj.Name, listener.Name),
			}
			err = buildSPC(spc, spcConf)
			if err != nil {
				var userErr controllerutils.UserError
				if errors.As(err, &userErr) {
					logger.Info("failed to build SecretProviderClass from user error: %s, sending warning event", userErr.UserMessage)
					g.events.Eventf(gwObj, corev1.EventTypeWarning, "InvalidInput", "invalid TLS configuration: %s", userErr.UserMessage)
					return ctrl.Result{}, nil
				}
				logger.Error(err, fmt.Sprintf("building SPC for listener %s: %s", listener.Name, err.Error()))
				return ctrl.Result{}, fmt.Errorf("building spc: %w", err)
			}

			logger.Info(fmt.Sprintf("reconciling SecretProviderClass %s for listener %s", spc.Name, listener.Name))
			if err = util.Upsert(ctx, g.client, spc); err != nil {
				fullErr := fmt.Errorf("failed to reconcile SecretProviderClass %s: %w", req.Name, err)
				logger.Error(err, fullErr.Error())
				g.events.Event(gwObj, corev1.EventTypeWarning, "FailedUpdateOrCreateSPC", fullErr.Error())
				return ctrl.Result{}, fullErr
			}

			logger.Info(fmt.Sprintf("preemptively attaching secret reference for listener %s", listener.Name))
			newCertRef := gatewayv1.SecretObjectReference{
				Namespace: to.Ptr(gatewayv1.Namespace(req.Namespace)),
				Group:     to.Ptr(gatewayv1.Group(corev1.GroupName)),
				Kind:      to.Ptr(gatewayv1.Kind("Secret")),
				Name:      gatewayv1.ObjectName(generateGwListenerCertName(gwObj.Name, listener.Name)),
			}
			gwObj.Spec.Listeners[index].TLS.CertificateRefs = []gatewayv1.SecretObjectReference{newCertRef}
			continue
		}
		// we should delete the SPC if it exists
		logger.Info(fmt.Sprintf("attempting to remove unused SPC %s", spc.Name))

		deletionSpc := &secv1.SecretProviderClass{}
		if err = g.client.Get(ctx, client.ObjectKeyFromObject(spc), deletionSpc); err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, fmt.Sprintf("failed to fetch SPC for deletion %s", spc.Name))
				return ctrl.Result{}, fmt.Errorf("fetching SPC for deletion: %w", err)
			}
			continue
		}

		if manifests.HasTopLevelLabels(deletionSpc.Labels) {
			// return if we fail to delete, but otherwise, keep going
			if err = g.client.Delete(ctx, deletionSpc); err != nil {
				if client.IgnoreNotFound(err) != nil {
					logger.Error(err, fmt.Sprintf("failed to delete SPC %s", spc.Name))
					return ctrl.Result{}, fmt.Errorf("deleting SPC: %w", err)
				}
				continue
			}
		}

	}

	logger.Info("reconciling Gateway resource with new secret refs for each TLS-enabled listener")
	if err = g.client.Update(ctx, gwObj); client.IgnoreNotFound(err) != nil {
		fullErr := fmt.Errorf("failed to reconcile Gateway resource %s: %w", req.Name, err)
		logger.Error(err, fullErr.Error())
		g.events.Event(gwObj, corev1.EventTypeWarning, "FailedUpdateOrCreateGateway", fullErr.Error())
		return ctrl.Result{}, fullErr
	}

	return ctrl.Result{}, nil
}

func generateGwListenerCertName(gw string, listener gatewayv1.SectionName) string {
	certName := fmt.Sprintf("kv-gw-cert-%s-%s", gw, string(listener))
	if len(certName) > 253 {
		certName = certName[:253]
	}

	return certName
}

func listenerIsKvEnabled(listener gatewayv1.Listener) bool {
	return listener.TLS != nil && listener.TLS.Options != nil && listener.TLS.Options[tlsCertKvUriAnnotation] != ""
}

func retrieveClientIdForListener(ctx context.Context, k8sclient client.Client, namespace string, options map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue) (string, error) {
	certUri := string(options[certUriTLSOption])
	saName := string(options[serviceAccountTLSOption])

	// validate user input
	if certUri != "" && saName == "" {
		return "", controllerutils.NewUserError(errors.New("user specified cert URI but no ServiceAccount in a listener"), "KeyVault Cert URI provided, but the required ServiceAccount option was not. Please provide a ServiceAccount via the TLS option kubernetes.azure.com/tls-cert-service-account")
	}
	if certUri == "" && saName != "" {
		return "", controllerutils.NewUserError(errors.New("user specified ServiceAccount but no cert URI in a listener"), "ServiceAccount for WorkloadIdentity provided, but KeyVault Cert URI was not. Please provide a TLS Cert URI via the TLS option kubernetes.azure.com/tls-cert-keyvault-uri")
	}

	// this should never happen since we check for this prior to this function call but just to be safe
	if certUri == "" && saName == "" {
		return "", controllerutils.NewUserError(errors.New("none of the required TLS options were specified"), "KeyVault Cert URI and ServiceAccount must both be specified to use TLS functionality in App Routing")
	}

	// pull service account
	wiSaClientId, err := controllerutils.GetServiceAccountAndVerifyWorkloadIdentity(ctx, k8sclient, saName, namespace)
	if err != nil {
		return "", err
	}
	return wiSaClientId, nil

}
