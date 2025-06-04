package spc

import (
	"fmt"
	"iter"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	netv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var ingressSecretProviderControllerName = controllername.New("keyvault", "ingress", "secret", "provider")

func NewIngressSecretProviderClassReconciler(manager ctrl.Manager, conf *config.Config, ingressManager util.IngressManager) error {
	metrics.InitControllerMetrics(ingressSecretProviderControllerName)
	if conf.DisableKeyvault {
		return nil
	}

	spcReconciler := &secretProviderClassReconciler[*netv1.Ingress]{
		name: ingressSecretProviderControllerName,
		toSpcOpts: func(ing *netv1.Ingress) iter.Seq2[spcOpts, error] {
			return ingressToSpcOpts(conf, ing, ingressManager)
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

func ingressToSpcOpts(conf *config.Config, ing *netv1.Ingress, ingressManager util.IngressManager) iter.Seq2[spcOpts, error] {
	if conf == nil {
		return func(yield func(spcOpts, error) bool) {
			yield(spcOpts{}, fmt.Errorf("config is nil"))
		}
	}

	if ing == nil {
		return func(yield func(spcOpts, error) bool) {
			yield(spcOpts{}, fmt.Errorf("ingress is nil"))
		}
	}

	opts := spcOpts{
		action:     actionReconcile,
		name:       getIngressSpcName(ing),
		namespace:  ing.GetNamespace(),
		clientId:   conf.MSIClientID,
		tenantId:   conf.TenantID,
		secretName: getIngressCertSecretName(ing),
		cloud:      conf.Cloud,
	}

	reconcile, err := shouldReconcileIngress(ingressManager, ing)
	if err != nil {
		return func(yield func(spcOpts, error) bool) {
			yield(spcOpts{}, fmt.Errorf("checking if ingress is managed: %w", err))
		}
	}

	if !reconcile {
		opts.action = actionCleanup
		return func(yield func(spcOpts, error) bool) {
			yield(opts, nil)
		}
	}

	uri := ing.Annotations[keyVaultUriKey]
	certRef, err := parseKeyVaultCertURI(uri)
	if err != nil {
		return func(yield func(spcOpts, error) bool) {
			yield(spcOpts{}, util.NewUserError(err, fmt.Sprintf("invalid Keyvault certificate URI: %s", uri)))
		}
	}

	opts.vaultName = certRef.vaultName
	opts.certName = certRef.certName
	opts.objectVersion = certRef.objectVersion

	return func(yield func(spcOpts, error) bool) {
		yield(opts, nil)
	}
}

func getIngressCertSecretName(ing *netv1.Ingress) string {
	if ing == nil {
		return ""
	}

	return "keyvault-" + ing.Name
}

func shouldReconcileIngress(ingressManager util.IngressManager, ing *netv1.Ingress) (bool, error) {
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
