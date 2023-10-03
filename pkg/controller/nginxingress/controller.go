package nginxingress

import (
	"context"
	"fmt"
	"time"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	finalizer = "approuting.kubernetes.azure.com/finalizer"
)

var (
	nginxIngressControllerReconcilerName = controllername.New("nginx", "ingress", "controller", "reconciler")
)

// nginxIngressControllerReconciler reconciles a NginxIngressController object
type nginxIngressControllerReconciler struct {
	client client.Client
	conf   *config.Config
}

// SetupWithManager sets up the controller with the Manager.
func SetupReconciler(conf *config.Config, mgr ctrl.Manager) error {
	return nginxIngressControllerReconcilerName.AddToController(
		ctrl.NewControllerManagedBy(mgr).For(&approutingv1alpha1.NginxIngressController{}),
		mgr.GetLogger(),
	).Complete(&nginxIngressControllerReconciler{client: mgr.GetClient(), conf: conf})
}

func (n *nginxIngressControllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	defer func() {
		metrics.HandleControllerReconcileMetrics(nginxIngressControllerReconcilerName, res, err)
	}()

	start := time.Now()
	lgr := log.FromContext(ctx, "nginxIngressController", req.NamespacedName)
	ctx = log.IntoContext(ctx, lgr)
	lgr.Info("starting to reconcile resources")
	defer lgr.Info("finished reconciling resources", "latencySec", time.Since(start).Seconds())

	var nginxIngressController approutingv1alpha1.NginxIngressController
	if err := n.client.Get(ctx, req.NamespacedName, &nginxIngressController); err != nil {
		if apierrors.IsNotFound(err) { // object was deleted, we clean up through finalizer
			lgr.Info("NginxIngressController not found")
			return ctrl.Result{}, nil
		}

		lgr.Error(err, "unable to fetch NginxIngressController")
		return ctrl.Result{}, err
	}

	// finalizer logic
	if !nginxIngressController.ObjectMeta.DeletionTimestamp.IsZero() { // object is being deleted
		// TODO: delete managed resources
		// TODO: is it better to delete managed resources here or ownership references?

		controllerutil.RemoveFinalizer(&nginxIngressController, finalizer)
		if err := n.client.Update(ctx, &nginxIngressController); err != nil {
			lgr.Error(err, "unable to remove finalizer")
			return ctrl.Result{}, fmt.Errorf("updating NginxIngressController to remove finalizer: %w", err)
		}
	}

	if !controllerutil.ContainsFinalizer(&nginxIngressController, finalizer) {
		controllerutil.AddFinalizer(&nginxIngressController, finalizer)
		if err := n.client.Update(ctx, &nginxIngressController); err != nil {
			lgr.Error(err, "unable to add finalizer")
			return ctrl.Result{}, fmt.Errorf("updating NginxIngressController to include finalizer: %w", err)
		}
	}

	if err := n.SetCollisionCount(ctx, &nginxIngressController); err != nil {
		lgr.Error(err, "unable to set collision count")
		return ctrl.Result{}, fmt.Errorf("setting collision count: %w", err)
	}

	for _, obj := range n.ManagedObjects(&nginxIngressController) {
		if err := util.Upsert(ctx, n.client, obj); err != nil {
			lgr.Error(err, "unable to upsert object", "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind, "namespace", obj.GetNamespace())
			return ctrl.Result{}, fmt.Errorf("upserting object: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func (n *nginxIngressControllerReconciler) ManagedObjects(nic *approutingv1alpha1.NginxIngressController) []client.Object {
	if nic == nil {
		return nil
	}

	nginxIngressCfg := &manifests.NginxIngressConfig{
		ControllerClass: "webapprouting.kubernetes.azure.com/nginx/" + nic.Name, // need to truncate and add to collision detection?
		ResourceName:    fmt.Sprintf("%s-%d", nic.Spec.ControllerName, nic.Status.CollisionCount),
		IcName:          nic.Spec.IngressClassName,
		ServiceConfig:   nil, // TODO: take in lb annotations
	}

	objs := []client.Object{}
	objs = append(objs, manifests.NginxIngressClass(n.conf, nic, nginxIngressCfg)...)
	objs = append(objs, manifests.NginxIngressControllerResources(n.conf, nic, nginxIngressCfg)...)
	return objs
}

func (n *nginxIngressControllerReconciler) SetCollisionCount(ctx context.Context, nic *approutingv1alpha1.NginxIngressController) error {
	lgr := log.FromContext(ctx)
	startingCollisionCount := nic.Status.CollisionCount

	for {
		collision, err := n.collides(ctx, nic)
		if err != nil {
			lgr.Error(err, "unable to determine collision")
			return fmt.Errorf("determining collision: %w", err)
		}

		if !collision {
			break
		}

		lgr.Info("collision detected, incrementing", "collisionCount", nic.Status.CollisionCount)
		nic.Status.CollisionCount++
	}

	if startingCollisionCount != nic.Status.CollisionCount {
		lgr.Info("setting new collision count", "collisionCount", nic.Status.CollisionCount, "startingCollisionCount", startingCollisionCount)
		if err := n.client.Status().Update(ctx, nic); err != nil {
			lgr.Error(err, "unable to update status")
			return fmt.Errorf("updating status: %w", err)
		}
	}

	return nil
}

func (n *nginxIngressControllerReconciler) collides(ctx context.Context, nic *approutingv1alpha1.NginxIngressController) (bool, error) {
	objs := n.ManagedObjects(nic)
	for _, obj := range objs {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())

		err := n.client.Get(ctx, client.ObjectKeyFromObject(obj), u)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}

		}

		if owner := util.FindOwnerKind(u.GetOwnerReferences(), nic.Kind); owner == nic.Name {
			continue
		}

		return true, nil
	}

	return false, nil
}
