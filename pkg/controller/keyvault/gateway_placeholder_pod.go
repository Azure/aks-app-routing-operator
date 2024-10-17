package keyvault

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	gatewayPlaceholderPodControllerName = controllername.New("gateway", "placeholder", "pod")
)

// GatewayPlaceholderPodReconciler reconciles a placeholder pod used to pull a Keyvault Cert into the user's cluster via
// Workload identity through the MSI attached to the Gateway's referenced ServiceAccount, which the pod uses. This way, when
// a request is made via the Kubelet to the CSI driver when the pod mounts the CSI driver volume, the Kubelet forwards
// the ServiceAccount token to the CSI driver, at which point the Keyvault Secrets Provider exchanges the ServiceAccount token
// for an AAD/MSI token, which it then uses to access the Keyvault and pull the secret. Once the secret has been pulled in and
// mounted by the placeholder pod, the placeholder pod is started
type GatewayPlaceholderPodReconciler struct {
	client client.Client
	cfg    config.Config
	events record.EventRecorder
}

func NewGatewayPlaceholderPodReconciler(mgr ctrl.Manager, conf config.Config) error {
	metrics.InitControllerMetrics(gatewayPlaceholderPodControllerName)
	if conf.DisableKeyvault {
		return nil
	}

	return gatewayPlaceholderPodControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(mgr).
			For(&gatewayv1.Gateway{}), mgr.GetLogger(),
	).Complete(&GatewayPlaceholderPodReconciler{
		client: mgr.GetClient(),
		cfg:    conf,
		events: mgr.GetEventRecorderFor("app-routing-operator"),
	})
}

func (g *GatewayPlaceholderPodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	res := ctrl.Result{}
	var err error

	// handle metrics
	defer func() {
		metrics.HandleControllerReconcileMetrics(gatewayPlaceholderPodControllerName, res, err)
	}()

	// set up logger
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return res, fmt.Errorf("creating logger for %s: %s", gatewaySecretProviderControllerName.String(), err)

	}
	logger = gatewaySecretProviderControllerName.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Name)

	gwObj := &gatewayv1.Gateway{}
	err = g.client.Get(ctx, req.NamespacedName, gwObj)

	if err != nil {
		return res, client.IgnoreNotFound(err)
	}

	// go through listeners to find kv options --> we need the client ID from the SA, plus the cert URI
	for _, listener := range gwObj.Spec.Listeners {
		userErr := validateTLSOptions(listener.TLS.Options)

	}

	return res, err
}
