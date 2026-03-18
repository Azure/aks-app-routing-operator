// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

var eventMirrorControllerName = controllername.New("keyvault", "event", "mirror")

const (
	involvedObjectKindField         = "involvedObject.kind"
	eventReasonFailedMount          = "FailedMount"
	eventReasonMountRotationFailed  = "MountRotationFailed"
	eventReasonSecretRotationFailed = "SecretRotationFailed"
	eventReasonFailedToCreateSecret = "FailedToCreateSecret"
	eventKindPod                    = "Pod"
)

// keyVaultFailureReasons is the set of event reasons emitted by the secrets-store-csi-driver
// and the kubelet that indicate a failure to pull or rotate secrets from Key Vault.
var keyVaultFailureReasons = map[string]bool{
	eventReasonFailedMount:          true,
	eventReasonMountRotationFailed:  true,
	eventReasonSecretRotationFailed: true,
	eventReasonFailedToCreateSecret: true,
}


// EventMirror copies events published to pod resources by the Keyvault CSI driver into ingress events.
// This allows users to easily determine why a certificate might be missing for a given ingress.
type EventMirror struct {
	client client.Client
	events record.EventRecorder
}

func NewEventMirror(manager ctrl.Manager, conf *config.Config) error {
	metrics.InitControllerMetrics(eventMirrorControllerName)
	if conf.DisableKeyvault {
		return nil
	}
	e := &EventMirror{
		client: manager.GetClient(),
		events: manager.GetEventRecorderFor("aks-app-routing-operator"),
	}
	return eventMirrorControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&corev1.Event{}).
			WithEventFilter(e.newPredicates()), manager.GetLogger(),
	).Complete(e)
}

func (e *EventMirror) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}

	// do metrics
	defer func() {
		// placing this call inside a closure allows for result and err to be bound after Reconcile executes
		// this makes sure they have the proper value
		// just calling defer metrics.HandleControllerReconcileMetrics(controllerName, result, err) would bind
		// the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(eventMirrorControllerName, result, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return result, err
	}
	logger = eventMirrorControllerName.AddToLogger(logger)

	logger.Info("getting event", "name", req.Name, "namespace", req.Namespace)
	event := &corev1.Event{}
	err = e.client.Get(ctx, req.NamespacedName, event)
	if err != nil {
		return result, client.IgnoreNotFound(err)
	}
	logger = logger.WithValues("generation", event.Generation)

	// Defensive check: the predicate should have already filtered this, but guard
	// here too so a direct Reconcile call (e.g. in tests) can't bypass the filter.
	if !isKeyVaultMountingError(event) {
		logger.Info("ignoring event, not keyvault mounting error")
		return result, nil
	}
	logger.Info("keyvault secret failure event",
		"reason", event.Reason,
		"message", event.Message,
		"involvedObject", event.InvolvedObject.Name,
		"source", event.Source.Component,
		"count", event.Count,
		"firstTime", event.FirstTimestamp,
		"lastTime", event.LastTimestamp,
	)

	// Get the owner (pod)
	podName := event.InvolvedObject.Name
	podNamespace := event.InvolvedObject.Namespace
	logger.Info("getting owner placeholder pod", "name", podName, "namespace", podNamespace)
	pod := &corev1.Pod{}
	pod.Name = podName
	pod.Namespace = podNamespace
	err = e.client.Get(ctx, client.ObjectKeyFromObject(pod), pod)
	if err != nil {
		return result, client.IgnoreNotFound(err)
	}
	if pod.Annotations == nil {
		logger.Info("ignoring event, pod has no annotations")
		return result, nil
	}

	// Get the owner (ingress)
	ingressName := pod.Annotations[ingressOwnerAnnotation]
	if ingressName == "" {
		logger.Info("ignoring event, pod has no ingress owner")
		return result, nil
	}

	ingressNamespace := pod.Namespace
	logger.Info("getting owner ingress", "name", ingressName, "namespace", ingressNamespace)
	ingress := &netv1.Ingress{}
	ingress.Namespace = ingressNamespace
	ingress.Name = ingressName
	err = e.client.Get(ctx, client.ObjectKeyFromObject(ingress), ingress)
	if err != nil {
		return result, client.IgnoreNotFound(err)
	}

	// Publish to the service also if ingress is owned by a service
	if name := util.FindOwnerKind(ingress.OwnerReferences, "Service"); name != "" {
		logger.Info("getting owner service", "name", name, "namespace", pod.Namespace)
		svcNamespace := pod.Namespace
		svc := &corev1.Service{}
		svc.Namespace = svcNamespace
		svc.Name = name
		err = e.client.Get(ctx, client.ObjectKeyFromObject(svc), svc)
		if err == nil {
			logger.Info("publishing keyvault failure warning event to service", "service", svc.Name, "namespace", svc.Namespace, "reason", event.Reason)
			e.events.Event(svc, corev1.EventTypeWarning, event.Reason, event.Message)
		}
		if err != nil && !k8serrors.IsNotFound(err) {
			return result, fmt.Errorf("getting owner service: %w", err)
		}
	}

	logger.Info("publishing keyvault failure warning event to ingress", "ingress", ingress.Name, "namespace", ingress.Namespace, "reason", event.Reason)
	e.events.Event(ingress, corev1.EventTypeWarning, event.Reason, event.Message)
	return result, nil
}

func (e *EventMirror) newPredicates() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			ev, ok := e.Object.(*corev1.Event)
			if !ok {
				return false
			}
			return isKeyVaultMountingError(ev)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

func isKeyVaultMountingError(event *corev1.Event) bool {
	if event == nil {
		return false
	}

	return event.InvolvedObject.Kind == eventKindPod &&
		keyVaultFailureReasons[event.Reason] &&
		strings.HasPrefix(event.InvolvedObject.Name, "keyvault-") &&
		strings.Contains(event.Message, "keyvault")
}
