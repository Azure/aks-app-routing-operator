// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"errors"
	"fmt"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

var (
	nginxSecretProviderControllerName = controllername.New("keyvault", "nginx", "secret", "provider")
)

// NginxSecretProviderClassReconciler manages a SecretProviderClass for each nginx ingress controller that
// has a Keyvault URI in its DefaultSSLCertificate field. The SPC is used to mirror the Keyvault values into
// a k8s secret so that it can be used by the CRD controller.
type NginxSecretProviderClassReconciler struct {
	client client.Client
	events record.EventRecorder
	config *config.Config
}

func NewNginxSecretProviderClassReconciler(manager ctrl.Manager, conf *config.Config) error {
	metrics.InitControllerMetrics(nginxSecretProviderControllerName)
	if conf.DisableKeyvault {
		return nil
	}
	return nginxSecretProviderControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&approutingv1alpha1.NginxIngressController{}), manager.GetLogger(),
	).Complete(&NginxSecretProviderClassReconciler{
		client: manager.GetClient(),
		events: manager.GetEventRecorderFor("aks-app-routing-operator"),
		config: conf,
	})
}

func (i *NginxSecretProviderClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}

	// do metrics
	defer func() {
		//placing this call inside a closure allows for result and err to be bound after Reconcile executes
		//this makes sure they have the proper value
		//just calling defer metrics.HandleControllerReconcileMetrics(controllerName, result, err) would bind
		//the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(nginxSecretProviderControllerName, result, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return result, err
	}
	logger = nginxSecretProviderControllerName.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	logger.Info("getting Nginx Ingress")
	nic := &approutingv1alpha1.NginxIngressController{}
	err = i.client.Get(ctx, req.NamespacedName, nic)
	if err != nil {
		return result, client.IgnoreNotFound(err)
	}
	logger = logger.WithValues("name", nic.Name, "generation", nic.Generation)

	spc := &secv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
			Kind:       "SecretProviderClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultNginxCertName(nic),
			Namespace: i.config.NS,
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: nic.APIVersion,
				Controller: util.ToPtr(true),
				Kind:       nic.Kind,
				Name:       nic.Name,
				UID:        nic.UID,
			}},
		},
	}
	logger = logger.WithValues("spc", spc.Name)
	logger.Info("building spc and upserting if managed with labels")

	if shouldDeploySpc(nic) {
		spcConf := SPCConfig{
			ClientId:        i.config.MSIClientID,
			TenantId:        i.config.TenantID,
			KeyvaultCertUri: *nic.Spec.DefaultSSLCertificate.KeyVaultURI,
			Name:            DefaultNginxCertName(nic),
			Cloud:           i.config.Cloud,
		}
		err = buildSPC(spc, spcConf)
		if err != nil {
			var userErr userError
			if errors.As(err, &userErr) {
				logger.Info(fmt.Sprintf("failed to build secret provider class for nginx ingress controller with error: %s. sending warning event", userErr.Error()))
				i.events.Eventf(nic, "Warning", "InvalidInput", "error while processing Keyvault reference: %s", userErr.UserError())
				return result, nil
			}

			logger.Error(err, fmt.Sprintf("failed to build secret provider class for nginx ingress controller with error: %s.", err.Error()))
			return result, err
		}

		logger.Info("reconciling secret provider class for ingress")
		err = util.Upsert(ctx, i.client, spc)
		if err != nil {
			i.events.Eventf(nic, "Warning", "FailedUpdateOrCreateSPC", "error while creating or updating SecretProviderClass needed to pull Keyvault reference: %s", err.Error())
			logger.Error(err, fmt.Sprintf("failed to upsert secret provider class for nginx ingress class with error: %s.", err.Error()))
		}
		return result, err
	} else {
		logger.Info("spc is either not managed or key vault uri was removed")
	}

	logger.Info("cleaning unused spc if ingress is managed")
	logger.Info("getting secret provider class for ingress")

	toCleanSPC := &secv1.SecretProviderClass{}

	err = i.client.Get(ctx, client.ObjectKeyFromObject(spc), toCleanSPC)
	if err != nil {
		return result, client.IgnoreNotFound(err)
	}

	if manifests.HasTopLevelLabels(toCleanSPC.Labels) {
		logger.Info("removing secret provider class for ingress")
		err = i.client.Delete(ctx, toCleanSPC)
		return result, client.IgnoreNotFound(err)
	} else {
		logger.Info("spc was not managed so spc was not removed")
	}

	return result, nil
}
