package spc

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

type secretProviderClassReconciler[objectType client.Object] struct {
	// config options
	name controllername.ControllerNamer

	// set during constructor
	client client.Client
	events record.EventRecorder
	config *config.Config
}

func (s *secretProviderClassReconciler[objectType]) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	defer func() {
		metrics.HandleControllerReconcileMetrics(s.name, result, retErr)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting logger from context: %w", err)
	}
	logger = s.name.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	logger.Info("getting Object")
	obj := *new(objectType)
	if err := s.client.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger = logger.WithValues("generation", obj.GetGeneration())

	// TODO: does this work?
	typeMeta, ok := obj.(metav1.TypeMeta)
	if !ok {
		return ctrl.Result{}, fmt.Errorf("object does not implement metav1.TypeMeta", obj)
	}

	spc := &secv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
			Kind:       "SecretProviderClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("keyvault-%s", obj.GetName()),
			Namespace: obj.GetNamespace(),
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: typeMeta.APIVersion,
				Controller: util.ToPtr(true),
				Kind:       typeMeta.Kind,
				Name:       obj.GetName(),
				UID:        obj.GetUID(),
			}},
		},
	}

	return ctrl.Result{}, nil
}
