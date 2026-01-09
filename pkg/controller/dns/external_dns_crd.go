package dns

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ExternalDNSCRDControllerName = controllername.New("externaldns", "crd")

type ExternalDNSCRDController struct {
	config *config.Config
	client client.Client
	events record.EventRecorder
}

func newExternalDNSCRDController(manager ctrl.Manager, config config.Config) error {
	return ExternalDNSCRDControllerName.AddToController(ctrl.NewControllerManagedBy(manager).
		For(&v1alpha1.ExternalDNS{}).
		Owns(&appsv1.Deployment{}), manager.GetLogger()).
		Complete(&ExternalDNSCRDController{
			config: &config,
			client: manager.GetClient(),
			events: manager.GetEventRecorderFor("aks-app-routing-operator"),
		})
}

func (e *ExternalDNSCRDController) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	defer func() {
		metrics.HandleControllerReconcileMetrics(ExternalDNSCRDControllerName, res, err)
	}()

	// set up logger
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("creating logger: %w", err)
	}
	logger = ExternalDNSCRDControllerName.AddToLogger(logger).WithValues("namespace", req.Namespace, "name", req.Name)

	obj := &v1alpha1.ExternalDNS{}
	if err = e.client.Get(ctx, req.NamespacedName, obj); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("externaldns crd object not found, will ignore not found error")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get externaldns object")
		return ctrl.Result{}, err
	}

	// verify serviceaccount
	if _, err = util.GetServiceAccountWorkloadIdentityClientId(ctx, e.client, obj.GetInputServiceAccount(), obj.GetNamespace()); err != nil {
		var userErr util.UserError
		if errors.As(err, &userErr) {
			logger.Info("failed to verify service account due to user error, sending warning event: " + userErr.UserError())
			e.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateOrCreateExternalDNSResources", "failed serviceaccount verification: %s", userErr.UserError())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to verify service account")
		return ctrl.Result{}, err
	}

	manifestsConf, err := generateManifestsConf(e.config, obj)
	if err != nil {
		var userErr util.UserError
		if errors.As(err, &userErr) {
			logger.Info("failed to generate manifests config due to user error, sending warning event: " + userErr.UserError())
			e.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateOrCreateExternalDNSResources", userErr.UserError())
			return ctrl.Result{}, nil
		}
	}

	err = deployExternalDNSResources(ctx, e.client, manifestsConf, []metav1.OwnerReference{{
		APIVersion: obj.APIVersion,
		Controller: util.ToPtr(true),
		Kind:       obj.Kind,
		Name:       obj.Name,
		UID:        obj.UID,
	}})
	if err != nil {
		logger.Error(err, "failed to upsert externaldns resources")
		e.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateOrCreateExternalDNSResources", "failed to deploy external DNS resources: %s", err.Error())
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
