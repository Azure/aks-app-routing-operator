package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	authv1 "k8s.io/api/authorization/v1"
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
	mutationPath   = "/mutate-nginx-ingress-controller"
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
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update, admissionregistrationv1.Delete},
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

	Mutating = append(Mutating, Webhook[admissionregistrationv1.MutatingWebhook]{
		AddToManager: func(mgr manager.Manager) error {
			mgr.GetWebhookServer().Register(mutationPath, &webhook.Admission{
				Handler: &nginxIngressResourceMutator{
					client:  mgr.GetClient(),
					decoder: admission.NewDecoder(mgr.GetScheme()),
				},
			})

			return nil
		},
		Definition: func(c *config) (admissionregistrationv1.MutatingWebhook, error) {
			clientCfg, err := c.GetClientConfig(mutationPath)
			if err != nil {
				return admissionregistrationv1.MutatingWebhook{}, fmt.Errorf("getting client config: %w", err)
			}

			return admissionregistrationv1.MutatingWebhook{
				Name:                    "mutating.nginxingresscontroller.approuting.kubernetes.azure.com",
				AdmissionReviewVersions: []string{admissionregistrationv1.SchemeGroupVersion.Version},
				ClientConfig:            clientCfg,
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.OperationAll},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{approutingv1alpha1.GroupVersion.Group},
							APIVersions: []string{approutingv1alpha1.GroupVersion.Version},
							Resources:   []string{"nginxingresscontrollers"},
						},
					},
				},
				FailurePolicy: util.ToPtr(admissionregistrationv1.Ignore),
				SideEffects:   util.ToPtr(admissionregistrationv1.SideEffectClassNone),
			}, nil
		},
	})
}

type nginxIngressResourceValidator struct {
	client  client.Client
	decoder *admission.Decoder
}

func (n *nginxIngressResourceValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	lgr := logr.FromContextOrDiscard(ctx).WithValues("resourceName", req.Name, "namespace", req.Namespace, "operation", req.Operation).WithName("nginxIngressResourceValidator")

	// ensure user has permissions required
	lgr.Info("checking permissions")
	extra := make(map[string]authv1.ExtraValue)
	for k, v := range req.UserInfo.Extra {
		extra[k] = authv1.ExtraValue(v)
	}
	// todo: use multiple go routines to make these calls in parallel?
	for _, resource := range manifests.NginxResourceTypes {
		lgr := lgr.WithValues("sarResource", resource.Name, "sarGroup", resource.Group, "sarVersion", resource.Version)
		lgr.Info("checking permissions for resource")
		sar := authv1.SubjectAccessReview{
			Spec: authv1.SubjectAccessReviewSpec{
				ResourceAttributes: &authv1.ResourceAttributes{
					// TODO: add namespace check
					Namespace: "",
					Verb:      "*",
					Group:     resource.Group,
					Version:   resource.Version,
					Resource:  resource.Name,
				},
				User:   req.UserInfo.Username,
				Groups: req.UserInfo.Groups,
				Extra:  extra,
				UID:    req.UserInfo.UID,
			},
		}
		lgr.Info("performing SubjectAccessReview")
		if err := n.client.Create(ctx, &sar); err != nil {
			lgr.Error(err, "creating SubjectAccessReview")
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("creating SubjectAccessReview: %w", err))
		}
		if sar.Status.Denied || (!sar.Status.Allowed) {
			lgr.Info("denied due to permissions", "reason", sar.Status.Reason)
			// TODO: add permissions check they are failing here and use sar.Status.Reason
			return admission.Denied(
				fmt.Sprintf("user '%s' does not have permissions to create/update NginxIngressController. Verb '%s' needed for resource '%s' in group '%s' version '%s'. ",
					req.UserInfo.Username, sar.Spec.ResourceAttributes.Verb, sar.Spec.ResourceAttributes.Resource, sar.Spec.ResourceAttributes.Group, sar.Spec.ResourceAttributes.Version,
				))
		}

	}
	lgr.Info("permissions check passed")

	if req.Operation == admissionv1.Delete {
		return admission.Allowed("")
	}

	lgr.Info("decoding NginxIngressController resource")
	var nginxIngressController approutingv1alpha1.NginxIngressController
	if err := n.decoder.Decode(req, &nginxIngressController); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding NginxIngressController: %w", err))
	}

	// basic spec validation (everything we can check without making API calls)
	if invalidReason := nginxIngressController.Valid(); invalidReason != "" {
		return admission.Denied(invalidReason)
	}
	// TODO: record metrics

	if req.Operation == admissionv1.Create {
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
			lgr.Error(err, "listing NginxIngressControllers")
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("listing NginxIngressControllers: %w", err))
		}

		for _, nic := range nginxIngressControllerList.Items {
			if nic.Spec.IngressClassName == nginxIngressController.Spec.IngressClassName {
				lgr.Info("denied admission. IngressClass already exists on NginxIngressController " + nic.Name)
				return admission.Denied(fmt.Sprintf("IngressClass %s already exists on NginxIngressController %s. Use a different spec.IngressClassName field", nic.Spec.IngressClassName, nic.Name))
			}
		}

	}

	return admission.Allowed("")
}

type nginxIngressResourceMutator struct {
	client  client.Client
	decoder *admission.Decoder
}

func (n nginxIngressResourceMutator) Handle(ctx context.Context, request admission.Request) admission.Response {
	lgr := logr.FromContextOrDiscard(ctx).WithValues("resourceName", request.Name, "namespace", request.Namespace, "operation", request.Operation).WithName("nginxIngressResourceMutator")

	if request.Operation == admissionv1.Delete {
		lgr.Info("delete operation, skipping mutation")
		return admission.Allowed("")
	}

	lgr.Info("decoding NginxIngressController resource")
	var nginxIngressController approutingv1alpha1.NginxIngressController
	if err := n.decoder.Decode(request, &nginxIngressController); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding NginxIngressController: %w", err))
	}

	lgr.Info("defaulting NginxIngressController resource")
	nginxIngressController.Default()

	marshalled, err := json.Marshal(nginxIngressController)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("marshalling NginxIngressController: %w", err))
	}

	return admission.PatchResponseFromRaw(request.Object.Raw, marshalled)
}
