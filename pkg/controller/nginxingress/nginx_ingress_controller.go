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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/keymutex"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	controllerClassMaxLen = 250
)

var (
	icCollisionErr   = errors.New("collision on the IngressClass")
	maxCollisionsErr = errors.New("max collisions reached")
)

var (
	nginxIngressControllerReconcilerName = controllername.New("nginx", "ingress", "controller", "reconciler")

	// collisionCountMu is used to prevent multiple nginxIngressController resources from determining their collisionCount at the same time. We use
	// a hashed key mutex because collisions can only occur when the nginxIngressController resources have the same spec.ControllerNamePrefix field.
	// This is the field used to key into this mutex. Using this mutex prevents a race condition of multiple nginxIngressController resources attempting
	// to determine their collisionCount at the same time then both attempting to create and take ownership of the same resources.
	collisionCountMu = keymutex.NewHashed(6) // 6 is the number of "buckets". It's not too big, not too small
)

// nginxIngressControllerReconciler reconciles a NginxIngressController object
type nginxIngressControllerReconciler struct {
	client                    client.Client
	conf                      *config.Config
	events                    record.EventRecorder
	defaultNicControllerClass string
}

// NewReconciler sets up the controller with the Manager.
func NewReconciler(conf *config.Config, mgr ctrl.Manager, defaultIngressClassControllerClass string) error {
	metrics.InitControllerMetrics(nginxIngressControllerReconcilerName)

	reconciler := &nginxIngressControllerReconciler{
		client:                    mgr.GetClient(),
		conf:                      conf,
		events:                    mgr.GetEventRecorderFor("aks-app-routing-operator"),
		defaultNicControllerClass: defaultIngressClassControllerClass,
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
	start := time.Now()
	lgr := log.FromContext(ctx, "nginxIngressController", req.NamespacedName)
	ctx = log.IntoContext(ctx, lgr)
	lgr.Info("reconciling NginxIngressController")
	defer lgr.Info("finished reconciling resource", "latencySec", time.Since(start).String())

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
	lgr = lgr.WithValues("generation", nginxIngressController.Generation)
	ctx = log.IntoContext(ctx, lgr)

	var managedRes []approutingv1alpha1.ManagedObjectReference = nil
	var controllerDeployment *appsv1.Deployment = nil
	var ingressClass *netv1.IngressClass = nil

	lockKey := nginxIngressController.Spec.ControllerNamePrefix
	collisionCountMu.LockKey(lockKey)
	defer collisionCountMu.UnlockKey(lockKey)

	lgr.Info("determining collision count")
	startingCollisionCount := nginxIngressController.Status.CollisionCount
	newCollisionCount, collisionCountErr := n.GetCollisionCount(ctx, &nginxIngressController)
	if collisionCountErr == nil && newCollisionCount != startingCollisionCount {
		nginxIngressController.Status.CollisionCount = newCollisionCount
		if err := n.client.Status().Update(ctx, &nginxIngressController); err != nil {
			lgr.Error(err, "unable to update collision count status")
			return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
		}

		return ctrl.Result{Requeue: true}, nil
	}
	defer func() { // defer is before checking err so that we can update status even if there is an error
		lgr.Info("updating status")
		n.updateStatus(&nginxIngressController, controllerDeployment, ingressClass, managedRes, collisionCountErr)
		if statusErr := n.client.Status().Update(ctx, &nginxIngressController); statusErr != nil {
			lgr.Error(statusErr, "unable to update NginxIngressController status")
			if err == nil {
				err = statusErr
			}
		}
	}()
	if collisionCountErr != nil {
		if isUnreconcilableError(collisionCountErr) {
			lgr.Info("unreconcilable collision count")
			return ctrl.Result{RequeueAfter: time.Minute}, nil // requeue in case cx fixes the unreconcilable reason
		}

		lgr.Error(collisionCountErr, "unable to get determine collision count")
		return ctrl.Result{}, fmt.Errorf("determining collision count: %w", collisionCountErr)
	}

	lgr.Info("calculating managed resources")
	resources := n.ManagedResources(&nginxIngressController)
	if resources == nil {
		return ctrl.Result{}, fmt.Errorf("unable to get managed resources")
	}
	controllerDeployment = resources.Deployment
	ingressClass = resources.IngressClass

	if &nginxIngressController.Spec.DefaultSSLCertificate != nil {
		lgr.Info("validating default ssl certificate secret")
		if manifests.IsValidDefaultSSLCertSecret(&nginxIngressController.Spec.DefaultSSLCertificate) {
			lgr.Info("Field in DefaultSSLCert secret left empty: default ssl cert will not be set")
		}
	}

	lgr.Info("reconciling managed resources")
	managedRes, err = n.ReconcileResource(ctx, &nginxIngressController, resources)
	if err != nil {
		lgr.Error(err, "unable to reconcile resource")
		return ctrl.Result{}, fmt.Errorf("reconciling resource: %w", err)
	}
	if replicas := resources.Deployment.Spec.Replicas; replicas != nil {
		lgr.Info(fmt.Sprintf("nginx deployment targets %d replicas", *replicas), "replicas", *replicas)
	}

	return ctrl.Result{}, nil
}

// ReconcileResource reconciles the NginxIngressController resources returning a list of managed resources.
func (n *nginxIngressControllerReconciler) ReconcileResource(ctx context.Context, nic *approutingv1alpha1.NginxIngressController, res *manifests.NginxResources) ([]approutingv1alpha1.ManagedObjectReference, error) {
	if nic == nil {
		return nil, errors.New("nginxIngressController cannot be nil")
	}
	if res == nil {
		return nil, errors.New("resources cannot be nil")
	}

	lgr := log.FromContext(ctx)

	var managedResourceRefs []approutingv1alpha1.ManagedObjectReference
	for _, obj := range res.Objects() {
		// TODO: upsert works fine for now but we want to set annotations exactly on the nginx service, we should use an alternative strategy for that in the future

		if err := util.Upsert(ctx, n.client, obj); err != nil {
			// publish an event so cx can see things like their policy is preventing us from creating a resource
			n.events.Eventf(nic, corev1.EventTypeWarning, "EnsuringManagedResourcesFailed", "Failed to ensure managed resource %s %s/%s: %s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName(), err.Error())

			lgr.Error(err, "unable to upsert object", "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind, "namespace", obj.GetNamespace())
			return nil, fmt.Errorf("upserting object: %w", err)
		}

		if manifests.HasTopLevelLabels(obj.GetLabels()) {
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

	nginxIngressCfg := ToNginxIngressConfig(nic, n.defaultNicControllerClass)
	if nginxIngressCfg == nil {
		return nil
	}

	res := manifests.GetNginxResources(n.conf, nginxIngressCfg)
	owner := manifests.GetOwnerRefs(nic, true)
	for _, obj := range res.Objects() {
		if manifests.HasTopLevelLabels(obj.GetLabels()) {
			obj.SetOwnerReferences(owner)
		}
	}

	return res
}

func (n *nginxIngressControllerReconciler) GetCollisionCount(ctx context.Context, nic *approutingv1alpha1.NginxIngressController) (int32, error) {
	lgr := log.FromContext(ctx)

	// it's not this fn's job to overwrite the collision count, so we revert any changes we make
	startingCollisionCount := nic.Status.CollisionCount
	defer func() {
		nic.Status.CollisionCount = startingCollisionCount
	}()

	for {
		lgr = lgr.WithValues("collisionCount", nic.Status.CollisionCount)

		if nic.Status.CollisionCount == approutingv1alpha1.MaxCollisions {
			lgr.Info("max collisions reached")
			return 0, maxCollisionsErr
		}

		collision, err := n.collides(ctx, nic)
		if err != nil {
			lgr.Error(err, "unable to determine collision")
			return 0, fmt.Errorf("determining collision: %w", err)
		}

		if collision == collisionIngressClass {
			// rare since our webhook guards against it, but it's possible
			lgr.Info("ingress class collision")
			return 0, icCollisionErr
		}

		if collision == collisionNone {
			break
		}

		lgr.Info("reconcilable collision detected, incrementing")
		nic.Status.CollisionCount++
	}

	return nic.Status.CollisionCount, nil
}

func (n *nginxIngressControllerReconciler) collides(ctx context.Context, nic *approutingv1alpha1.NginxIngressController) (collision, error) {
	lgr := log.FromContext(ctx)

	// ignore any collisions on default nic for migration story. Need to acquire ownership of old app routing resources
	if IsDefaultNic(nic) {
		return collisionNone, nil
	}

	res := n.ManagedResources(nic)
	if res == nil {
		return collisionNone, fmt.Errorf("getting managed objects")
	}

	for _, obj := range res.Objects() {
		lgr := lgr.WithValues("kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName(), "namespace", obj.GetNamespace())

		// if we won't own the resource, we don't care about collisions.
		// this is most commonly used for namespaces since we shouldn't own
		// namespaces
		if !manifests.HasTopLevelLabels(obj.GetLabels()) {
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
			lgr.Info("collision is with an IngressClass")
			return collisionIngressClass, nil
		}

		return collisionOther, nil
	}

	lgr.Info("no collisions detected")
	return collisionNone, nil
}

// updateStatus updates the status of the NginxIngressController resource. If a nil controller Deployment or IngressClass is passed, the status is defaulted for those fields if they are not already set.
func (n *nginxIngressControllerReconciler) updateStatus(nic *approutingv1alpha1.NginxIngressController, controllerDeployment *appsv1.Deployment, ic *netv1.IngressClass, managedResourceRefs []approutingv1alpha1.ManagedObjectReference, err error) {
	n.updateStatusManagedResourceRefs(nic, managedResourceRefs)

	n.updateStatusIngressClass(nic, ic)

	// default conditions
	if controllerDeployment == nil || controllerDeployment.CreationTimestamp.IsZero() {
		n.updateStatusNilDeployment(nic)
	} else {
		for _, cond := range controllerDeployment.Status.Conditions {
			switch cond.Type {
			case appsv1.DeploymentProgressing:
				n.updateStatusControllerProgressing(nic, cond)
			case appsv1.DeploymentAvailable:
				n.updateStatusControllerAvailable(nic, cond)
			}
		}
	}

	n.updateStatusControllerReplicas(nic, controllerDeployment)
	n.updateStatusAvailable(nic)

	// error checking at end to take precedence over other conditions
	n.updateStatusFromError(nic, err)
}

func (n *nginxIngressControllerReconciler) updateStatusManagedResourceRefs(nic *approutingv1alpha1.NginxIngressController, managedResourceRefs []approutingv1alpha1.ManagedObjectReference) {
	if managedResourceRefs == nil {
		return
	}

	nic.Status.ManagedResourceRefs = managedResourceRefs
}

func (n *nginxIngressControllerReconciler) updateStatusIngressClass(nic *approutingv1alpha1.NginxIngressController, ic *netv1.IngressClass) {
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
}

func (n *nginxIngressControllerReconciler) updateStatusNilDeployment(nic *approutingv1alpha1.NginxIngressController) {
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
}

func (n *nginxIngressControllerReconciler) updateStatusControllerReplicas(nic *approutingv1alpha1.NginxIngressController, deployment *appsv1.Deployment) {
	if deployment == nil {
		return
	}

	nic.Status.ControllerReadyReplicas = deployment.Status.ReadyReplicas
	nic.Status.ControllerAvailableReplicas = deployment.Status.AvailableReplicas
	nic.Status.ControllerUnavailableReplicas = deployment.Status.UnavailableReplicas
	nic.Status.ControllerReplicas = deployment.Status.Replicas
}

func (n *nginxIngressControllerReconciler) updateStatusAvailable(nic *approutingv1alpha1.NginxIngressController) {
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

func (n *nginxIngressControllerReconciler) updateStatusFromError(nic *approutingv1alpha1.NginxIngressController, err error) {
	if errors.Is(err, icCollisionErr) {
		nic.SetCondition(metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeProgressing,
			Status:  metav1.ConditionFalse,
			Reason:  "IngressClassCollision",
			Message: "IngressClass already exists and is not owned by this controller",
		})
		n.events.Event(nic, corev1.EventTypeWarning, "IngressClassCollision", "IngressClass already exists and is not owned by this controller. Change the spec.IngressClassName to an unused IngressClass name in a new NginxIngressController.")
	}
	if errors.Is(err, maxCollisionsErr) {
		nic.SetCondition(metav1.Condition{
			Type:    approutingv1alpha1.ConditionTypeProgressing,
			Status:  metav1.ConditionFalse,
			Reason:  "TooManyCollisions",
			Message: "Too many collisions with existing resources",
		})
		n.events.Event(nic, corev1.EventTypeWarning, "TooManyCollisions", "Too many collisions with existing resources. Change the spec.ControllerNamePrefix to something more unique in a new NginxIngressController.")
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

func isUnreconcilableError(err error) bool {
	return errors.Is(err, icCollisionErr) || errors.Is(err, maxCollisionsErr)
}

func ToNginxIngressConfig(nic *approutingv1alpha1.NginxIngressController, defaultNicControllerClass string) *manifests.NginxIngressConfig {
	if nic == nil {
		return nil
	}

	cc := "approuting.kubernetes.azure.com/" + url.PathEscape(nic.Name)
	if len(cc) > controllerClassMaxLen {
		cc = cc[:controllerClassMaxLen]
	}

	resourceName := fmt.Sprintf("%s-%d", nic.Spec.ControllerNamePrefix, nic.Status.CollisionCount)

	if IsDefaultNic(nic) {
		cc = defaultNicControllerClass
		resourceName = DefaultNicResourceName
	}

	nginxIng := &manifests.NginxIngressConfig{
		ControllerClass: cc,
		ResourceName:    resourceName,
		IcName:          nic.Spec.IngressClassName,
		ServiceConfig: &manifests.ServiceConfig{
			Annotations: nic.Spec.LoadBalancerAnnotations,
		},
	}

	DefaultSSLCert := &nic.Spec.DefaultSSLCertificate
	if DefaultSSLCert.Secret.Name != "" && DefaultSSLCert.Secret.Namespace != "" {
		nginxIng.DefaultSSLCertificate = &approutingv1alpha1.DefaultSSLCertificate{
			Secret: nic.Spec.DefaultSSLCertificate.Secret,
		}
	}

	return nginxIng
}
