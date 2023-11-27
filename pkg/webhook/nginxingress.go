package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/nginxingress"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	authv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	validationPath = "/validate-nginx-ingress-controller"
	mutationPath   = "/mutate-nginx-ingress-controller"
)

var (
	nginxResourceValidationName = controllername.New("nginx", "ingress", "resource", "validator")
	nginxResourceMutationName   = controllername.New("nginx", "ingress", "resource", "mutator")
)

func init() {
	Validating = append(Validating, Webhook[admissionregistrationv1.ValidatingWebhook]{
		AddToManager: func(mgr manager.Manager) error {
			metrics.InitControllerMetrics(nginxResourceValidationName)
			mgr.GetWebhookServer().Register(validationPath, &webhook.Admission{
				Handler: &nginxIngressResourceValidator{
					client:       mgr.GetClient(),
					decoder:      admission.NewDecoder(mgr.GetScheme()),
					authenticate: SarAuthenticateNginxIngressController,
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
			metrics.InitControllerMetrics(nginxResourceMutationName)
			mgr.GetWebhookServer().Register(mutationPath, &webhook.Admission{
				Handler: &nginxIngressResourceMutator{
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

type authenticateFn func(ctx context.Context, lgr logr.Logger, cl client.Client, req admission.Request) (string, error)

type nginxIngressResourceValidator struct {
	client  client.Client
	decoder *admission.Decoder
	// authenticate is a function that checks if the request user is authorized to perform the request.
	// The returned string indicates whether the user is allowed, empty string indicates allowed and non-empty will
	// be equal to the reason why they're not allowed. Error will be returned if something goes wrong while verifying the user can request
	authenticate authenticateFn
}

// SarAuthenticateNginxIngressController checks if the user is allowed to perform a request against an NginxIngressController resource. If the user is allowed it returns an empty string, otherwise it returns the reason why they're not allowed.
func SarAuthenticateNginxIngressController(ctx context.Context, lgr logr.Logger, cl client.Client, req admission.Request) (string, error) {
	// ensure user has permissions required
	lgr.Info("checking permissions")
	extra := make(map[string]authv1.ExtraValue)
	for k, v := range req.UserInfo.Extra {
		extra[k] = authv1.ExtraValue(v)
	}
	for _, resource := range manifests.NginxResourceTypes {
		lgr := lgr.WithValues("sarResource", resource.Name, "sarGroup", resource.Group, "sarVersion", resource.Version)
		lgr.Info("checking permissions for resource")
		sar := authv1.SubjectAccessReview{
			Spec: authv1.SubjectAccessReviewSpec{
				ResourceAttributes: &authv1.ResourceAttributes{
					// TODO: add namespace check, this is a bit harder because we need to check if resource is namespaced
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
		if err := cl.Create(ctx, &sar); err != nil {
			lgr.Error(err, "creating SubjectAccessReview")
			return "", fmt.Errorf("creating SubjectAccessReview: %w", err)
		}
		if sar.Status.Denied || (!sar.Status.Allowed) {
			lgr.Info("denied due to permissions", "reason", sar.Status.Reason)
			return fmt.Sprintf("user '%s' does not have permissions to create/update NginxIngressController. Verb '%s' needed for resource '%s' in group '%s' version '%s'.",
				req.UserInfo.Username, sar.Spec.ResourceAttributes.Verb, sar.Spec.ResourceAttributes.Resource, sar.Spec.ResourceAttributes.Group, sar.Spec.ResourceAttributes.Version,
			), nil
		}

	}
	lgr.Info("permissions check passed")
	return "", nil
}

func (n *nginxIngressResourceValidator) Handle(ctx context.Context, req admission.Request) (resp admission.Response) {
	var err error
	defer func() {
		metrics.HandleWebhookHandlerMetrics(nginxResourceValidationName, resp, err)
	}()

	lgr := logr.FromContextOrDiscard(ctx).WithValues("resourceName", req.Name, "namespace", req.Namespace, "operation", req.Operation).WithName(nginxResourceValidationName.LoggerName())
	lgr.Info("validating NginxIngressController request")

	// ensure user has permissions required
	var cantPerform string
	cantPerform, err = n.authenticate(ctx, lgr, n.client, req)
	if err != nil {
		lgr.Error(err, "checking permissions")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if cantPerform != "" {
		lgr.Info("denied due to permissions", "reason", cantPerform)
		return admission.Denied(cantPerform)
	}

	if req.Operation == admissionv1.Delete {
		return admission.Allowed("")
	}

	lgr.Info("decoding NginxIngressController resource")
	var nginxIngressController approutingv1alpha1.NginxIngressController
	if err = n.decoder.Decode(req, &nginxIngressController); err != nil {
		lgr.Error(err, "decoding nginx ingress controller")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding NginxIngressController: %w", err))
	}

	// basic spec validation (everything we can check without making API calls)
	if invalidReason := nginxIngressController.Valid(); invalidReason != "" {
		return admission.Denied(invalidReason)
	}

	if req.Operation == admissionv1.Create {
		if nginxingress.IsDefaultNic(&nginxIngressController) {
			// need to allow for ic to exist already for the default migration case (migrating from non-crd versions of app routing to crd versions)
			return admission.Allowed("")
		}

		lgr.Info("checking if it collides")
		collides, reason, err := nginxIngressController.Collides(ctx, n.client)
		if err != nil {
			lgr.Error(err, "checking if it collides")
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("checking if it collides: %w", err))
		}

		if collides {
			lgr.Info("denied due to collision", "reason", reason)
			return admission.Denied(reason)
		}
	}

	lgr.Info("admission allowed")
	return admission.Allowed("")
}

type nginxIngressResourceMutator struct {
	decoder *admission.Decoder
}

func (n nginxIngressResourceMutator) Handle(ctx context.Context, request admission.Request) (resp admission.Response) {
	var err error
	defer func() {
		metrics.HandleWebhookHandlerMetrics(nginxResourceMutationName, resp, err)
	}()

	lgr := logr.FromContextOrDiscard(ctx).WithValues("resourceName", request.Name, "namespace", request.Namespace, "operation", request.Operation).WithName(nginxResourceMutationName.LoggerName())

	if request.Operation == admissionv1.Delete {
		lgr.Info("delete operation, skipping mutation")
		return admission.Allowed("")
	}

	lgr.Info("decoding NginxIngressController resource")
	var nginxIngressController approutingv1alpha1.NginxIngressController
	if err = n.decoder.Decode(request, &nginxIngressController); err != nil {
		lgr.Error(err, "decoding nginx ingress controller")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding NginxIngressController: %w", err))
	}

	lgr.Info("defaulting NginxIngressController resource")
	nginxIngressController.Default()

	var marshalled []byte
	marshalled, err = json.Marshal(nginxIngressController)
	if err != nil {
		lgr.Error(err, "encoding nginx ingress controller")
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("marshalling NginxIngressController: %w", err))
	}

	return admission.PatchResponseFromRaw(request.Object.Raw, marshalled)
}
