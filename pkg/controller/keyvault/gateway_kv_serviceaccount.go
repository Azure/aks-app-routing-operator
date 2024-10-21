package keyvault

import (
	"context"

	cfg "github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var kvSaControllerName = controllername.New("gateway", "keyvault", "serviceaccount")

// KvServiceAccountReconciler reconciles the default "azure-app-routing-kv" ServiceAccount in each namespace where users
// create a Gateway resource with the Keyvault cert TLS option. Users can tie this ServiceAccount to their own MSI via
// workload identity through annotations on their Gateway resources.
type KvServiceAccountReconciler struct {
	client client.Client
	events record.EventRecorder
	config cfg.Config
}

func NewKvServiceAccountReconciler(mgr ctrl.Manager, config cfg.Config) error {
	metrics.InitControllerMetrics(kvSaControllerName)

	return kvSaControllerName.AddToController(ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Gateway{}), mgr.GetLogger()).
		Complete(&KvServiceAccountReconciler{
			client: mgr.GetClient(),
			events: mgr.GetEventRecorderFor("app-routing-operator"),
			config: config,
		})

}

func (k *KvServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	res := ctrl.Result{}
	var err error

	defer func() {
		metrics.HandleControllerReconcileMetrics(kvSaControllerName, res, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = kvSaControllerName.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	gwObj := &gatewayv1.Gateway{}
	if err = k.client.Get(ctx, req.NamespacedName, gwObj); err != nil {
		return res, client.IgnoreNotFound(err)
	}

	toCreate := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      appRoutingSaName,
			Namespace: req.Namespace,
		},
	}

	logger.Info("checking for existing ServiceAccount")

	existing := &corev1.ServiceAccount{}
	err = k.client.Get(ctx, types.NamespacedName{Name: toCreate.Name, Namespace: toCreate.Namespace}, existing)
	if client.IgnoreNotFound(err) != nil {
		logger.Error(err, "failed to fetch existing app routing serviceaccount")
		return res, err
	}
	if err != nil {
		// it was found -- we need to grab existing MSI if there is one
	}

	return res, err
}
