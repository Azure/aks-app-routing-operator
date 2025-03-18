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

var ClusterExternalDNSControllerName = controllername.New("cluster", "externaldns", "crd")

func NewClusterExternalDNSController(mgr ctrl.Manager, config *config.Config) error {
	return ClusterExternalDNSControllerName.AddToController(
		ctrl.NewControllerManagedBy(mgr).
			For(&v1alpha1.ClusterExternalDNS{}).
			For(&v1alpha1.ExternalDNS{}).
			Owns(&appsv1.Deployment{}).
			Owns(&corev1.ConfigMap{}), mgr.GetLogger()).Complete(
		&ClusterExternalDNSController{
			config: config,
			client: mgr.GetClient(),
			events: mgr.GetEventRecorderFor("aks-app-routing-operator"),
		})

}

type ClusterExternalDNSController struct {
	config *config.Config
	client client.Client
	events record.EventRecorder
}

func (c *ClusterExternalDNSController) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	// set up metrics
	defer func() {
		metrics.HandleControllerReconcileMetrics(ClusterExternalDNSControllerName, res, err)
	}()

	// set up logger
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("setting up logger: %s", err)
	}

	logger = ClusterExternalDNSControllerName.AddToLogger(logger).WithValues("namespace", req.Namespace, "name", req.Name)

	// get the ClusterExternalDNS object
	obj := &v1alpha1.ClusterExternalDNS{}
	if err = c.client.Get(ctx, req.NamespacedName, obj); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("ClusterExternalDNS object not found, will ignore not found error")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting ClusterExternalDNS object: %w", err)
	}

	// verify serviceaccount
	if _, err = util.GetServiceAccountAndVerifyWorkloadIdentity(ctx, c.client, obj.GetInputServiceAccount(), obj.GetResourceNamespace()); err != nil {
		var userErr util.UserError
		if errors.As(err, &userErr) {
			logger.Info("failed to verify service account due to user error, sending warning event: " + userErr.UserError())
			c.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateOrCreateExternalDNSResources", "failed serviceaccount verification: %s", userErr.UserError())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to verify service account")
		return ctrl.Result{}, err
	}

	manifestsConf, err := generateManifestsConf(c.config, obj)
	if err != nil {
		var userErr util.UserError
		if errors.As(err, &userErr) {
			logger.Info("failed to generate manifests config due to user error, sending warning event: " + userErr.UserError())
			c.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateOrCreateExternalDNSResources", userErr.UserError())
			return ctrl.Result{}, nil
		}
	}

	err = deployExternalDNSResources(ctx, c.client, manifestsConf, []metav1.OwnerReference{{
		APIVersion: obj.APIVersion,
		Controller: util.ToPtr(true),
		Kind:       obj.Kind,
		Name:       obj.Name,
		UID:        obj.UID,
	}})

	if err != nil {
		logger.Error(err, "failed to upsert externaldns resources")
		c.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateOrCreateExternalDNSResources", "failed to deploy external DNS resources: %s", err.Error())
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
