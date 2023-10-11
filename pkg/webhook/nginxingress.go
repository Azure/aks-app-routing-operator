package webhook

import (
	"context"
	"fmt"
	"net/http"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
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
	AddToManagerFns = append(AddToManagerFns, func(mgr manager.Manager) error {
		mgr.GetWebhookServer().Register(validationPath, &webhook.Admission{
			Handler: &nginxIngressResourceValidator{
				client: mgr.GetClient(),
			},
		})

		return nil
	})
}

type nginxIngressResourceValidator struct {
	client  client.Client
	decoder *admission.Decoder // todo: do we need to instantiate this?
}

func (n *nginxIngressResourceValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation == admissionv1.Create {
		var nginxIngressController approutingv1alpha1.NginxIngressController
		if err := n.decoder.Decode(req, &nginxIngressController); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding NginxIngressController: %w", err))
		}

		ic := &netv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: nginxIngressController.Spec.IngressClassName,
			},
		}
		err := n.client.Get(ctx, client.ObjectKeyFromObject(ic), ic)
		if err == nil {
			return admission.Denied(fmt.Sprintf("IngressClass %s already exists. Delete or use a different spec.IngressClassName field", ic.Name))
		}
		if !k8serrors.IsNotFound(err) {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("getting IngressClass %s: %w", ic.Name, err))
		}

		// list nginx ingress controllers
		var nginxIngressControllerList approutingv1alpha1.NginxIngressControllerList
		if err := n.client.List(ctx, &nginxIngressControllerList); err != nil {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("listing NginxIngressControllers: %w", err))
		}

		for _, nic := range nginxIngressControllerList.Items {
			if nic.Spec.IngressClassName == nginxIngressController.Spec.IngressClassName {
				return admission.Denied(fmt.Sprintf("IngressClass %s already exists on NginxIngressController %s. Use a different spec.IngressClassName field", nic.Spec.IngressClassName, nic.Name))
			}
		}

		// TODO: check user permissions with SubjectAccessReview
	}

	return admission.Allowed("")
}
