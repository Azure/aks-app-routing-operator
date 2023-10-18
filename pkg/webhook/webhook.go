package webhook

import (
	"context"
	"fmt"

	globalCfg "github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/open-policy-agent/cert-controller/pkg/rotator"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type webhookType interface {
	admissionregistrationv1.ValidatingWebhook | admissionregistrationv1.MutatingWebhook
}

type Webhook[T webhookType] struct {
	AddToManager func(manager.Manager) error
	Definition   func(c *config) (T, error)
}

var Validating []Webhook[admissionregistrationv1.ValidatingWebhook]

type config struct {
	serviceName, namespace string
	port                   int32
	certDir                string

	validatingWebhookConfigName string
}

func New(globalCfg *globalCfg.Config, port int32, certsDir string) (*config, error) {
	if globalCfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	c := &config{
		serviceName:                 globalCfg.OperatorWebhookService,
		namespace:                   globalCfg.OperatorNs,
		port:                        port,
		certDir:                     certsDir,
		validatingWebhookConfigName: "app-routing-validating",
	}

	return c, nil
}

func (c *config) EnsureWebhookConfigurations(ctx context.Context, cl client.Client) error {
	lgr := log.FromContext(ctx).WithName("webhooks")

	lgr.Info("calculating ValidatingWebhookConfiguration")
	var validatingWhs []admissionregistrationv1.ValidatingWebhook
	for _, wh := range Validating {
		wh, err := wh.Definition(c)
		if err != nil {
			return fmt.Errorf("getting webhook definition: %w", err)
		}

		validatingWhs = append(validatingWhs, wh)
	}

	validatingWhc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ValidatingWebhookConfiguration",
			APIVersion: "admissionregistration.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: c.validatingWebhookConfigName,
			Labels: map[string]string{
				// https://learn.microsoft.com/en-us/azure/aks/faq#can-admission-controller-webhooks-impact-kube-system-and-internal-aks-namespaces
				"admissions.enforcer/disabled": "true",
			},
		},
		Webhooks: validatingWhs,
	}

	// todo: add ownership references to app-routing-system ns

	lgr.Info("ensuring webhook configuration")
	whs := []client.Object{validatingWhc}
	for _, wh := range whs {
		copy := wh.DeepCopyObject().(client.Object)
		lgr := lgr.WithValues("webhook", wh.GetName())
		lgr.Info("upserting webhook configuration")

		if err := util.Upsert(ctx, cl, copy); err != nil {
			return fmt.Errorf("upserting webhook configuration: %w", err)
		}
	}

	lgr.Info("finished ensuring webhook configuration")
	return nil
}

func (c *config) AddCertManager(ctx context.Context, mgr manager.Manager, certsReady chan struct{}) error {
	lgr := log.FromContext(ctx).WithName("cert-manager")

	lgr.Info("calculating webhooks for cert-manager")
	webhooks := make([]rotator.WebhookInfo, 0)
	webhooks = append(webhooks, rotator.WebhookInfo{
		Name: c.validatingWebhookConfigName,
		Type: rotator.Validating,
	})

	lgr.Info("adding cert-manager to controller manager")
	cm := &certManager{
		SecretName:     "app-routing-webhook-secret",
		CertDir:        c.certDir,
		ServiceName:    c.serviceName,
		Namespace:      c.namespace,
		Webhooks:       webhooks,
		CAName:         "approuting.kubernetes.azure.com",
		CAOrganization: "Microsoft",
		Ready:          certsReady,
	}
	if err := cm.addToManager(ctx, mgr); err != nil {
		return fmt.Errorf("adding rotation: %w", err)
	}

	lgr.Info("finished adding cert-manager to controller manager")
	return nil
}

func (c *config) AddWebhooks(mgr manager.Manager) error {
	for _, wh := range Validating {
		if err := wh.AddToManager(mgr); err != nil {
			return fmt.Errorf("adding webhook to manager: %w", err)
		}
	}

	return nil
}

func (c *config) GetClientConfig(path string) (admissionregistrationv1.WebhookClientConfig, error) {
	return admissionregistrationv1.WebhookClientConfig{
		Service: &admissionregistrationv1.ServiceReference{
			Name:      c.serviceName,
			Namespace: c.namespace,
			Port:      &c.port,
			Path:      &path,
		},
	}, nil
}
