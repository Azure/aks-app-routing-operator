package spc

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	netv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		toSpcOpts: func(_ context.Context, _ client.Client, ing *netv1.Ingress) iter.Seq2[spcOpts, error] {
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
	return func(yield func(spcOpts, error) bool) {
		if conf == nil {
			yield(spcOpts{}, errors.New("config is nil"))
			return
		}

		if ing == nil {
			yield(spcOpts{}, errors.New("ingress is nil"))
			return
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
			yield(spcOpts{}, fmt.Errorf("checking if ingress is managed: %w", err))
			return
		}

		if !reconcile {
			opts.action = actionCleanup
			yield(opts, nil)
			return
		}

		uri := ing.Annotations[keyVaultUriKey]
		certRef, err := parseKeyVaultCertURI(uri)
		if err != nil {
			yield(spcOpts{}, util.NewUserError(err, fmt.Sprintf("invalid Keyvault certificate URI: %s", uri)))
			return
		}

		opts.vaultName = certRef.vaultName
		opts.certName = certRef.certName
		opts.objectVersion = certRef.objectVersion

		if strings.ToLower(ing.Annotations[tlsCertManagedAnnotation]) == "true" {
			opts.modifyOwner = func(obj client.Object) error {
				ingress, ok := obj.(*netv1.Ingress)
				if !ok {
					return fmt.Errorf("object is not an Ingress: %T", obj)
				}

				hosts := []string{}
				for _, rule := range ingress.Spec.Rules {
					if host := rule.Host; host != "" {
						hosts = append(hosts, host)
					}
				}

				ingress.Spec.TLS = []netv1.IngressTLS{
					{
						SecretName: opts.secretName,
						Hosts:      hosts,
					},
				}

				return nil
			}
		}

		yield(opts, nil)
	}
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

var getIngressCertSecretName = getIngressSpcName

func getIngressSpcName(ing *netv1.Ingress) string {
	if ing == nil {
		return ""
	}

	return "keyvault-" + ing.Name
}
