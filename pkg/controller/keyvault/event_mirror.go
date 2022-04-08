// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
)

// EventMirror copies events published to pod resources by the Keyvault CSI driver into ingress events.
// This allows users to easily determine why a certificate might be missing for a given ingress.
type EventMirror struct {
	client client.Client
	events record.EventRecorder
}

func NewEventMirror(manager ctrl.Manager, conf *config.Config) error {
	if conf.DisableKeyvault {
		return nil
	}
	e := &EventMirror{
		client: manager.GetClient(),
		events: manager.GetEventRecorderFor("aks-app-routing-operator"),
	}
	return ctrl.
		NewControllerManagedBy(manager).
		For(&corev1.Event{}).
		WithEventFilter(e.newPredicates()).
		Complete(e)
}

func (e *EventMirror) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = logger.WithName("eventMirror")

	event := &corev1.Event{}
	err = e.client.Get(ctx, req.NamespacedName, event)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	// Filter to include only keyvault mounting errors
	if event.InvolvedObject.Kind != "Pod" ||
		event.Reason != "FailedMount" ||
		!strings.HasPrefix(event.InvolvedObject.Name, "keyvault-") ||
		!strings.Contains(event.Message, "keyvault") {
		return ctrl.Result{}, nil
	}

	// Get the owner (pod)
	pod := &corev1.Pod{}
	pod.Name = event.InvolvedObject.Name
	pod.Namespace = event.InvolvedObject.Namespace
	err = e.client.Get(ctx, client.ObjectKeyFromObject(pod), pod)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if pod.Annotations == nil {
		return ctrl.Result{}, nil
	}

	// Get the owner (ingress)
	ingress := &netv1.Ingress{}
	ingress.Namespace = pod.Namespace
	ingress.Name = pod.Annotations["aks.io/ingress-owner"]
	err = e.client.Get(ctx, client.ObjectKeyFromObject(ingress), ingress)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	// Publish to the service also if ingress is owned by a service
	if len(ingress.OwnerReferences) > 0 && ingress.OwnerReferences[0].Kind == "Service" {
		svc := &corev1.Service{}
		svc.Namespace = pod.Namespace
		svc.Name = ingress.OwnerReferences[0].Name
		err = e.client.Get(ctx, client.ObjectKeyFromObject(svc), svc)
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		if err != nil {
			return ctrl.Result{}, err
		}
		e.events.Event(svc, "Warning", "FailedMount", event.Message)
	}

	e.events.Event(ingress, "Warning", "FailedMount", event.Message)
	return ctrl.Result{}, nil
}

func (e *EventMirror) newPredicates() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
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
