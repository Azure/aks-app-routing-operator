// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	kvcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
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
			Labels:    ing.Labels,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: ing.APIVersion,
				Controller: util.BoolPtr(true),
				Kind:       ing.Kind,
				Name:       ing.Name,
				UID:        ing.UID,
			}},
		},
	}
	logger = logger.WithValues("spc", spc.Name)
	ok, err := i.buildSPC(ing, spc)
	if err != nil {
		logger.Info("failed to build secret provider class for ingress, user input invalid. sending warning event")
		i.events.Eventf(ing, "Warning", "InvalidInput", "error while processing Keyvault reference: %s", err)
		return result, nil
	}
	if ok {
		logger.Info("reconciling secret provider class for ingress")
		err = util.Upsert(ctx, i.client, spc)
		return result, err
	}

	logger.Info("getting secret provider class for ingress")
	err = i.client.Get(ctx, client.ObjectKeyFromObject(spc), spc)
	if err != nil {
		return result, client.IgnoreNotFound(err)
	}

	if len(spc.Labels) != 0 && manifests.HasRequiredLabels(spc.Labels, manifests.GetTopLevelLabels()) {
		logger.Info("removing secret provider class for ingress")
		err = i.client.Delete(ctx, spc)
		return result, client.IgnoreNotFound(err)
	}

	return result, nil
}

func (i *IngressSecretProviderClassReconciler) buildSPC(ing *netv1.Ingress, spc *secv1.SecretProviderClass) (bool, error) {
	if ing.Spec.IngressClassName == nil || ing.Annotations == nil {
		return false, nil
	}

	if len(spc.Labels) == 0 || !(manifests.HasRequiredLabels(spc.Labels, manifests.GetTopLevelLabels())) {
		return false, nil
	}

	managed := i.ingressManager.IsManaging(ing)
	if !managed {
		return false, nil
	}

	certURI := ing.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"]
	if certURI == "" {
		return false, nil
	}

	uri, err := url.Parse(certURI)
	if err != nil {
		return false, err
	}
	vaultName := strings.Split(uri.Host, ".")[0]
	chunks := strings.Split(uri.Path, "/")
	if len(chunks) < 3 {
		return false, fmt.Errorf("invalid secret uri: %s", certURI)
	}
	secretName := chunks[2]
	p := map[string]interface{}{
		"objectName": secretName,
		"objectType": "secret",
	}
	if len(chunks) > 3 {
		p["objectVersion"] = chunks[3]
	}

	params, err := json.Marshal(p)
	if err != nil {
		return false, err
	}
	objects, err := json.Marshal(map[string]interface{}{"array": []string{string(params)}})
	if err != nil {
		return false, err
	}

	spc.Spec = secv1.SecretProviderClassSpec{
		Provider: secv1.Provider("azure"),
		SecretObjects: []*secv1.SecretObject{{
			SecretName: fmt.Sprintf("keyvault-%s", ing.Name),
			Type:       "kubernetes.io/tls",
			Data: []*secv1.SecretObjectData{
				{
					ObjectName: secretName,
					Key:        "tls.key",
				},
				{
					ObjectName: secretName,
					Key:        "tls.crt",
				},
			},
		}},
		// https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/getting-started/usage/#create-your-own-secretproviderclass-object
		Parameters: map[string]string{
			"keyvaultName":           vaultName,
			"useVMManagedIdentity":   "true",
			"userAssignedIdentityID": i.config.MSIClientID,
			"tenantId":               i.config.TenantID,
			"objects":                string(objects),
		},
	}

	if i.config.Cloud != "" {
		spc.Spec.Parameters[kvcsi.CloudNameParameter] = i.config.Cloud
	}

	return true, nil
}
