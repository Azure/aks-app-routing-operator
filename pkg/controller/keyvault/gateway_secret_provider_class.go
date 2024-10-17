package keyvault

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	metrics.InitControllerMetrics(gatewaySecretProviderControllerName)
	if conf.DisableKeyvault {
		return nil
	}
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
	logger = nginxSecretProviderControllerName.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	// retrieve gateway resource from request + log the get attempt, but ignore not found
	gwObj := &gatewayv1.Gateway{}
	err = g.client.Get(ctx, req.NamespacedName, gwObj)

	if err != nil {
		return res, client.IgnoreNotFound(err)
	}

	// check its TLS options - needs to have both
	// cert uri and either serviceaccount name or clientid --> if one without the other, propagate a warning event
	for _, listener := range gwObj.Spec.Listeners {
		isActive, userErr := validateTLSOptions(listener.TLS.Options)
		if userErr != nil {
			// write event to Gateway resource for this listener
			g.events.Eventf(gwObj, Warning.String(), "InvalidConfig", "invalid configuration for Gateway resource detected: %s", userErr)
			continue
		} else if !isActive {
			continue
		}
		// otherwise it's active + valid - build SPC
		certUri := listener.TLS.Options[tlsCertKvUriOption]

		spc := &secv1.SecretProviderClass{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "secrets-store.csi.x-k8s.io/v1",
				Kind:       "SecretProviderClass",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateGatewayCertName(gwObj.Name),
				Namespace: g.config.NS,
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
		logger.Info("building spc for Gateway resource and upserting if managed with labels")
		upsertSPC, err := buildSPC(gwObj, spc, g.config, clientId)

	}

	// if it has both,
	// preemptively attach secret ref to Gateway resource

	return res, err
}

func GenerateGatewayCertName(gatewayName string) string {
	template := fmt.Sprintf("keyvault-gateway-cert-%s", gatewayName)
	if len(template) > 253 {
		template = template[:253]
	}

	return template
}
