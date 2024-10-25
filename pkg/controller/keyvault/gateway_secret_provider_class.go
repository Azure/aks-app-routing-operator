package keyvault

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	gatewaySecretProviderControllerName = controllername.New("keyvault", "gateway", "secret", "provider")
)

const (
	saNameUriErr = "`App Routing requires a Certificate URI as well as a ServiceAccount that references an Identity that has access to your Keyvault in order to pull TLS certificates. Make sure both are specified in your Listener's TLS options for all listeners that require TLS certs."
)

// GatewaySecretProviderClassReconciler manages a SecretProviderClass for Gateway resource that specifies a ServiceAccount
// and Keyvault URI in its TLS options field. The SPC is used to mirror the Keyvault values into
// a k8s secret so that it can be used by the CRD controller.
type GatewaySecretProviderClassReconciler struct {
	client client.Client
	events record.EventRecorder
	config *config.Config
}

func NewGatewaySecretClassProviderReconciler(manager ctrl.Manager, conf *config.Config) error {
	if conf.DisableKeyvault {
		return nil
	}
	metrics.InitControllerMetrics(gatewaySecretProviderControllerName)

	return gatewaySecretProviderControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&gatewayv1.Gateway{}), manager.GetLogger(),
	).Complete(&GatewaySecretProviderClassReconciler{
		client: manager.GetClient(),
		events: manager.GetEventRecorderFor("aks-app-routing-operator"),
		config: conf,
	})
}

func (g *GatewaySecretProviderClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	res := ctrl.Result{}
	var err error

	// set up metrics given result/error
	defer func() {
		metrics.HandleControllerReconcileMetrics(gatewaySecretProviderControllerName, res, err)
	}()

	// set up logger
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return res, err
	}
	logger = gatewaySecretProviderControllerName.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	// retrieve gateway resource from request + log the get attempt, but ignore not found
	gwObj := &gatewayv1.Gateway{}
	err = g.client.Get(ctx, req.NamespacedName, gwObj)

	if err != nil {
		return res, client.IgnoreNotFound(err)
	}

	// check its TLS options - needs to have both
	// cert uri and either serviceaccount name or clientid --> if one without the other, propagate a warning event
	for index, listener := range gwObj.Spec.Listeners {
		spc := &secv1.SecretProviderClass{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "secrets-store.csi.x-k8s.io/v1",
				Kind:       "SecretProviderClass",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateGwListenerCertName(gwObj.Name, listener.Name),
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
			clientId, err := retrieveClientIdFromListener(ctx, g.client, req.Namespace, listener.TLS.Options)
			if err != nil {
				var userErr userError
				if errors.As(err, &userErr) {
					logger.Info(fmt.Sprintf("failed to fetch clientId for SPC for listener %s due to user error: %q, sending warning event", listener.Name, userErr.userMessage))
					g.events.Eventf(gwObj, Warning.String(), "InvalidInput", "invalid TLS configuration: %s", userErr.userMessage)
					return res, nil
				}
				logger.Error(err, fmt.Sprintf("failed to fetch clientId for listener %s: %q", listener.Name, err.Error()))
				return res, err
			}

			// otherwise it's active + valid - build SPC
			certUri := string(listener.TLS.Options[certUriTLSOption])

			logger.Info("building spc for Gateway resource and upserting ")
			spcConf := SPCConfig{
				ClientId:        clientId,
				TenantId:        g.config.TenantID,
				KeyvaultCertUri: certUri,
				Name:            GenerateGwListenerCertName(gwObj.Name, listener.Name),
			}
			err = buildSPC(spc, spcConf)
			if err != nil {
				var userErr userError
				if errors.As(err, &userErr) {
					logger.Info("failed to build SecretProviderClass from user error: %q sending warning event", userErr.userMessage)
					g.events.Eventf(gwObj, Warning.String(), "InvalidInput", "invalid TLS configuration: %s", userErr.userMessage)
					return res, nil
				}
				logger.Error(err, fmt.Sprintf("building SPC for listener %s: %s", listener.Name, err.Error()))
				return res, err
			}

			logger.Info(fmt.Sprintf("reconciling SecretProviderClass %s for listener %s", spc.Name, listener.Name))
			if err := util.Upsert(ctx, g.client, spc); err != nil {
				errString := fmt.Sprintf("failed to reconcile SecretProviderClass %s: %q", req.Name, err)
				logger.Error(err, errString)
				g.events.Event(gwObj, Warning.String(), "FailedUpdateOrCreateSPC", errString)
				return res, err
			}

			logger.Info(fmt.Sprintf("preemptively attaching secret reference for listener %s", listener.Name))
			newCertRef := gatewayv1.SecretObjectReference{
				Namespace: to.Ptr(gatewayv1.Namespace(req.Namespace)),
				Group:     to.Ptr(gatewayv1.Group(corev1.GroupName)),
				Kind:      to.Ptr(gatewayv1.Kind("Secret")),
				Name:      gatewayv1.ObjectName(GenerateGwListenerCertName(gwObj.Name, listener.Name)),
			}
			gwObj.Spec.Listeners[index].TLS.CertificateRefs = []gatewayv1.SecretObjectReference{newCertRef}

		} else {
			// we should delete the SPC if it exists
			logger.Info(fmt.Sprintf("attempting to remove unused SPC %s", spc.Name))

			deletionSpc := &secv1.SecretProviderClass{}
			if err := client.IgnoreNotFound(g.client.Get(ctx, client.ObjectKeyFromObject(spc), deletionSpc)); err != nil {
				return res, err
			}

			if manifests.HasTopLevelLabels(deletionSpc.Labels) {
				// return if we fail to delete, but otherwise, keep going
				if err := g.client.Delete(ctx, deletionSpc); client.IgnoreNotFound(err) != nil {
					return res, err
				}
			}
		}
	}

	logger.Info("reconciling Gateway resource with new secret refs for each TLS-enabled listener")
	if err := util.Upsert(ctx, g.client, gwObj); err != nil {
		errString := fmt.Sprintf("failed to reconcile Gateway resource %s: %q", req.Name, err)
		logger.Error(err, errString)
		g.events.Event(gwObj, Warning.String(), "FailedUpdateOrCreateGateway", errString)
		return res, err
	}

	return res, err
}

