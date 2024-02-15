// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"encoding/json"
	"fmt"
	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"net/url"
	"strings"

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
	kvcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
)

var (
	nginxSecretProviderControllerName = controllername.New("keyvault", "nginx", "secret", "provider")
	NginxNamePrefix                   = "keyvault-nginx-"
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
	logger = logger.WithValues("name", nic.Name, "namespace", "app-routing-system", "generation", nic.Generation)

	spc := &secv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
			Kind:       "SecretProviderClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultNginxCertName(nic),
			Namespace: "app-routing-system",
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: nic.APIVersion,
				Controller: util.BoolPtr(true),
				Kind:       nic.Kind,
				Name:       nic.Name,
				UID:        nic.UID,
			}},
		},
	}
	logger = logger.WithValues("spc", spc.Name)
	ok, err := i.buildSPC(nic, spc)
	if err != nil {
		logger.Info("failed to build secret provider class for ingress, user input invalid. sending warning event")
		i.events.Eventf(nic, "Warning", "InvalidInput", "error while processing Keyvault reference: %s", err)
		return result, nil
	}
	if ok {
		logger.Info("reconciling secret provider class for ingress")
		err = util.Upsert(ctx, i.client, spc)
		if err != nil {
			i.events.Eventf(nic, "Warning", "FailedUpdateOrCreateSPC", "error while creating or updating SecretProviderClass needed to pull Keyvault reference: %s", err)
		}
		return result, err
	}

	logger.Info("cleaning unused managed spc for ingress")
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
	}

	return result, nil
}

func (i *NginxSecretProviderClassReconciler) buildSPC(nic *approutingv1alpha1.NginxIngressController, spc *secv1.SecretProviderClass) (bool, error) {
	if nic.Spec.IngressClassName == "" {
		return false, nil
	}
	if nic.Spec.DefaultSSLCertificate == nil || nic.Spec.DefaultSSLCertificate.KeyVaultURI == nil {
		return false, nil
	}
	
	certURI := *nic.Spec.DefaultSSLCertificate.KeyVaultURI
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
			SecretName: DefaultNginxCertName(nic),
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

// DefaultNginxCertName returns a default name for the nginx certificate name using the IngressClassName from the spec.
// Truncates characters in the IngressClassName passed the max secret length (255) if the IngressClassName and the default namespace are over the limit
func DefaultNginxCertName(nic *approutingv1alpha1.NginxIngressController) string {
	secretMaxSize := 255
	certName := NginxNamePrefix + nic.Name

	if len(certName) > secretMaxSize {
		return certName[0:secretMaxSize]
	}

	return certName
}
