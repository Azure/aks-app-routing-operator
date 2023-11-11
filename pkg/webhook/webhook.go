package webhook

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	globalCfg "github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type webhookType interface {
	admissionregistrationv1.ValidatingWebhook | admissionregistrationv1.MutatingWebhook
}

// Webhook defines a webhook that can be registered and added to the manager
type Webhook[T webhookType] struct {
	AddToManager func(manager.Manager) error
	Definition   func(c *config) (T, error)
}

// Validating is a list of ValidatingWebhook to be registered. Append to this slice to register more webhooks
var Validating []Webhook[admissionregistrationv1.ValidatingWebhook]

// Mutating is a list of MutatingWebhook to be registered. Append to this slice to register more webhooks
var Mutating []Webhook[admissionregistrationv1.MutatingWebhook]

type config struct {
	serviceName, namespace, serviceUrl string
	port                               int32
	certDir                            string
	certName, caName, keyName          string

	validatingWebhookConfigName string
	mutatingWebhookConfigName   string

	validatingWebhooks []Webhook[admissionregistrationv1.ValidatingWebhook]
	mutatingWebhooks   []Webhook[admissionregistrationv1.MutatingWebhook]
}

// New returns a new webhook config
func New(globalCfg *globalCfg.Config) (*config, error) {
	if globalCfg == nil {
		return nil, errors.New("config is nil")
	}

	return &config{
		serviceName:                 globalCfg.OperatorWebhookService,
		namespace:                   globalCfg.OperatorNs,
		serviceUrl:                  globalCfg.OperatorWebhookServiceUrl,
		port:                        int32(globalCfg.WebhookPort),
		certDir:                     globalCfg.CertDir,
		validatingWebhookConfigName: "app-routing-validating",
		mutatingWebhookConfigName:   "app-routing-mutating",
		validatingWebhooks:          Validating,
		mutatingWebhooks:            Mutating,
		certName:                    globalCfg.CertName,
		caName:                      globalCfg.CaName,
		keyName:                     globalCfg.KeyName,
	}, nil
}

// EnsureWebhookConfigurations ensures the webhook configurations exist in the cluster in the desired state
func (c *config) EnsureWebhookConfigurations(ctx context.Context, cl client.Client, globalCfg *globalCfg.Config) error {
	lgr := log.FromContext(ctx).WithName("webhooks")

	// todo: need to make this a reconciler? that's constantly running

	lgr.Info("calculating ValidatingWebhookConfiguration")
	var validatingWhs []admissionregistrationv1.ValidatingWebhook
	for _, wh := range c.validatingWebhooks {
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

	lgr.Info("calculating MutatingWebhookConfiguration")
	var mutatingWhs []admissionregistrationv1.MutatingWebhook
	for _, wh := range c.mutatingWebhooks {
		wh, err := wh.Definition(c)
		if err != nil {
			return fmt.Errorf("getting webhook definition: %w", err)
		}

		mutatingWhs = append(mutatingWhs, wh)
	}

	mutatingWhc := &admissionregistrationv1.MutatingWebhookConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MutatingWebhookConfiguration",
			APIVersion: "admissionregistration.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: c.mutatingWebhookConfigName,
			Labels: map[string]string{
				// https://learn.microsoft.com/en-us/azure/aks/faq#can-admission-controller-webhooks-impact-kube-system-and-internal-aks-namespaces
				"admissions.enforcer/disabled": "true",
			},
		},
		Webhooks: mutatingWhs,
	}

	lgr.Info("ensuring namespace exists")
	appRoutingNamespace := &corev1.Namespace{}
	appRoutingNamespace = manifests.Namespace(globalCfg)
	if err := util.Upsert(ctx, cl, appRoutingNamespace); err != nil {
		return fmt.Errorf("upserting namespace: %w", err)
	}

	ownerRef := manifests.GetOwnerRefs(appRoutingNamespace, false)

	lgr.Info("ensuring webhook configuration")
	whs := []client.Object{validatingWhc, mutatingWhc}
	for _, wh := range whs {
		wh.SetOwnerReferences(ownerRef)
		copy := wh.DeepCopyObject().(client.Object)
		lgr := lgr.WithValues("resource", wh.GetName(), "resourceKind", wh.GetObjectKind().GroupVersionKind().Kind)
		lgr.Info("upserting resource")

		if err := util.Upsert(ctx, cl, copy); err != nil {
			return fmt.Errorf("upserting resource: %w", err)
		}
	}

	lgr.Info("finished ensuring webhook configuration")
	return nil
}

// AddWebhooks adds the webhooks to the manager
func (c *config) AddWebhooks(mgr manager.Manager) error {
	for _, wh := range c.validatingWebhooks {
		if err := wh.AddToManager(mgr); err != nil {
			return fmt.Errorf("adding webhook to manager: %w", err)
		}
	}

	for _, wh := range c.mutatingWebhooks {
		if err := wh.AddToManager(mgr); err != nil {
			return fmt.Errorf("adding webhook to manager: %w", err)
		}
	}

	return nil
}

// GetClientConfig returns the client config for the webhook service. path should start with a /
func (c *config) GetClientConfig(path string) (admissionregistrationv1.WebhookClientConfig, error) {
	caPath := filepath.Join(c.certDir, c.caName)
	caBundle, err := os.ReadFile(caPath)
	if err != nil {
		return admissionregistrationv1.WebhookClientConfig{}, fmt.Errorf("reading ca bundle: %w", err)
	}

	whClient := admissionregistrationv1.WebhookClientConfig{
		CABundle: caBundle,
	}

	if c.serviceUrl != "" {
		whClient.URL = util.ToPtr(c.serviceUrl + path)

	} else {
		whClient.Service = &admissionregistrationv1.ServiceReference{
			Name:      c.serviceName,
			Namespace: c.namespace,
			Port:      &c.port,
			Path:      &path,
		}
	}

	return whClient, nil
}
