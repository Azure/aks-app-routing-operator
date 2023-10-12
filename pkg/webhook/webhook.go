package webhook

import (
	"context"
	"fmt"

	globalCfg "github.com/Azure/aks-app-routing-operator/pkg/config"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	certDir = "/tmp/k8s-webhook-server/serving-certs/"
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
	url                    string

	// caPEM is a PEM-encoded CA bundle
	ca []byte

	mgr manager.Manager
}

func New(globalCfg *globalCfg.Config, mgr manager.Manager) (*config, error) {
	if globalCfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	serviceName := globalCfg.OperatorWebhookService
	namespace := globalCfg.OperatorNs

	cert, err := genCert(serviceName, namespace)
	if err != nil {
		return nil, fmt.Errorf("generating cert: %w", err)
	}

	if err := cert.save(certDir); err != nil {
		return nil, fmt.Errorf("saving cert: %w", err)
	}

	// TODO: parameterize port
	port := int32(9443)
	c := &config{
		serviceName: serviceName,
		namespace:   namespace,
		port:        port,
		url:         fmt.Sprintf("https://%s.%s.svc.cluster.local:%d", serviceName, namespace, port),
		ca:          cert.ca,
		mgr:         mgr,
	}

	for _, wh := range Validating {
		if err := wh.AddToManager(mgr); err != nil {
			return nil, fmt.Errorf("adding webhook to manager: %w", err)
		}
	}

	return c, nil
}

// Start implements manager.Runnable which lets us use leader election to connect to the webhook instance
// with our self signed cert
func (c *config) Start(ctx context.Context) error {
	lgr := log.FromContext(ctx).WithName("webhooks")
	lgr.Info("setting up")

	var validatingWhs []admissionregistrationv1.ValidatingWebhook
	for _, wh := range Validating {
		wh, err := wh.Definition(c)
		if err != nil {
			return fmt.Errorf("getting webhook definition: %w", err)
		}

		validatingWhs = append(validatingWhs, wh)
	}

	validatingWhc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-routing-validating",
			Labels: map[string]string{
				// https://learn.microsoft.com/en-us/azure/aks/faq#can-admission-controller-webhooks-impact-kube-system-and-internal-aks-namespaces
				"admissions.enforcer/disabled": "true",
			},
		},
		Webhooks: validatingWhs,
	}
	mutatingWhc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-routing-mutating",
			Labels: map[string]string{
				// https://learn.microsoft.com/en-us/azure/aks/faq#can-admission-controller-webhooks-impact-kube-system-and-internal-aks-namespaces
				"admissions.enforcer/disabled": "true",
			},
		},
	}

	// set ownership references so webhook is deleted when operator is deleted
	// set ownership references to app-routing-system ns for now ?? we need a workaround to delete
	// TODO: maybe users have to manually delete?
	// TODO: no ownership ref from global to namespaced
	// owners := manifests.GetOwnerRefs(operator)
	// whCfg.SetOwnerReferences(owners)

	// TODO: how does this work with multiple replicas and leader election? this seems very sketchy
	cl := c.mgr.GetClient()
	whs := []client.Object{validatingWhc, mutatingWhc}
	for _, wh := range whs {
		copy := wh.DeepCopyObject().(client.Object)
		lgr := lgr.WithValues("webhook", wh.GetName())
		lgr.Info("creating webhook configuration")
		if err := cl.Create(ctx, copy); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("creating webhook configuration: %w", err)
			}

			lgr.Info("webhook configuration already exists, patching")
			if err := cl.Patch(ctx, wh, client.MergeFrom(wh)); err != nil { // use MergeFrom to overwrite lists
				return fmt.Errorf("patching webhook configuration: %w", err)
			}
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
		CABundle: c.ca, // TODO: how does this work with multi replicas?
	}, nil
}
