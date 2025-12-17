package metrics

import (
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
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

	DefaultDomainClientCallsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "app_routing_default_domain_client_calls_total",
		Help: "Total number of calls to the default domain service",
	}, []string{"result"})

	DefaultDomainClientErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "app_routing_default_domain_client_errors_total",
		Help: "Total number of errors from the default domain service",
	})
)

const (
	LabelError        = "error"
	LabelRequeueAfter = "requeue_after"
	LabelRequeue      = "requeue"
	LabelSuccess      = "success"
	LabelNotFound     = "not_found"
)

func init() {
	metrics.Registry.MustRegister(AppRoutingReconcileErrors, AppRoutingReconcileTotal, DefaultDomainClientCallsTotal, DefaultDomainClientErrors)
}

// HandleControllerReconcileMetrics is meant to be called within a defer for each controller.
// This lets us put all the metric handling in one place, rather than duplicating it in every controller
func HandleControllerReconcileMetrics(controllerName controllername.ControllerNamer, result ctrl.Result, err error) {
	cn := controllerName.MetricsName()

	switch {
	// apierrors.IsNotFound is ignored by controllers so this should too
	case err != nil && !apierrors.IsNotFound(err):
		AppRoutingReconcileTotal.WithLabelValues(cn, LabelError).Inc()
		AppRoutingReconcileErrors.WithLabelValues(cn).Inc()
	case result.RequeueAfter > 0:
		AppRoutingReconcileTotal.WithLabelValues(cn, LabelRequeueAfter).Inc()
	case result.Requeue:
		AppRoutingReconcileTotal.WithLabelValues(cn, LabelRequeue).Inc()
	default:
		AppRoutingReconcileTotal.WithLabelValues(cn, LabelSuccess).Inc()
	}
}

// HandleWebhookHandlerMetrics is meant to be called within a defer for each webhook handler func.
func HandleWebhookHandlerMetrics(controllerName controllername.ControllerNamer, result admission.Response, err error) {
	cn := controllerName.MetricsName()

	switch {
	case err != nil && !apierrors.IsNotFound(err):
		AppRoutingReconcileTotal.WithLabelValues(cn, LabelError).Inc()
		AppRoutingReconcileErrors.WithLabelValues(cn).Inc()
	case result.Allowed == false:
		AppRoutingReconcileTotal.WithLabelValues(cn, LabelError).Inc()
		AppRoutingReconcileErrors.WithLabelValues(cn).Inc()
	default:
		AppRoutingReconcileTotal.WithLabelValues(cn, LabelSuccess).Inc()
	}
}

func InitControllerMetrics(controllerName controllername.ControllerNamer) {
	cn := controllerName.MetricsName()
	AppRoutingReconcileTotal.WithLabelValues(cn, LabelError).Add(0)
	AppRoutingReconcileTotal.WithLabelValues(cn, LabelRequeueAfter).Add(0)
	AppRoutingReconcileTotal.WithLabelValues(cn, LabelRequeue).Add(0)
	AppRoutingReconcileTotal.WithLabelValues(cn, LabelSuccess).Add(0)

	AppRoutingReconcileErrors.WithLabelValues(cn).Add(0)
}
