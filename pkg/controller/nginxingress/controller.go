package nginxingress

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/keymutex"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	controllerClassMaxLen = 250
)

var (
	nginxIngressControllerReconcilerName = controllername.New("nginx", "ingress", "controller", "reconciler")

	// collisionCountMu is used to prevent multiple nginxIngressController resources from determining their collisionCount at the same time. We use
	// a hashed key mutex because collisions can only occur when the nginxIngressController resources have the same spec.ControllerName field. This
	// is the field used to key into this mutex.
	collisionCountMu = keymutex.NewHashed(10) // 10 is the number of "buckets". It's not too big, not too small todo: add more
)

// nginxIngressControllerReconciler reconciles a NginxIngressController object
type nginxIngressControllerReconciler struct {
	client        client.Client
	conf          *config.Config
	interval      time.Duration
	retryInterval time.Duration
}

// NewReconciler sets up the controller with the Manager.
func NewReconciler(conf *config.Config, mgr ctrl.Manager) error {
	metrics.InitControllerMetrics(nginxIngressControllerReconcilerName)

	reconciler := &nginxIngressControllerReconciler{client: mgr.GetClient(), conf: conf}

	// start event-based reconciler
	if err := nginxIngressControllerReconcilerName.AddToController(
		ctrl.NewControllerManagedBy(mgr).For(&approutingv1alpha1.NginxIngressController{}),
		mgr.GetLogger(),
	).Complete(reconciler); err != nil {
		return err
	}

	// start periodic reconciliation loop reconciler
	return mgr.Add(reconciler)
}

// Start starts the NginxIngressController reconciler to continuously reconcile NginxIngressController resources existing in the cluster.
// This reconciles any NginxIngressController resources that existed before the operator started and makes our upgrade story work. It also
// reconciles / reverts any changes made to our managed resources by users.
func (n *nginxIngressControllerReconciler) Start(ctx context.Context) error {
	lgr := nginxIngressControllerReconcilerName.AddToLogger(log.FromContext(ctx))
	lgr.Info("starting reconciler")
	defer lgr.Info("stopping reconciler")

	interval := time.Nanosecond // run immediately when starting up
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(util.Jitter(interval, 0.3)):
		}

		// list all NginxIngressController resources and reconcile them
		var list approutingv1alpha1.NginxIngressControllerList
		if err := n.client.List(ctx, &list); err != nil {
			lgr.Error(err, "unable to list NginxIngressController resources")
			interval = n.retryInterval
			continue
		}

		for _, nic := range list.Items {
			// TODO: maybe use multiple go routines?
			// TODO: handle metrics
			if err := n.ReconcileResource(ctx, &nic); err != nil {
				lgr.Error(err, "unable to reconcile NginxIngressController resource", "namespace", nic.Namespace, "name", nic.Name)
				interval = n.retryInterval
				continue
			}
		}

		interval = n.interval
	}

}

// Reconcile immediately reconciles the NginxIngressController resource based on events
func (n *nginxIngressControllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	defer func() {
		metrics.HandleControllerReconcileMetrics(nginxIngressControllerReconcilerName, res, err)
	}()

	lgr := log.FromContext(ctx, "nginxIngressController", req.NamespacedName)
	var nginxIngressController approutingv1alpha1.NginxIngressController
	if err := n.client.Get(ctx, req.NamespacedName, &nginxIngressController); err != nil {
		if apierrors.IsNotFound(err) { // object was deleted
			lgr.Info("NginxIngressController not found")
			return ctrl.Result{}, nil
		}

		lgr.Error(err, "unable to fetch NginxIngressController")
		return ctrl.Result{}, err
	}

	// no need for a finalizer, we use owner references to clean everything up since everything is in-cluster

	if err := n.ReconcileResource(ctx, &nginxIngressController); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling resource: %w", err)
	}

	return ctrl.Result{}, nil
}

// ReconcileResource reconciles the NginxIngressController resource
func (n *nginxIngressControllerReconciler) ReconcileResource(ctx context.Context, nic *approutingv1alpha1.NginxIngressController) error {
	if nic == nil {
		return nil
	}

	start := time.Now()
	lgr := log.FromContext(ctx, "nginxIngressController", nic.GetName())
	ctx = log.IntoContext(ctx, lgr)
	lgr.Info("starting to reconcile resource")
	defer lgr.Info("finished reconciling resource", "latencySec", time.Since(start).Seconds())

	lockKey := nic.Spec.ControllerName
	collisionCountMu.LockKey(lockKey)
	defer collisionCountMu.UnlockKey(lockKey)

	if err := n.SetCollisionCount(ctx, nic); err != nil {
		lgr.Error(err, "unable to set collision count")
		return fmt.Errorf("setting collision count: %w", err)
	}

	for _, obj := range n.ManagedObjects(nic) {
		if err := util.Upsert(ctx, n.client, obj); err != nil {
			lgr.Error(err, "unable to upsert object", "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind, "namespace", obj.GetNamespace())
			return fmt.Errorf("upserting object: %w", err)
		}
	}

	return nil
}

func (n *nginxIngressControllerReconciler) ManagedObjects(nic *approutingv1alpha1.NginxIngressController) []client.Object {
	if nic == nil {
		return nil
	}

	// TODO: should use controller name instead of ingress class name, it better represents the resource
	// TODO: need to specify limits for length of resource name. that makes this easy
	// really would like some way of guaranteeing uniqueness for cc. Just doesn't work well for collisions
	// after truncating
	cc := "webapprouting.kubernetes.azure.com/nginx/" + url.PathEscape(nic.Name)
	suffix := strconv.Itoa(int(nic.Status.CollisionCount))
	if len(cc)+len(suffix) > controllerClassMaxLen {
		cc = cc[:controllerClassMaxLen-len(suffix)]
	}
	cc = cc + suffix

	nginxIngressCfg := &manifests.NginxIngressConfig{
		ControllerClass: cc,
		ResourceName:    fmt.Sprintf("%s-%d", nic.Spec.ControllerName, nic.Status.CollisionCount),
		IcName:          nic.Spec.IngressClassName,
		ServiceConfig:   nil, // TODO: take in lb annotations
	}

	var objs []client.Object
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

func (n *nginxIngressControllerReconciler) NeedLeaderElection() bool {
	return true
}
