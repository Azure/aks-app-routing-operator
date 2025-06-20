package spc

import (
	"context"
	"errors"
	"iter"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var nginxSecretProviderControllerName = controllername.New("keyvault", "nginx", "secret", "provider")

func NewNginxSecretProviderClassReconciler(manager ctrl.Manager, conf *config.Config) error {
	metrics.InitControllerMetrics(nginxSecretProviderControllerName)
	if conf.DisableKeyvault {
		return nil
	}

	spcReconciler := &secretProviderClassReconciler[*approutingv1alpha1.NginxIngressController]{
		name: nginxSecretProviderControllerName,
		toSpcOpts: func(_ context.Context, _ client.Client, nic *approutingv1alpha1.NginxIngressController) iter.Seq2[spcOpts, error] {
			return nicToSpcOpts(conf, nic)
		},

		client: manager.GetClient(),
		events: manager.GetEventRecorderFor("aks-app-routing-operator"),
		config: conf,
	}

	return nginxSecretProviderControllerName.AddToController(
		ctrl.NewControllerManagedBy(manager).
			For(&approutingv1alpha1.NginxIngressController{}).
			Owns(&secv1.SecretProviderClass{}),
		manager.GetLogger(),
	).Complete(spcReconciler)
}

func nicToSpcOpts(conf *config.Config, nic *approutingv1alpha1.NginxIngressController) iter.Seq2[spcOpts, error] {
	return func(yield func(spcOpts, error) bool) {
		if conf == nil {
			yield(spcOpts{}, errors.New("config is nil"))
			return
		}

		if nic == nil {
			yield(spcOpts{}, errors.New("nginx ingress controller is nil"))
			return
		}

		opts := spcOpts{
			action:     actionReconcile,
			name:       nicDefaultCertName(nic),
			namespace:  conf.NS,
			clientId:   config.Flags.MSIClientID,
			tenantId:   conf.TenantID,
			cloud:      conf.Cloud,
			secretName: NicDefaultSecretName(nic),
		}

		if !ShouldReconcileNic(nic) {
			opts.action = actionCleanup
			yield(opts, nil)
			return
		}

		certRef, err := parseKeyVaultCertURI(*uri)
		if err != nil {
			yield(opts, util.NewUserError(err, "unable to parse KeyVault URI for Nginx Ingress Controller"))
			return
		}

		opts.vaultName = certRef.vaultName
		opts.certName = certRef.certName
		opts.objectVersion = certRef.objectVersion
		yield(opts, nil)
	}
}

// NicDefaultSecretName returns the default secret name for the NginxIngressController.
var NicDefaultSecretName = nicDefaultCertName

// nicDefaultCertName returns the default certificate name for the NginxIngressController.
func nicDefaultCertName(nic *approutingv1alpha1.NginxIngressController) string {
	if nic == nil {
		return ""
	}

	name := "keyvault-nginx-" + nic.GetName()
	if len(name) > 253 {
		name = name[:253]
	}

	return name
}

// ShouldReconcileNic checks if the NginxIngressController should be reconciled for KeyVault integration
func ShouldReconcileNic(nic *approutingv1alpha1.NginxIngressController) bool {
	if nic == nil || nic.Spec.DefaultSSLCertificate == nil || nic.Spec.DefaultSSLCertificate.KeyVaultURI == nil || *nic.Spec.DefaultSSLCertificate.KeyVaultURI == "" {
		return false
	}
	return true
}
