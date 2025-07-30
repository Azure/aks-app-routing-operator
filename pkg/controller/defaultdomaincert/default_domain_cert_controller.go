package defaultdomaincert

import (
	"context"
	"errors"
	"fmt"
	"time"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/store"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var name = controllername.New("default", "domain", "cert", "reconciler")

type defaultDomainCertControllerReconciler struct {
	client client.Client
	events record.EventRecorder
	conf   *config.Config
	store  store.Store
}

func NewReconciler(conf *config.Config, mgr ctrl.Manager, store store.Store) error {
	metrics.InitControllerMetrics(name)

	reconciler := &defaultDomainCertControllerReconciler{
		client: mgr.GetClient(),
		events: mgr.GetEventRecorderFor("aks-app-routing-operator"),
		conf:   conf,
	}

	if err := name.AddToController(
		ctrl.NewControllerManagedBy(mgr).
			For(&approutingv1alpha1.DefaultDomainCertificate{}).
			Owns(&corev1.Secret{}),
		mgr.GetLogger(),
	).Complete(reconciler); err != nil {
		return fmt.Errorf("building the controller: %w", err)
	}

	return nil
}

func (d *defaultDomainCertControllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	start := time.Now()
	lgr := log.FromContext(ctx, "name", req.Name, "namespace", req.Namespace)
	ctx = log.IntoContext(ctx, lgr)

	lgr.Info("reconciling DefaultDomainCertificate")
	defer func() {
		lgr.Info("reconcile finished", "latency", time.Since(start))
	}()

	var defaultDomainCertificate approutingv1alpha1.DefaultDomainCertificate
	if err := d.client.Get(ctx, req.NamespacedName, &defaultDomainCertificate); err != nil {
		if apierrors.IsNotFound(err) { // object was deleted
			lgr.Info("DefaultDomainCertificate not found")
			return ctrl.Result{}, nil
		}

		lgr.Error(err, "unable to get DefaultDomainCertificate")
		return ctrl.Result{}, err
	}

	if defaultDomainCertificate.Spec.Target.Secret == nil {
		err := errors.New("DefaultDomainCertificate has no target secret specified")
		lgr.Error(err, "DefaultDomainCertificate has no target specified, this should be blocked by CRD CEL validation")
		return ctrl.Result{}, err
	}

	lgr = lgr.WithValues("secretTarget", *defaultDomainCertificate.Spec.Target.Secret)
	ctx = log.IntoContext(ctx, lgr)

	lgr.Info("upserting Secret for DefaultDomainCertificate")
	secret := d.getSecret(&defaultDomainCertificate)
	if err := util.Upsert(ctx, d.client, secret); err != nil {
		lgr.Error(err, "failed to upsert Secret for DefaultDomainCertificate")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (d *defaultDomainCertControllerReconciler) getSecret(defaultDomainCertificate *approutingv1alpha1.DefaultDomainCertificate) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *defaultDomainCertificate.Spec.Target.Secret,
			Namespace: defaultDomainCertificate.Namespace,
			Labels:    manifests.GetTopLevelLabels(),
		},
		Data: map[string][]byte{
			"tls.crt": []byte("dummy-cert"),
			"tls.key": []byte("dummy-key"),
		},
	}

	owner := manifests.GetOwnerRefs(defaultDomainCertificate, true)
	secret.SetOwnerReferences(owner)

	return secret
}
