package webhook

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/go-logr/logr"
	"github.com/open-policy-agent/cert-controller/pkg/rotator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type certManager struct {
	SecretName             string
	CertDir                string
	ServiceName, Namespace string
	Webhooks               []rotator.WebhookInfo

	CAName, CAOrganization string

	Ready chan struct{}
}

func (c *certManager) addToManager(ctx context.Context, mgr manager.Manager, lgr logr.Logger) error {
	lgr.Info("ensuring webhook cert secret")
	if err := c.ensureSecret(ctx, mgr.GetClient()); err != nil {
		return fmt.Errorf("ensuring secret: %w", err)
	}

	// workaround for https://github.com/open-policy-agent/cert-controller/issues/53
	certsMounted := make(chan struct{})
	go c.pollForCertsMounted(lgr, certsMounted)
	certsRotated := make(chan struct{})
	go c.signalReady(lgr, certsMounted, certsRotated)

	if err := rotator.AddRotator(mgr, &rotator.CertRotator{
		SecretKey: types.NamespacedName{
			Name:      c.SecretName,
			Namespace: c.Namespace,
		},
		CertDir:                c.CertDir,
		CAName:                 c.CAName,
		CAOrganization:         c.CAOrganization,
		DNSName:                fmt.Sprintf("%s.%s.svc", c.ServiceName, c.Namespace),
		IsReady:                certsRotated,
		Webhooks:               c.Webhooks,
		RestartOnSecretRefresh: true,
		RequireLeaderElection:  true,
		ExtKeyUsages: &[]x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
	}); err != nil {
		return fmt.Errorf("adding rotator: %w", err)
	}

	return nil
}

func (c *certManager) signalReady(lgr logr.Logger, certsMounted, certsRotated <-chan struct{}) {
	select {
	case <-certsRotated:
		lgr.Info("certs rotated")
		close(c.Ready)
	case <-certsMounted:
		waitTime := 25 * time.Second
		lgr.Info(fmt.Sprintf("certs mounted but may not be fully rotated, waiting %s", waitTime))

		select {
		case <-certsRotated:
			lgr.Info("certs rotated")
		case <-time.After(waitTime):
			lgr.Info("waited for certs to be rotated, continuing")
		}
		close(c.Ready)
	}
}

func (c *certManager) pollForCertsMounted(lgr logr.Logger, certsMounted chan<- struct{}) {
	if err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   2,
		Jitter:   1,
		Steps:    10,
	}, c.areCertsMounted); err != nil {
		lgr.Error(err, "waiting for certs to be mounted")
		return
	}

	lgr.Info("certs mounted")
	close(certsMounted)
}

func (c *certManager) areCertsMounted() (bool, error) { // we don't use the error return but need the fn to match this form for the exponential backoff package
	certFile := path.Join(c.CertDir, "tls.crt")
	_, err := os.Stat(certFile)
	if err == nil {
		return true, nil
	}

	return false, nil
}

func (c *certManager) ensureSecret(ctx context.Context, cl client.Client) error {
	secrets := &corev1.SecretList{}
	if err := cl.List(ctx, secrets, client.InNamespace(c.Namespace)); err != nil {
		return fmt.Errorf("listing secrets: %w", err)
	}

	for _, s := range secrets.Items {
		if s.Name == c.SecretName {
			// secret already exists
			return nil
		}
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.SecretName,
			Namespace: c.Namespace,
			Labels:    manifests.GetTopLevelLabels(),
		},
	}
	if err := cl.Create(ctx, secret); err != nil {
		return fmt.Errorf("creating secret: %w", err)
	}

	return nil
}
