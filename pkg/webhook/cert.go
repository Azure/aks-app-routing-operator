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

	CertsMounted chan struct{}
	Ready        chan struct{}
}

func (c *certManager) addToManager(ctx context.Context, mgr manager.Manager, lgr logr.Logger) error {
	lgr.Info("ensuring webhook cert secret")
	if err := c.ensureSecret(ctx, mgr); err != nil {
		return fmt.Errorf("ensuring secret: %w", err)
	}

	// workaround for https://github.com/open-policy-agent/cert-controller/issues/53
	// this isn't great, but it's the best we can do for now
	go func() {
		checkCertsMounted := func() (bool, error) {
			lgr.Info("checking if certs are mounted")
			certFile := path.Join(c.CertDir, "tls.crt")
			_, err := os.Stat(certFile)
			if err == nil {
				return true, nil
			}

			return false, nil
		}

		if err := wait.ExponentialBackoff(wait.Backoff{
			Duration: 1 * time.Second,
			Factor:   2,
			Jitter:   1,
			Steps:    10,
		}, checkCertsMounted); err != nil {
			lgr.Error(err, "waiting for certs to be mounted")
			return
		}

		lgr.Info("certs mounted")
		close(c.CertsMounted)
	}()

	if err := rotator.AddRotator(mgr, &rotator.CertRotator{
		SecretKey: types.NamespacedName{
			Name:      c.SecretName,
			Namespace: c.Namespace,
		},
		CertDir:                c.CertDir,
		CAName:                 c.CAName,
		CAOrganization:         c.CAOrganization,
		DNSName:                fmt.Sprintf("%s.%s.svc", c.ServiceName, c.Namespace),
		IsReady:                c.Ready,
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

func (c *certManager) ensureSecret(ctx context.Context, mgr manager.Manager) error {
	secrets := &corev1.SecretList{}
	if err := mgr.GetAPIReader().List(ctx, secrets, client.InNamespace(c.Namespace)); err != nil {
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

	if err := mgr.GetClient().Create(ctx, secret); err != nil {
		return fmt.Errorf("creating secret: %w", err)
	}

	return nil
}
