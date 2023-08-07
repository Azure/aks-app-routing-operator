package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	AppRoutingReconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "app_routing_reconcile_total",
		Help: "Total number of reconciliations per controller",
	}, []string{"controller", "result"})

	AppRoutingReconcileErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "app_routing_reconcile_errors_total",
		Help: "Total number of reconciliation errors per controller",
	}, []string{"controller"})
)

const (
	LabelError        = "error"
	LabelRequeueAfter = "requeue_after"
	LabelRequeue      = "requeue"
	LabelSuccess      = "success"
)

func init() {
	metrics.Registry.MustRegister(AppRoutingReconcileErrors, AppRoutingReconcileTotal)
}

// HandleControllerReconcileMetrics is meant to be called within a defer for each controller.
// This lets us put all the metric handling in one place, rather than duplicating it in every controller
func HandleControllerReconcileMetrics(controllerName string, result ctrl.Result, err error) {
	switch {
	// apierrors.IsNotFound is ignored by controllers so this should too
	case err != nil && !apierrors.IsNotFound(err):
		AppRoutingReconcileTotal.WithLabelValues(controllerName, LabelError).Inc()
		AppRoutingReconcileErrors.WithLabelValues(controllerName).Inc()
	case result.RequeueAfter > 0:
		AppRoutingReconcileTotal.WithLabelValues(controllerName, LabelRequeueAfter).Inc()
	case result.Requeue:
		AppRoutingReconcileTotal.WithLabelValues(controllerName, LabelRequeue).Inc()
	default:
		AppRoutingReconcileTotal.WithLabelValues(controllerName, LabelSuccess).Inc()
	}
}

func InitControllerMetrics(controllerName string) {
	AppRoutingReconcileTotal.WithLabelValues(controllerName, LabelError).Add(0)
	AppRoutingReconcileTotal.WithLabelValues(controllerName, LabelRequeueAfter).Add(0)
	AppRoutingReconcileTotal.WithLabelValues(controllerName, LabelRequeue).Add(0)
	AppRoutingReconcileTotal.WithLabelValues(controllerName, LabelSuccess).Add(0)

	AppRoutingReconcileErrors.WithLabelValues(controllerName).Add(0)
}