func GenerateGwListenerCertName(gw string, listener gatewayv1.SectionName) string {
	template := fmt.Sprintf("kv-gw-cert-%s-%s", gw, string(listener))
	if len(template) > 253 {
		template = template[:253]
	}

	return template
}

func listenerIsKvEnabled(listener gatewayv1.Listener) bool {
	return listener.TLS != nil && listener.TLS.Options != nil && listener.TLS.Options[tlsCertKvUriAnnotation] != ""
}

func retrieveClientIdFromListener(ctx context.Context, k8sclient client.Client, namespace string, options map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue) (string, error) {
	certUri := string(options[certUriTLSOption])
	saName := string(options[serviceAccountTLSOption])
	inputClientId := string(options[clientIdTLSOption])

	// validate user input
	if certUri != "" && (saName == "" && inputClientId == "") {
		return "", newUserError(errors.New("user specified cert URI but no serviceaccount or clientid in a listener"), "detected Keyvault Cert URI, but no ServiceAccount or Client ID was provided")
	}
	if certUri == "" && (saName != "" || inputClientId != "") {
		return "", newUserError(errors.New("user specified clientId or SA but no cert URI in a listener"), "detected identity for WorkloadIdentity, but no Keyvault Certificate URI was provided")
	}
	if saName != "" && inputClientId != "" {
		return "", newUserError(errors.New("user specified both serviceaccount and a clientId in the same listener"), "both ServiceAccountName and ClientId have been specified, please specify one or the other")
	}

	// this should never happen since we check for this prior to this function call but just to be safe
	if certUri == "" && saName == "" && inputClientId == "" {
		return "", newUserError(errors.New("none of the required TLS options were specified"), "none of cert URI, clientId, or service account were specified")
	}

	var ret string
	var err error
	switch inputClientId {
	case "":
		// pull service account
		wiSa := &corev1.ServiceAccount{}
		err = k8sclient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: saName}, wiSa)
		if err != nil {
			if client.IgnoreNotFound(err) == nil {
				return "", newUserError(fmt.Errorf("user-specified serviceAccount %s does not exist", saName), fmt.Sprintf("serviceAccount %s does not exist", saName))
			}
			return "", fmt.Errorf("fetching serviceAccount %s: %s", saName, err)
		}

		if wiSa.Annotations == nil || wiSa.Annotations[wiSaClientIdAnnotation] == "" {
			errString := fmt.Sprintf("workload identity MSI client ID must be specified for serviceAccount %s with annotation %s", saName, wiSaClientIdAnnotation)
			return "", newUserError(errors.New("user-specified service account doesn't contain annotation with clientId"), errString)
		}
		ret = wiSa.Annotations[wiSaClientIdAnnotation]

	default:
		ret = inputClientId
	}

	return ret, nil
}
