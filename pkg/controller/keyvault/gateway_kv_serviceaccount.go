package keyvault

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
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
}

func NewKvServiceAccountReconciler(mgr ctrl.Manager) error {
	metrics.InitControllerMetrics(kvSaControllerName)

	return kvSaControllerName.AddToController(ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Gateway{}), mgr.GetLogger()).
		Complete(&KvServiceAccountReconciler{
			client: mgr.GetClient(),
			events: mgr.GetEventRecorderFor("app-routing-operator"),
		})

}

func (k *KvServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	res = ctrl.Result{}
	err = nil

	defer func() {
		metrics.HandleControllerReconcileMetrics(kvSaControllerName, res, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return
	}
	logger = kvSaControllerName.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	gwObj := &gatewayv1.Gateway{}
	if err = k.client.Get(ctx, req.NamespacedName, gwObj); err != nil {
		err = client.IgnoreNotFound(err)
		return
	}

	clientId, err := extractClientIdForManagedSa(gwObj)

	if err != nil {
		var userErr userError
		if errors.As(err, &userErr) {
			err = nil
			logger.Info("user error while extracting clientId from Gateway: %s", userErr.userMessage)
			k.events.Event(gwObj, Warning.String(), "InvalidInput", userErr.userMessage)
			return
		}
		logger.Error(err, "failed to extract clientId from Gateway object")
		return
	}

	if clientId == "" {
		return
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
		return
	}

	if existing.Annotations != nil && existing.Annotations[wiSaClientIdAnnotation] != "" {
		existingClientId := existing.Annotations[wiSaClientIdAnnotation]
		if existingClientId != clientId {
			errText := fmt.Sprintf("gateway specifies clientId %s but azure-app-routing-kv ServiceAccount already uses clientId %s", clientId, existingClientId)
			logger.Info(errText)
			k.events.Event(gwObj, Warning.String(), "InvalidInput", errText)
			return
		}
	}

	toCreate.Annotations = map[string]string{wiSaClientIdAnnotation: clientId}

	err = util.Upsert(ctx, k.client, toCreate)

	return
}

func extractClientIdForManagedSa(gwObj *gatewayv1.Gateway) (string, error) {
	var ret string

	if gwObj.Spec.GatewayClassName != istioGatewayClassName {
		return "", nil
	}

	if gwObj.Spec.Listeners == nil || len(gwObj.Spec.Listeners) == 0 {
		return "", nil
	}
	for _, listener := range gwObj.Spec.Listeners {
		if listener.TLS == nil || listener.TLS.Options == nil {
			continue
		}
		if listener.TLS.Options[clientIdTLSOption] != "" {
			if ret != "" && string(listener.TLS.Options[clientIdTLSOption]) != ret {
				return "", newUserError(fmt.Errorf("user specified multiple clientIds in one gateway resource"), "multiple unique clientIds specified. Please select one clientId to associate with the azure-app-routing-kv ServiceAccount")
			}
			ret = string(listener.TLS.Options[clientIdTLSOption])
		}
	}

	return ret, nil
}
