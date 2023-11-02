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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
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
	collisionCountMu = keymutex.NewHashed(6) // 6 is the number of "buckets". It's not too big, not too small
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
		ctrl.NewControllerManagedBy(mgr).
			For(&approutingv1alpha1.NginxIngressController{}).
			Owns(&appsv1.Deployment{}),
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

	lockKey := nginxIngressController.Spec.ControllerNamePrefix
	collisionCountMu.LockKey(lockKey)
	defer collisionCountMu.UnlockKey(lockKey)
	if err := n.SetCollisionCount(ctx, &nginxIngressController); err != nil {
		lgr.Error(err, "unable to set collision count")
		return ctrl.Result{}, fmt.Errorf("setting collision count: %w", err)
	}

	resources := n.ManagedResources(&nginxIngressController)
	if resources == nil {
		return ctrl.Result{}, fmt.Errorf("unable to get managed resources")
	}

	managedRes, err := n.ReconcileResource(ctx, &nginxIngressController, resources)
	defer func() {
		n.updateStatus(&nginxIngressController, resources.Deployment, resources.IngressClass, managedRes)
		if statusErr := n.client.Status().Update(ctx, &nginxIngressController); statusErr != nil {
			lgr.Error(statusErr, "unable to update NginxIngressController status")
			if err == nil {
				err = statusErr
			}
		}
	}()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling resource: %w", err)
	}

	return ctrl.Result{}, nil
}

// ReconcileResource reconciles the NginxIngressController resources returning a list of managed resources.
func (n *nginxIngressControllerReconciler) ReconcileResource(ctx context.Context, nic *approutingv1alpha1.NginxIngressController, res *manifests.NginxResources) ([]approutingv1alpha1.ManagedObjectReference, error) {
	if nic == nil {
		return nil, errors.New("nginxIngressController cannot be nil")
	}

	start := time.Now()
	lgr := log.FromContext(ctx, "nginxIngressController", nic.GetName())
	ctx = log.IntoContext(ctx, lgr)
	lgr.Info("starting to reconcile resource")
	defer lgr.Info("finished reconciling resource", "latencySec", time.Since(start).Seconds())

	var managedResourceRefs []approutingv1alpha1.ManagedObjectReference
	for _, obj := range res.Objects() {
		if err := util.Upsert(ctx, n.client, obj); err != nil {
			lgr.Error(err, "unable to upsert object", "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind, "namespace", obj.GetNamespace())
			return nil, fmt.Errorf("upserting object: %w", err)
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

	return managedResourceRefs, nil
}

func (n *nginxIngressControllerReconciler) ManagedResources(nic *approutingv1alpha1.NginxIngressController) *manifests.NginxResources {
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

	res := manifests.GetNginxResources(n.conf, nginxIngressCfg)
	owner := manifests.GetOwnerRefs(nic, true)
	for _, obj := range res.Objects() {
		if managedByUs(obj) {
			obj.SetOwnerReferences(owner)
		}
	}

	return res
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

	res := n.ManagedResources(nic)
	if res == nil {
		return collisionNone, fmt.Errorf("getting managed objects")
	}

	for _, obj := range res.Objects() {
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
			lgr.Info("the nginxIngressController owns this resource")
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

// updateStatus updates the status of the NginxIngressController resource. If a nil controller Deployment or IngressClass is passed, the status is defaulted for those fields if they are not already set.
func (n *nginxIngressControllerReconciler) updateStatus(nic *approutingv1alpha1.NginxIngressController, controllerDeployment *appsv1.Deployment, ic *netv1.IngressClass, managedResourceRefs []approutingv1alpha1.ManagedObjectReference) {
	if managedResourceRefs != nil {
		nic.Status.ManagedResourceRefs = managedResourceRefs
	}

	if ic == nil || ic.CreationTimestamp.IsZero() {
		nic.SetCondition(metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeIngressClassReady,
			Status:  metav1.ConditionUnknown,
			Reason:  "IngressClassUnknown",
			Message: "IngressClass is unknown",
		})
	} else {
		nic.SetCondition(metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeIngressClassReady,
			Status:  "True",
			Reason:  "IngressClassIsReady",
			Message: "Ingress Class is up-to-date ",
		})
	}

	// default conditions
	if controllerDeployment == nil || controllerDeployment.CreationTimestamp.IsZero() {
		nic.SetCondition(metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeControllerAvailable,
			Status:  metav1.ConditionUnknown,
			Reason:  "ControllerDeploymentUnknown",
			Message: "Controller deployment is unknown",
		})
		nic.SetCondition(metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeProgressing,
			Status:  metav1.ConditionUnknown,
			Reason:  "ControllerDeploymentUnknown",
			Message: "Controller deployment is unknown",
		})
	} else {
		nic.Status.ControllerReadyReplicas = controllerDeployment.Status.ReadyReplicas
		nic.Status.ControllerAvailableReplicas = controllerDeployment.Status.AvailableReplicas
		nic.Status.ControllerUnavailableReplicas = controllerDeployment.Status.UnavailableReplicas
		nic.Status.ControllerReplicas = controllerDeployment.Status.Replicas

		for _, cond := range controllerDeployment.Status.Conditions {
			switch cond.Type {
			case appsv1.DeploymentProgressing:
				n.updateStatusControllerProgressing(nic, cond)
			case appsv1.DeploymentAvailable:
				n.updateStatusControllerAvailable(nic, cond)
			}
		}
	}

	controllerAvailable := nic.GetCondition(approutingv1alpha1.ConditionTypeControllerAvailable)
	icAvailable := nic.GetCondition(approutingv1alpha1.ConditionTypeIngressClassReady)
	if controllerAvailable != nil && icAvailable != nil && controllerAvailable.Status == metav1.ConditionTrue && icAvailable.Status == metav1.ConditionTrue {
		nic.SetCondition(metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeAvailable,
			Status:  metav1.ConditionTrue,
			Reason:  "ControllerIsAvailable",
			Message: "Controller Deployment has minimum availability and IngressClass is up-to-date",
		})
	} else {
		nic.SetCondition(metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeAvailable,
			Status:  metav1.ConditionFalse,
			Reason:  "ControllerIsNotAvailable",
			Message: "Controller Deployment does not have minimum availability or IngressClass is not up-to-date",
		})
	}
}

