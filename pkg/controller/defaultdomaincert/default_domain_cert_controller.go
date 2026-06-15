package defaultdomaincert

import (
	"context"
	"errors"
	"fmt"
	"time"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	defaultdomain "github.com/Azure/aks-app-routing-operator/pkg/clients/default-domain"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/tls"
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

// defaultDomainClient is an interface for fetching TLS certificates from the default domain service
type defaultDomainClient interface {
	GetTLSCertificate(ctx context.Context) (*defaultdomain.TLSCertificate, error)
}

type defaultDomainCertControllerReconciler struct {
	client              client.Client
	events              record.EventRecorder
	conf                *config.Config
	defaultDomainClient defaultDomainClient
}

func NewReconciler(conf *config.Config, mgr ctrl.Manager, defaultDomainClient *defaultdomain.CachedClient) error {
	metrics.InitControllerMetrics(name)

	reconciler := &defaultDomainCertControllerReconciler{
		client:              mgr.GetClient(),
		events:              mgr.GetEventRecorderFor("aks-app-routing-operator"),
		conf:                conf,
		defaultDomainClient: defaultDomainClient,
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

	lgr.Info("generating Secret for DefaultDomainCertificate")
	secret, certInfo, err := d.generateSecret(ctx, defaultDomainCertificate)
	if err != nil {
		if util.IsNotFound(err) {
			lgr.Info("default domain certificate not available yet, requeuing to wait for it to be issued")
			if err := d.setUnavailable(ctx, defaultDomainCertificate, "CertificateNotReady", "Certificate not ready yet, waiting for it to be issued"); err != nil {
				lgr.Error(err, "failed to update status for DefaultDomainCertificate")
				return ctrl.Result{}, err
			}

			// we use a Jitter here to avoid thundering herd issues if many DefaultDomainCertificates are waiting for certs
			requeueAfter := util.Jitter(30*time.Second, 0.25)
			lgr.Info("requeuing DefaultDomainCertificate", "requeueAfter", requeueAfter.Truncate(time.Second).String())
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}

		err := fmt.Errorf("generating Secret for DefaultDomainCertificate: %w", err)
		lgr.Error(err, "failed to generate Secret for DefaultDomainCertificate")
		return ctrl.Result{}, err
	}

	if certInfo != nil {
		lgr.Info("generated Secret from default domain certificate",
			"certSubject", certInfo.Subject,
			"certDNSNames", certInfo.DNSNames,
			"certNotAfter", certInfo.NotAfter.UTC().Format(time.RFC3339))
	}

	// Refuse to adopt a Secret that App Routing doesn't manage. Overwriting a foreign
	// Secret can destroy another controller's data, and if its immutable fields (e.g.
	// type) differ from ours, Upsert fails on every reconcile and the
	// DefaultDomainCertificate never reports a status. Surface the conflict instead.
	existing := &corev1.Secret{}
	switch getErr := d.client.Get(ctx, client.ObjectKeyFromObject(secret), existing); {
	case getErr == nil && !manifests.HasTopLevelLabels(existing.Labels):
		msg := fmt.Sprintf("Secret %s/%s already exists and is not managed by App Routing", secret.Namespace, secret.Name)
		d.events.Eventf(defaultDomainCertificate, corev1.EventTypeWarning, "ConflictingSecretExists", msg)
		lgr.Info("refusing to overwrite Secret not managed by App Routing")
		if err := d.setUnavailable(ctx, defaultDomainCertificate, "ConflictingSecretExists", msg); err != nil {
			lgr.Error(err, "failed to update status for DefaultDomainCertificate")
			return ctrl.Result{}, err
		}
		// Requeue periodically: we don't receive watch events for a Secret we don't
		// own, so we re-check in case the conflicting Secret is later removed.
		return ctrl.Result{RequeueAfter: util.Jitter(30*time.Second, 0.25)}, nil
	case getErr != nil && !apierrors.IsNotFound(getErr):
		return ctrl.Result{}, fmt.Errorf("checking for existing Secret: %w", getErr)
	}

	lgr.Info("upserting Secret for DefaultDomainCertificate")
	if err := util.Upsert(ctx, d.client, secret); err != nil {
		msg := fmt.Sprintf("Failed to apply Secret %s/%s for DefaultDomainCertificate: %s", secret.Namespace, secret.Name, err.Error())
		d.events.Eventf(defaultDomainCertificate, corev1.EventTypeWarning, "ApplyingCertificateSecretFailed", msg)
		lgr.Error(err, "failed to upsert Secret for DefaultDomainCertificate")
		// Best-effort: surface the failure on the resource so consumers (and
		// `kubectl wait`) see why it isn't Available instead of timing out blindly.
		if statusErr := d.setUnavailable(ctx, defaultDomainCertificate, "ApplyingCertificateSecretFailed", msg); statusErr != nil {
			lgr.Error(statusErr, "failed to update status for DefaultDomainCertificate")
		}
		return ctrl.Result{}, err
	}
	lgr.Info("successfully upserted Secret for DefaultDomainCertificate")

	// Update the status of the DefaultDomainCertificate
	if certInfo != nil && len(certInfo.DNSNames) > 0 {
		defaultDomainCertificate.Status.Domain = certInfo.DNSNames[0]
	}
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

	lgr.Info("DefaultDomainCertificate is Available", "domain", defaultDomainCertificate.Status.Domain)
	return ctrl.Result{}, nil
}

// setUnavailable marks the DefaultDomainCertificate's Available condition False with the
// given reason and message so failures are visible on the resource rather than only in logs.
func (d *defaultDomainCertControllerReconciler) setUnavailable(ctx context.Context, ddc *approutingv1alpha1.DefaultDomainCertificate, reason, message string) error {
	ddc.SetCondition(metav1.Condition{
		Type:    approutingv1alpha1.DefaultDomainCertificateConditionTypeAvailable,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	return d.client.Status().Update(ctx, ddc)
}

func (d *defaultDomainCertControllerReconciler) generateSecret(ctx context.Context, defaultDomainCertificate *approutingv1alpha1.DefaultDomainCertificate) (*corev1.Secret, *tls.CertificateInfo, error) {
	cert, key, certInfo, err := d.getAndVerifyCertAndKeyFromClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("getting and verifying cert and key: %w", err)
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

	return secret, certInfo, nil
}

func (d *defaultDomainCertControllerReconciler) getAndVerifyCertAndKeyFromClient(ctx context.Context) ([]byte, []byte, *tls.CertificateInfo, error) {
	tlsCert, err := d.defaultDomainClient.GetTLSCertificate(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get TLS certificate from client: %w", err)
	}

	if tlsCert.Key == nil || len(tlsCert.Key) == 0 {
		return nil, nil, nil, fmt.Errorf("TLS certificate key is empty")
	}

	if tlsCert.Cert == nil || len(tlsCert.Cert) == 0 {
		return nil, nil, nil, fmt.Errorf("TLS certificate cert is empty")
	}

	certInfo, err := tls.ParseTLSCertificate(tlsCert.Cert, tlsCert.Key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("validating cert and key: %w", err)
	}

	return tlsCert.Cert, tlsCert.Key, certInfo, nil
}
