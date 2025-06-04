package spc

import (
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	netv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var ingressSecretProviderControllerName = controllername.New("keyvault", "ingress", "secret", "provider")

func NewIngressSecretProviderClassReconciler(manager ctrl.Manager, conf *config.Config, ingressManager IngressManager) error {
	metrics.InitControllerMetrics(ingressSecretProviderControllerName)
	if conf.DisableKeyvault {
		return nil
	}

	spcReconciler := &secretProviderClassReconciler[*netv1.Ingress]{
		name:     ingressSecretProviderControllerName,
		spcNamer: getIngressSpcName,
		shouldReconcile: func(ing *netv1.Ingress) (bool, error) {
			return shouldReconcileIngress(ingressManager, ing)
		},
		toSpcOpts: func(ing *netv1.Ingress) (spcOpts, error) {
			return ingressToSpcOpts(conf, ing)
		},

		client: manager.GetClient(),
		events: manager.GetEventRecorderFor("aks-app-routing-operator"),
		config: conf,
	}

	return ingressSecretProviderControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&netv1.Ingress{}).
			Owns(&secv1.SecretProviderClass{}),
		manager.GetLogger(),
	).Complete(spcReconciler)
}

func shouldReconcileIngress(ingressManager IngressManager, ing *netv1.Ingress) (bool, error) {
	isManaged, err := ingressManager.IsManaging(ing)
	if err != nil {
		return false, fmt.Errorf("checking if ingress %s is managed: %w", ing.Name, err)
	}

	if ing == nil {
		return false, fmt.Errorf("ingress is nil")
	}

	if ing.Annotations == nil {
		return false, nil
	}

	if _, ok := ing.Annotations[keyVaultUriKey]; !ok {
		return false, nil
	}

	return isManaged, nil
}

func getIngressSpcName(ing *netv1.Ingress) string {
	if ing == nil {
		return ""
	}

	return "keyvault-" + ing.Name
}

func ingressToSpcOpts(conf *config.Config, ing *netv1.Ingress) (spcOpts, error) {
	if conf == nil {
		return spcOpts{}, fmt.Errorf("config is nil")
	}

	if ing == nil {
		return spcOpts{}, fmt.Errorf("ingress is nil")
	}

	uri := ing.Annotations[keyVaultUriKey]
	certRef, err := parseKeyVaultCertURI(uri)
	if err != nil {
		return spcOpts{}, util.NewUserError(err, fmt.Sprintf("invalid Keyvault certificate URI: %s", uri))
	}

	return spcOpts{
		clientId:      conf.MSIClientID,
		tenantId:      conf.TenantID,
		vaultName:     certRef.vaultName,
		certName:      certRef.certName,
		objectVersion: certRef.objectVersion,
		secretName:    getIngressCertSecretName(ing),
	}, nil
}

func getIngressCertSecretName(ing *netv1.Ingress) string {
	if ing == nil {
		return ""
	}

	return "keyvault-" + ing.Name
}
