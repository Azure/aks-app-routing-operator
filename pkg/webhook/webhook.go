package webhook

import (
	"context"
	"fmt"

	globalCfg "github.com/Azure/aks-app-routing-operator/pkg/config"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	certDir = "/tmp/k8s-webhook-server/serving-certs"
)

// AddToManagerFuncs is a list of functions to add all Webhooks to the Manager
var AddToManagerFns []func(manager.Manager) error

// AddToManager adds all Webhooks to the Manager
func AddToManager(m manager.Manager) error {
	for _, f := range AddToManagerFns {
		if err := f(m); err != nil {
			return err
		}
	}

	return nil

}

type config struct {
	serviceName, namespace string
	port                   int32
	url                    string

	// caPEM is a PEM-encoded CA bundle
	ca []byte
}

func New(globalCfg globalCfg.Config) (*config, error) {
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
	}

	return c, nil
}

func (c *config) Start(ctx context.Context, operator *appsv1.Deployment, cl client.Client) error {
	lgr := log.FromContext(ctx).WithName("webhooks")
	lgr.Info("setting up")

	validatingWh := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-routing-validating",
			Labels: map[string]string{
				// https://learn.microsoft.com/en-us/azure/aks/faq#can-admission-controller-webhooks-impact-kube-system-and-internal-aks-namespaces
				"admissions.enforcer/disabled": "true",
			},
		},
	}
	mutatingWh := &admissionregistrationv1.MutatingWebhookConfiguration{
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

	whs := []client.Object{validatingWh, mutatingWh}
	for _, wh := range whs {
		lgr := lgr.WithValues("webhook", wh.GetName())
		lgr.Info("creating webhook configuration")
		if err := cl.Create(ctx, wh); err != nil {
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
