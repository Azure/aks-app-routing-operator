package dns

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var externalDNSCrdControllerName = controllername.New("external", "dns", "crd")

func NewExternalDNSCRDController(mgr ctrl.Manager, conf config.Config) error {
	metrics.InitControllerMetrics(externalDNSCrdControllerName)

	return externalDNSCrdControllerName.AddToController(ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ExternalDNS{}), mgr.GetLogger()).Complete(
		&ExternalDNSCRDReconciler{
			client: mgr.GetClient(),
			config: conf,
			events: mgr.GetEventRecorderFor("aks-app-routing-operator"),
		},
	)
}

type ExternalDNSCRDReconciler struct {
	client client.Client
	config config.Config
	events record.EventRecorder
}

func (e *ExternalDNSCRDReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, retErr error) {
	defer func() {
		metrics.HandleControllerReconcileMetrics(externalDNSCrdControllerName, res, retErr)
	}()

	// set up logger
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("creating logger: %w", err)
	}
	logger = externalDNSCrdControllerName.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	externalDNSObj := &v1alpha1.ExternalDNS{}
	if err = e.client.Get(ctx, req.NamespacedName, externalDNSObj); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("retrieving externalDNS object: %s", err)
		}
		return ctrl.Result{}, nil
	}

	// construct externalDNSConfig
	externalDnsConf := manifests.ExternalDnsConfig{}

	return ctrl.Result{}, nil
}
