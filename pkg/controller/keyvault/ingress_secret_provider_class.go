// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

// IngressSecretProviderClassReconciler manages a SecretProviderClass for each ingress resource that
// references a Keyvault certificate. The SPC is used to mirror the Keyvault values into a k8s secret
// so that it can be used by the ingress controller.
type IngressSecretProviderClassReconciler struct {
	client client.Client
	events record.EventRecorder
	config *config.Config
}

func NewIngressSecretProviderClassReconciler(manager ctrl.Manager, conf *config.Config) error {
	if conf.DisableKeyvault {
		return nil
	}
	return ctrl.
		NewControllerManagedBy(manager).
		For(&netv1.Ingress{}).
		Complete(&IngressSecretProviderClassReconciler{
			client: manager.GetClient(),
			events: manager.GetEventRecorderFor("aks-app-routing-operator"),
			config: conf,
		})
}

func (i *IngressSecretProviderClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger = logger.WithName("secretProviderClassReconciler")

	ing := &netv1.Ingress{}
	err = i.client.Get(ctx, req.NamespacedName, ing)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
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
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: ing.APIVersion,
				Controller: util.BoolPtr(true),
				Kind:       ing.Kind,
				Name:       ing.Name,
				UID:        ing.UID,
			}},
		},
	}
	ok, err := i.buildSPC(ing, spc)
	if err != nil {
		i.events.Eventf(ing, "Warning", "InvalidInput", "error while processing Keyvault reference: %s", err)
		return ctrl.Result{}, nil
	}
	if ok {
		logger.Info("reconciling secret provider class for ingress")
		return ctrl.Result{}, util.Upsert(ctx, i.client, spc)
	}

	err = i.client.Get(ctx, client.ObjectKeyFromObject(spc), spc)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("removing secret provider class for ingress")
	return ctrl.Result{}, i.client.Delete(ctx, spc)
}

func (i *IngressSecretProviderClassReconciler) buildSPC(ing *netv1.Ingress, spc *secv1.SecretProviderClass) (bool, error) {
	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != manifests.IngressClass || ing.Annotations == nil {
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
		Parameters: map[string]string{
			"keyvaultName":           vaultName,
			"useVMManagedIdentity":   "true",
			"userAssignedIdentityID": i.config.MSIClientID,
			"tenantId":               i.config.TenantID,
			"objects":                string(objects),
		},
	}

	if cloud := i.config.Cloud; cloud != "" {
		spc.Spec.Parameters["cloudName"] = cloud
	}

	return true, nil
}
