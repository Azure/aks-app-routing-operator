package webhook

import (
	"context"
	"fmt"
	"net/http"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	validationPath = "/validate-nginx-ingress-controller"
)

func init() {
	Validating = append(Validating, Webhook[admissionregistrationv1.ValidatingWebhook]{
		AddToManager: func(mgr manager.Manager) error {
			mgr.GetWebhookServer().Register(validationPath, &webhook.Admission{
				Handler: &nginxIngressResourceValidator{
					client:  mgr.GetClient(),
					decoder: admission.NewDecoder(mgr.GetScheme()),
				},
			})

			return nil
		},
		Definition: func(c *config) (admissionregistrationv1.ValidatingWebhook, error) {
			clientCfg, err := c.GetClientConfig(validationPath)
			if err != nil {
				return admissionregistrationv1.ValidatingWebhook{}, fmt.Errorf("getting client config: %w", err)
			}

			return admissionregistrationv1.ValidatingWebhook{
				Name:                    "validating.nginxingresscontroller.approuting.kubernetes.azure.com",
				AdmissionReviewVersions: []string{admissionregistrationv1.SchemeGroupVersion.Version},
				ClientConfig:            clientCfg,
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{approutingv1alpha1.GroupVersion.Group},
							APIVersions: []string{approutingv1alpha1.GroupVersion.Version},
							Resources:   []string{"nginxingresscontrollers"},
						},
					},
				},
				FailurePolicy: util.ToPtr(admissionregistrationv1.Fail), // need this because we have to check permissions, better to fail than let a request through
				SideEffects:   util.ToPtr(admissionregistrationv1.SideEffectClassNone),
			}, nil
		},
	})
}

type nginxIngressResourceValidator struct {
	client  client.Client
	decoder *admission.Decoder // todo: do we need to instantiate this?
}

func (n *nginxIngressResourceValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	lgr := logr.FromContextOrDiscard(ctx).WithValues("name", req.Name, "namespace", req.Namespace, "operation", req.Operation)

	// TODO: record metrics

	if req.Operation == admissionv1.Create {
		lgr.Info("decoding NginxIngressController resource")
		var nginxIngressController approutingv1alpha1.NginxIngressController
		if err := n.decoder.Decode(req, &nginxIngressController); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding NginxIngressController: %w", err))
		}

		lgr.Info("checking if IngressClass already exists")
		ic := &netv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: nginxIngressController.Spec.IngressClassName,
			},
		}
		lgr.Info("attempting to get IngressClass " + ic.Name)
		err := n.client.Get(ctx, client.ObjectKeyFromObject(ic), ic)
		if err == nil {
			return admission.Denied(fmt.Sprintf("IngressClass %s already exists. Delete or use a different spec.IngressClassName field", ic.Name))
		}
		if !k8serrors.IsNotFound(err) {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("getting IngressClass %s: %w", ic.Name, err))
		}

		// list nginx ingress controllers
		lgr.Info("listing NginxIngressControllers to check for collisions")
		var nginxIngressControllerList approutingv1alpha1.NginxIngressControllerList
		if err := n.client.List(ctx, &nginxIngressControllerList); err != nil {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("listing NginxIngressControllers: %w", err))
		}

		for _, nic := range nginxIngressControllerList.Items {
			if nic.Spec.IngressClassName == nginxIngressController.Spec.IngressClassName {
				lgr.Info("IngressClass already exists on NginxIngressController " + nic.Name)
				return admission.Denied(fmt.Sprintf("IngressClass %s already exists on NginxIngressController %s. Use a different spec.IngressClassName field", nic.Spec.IngressClassName, nic.Name))
			}
		}

		// TODO: check user permissions with SubjectAccessReview
	}

	return admission.Allowed("")
}
