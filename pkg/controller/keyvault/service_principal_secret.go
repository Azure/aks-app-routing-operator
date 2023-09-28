// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/go-logr/logr"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

var (
	servicePrincipalSecretControllerName = controllername.New("keyvault", "service", "principal", "secret")
)

// ServicePrincipalSecretController manages a generic secret that contains the service principal creds for accessing the keyvault when using a service-principal cluster
// Keyvault secrets referenced by each secret provider class managed by IngressSecretProviderClassReconciler.
//
// This is necessitated by the Keyvault CSI implementation, which requires at least one mount
// in order to start mirroring the Keyvault values into corresponding Kubernetes secret(s).
type ServicePrincipalSecretController struct {
	client         client.Client
	config         *config.Config
	ingressManager IngressManager
}

func NewServicePrincipalSecretController(manager ctrl.Manager, conf *config.Config, ingressManager IngressManager) error {
	metrics.InitControllerMetrics(servicePrincipalSecretControllerName)
	lgr := manager.GetLogger()
	if !conf.EnableServicePrincipal {
		lgr.Info("Service Principal Secret Controller disabled")
		return nil
	}
	lgr.Info("Service Principal Secret Controller enabled")
	return servicePrincipalSecretControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&secv1.SecretProviderClass{}), manager.GetLogger(),
	).Complete(&ServicePrincipalSecretController{client: manager.GetClient(), config: conf, ingressManager: ingressManager})
}

type ServicePrincipalAzureJSON struct {
	ClientId     string `json:"appId"`
	ClientSecret string `json:"password"`
}

func (s *ServicePrincipalSecretController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}

	// do metrics
	defer func() {
		//placing this call inside a closure allows for result and err to be bound after Reconcile executes
		//this makes sure they have the proper value
		//just calling defer metrics.HandleControllerReconcileMetrics(controllerName, result, err) would bind
		//the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(servicePrincipalSecretControllerName, result, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return result, err
	}
	logger = placeholderPodControllerName.AddToLogger(logger)

	// read /etc/kubernetes/azure.json
	// get service principal creds
	// create a secret with the creds
	logger.Info("reading azure.json from mount")
	// check file exists
	if _, err := os.Stat("/etc/kubernetes/azure.json"); os.IsNotExist(err) {
		return result, fmt.Errorf("azure.json not found, check service principal azure.json is mounted on the operator pod")
	}

	spc := &secv1.SecretProviderClass{}
	err = s.client.Get(ctx, req.NamespacedName, spc)
	if err != nil {
		return result, client.IgnoreNotFound(err)
	}
	logger = logger.WithValues("name", spc.Name, "namespace", spc.Namespace, "generation", spc.Generation)

	sps := &corev1.Secret{}

	n := types.NamespacedName{Namespace: sps.Namespace, Name: sps.Name}
	_ = s.client.Get(ctx, n, sps)
	if len(sps.Data) > 0 {
		logger.Info("service principal secret already exists")
		return result, nil
	}

	logger.Info("service principal secret does not exist, creating")
	sps = &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spc.Name,
			Namespace: spc.Namespace,
		},
		Data: map[string][]byte{},
	}
	logger.Info("reconciling service principal secret")

	if err = util.Upsert(ctx, s.client, sps); err != nil {
		return result, err
	}

	return result, nil
}
