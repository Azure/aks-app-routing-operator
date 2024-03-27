// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	ingressSecretProviderControllerName = controllername.New("keyvault", "ingress", "secret", "provider")
)

// IngressSecretProviderClassReconciler manages a SecretProviderClass for each ingress resource that
// references a Keyvault certificate. The SPC is used to mirror the Keyvault values into a k8s secret
// so that it can be used by the ingress controller.
type IngressSecretProviderClassReconciler struct {
	client         client.Client
	events         record.EventRecorder
	config         *config.Config
	ingressManager IngressManager
}

func NewIngressSecretProviderClassReconciler(manager ctrl.Manager, conf *config.Config, ingressManager IngressManager) error {
	metrics.InitControllerMetrics(ingressSecretProviderControllerName)
	if conf.DisableKeyvault {
		return nil
	}
	return ingressSecretProviderControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&netv1.Ingress{}), manager.GetLogger(),
	).Complete(&IngressSecretProviderClassReconciler{
		client:         manager.GetClient(),
		events:         manager.GetEventRecorderFor("aks-app-routing-operator"),
		config:         conf,
		ingressManager: ingressManager,
	})
}

func (i *IngressSecretProviderClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}

	// do metrics
	defer func() {
		//placing this call inside a closure allows for result and err to be bound after Reconcile executes
		//this makes sure they have the proper value
		//just calling defer metrics.HandleControllerReconcileMetrics(controllerName, result, err) would bind
		//the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(ingressSecretProviderControllerName, result, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return result, err
	}
	logger = ingressSecretProviderControllerName.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	logger.Info("getting Ingress")
	ing := &netv1.Ingress{}
	err = i.client.Get(ctx, req.NamespacedName, ing)
	if err != nil {
		return result, client.IgnoreNotFound(err)
	}
	logger = logger.WithValues("name", ing.Name, "namespace", ing.Namespace, "generation", ing.Generation)

	spc := &secv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
			Kind:       "SecretProviderClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("keyvault-%s", ing.Name),
			Namespace: ing.Namespace,
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: ing.APIVersion,
				Controller: util.ToPtr(true),
				Kind:       ing.Kind,
				Name:       ing.Name,
				UID:        ing.UID,
			}},
		},
	}
	logger = logger.WithValues("spc", spc.Name)

	// Checking if we manage the ingress. All false cases without an error are assumed that we don't manage it
	var isManaged bool
	if isManaged, err = i.ingressManager.IsManaging(ing); err != nil {
		return result, fmt.Errorf("determining if ingress is managed: %w", err)
	}

	if isManaged {
		var upsertSPC bool

		if upsertSPC, err = buildSPC(ing, spc, i.config); err != nil {
			var userErr userError
			if errors.As(err, &userErr) {
				logger.Info(fmt.Sprintf("failed to build secret provider class for ingress with error: %s. sending warning event"), userErr.Error())
				i.events.Eventf(ing, "Warning", "InvalidInput", "error while processing Keyvault reference: %s", userErr.UserError())
				return result, nil
			}
			return result, err
		}

		if upsertSPC {
			logger.Info("reconciling secret provider class for ingress")
			if err = util.Upsert(ctx, i.client, spc); err != nil {
				i.events.Eventf(ing, "Warning", "FailedUpdateOrCreateSPC", "error while creating or updating SecretProviderClass needed to pull Keyvault reference: %s", err.Error())
			}
			return result, err
		}
	}

	logger.Info("cleaning unused managed spc for ingress")
	logger.Info("getting secret provider class for ingress")

	toCleanSPC := &secv1.SecretProviderClass{}

	if err = i.client.Get(ctx, client.ObjectKeyFromObject(spc), toCleanSPC); err != nil {
		return result, client.IgnoreNotFound(err)
	}

	if manifests.HasTopLevelLabels(toCleanSPC.Labels) {
		logger.Info("removing secret provider class for ingress")
		err = i.client.Delete(ctx, toCleanSPC)
		return result, client.IgnoreNotFound(err)
	}

	return result, nil
}
