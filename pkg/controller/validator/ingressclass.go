package validator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/informer"
	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type uniqueIngressClassControllerValidator struct {
	controller string
	icInfomer  informer.IngressClass
	client     client.Client
	logger     logr.Logger
}

func NewUniqueIngressClassControllerValidator(manager ctrl.Manager, controller string, icInformer informer.IngressClass) error {
	return builder.WebhookManagedBy(manager).
		For(&netv1.IngressClass{}).
		WithValidator(&uniqueIngressClassControllerValidator{
			controller: controller,
			icInfomer:  icInformer,
			client:     manager.GetClient(),
			logger:     manager.GetLogger().WithName("ingressClassControllerValidator"),
		}).
		Complete()
}

func (u *uniqueIngressClassControllerValidator) validate(ctx context.Context, obj runtime.Object) error {
	start := time.Now()
	u.logger.Info("starting to validate ingress class controller")
	defer func() {
		u.logger.Info("finished validating ingress class controller", "latencySec", time.Since(start).Seconds())
	}()

	ic, ok := obj.(*netv1.IngressClass)
	if !ok {
		return fmt.Errorf("expected ingressClass but got %T", obj)
	}

	if ic.Spec.Controller != u.controller {
		return nil
	}

	if u.icInfomer == nil {
		return errors.New("ingressClass informer is nil")
	}

	// is this enough to handle race conditions? what if someone tries to create two ingressclasses at the same time?
	if u.icInfomer.Informer().HasSynced() {
		return errors.New("not ready to handle request yet, informer hasn't synced yet")
	}

	// verify that no other ingress classes are using this controller
	ics, err := u.icInfomer.ByController(u.controller)
	if err != nil {
		return err
	}

	for _, existingIc := range ics {
		if existingIc.GetDeletionTimestamp() != nil {
			continue
		}

		if existingIc.GetUID() != ic.GetUID() {
			return fmt.Errorf("another ingress class %s/%s already consumes controller %s", existingIc.GetNamespace(), existingIc.GetName(), u.controller)
		}
	}

	return nil
}

func (u *uniqueIngressClassControllerValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	return u.validate(ctx, obj)
}

func (u *uniqueIngressClassControllerValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	return u.validate(ctx, newObj)
}

func (u *uniqueIngressClassControllerValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}
