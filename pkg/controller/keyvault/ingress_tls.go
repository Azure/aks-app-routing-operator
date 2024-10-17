package keyvault

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ingressTlsControllerName = controllername.New("keyvault", "ingress", "tls")

// ingressTlsReconciler manages the TLS spec of an Ingress object using the Key Vault integration. Ingresses with
// the App Routing Key Vault annotation will have their TLS spec populated with the Key Vault secret name.
type ingressTlsReconciler struct {
	client         client.Client
	events         record.EventRecorder
	ingressManager IngressManager
}

func NewIngressTlsReconciler(manager ctrl.Manager, conf *config.Config, ingressManager IngressManager) error {
	metrics.InitControllerMetrics(ingressTlsControllerName)

	if conf.DisableKeyvault {
		return nil
	}

	return ingressTlsControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&netv1.Ingress{}), manager.GetLogger(),
	).Complete(&ingressTlsReconciler{
		client:         manager.GetClient(),
		events:         manager.GetEventRecorderFor("aks-app-routing-operator"),
		ingressManager: ingressManager,
	})
}

func (i *ingressTlsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	defer func() {
		metrics.HandleControllerReconcileMetrics(ingressTlsControllerName, res, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, errors.New("getting logger from context")
	}
	logger = ingressTlsControllerName.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	logger.Info("getting Ingress")
	ing := &netv1.Ingress{}
	if err = i.client.Get(ctx, req.NamespacedName, ing); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	managed, err := i.ingressManager.IsManaging(ing)
	if err != nil {
		logger.Error(err, "error checking if ingress is managed")
		return ctrl.Result{}, fmt.Errorf("checking if ingress is managed: %w", err)
	}

	if !managed {
		logger.Info("ingress is not managed by app routing")
		return ctrl.Result{}, nil
	}

	if val, ok := ing.Annotations[tlsCertManagedAnnotation]; !ok || strings.ToLower(val) != "true" {
		logger.Info("ingress does not have managed annotation")
		return ctrl.Result{}, nil
	}

	if _, ok := ing.Annotations[tlsCertKvUriOption]; !ok {
		logger.Info("ingress does not have keyvault annotation")
		i.events.Eventf(ing, "Warning", "KeyvaultUriAnnotationMissing", "Ingress has %[1]s annotation but is missing %[2]s annotation. %[2]s annotation is needed to manage Ingress TLS.", tlsCertManagedAnnotation, tlsCertKvUriOption)
		return ctrl.Result{}, nil
	}

	oldTls := ing.Spec.TLS
	logger.Info("adding TLS spec to ingress")
	ing.Spec.TLS = []netv1.IngressTLS{
		{
			SecretName: certSecretName(ing.Name),
			Hosts:      []string{},
		},
	}

	for _, rule := range ing.Spec.Rules {
		if host := rule.Host; host != "" {
			ing.Spec.TLS[0].Hosts = append(ing.Spec.TLS[0].Hosts, host)
		}
	}

	if !reflect.DeepEqual(oldTls, ing.Spec.TLS) {
		logger.Info("overwriting TLS spec on ingress", "old", fmt.Sprintf("%s", oldTls), "new", fmt.Sprintf("%s", ing.Spec.TLS))
	}

	if err := util.Upsert(ctx, i.client, ing); err != nil {
		logger.Error(err, "error updating ingress")
		return ctrl.Result{}, fmt.Errorf("updating ingress: %w", err)
	}

	return ctrl.Result{}, nil
}
