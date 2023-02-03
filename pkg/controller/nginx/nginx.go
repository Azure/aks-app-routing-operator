package nginx

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/informer"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/ingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type nginx struct {
	manager          manager.Manager
	ingClassInformer informer.IngressClass
	conf             *config.Config
	self             *appsv1.Deployment
	controller       string
}

// New adds all resources required for Nginx to the manager
func New(m manager.Manager, conf *config.Config, self *appsv1.Deployment, ingClassInformer informer.IngressClass, controller string) error {
	if ingClassInformer == nil {
		return errors.New("ingressClassInformer is nil")
	}

	n := &nginx{
		manager:          m,
		conf:             conf,
		self:             self,
		ingClassInformer: ingClassInformer,
		controller:       controller,
	}

	if err := n.addIngressControllerReconciler(); err != nil {
		return err
	}

	if err := n.addConcurrencyWatchdog(); err != nil {
		return err
	}

	if err := n.addIngressSecretProviderClassReconciler(); err != nil {
		return err
	}

	if err := n.addPlaceholderPodController(); err != nil {
		return err
	}

	return nil
}

func (n *nginx) addIngressControllerReconciler() error {
	return ingress.NewIngressControllerReconciler(n.manager, n.ingClassInformer, n.provisionFn())
}

func (n *nginx) addConcurrencyWatchdog() error {
	return ingress.NewConcurrencyWatchdog(n.manager, n.conf)
}

func (n *nginx) addIngressSecretProviderClassReconciler() error {
	return keyvault.NewIngressSecretProviderClassReconciler(n.manager, n.conf, n.isConsuming)
}

func (n *nginx) addPlaceholderPodController() error {
	return keyvault.NewPlaceholderPodController(n.manager, n.conf, n.isConsuming)
}

func (n *nginx) consumingIcs() ([]*netv1.IngressClass, error) {
	ics, err := n.ingClassInformer.ByController(n.controller)
	if err != nil {
		return nil, err
	}

	validIcs := make([]*netv1.IngressClass, 0)
	for _, ic := range ics {
		if ic.GetDeletionTimestamp() != nil {
			validIcs = append(validIcs, ic)
		}
	}

	return validIcs, nil
}

func (n *nginx) isConsuming(i *netv1.Ingress) (bool, error) {
	consumingIcs, err := n.consumingIcs()
	if err != nil {
		return false, err
	}

	for _, ic := range consumingIcs {
		if ic.Name == *i.Spec.IngressClassName {
			return true, nil
		}
	}

	return false, nil
}

func (n *nginx) provisionFn() ingress.ProvisionFn {
	return func(ctx context.Context, c client.Client) error {
		log := logr.FromContextOrDiscard(ctx)

		ics, err := n.consumingIcs()
		if err != nil {
			return err
		}

		if len(ics) == 0 {
			log.Info(fmt.Sprintf("no ingressClasses consuming %s controller found", n.controller))
			return nil
		}

		if len(ics) > 1 {
			return errors.New(fmt.Sprintf("multiple ingressClasses consuming %s controller found when max of one is allowed", n.controller))
		}

		ic := ics[0]
		resources := manifests.NginxIngressControllerResources(n.conf, n.self, ic)
		for _, res := range resources {
			copy := res.DeepCopyObject().(client.Object)
			if copy.GetDeletionTimestamp() != nil {
				if err := c.Delete(ctx, copy); !k8serrors.IsNotFound(err) {
					log.Error(err, "deleting unneeded resources")
				}
				continue
			}

			if err := util.Upsert(ctx, c, copy); err != nil {
				return err
			}
		}

		return nil
	}
}
