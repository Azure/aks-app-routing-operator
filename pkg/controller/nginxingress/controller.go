package nginxingress

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	collisionCountMu = keymutex.NewHashed(6) // 6 is the number of "buckets". It's not too big, not too small todo: add more details
)

// collision represents the type of collision that occurred when reconciling an nginxIngressController resource.
// will be used to help determine the way we should handle the collision.
type collision int

const (
	collisionNone collision = iota
	collisionIngressClass
	collisionOther
)

// nginxIngressControllerReconciler reconciles a NginxIngressController object
type nginxIngressControllerReconciler struct {
	client client.Client
	conf   *config.Config
}

// NewReconciler sets up the controller with the Manager.
func NewReconciler(conf *config.Config, mgr ctrl.Manager) error {
	metrics.InitControllerMetrics(nginxIngressControllerReconcilerName)

	reconciler := &nginxIngressControllerReconciler{
		client: mgr.GetClient(),
		conf:   conf,
	}

	if err := nginxIngressControllerReconcilerName.AddToController(
		ctrl.NewControllerManagedBy(mgr).For(&approutingv1alpha1.NginxIngressController{}),
		mgr.GetLogger(),
	).Complete(reconciler); err != nil {
		return err
	}

	return nil
}

func (n *nginxIngressControllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	lgr := log.FromContext(ctx, "nginxIngressController", req.NamespacedName, "reconciler", "event")
	ctx = log.IntoContext(ctx, lgr)

	defer func() {
		metrics.HandleControllerReconcileMetrics(nginxIngressControllerReconcilerName, res, err)
	}()

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

	lockKey := nic.Spec.ControllerNamePrefix
	collisionCountMu.LockKey(lockKey)
	defer collisionCountMu.UnlockKey(lockKey)

	if err := n.SetCollisionCount(ctx, nic); err != nil {
		lgr.Error(err, "unable to set collision count")
		return fmt.Errorf("setting collision count: %w", err)
	}

	var managedResourceRefs []approutingv1alpha1.ManagedObjectReference
	for _, obj := range n.ManagedObjects(nic) {
		if err := util.Upsert(ctx, n.client, obj); err != nil {
			lgr.Error(err, "unable to upsert object", "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind, "namespace", obj.GetNamespace())
			return fmt.Errorf("upserting object: %w", err)
		}

		if managedByUs(obj) {
			managedResourceRefs = append(managedResourceRefs, approutingv1alpha1.ManagedObjectReference{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
				Kind:      obj.GetObjectKind().GroupVersionKind().Kind,
				APIGroup:  obj.GetObjectKind().GroupVersionKind().Group,
			})
		}
	}

	lgr.Info("updating ManagedResourceRefs status")
	nic.Status.ManagedResourceRefs = managedResourceRefs
	if err := n.client.Status().Update(ctx, nic); err != nil {
		lgr.Error(err, "unable to update status")
		return fmt.Errorf("updating status: %w", err)
	}

	return nil
}

func (n *nginxIngressControllerReconciler) ManagedObjects(nic *approutingv1alpha1.NginxIngressController) []client.Object {
	if nic == nil {
		return nil
	}

	cc := "webapprouting.kubernetes.azure.com/nginx/" + url.PathEscape(nic.Name)

	// it's impossible for this to happen because we enforce nic.Name to be less than 101 characters
	if len(cc) > controllerClassMaxLen {
		cc = cc[:controllerClassMaxLen]
	}

	nginxIngressCfg := &manifests.NginxIngressConfig{
		ControllerClass: cc,
		ResourceName:    fmt.Sprintf("%s-%d", nic.Spec.ControllerNamePrefix, nic.Status.CollisionCount),
		IcName:          nic.Spec.IngressClassName,
		ServiceConfig:   nil, // TODO: take in lb annotations
	}

	var objs []client.Object
	objs = append(objs, manifests.NginxIngressClass(n.conf, nic, nginxIngressCfg)...)
	objs = append(objs, manifests.NginxIngressControllerResources(n.conf, nic, nginxIngressCfg)...)

	owner := manifests.GetOwnerRefs(nic)
	for _, obj := range objs {
		obj.SetOwnerReferences(owner)
	}

	return objs
}

func (n *nginxIngressControllerReconciler) SetCollisionCount(ctx context.Context, nic *approutingv1alpha1.NginxIngressController) error {
	lgr := log.FromContext(ctx)
	startingCollisionCount := nic.Status.CollisionCount

	// there's a limit to how many times we should try to find the collision count, we don't want to put too much stress on api server
	// TODO: we should set a condition when hit + jitter retry interval
	for i := 0; i < 10; i++ {
		collision, err := n.collides(ctx, nic)
		if err != nil {
			lgr.Error(err, "unable to determine collision")
			return fmt.Errorf("determining collision: %w", err)
		}

		if collision == collisionIngressClass {
			lgr.Info("ingress class collision")
			meta.SetStatusCondition(&nic.Status.Conditions, metav1.Condition{
				Type:               approutingv1alpha1.ConditionTypeIngressClassReady,
				Status:             "Collision",
				ObservedGeneration: nic.Generation,
				Message:            fmt.Sprintf("IngressClass %s already exists in the cluster and isn't owned by this resource. Delete the IngressClass or recreate this resource with a different spec.IngressClass field.", nic.Spec.IngressClassName),
				Reason:             "IngressClassCollision",
			})
			if err := n.client.Status().Update(ctx, nic); err != nil {
				lgr.Error(err, "unable to update status")
				return fmt.Errorf("updating status with IngressClass collision")
			}

			return nil // this isn't an error, it's caused by a race condition involving our webhook
		}

		if collision == collisionNone {
			break
		}

		lgr.Info("reconcilable collision detected, incrementing", "collisionCount", nic.Status.CollisionCount)
		nic.Status.CollisionCount++

		if i == 9 {
			return errors.New("too many collisions")
		}
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

func (n *nginxIngressControllerReconciler) collides(ctx context.Context, nic *approutingv1alpha1.NginxIngressController) (collision, error) {
	lgr := log.FromContext(ctx)

	objs := n.ManagedObjects(nic)
	for _, obj := range objs {
		lgr := lgr.WithValues("kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName(), "namespace", obj.GetNamespace())

		// if we won't own the resource, we don't care about collisions.
		// this is most commonly used for namespaces since we shouldn't own
		// namespaces
		if !managedByUs(obj) {
			lgr.Info("skipping collision check because we don't own the resource")
			continue
		}

		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())

		err := n.client.Get(ctx, client.ObjectKeyFromObject(obj), u)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}

			return collisionNone, fmt.Errorf("getting object: %w", err)
		}

		if owner := util.FindOwnerKind(u.GetOwnerReferences(), nic.Kind); owner == nic.Name {
			continue
		}

		lgr.Info("collision detected")
		if obj.GetObjectKind().GroupVersionKind().Kind == "IngressClass" {
			return collisionIngressClass, nil
		}

		return collisionOther, nil
	}

	lgr.Info("no collisions detected")
	return collisionNone, nil
}

func managedByUs(obj client.Object) bool {
	for k, v := range manifests.GetTopLevelLabels() {
		if obj.GetLabels()[k] != v {
			return false
		}
	}

	return true
}
