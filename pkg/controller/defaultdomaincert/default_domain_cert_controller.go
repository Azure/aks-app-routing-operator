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
	"github.com/Azure/aks-app-routing-operator/pkg/tls"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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

	if err := store.AddFile(conf.DefaultDomainCertPath); err != nil {
		return fmt.Errorf("adding default domain cert %s to store: %w", conf.DefaultDomainCertPath, err)
	}

	if err := store.AddFile(conf.DefaultDomainKeyPath); err != nil {
		return fmt.Errorf("adding default domain key %s to store: %w", conf.DefaultDomainKeyPath, err)
	}

	reconciler := &defaultDomainCertControllerReconciler{
		client: mgr.GetClient(),
		events: mgr.GetEventRecorderFor("aks-app-routing-operator"),
		conf:   conf,
		store:  store,
	}

	if _, _, err := reconciler.getAndVerifyCertAndKey(); err != nil {
		return fmt.Errorf("verifying cert and key: %w", err)
	}

	if err := name.AddToController(
		ctrl.NewControllerManagedBy(mgr).
			For(&approutingv1alpha1.DefaultDomainCertificate{}).
			Owns(&corev1.Secret{}).
			WatchesRawSource(source.Func(reconciler.sendRotationEvents)),
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

	defaultDomainCertificate := &approutingv1alpha1.DefaultDomainCertificate{}
	if err := d.client.Get(ctx, req.NamespacedName, defaultDomainCertificate); err != nil {
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
	secret, err := d.generateSecret(defaultDomainCertificate)
	if err != nil {
		err := fmt.Errorf("generating Secret for DefaultDomainCertificate: %w", err)
		lgr.Error(err, "failed to generate Secret for DefaultDomainCertificate")
		return ctrl.Result{}, err
	}

	if err := util.Upsert(ctx, d.client, secret); err != nil {
		d.events.Eventf(defaultDomainCertificate, corev1.EventTypeWarning, "ApplyingCertificateSecretFailed", "Failed to apply Secret for DefaultDomainCertificate: %s", err.Error())
		lgr.Error(err, "failed to upsert Secret for DefaultDomainCertificate")
		return ctrl.Result{}, err
	}

	// Update the status of the DefaultDomainCertificate
	defaultDomainCertificate.SetCondition(metav1.Condition{
		Type:    approutingv1alpha1.DefaultDomainCertificateConditionTypeAvailable,
		Status:  metav1.ConditionTrue,
		Reason:  "CertificateSecretApplied",
		Message: fmt.Sprintf("Secret %s/%s successfully applied for DefaultDomainCertificate", secret.Namespace, secret.Name),
	})
	if err := d.client.Status().Update(ctx, defaultDomainCertificate); err != nil {
		lgr.Error(err, "failed to update status for DefaultDomainCertificate")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (d *defaultDomainCertControllerReconciler) generateSecret(defaultDomainCertificate *approutingv1alpha1.DefaultDomainCertificate) (*corev1.Secret, error) {
	cert, key, err := d.getAndVerifyCertAndKey()
	if err != nil {
		return nil, fmt.Errorf("getting and verifying cert and key: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *defaultDomainCertificate.Spec.Target.Secret,
			Namespace: defaultDomainCertificate.Namespace,
			Labels:    manifests.GetTopLevelLabels(),
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": cert,
			"tls.key": key,
		},
	}

	owner := manifests.GetOwnerRefs(defaultDomainCertificate, true)
	secret.SetOwnerReferences(owner)

	return secret, nil
}

func (d *defaultDomainCertControllerReconciler) getAndVerifyCertAndKey() ([]byte, []byte, error) {
	key, ok := d.store.GetContent(d.conf.DefaultDomainKeyPath)
	if key == nil || !ok {
		return nil, nil, fmt.Errorf("failed to get default domain key from store")
	}

	cert, ok := d.store.GetContent(d.conf.DefaultDomainCertPath)
	if cert == nil || !ok {
		return nil, nil, fmt.Errorf("failed to get default domain cert from store")
	}

	if _, err := tls.ParseTLSCertificate(cert, key); err != nil {
		return nil, nil, fmt.Errorf("validating cert and key: %w", err)
	}

	return cert, key, nil
}

// sendRotationEvents listens for store rotation events and triggers reconciles
// for all DefaultDomainCertificate resources when certificate files are rotated
func (d *defaultDomainCertControllerReconciler) sendRotationEvents(ctx context.Context, queue workqueue.TypedRateLimitingInterface[ctrl.Request]) error {
	go func() {
		logger := log.FromContext(ctx)
		logger = logger.WithName("rotation-watcher")
		logger.Info("starting rotation event watcher")
		defer func() {
			logger.Info("rotation event watcher stopped")
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case event := <-d.store.RotationEvents():
				if event.Path != d.conf.DefaultDomainCertPath && event.Path != d.conf.DefaultDomainKeyPath {
					logger.Info("non-certificate file rotated " + event.Path)
					continue
				}

				ddcList := &approutingv1alpha1.DefaultDomainCertificateList{}
				if err := d.client.List(ctx, ddcList); err != nil {
					// an error here is not ideal but if we are failing to list or failing to requeue controller runtime
					// resync period of 10 hours will catch it
					logger.Error(err, "failed to list DefaultDomainCertificate resources")
					continue
				}

				for _, ddc := range ddcList.Items {
					queue.Add(reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      ddc.Name,
							Namespace: ddc.Namespace,
						},
					})
				}
			}
		}
	}()

	return nil
}
