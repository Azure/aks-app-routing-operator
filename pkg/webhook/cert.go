package webhook

import (
	"context"
	"crypto/x509"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/open-policy-agent/cert-controller/pkg/rotator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func (c *certManager) addToManager(ctx context.Context, mgr manager.Manager) error {
	// TODO: is this needed?
	if err := c.ensureSecret(ctx, mgr); err != nil {
		return fmt.Errorf("ensuring secret: %w", err)
	}

	dnsNames := []string{
		fmt.Sprintf("%s.%s.svc", c.ServiceName, c.Namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", c.ServiceName, c.Namespace),
	}

	if err := rotator.AddRotator(mgr, &rotator.CertRotator{
		SecretKey: types.NamespacedName{
			Name:      c.SecretName,
			Namespace: c.Namespace,
		},
		CertDir:                c.CertDir,
		CAName:                 c.CAName,
		CAOrganization:         c.CAOrganization,
		DNSName:                dnsNames[0],
		ExtraDNSNames:          dnsNames,
		IsReady:                c.Ready,
		Webhooks:               c.Webhooks,
		RestartOnSecretRefresh: true,
		ExtKeyUsages: &[]x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		RequireLeaderElection: false, // todo: testing this out in false
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