func (n *nginxIngressControllerReconciler) updateStatusControllerAvailable(nic *approutingv1alpha1.NginxIngressController, availableCondition appsv1.DeploymentCondition) {
	if availableCondition.Type != appsv1.DeploymentAvailable {
		return
	}

	var cond metav1.Condition
	switch availableCondition.Status {
	case corev1.ConditionTrue:
		cond = metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeControllerAvailable,
			Status:  metav1.ConditionTrue,
			Reason:  "ControllerDeploymentAvailable",
			Message: "Controller Deployment is available",
		}
	case corev1.ConditionFalse:
		cond = metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeControllerAvailable,
			Status:  metav1.ConditionFalse,
			Reason:  "ControllerDeploymentNotAvailable",
			Message: "Controller Deployment is not available",
		}
	default:
		cond = metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeControllerAvailable,
			Status:  metav1.ConditionUnknown,
			Reason:  "ControllerDeploymentUnknown",
			Message: "Controller Deployment is in an unknown state",
		}
	}

	nic.SetCondition(cond)
}

func (n *nginxIngressControllerReconciler) updateStatusControllerProgressing(nic *approutingv1alpha1.NginxIngressController, progressingCondition appsv1.DeploymentCondition) {
	if progressingCondition.Type != appsv1.DeploymentProgressing {
		return
	}

	var cond metav1.Condition
	switch progressingCondition.Status {
	case corev1.ConditionTrue:
		cond = metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeProgressing,
			Status:  metav1.ConditionTrue,
			Reason:  "ControllerDeploymentProgressing",
			Message: "Controller Deployment has successfully progressed",
		}
	case corev1.ConditionFalse:
		cond = metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeProgressing,
			Status:  metav1.ConditionFalse,
			Reason:  "ControllerDeploymentNotProgressing",
			Message: "Controller Deployment has failed to progress",
		}
	default:
		cond = metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeProgressing,
			Status:  metav1.ConditionUnknown,
			Reason:  "ControllerDeploymentProgressingUnknown",
			Message: "Controller Deployment progress is unknown",
		}
	}

	nic.SetCondition(cond)
}

func managedByUs(obj client.Object) bool {
	for k, v := range manifests.GetTopLevelLabels() {
		if obj.GetLabels()[k] != v {
			return false
		}
	}

	return true
}
